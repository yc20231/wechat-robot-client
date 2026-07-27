package plugins

import (
	"strings"
	"testing"

	"wechat-robot-client/interface/plugin"
	"wechat-robot-client/model"
)

func TestPendingEcommerceImageKeySeparatesSendersAndConversations(t *testing.T) {
	first := pendingEcommerceImageKey("robot", &model.Message{FromWxID: "group-1", SenderWxID: "user-1"})
	second := pendingEcommerceImageKey("robot", &model.Message{FromWxID: "group-1", SenderWxID: "user-2"})
	third := pendingEcommerceImageKey("robot", &model.Message{FromWxID: "group-2", SenderWxID: "user-1"})
	if first == second || first == third || second == third {
		t.Fatalf("pending keys must be isolated: %q %q %q", first, second, third)
	}
	for _, want := range []string{"robot", "group-1", "user-1"} {
		if !strings.Contains(first, want) {
			t.Fatalf("key %q does not contain %q", first, want)
		}
	}
}

func TestPendingEcommerceImageKeyUsesPrivateContactAsRequester(t *testing.T) {
	key := pendingEcommerceImageKey("robot", &model.Message{FromWxID: "friend-1"})
	if !strings.Contains(key, "friend-1:friend-1") {
		t.Fatalf("private key = %q, want contact used as conversation and requester", key)
	}
}

func TestApplyPendingEcommerceChoiceRestoresQuotedImageAndRequest(t *testing.T) {
	message := &model.Message{ID: 900, Type: model.MsgTypeText, Content: "@机器人 B"}
	ctx := &plugin.MessageContext{Message: message, MessageContent: "@机器人 B"}
	pending := &pendingEcommerceImageRequest{
		RefMessageID:     885,
		RefMessageMsgID:  1211581018536705253,
		RefAttachmentURL: "https://example.com/marker.jpg",
		OriginalRequest:  "参考这张图，做一张适用于这个产品的电商主图",
	}

	applyPendingEcommerceChoice(ctx, pending, ecommerceStylePromotional)

	if ctx.ReferMessage == nil || ctx.ReferMessage.ID != 885 || ctx.ReferMessage.Type != model.MsgTypeImage {
		t.Fatalf("refer message = %#v, want restored image 885", ctx.ReferMessage)
	}
	for _, want := range []string{pending.OriginalRequest, "有背景和卖点文案", "不得使用纯白背景"} {
		if !strings.Contains(ctx.MessageContent, want) || !strings.Contains(ctx.Message.Content, want) {
			t.Fatalf("resolved content %q / %q does not contain %q", ctx.MessageContent, ctx.Message.Content, want)
		}
	}
}
