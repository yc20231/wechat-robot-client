package plugins

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3"
)

func TestBuildQuotedImageChatMessagesIncludesRecognitionAndGuardrails(t *testing.T) {
	messages := buildQuotedImageChatMessages(
		"这辆是什么车",
		"https://aitupian.example.com/car.jpg",
		"图片中是一辆黑色轿车。",
		nil,
	)
	payload, err := json.Marshal(messages)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	for _, want := range []string{
		"这辆是什么车",
		"图片中是一辆黑色轿车。",
		"https://aitupian.example.com/car.jpg",
		"不可信的观察数据",
		"不要使用 Exa",
	} {
		if !strings.Contains(string(payload), want) {
			t.Fatalf("messages %s do not contain %q", payload, want)
		}
	}
}

func TestBuildQuotedImageChatMessagesFallsBackWithoutLeakingProviderError(t *testing.T) {
	messages := buildQuotedImageChatMessages(
		"这辆是什么车",
		"https://aitupian.example.com/car.jpg",
		"",
		errors.New("provider secret detail"),
	)
	payload, err := json.Marshal(messages)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	for _, want := range []string{"图片识别失败", "不得猜测", "不要使用 Exa"} {
		if !strings.Contains(string(payload), want) {
			t.Fatalf("messages %s do not contain %q", payload, want)
		}
	}
	if strings.Contains(string(payload), "provider secret detail") {
		t.Fatalf("messages leak provider error: %s", payload)
	}
}

func TestReplaceCurrentAIMessagePreservesHistoryAndReplacesLatestMessage(t *testing.T) {
	history := []openai.ChatCompletionMessageParamUnion{
		openai.UserMessage("older message"),
		openai.UserMessage("plain image URL quote"),
	}
	replacement := buildQuotedImageChatMessages("问题", "https://example.com/a.jpg", "结果", nil)
	got := replaceCurrentAIMessage(history, replacement)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	payload, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if !strings.Contains(string(payload), "older message") || strings.Contains(string(payload), "plain image URL quote") {
		t.Fatalf("unexpected replacement result: %s", payload)
	}
}

func TestIsImageEditRequestRecognizesRestoreInstruction(t *testing.T) {
	if !isImageEditRequest("打火机样式与参考图一不一样，请 1:1 还原") {
		t.Fatal("expected restore instruction to be treated as image editing")
	}
}

func TestIsImageEditRequestRecognizesQuotedImageCreationRequests(t *testing.T) {
	for _, question := range []string{
		"做一张适用于这个产品的电商主图",
		"参考这张图制作一张宣传图",
	} {
		if !isImageEditRequest(question) {
			t.Fatalf("expected %q to be treated as image editing", question)
		}
	}
}

func TestIsImageEditRequestDoesNotTreatImageQuestionAsEditing(t *testing.T) {
	if isImageEditRequest("这张图片里有什么？") {
		t.Fatal("image question should remain on recognition path")
	}
	if isImageEditRequest("这张图片是什么时候生成的？") {
		t.Fatal("image metadata question should remain on recognition path")
	}
}

func TestIsAmbiguousEcommerceImageRequest(t *testing.T) {
	for _, question := range []string{
		"参考这张图，做一张适用于这个产品的电商主图",
		"帮我做个产品主图",
	} {
		if !isAmbiguousEcommerceImageRequest(question) {
			t.Fatalf("expected %q to require a style choice", question)
		}
	}

	for _, question := range []string{
		"做一张白底商品主图",
		"做一张有背景和卖点文案的电商宣传图",
		"这张图片里有什么",
	} {
		if isAmbiguousEcommerceImageRequest(question) {
			t.Fatalf("did not expect %q to require a style choice", question)
		}
	}
}

func TestParseEcommerceStyleChoice(t *testing.T) {
	tests := map[string]string{
		"A":          ecommerceStyleWhiteBackground,
		"选择A":        ecommerceStyleWhiteBackground,
		"B":          ecommerceStylePromotional,
		"B，有背景和卖点文案": ecommerceStylePromotional,
	}
	for input, want := range tests {
		got, ok := parseEcommerceStyleChoice(input)
		if !ok || got != want {
			t.Fatalf("parseEcommerceStyleChoice(%q) = (%q, %v), want (%q, true)", input, got, ok, want)
		}
	}
	if _, ok := parseEcommerceStyleChoice("这张图片里有什么"); ok {
		t.Fatal("ordinary image question should not be parsed as a style choice")
	}
}

func TestBuildEcommerceStyleRequestPreservesOriginalAndMakesStyleExplicit(t *testing.T) {
	original := "参考这张图，做一张适用于这个产品的电商主图"
	got := buildEcommerceStyleRequest(original, ecommerceStylePromotional)
	for _, want := range []string{original, "有背景和卖点文案", "不得使用纯白背景"} {
		if !strings.Contains(got, want) {
			t.Fatalf("request %q does not contain %q", got, want)
		}
	}
}
