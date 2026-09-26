package safetyreminder

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestCustomTopicPhotoUsesMatchingScene(t *testing.T) {
	directory := t.TempDir()
	photoPath := filepath.Join(directory, "scene.png")
	photo := image.NewRGBA(image.Rect(0, 0, 2, 2))
	photo.Set(0, 0, color.RGBA{R: 50, G: 120, B: 80, A: 255})
	var pngBytes bytes.Buffer
	if err := png.Encode(&pngBytes, photo); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(photoPath, pngBytes.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	topic := Topic{Focus: "注塑机合模防护", Points: [3]string{"检查安全门", "确认急停", "禁止伸手"}, Slogan: "先确认再开机", Photo: "scene.png"}
	topicBytes, err := json.Marshal([]Topic{topic})
	if err != nil {
		t.Fatal(err)
	}
	topicsPath := filepath.Join(directory, "topics.json")
	if err := os.WriteFile(topicsPath, topicBytes, 0600); err != nil {
		t.Fatal(err)
	}
	topics, err := LoadTopics(topicsPath)
	if err != nil {
		t.Fatal(err)
	}
	content, err := ContentForDate(time.Date(2026, 10, 9, 0, 0, 0, 0, time.Local), topics)
	if err != nil {
		t.Fatal(err)
	}
	if content.PhotoPath != photoPath {
		t.Fatalf("photo path = %q, want %q", content.PhotoPath, photoPath)
	}
	html, err := renderPhotoHTML(content)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"2026年10月9日 星期五", "注塑机合模防护", "检查安全门", "data:image/png;base64,"} {
		if !strings.Contains(html, expected) {
			t.Fatalf("photo poster missing %q", expected)
		}
	}
}

func TestDefaultTopicsAndDailySelection(t *testing.T) {
	topics, err := LoadTopics("")
	if err != nil {
		t.Fatal(err)
	}
	if len(topics) != 120 {
		t.Fatalf("expected 120 topics, got %d", len(topics))
	}

	first, err := ContentForDate(time.Date(2026, 7, 24, 8, 0, 0, 0, time.Local), topics)
	if err != nil {
		t.Fatal(err)
	}
	sameDay, _ := ContentForDate(time.Date(2026, 7, 24, 20, 0, 0, 0, time.Local), topics)
	nextDay, _ := ContentForDate(time.Date(2026, 7, 25, 8, 0, 0, 0, time.Local), topics)
	if first.Focus != sameDay.Focus {
		t.Fatal("same date selected different topics")
	}
	if first.Focus == nextDay.Focus {
		t.Fatal("consecutive dates selected the same topic")
	}
}

func TestDefaultTopicsAreConciseActionableReminders(t *testing.T) {
	topics, err := LoadTopics("")
	if err != nil {
		t.Fatal(err)
	}
	allowedPrefixes := []string{"检查", "严禁", "注意", "及时", "确认"}
	seenFocuses := make(map[string]bool, len(topics))
	for index, topic := range topics {
		if seenFocuses[topic.Focus] {
			t.Errorf("topic %d repeats focus: %s", index+1, topic.Focus)
		}
		seenFocuses[topic.Focus] = true
		if strings.HasSuffix(topic.Focus, "风险") {
			t.Errorf("topic %d focus redundantly ends with risk: %s", index+1, topic.Focus)
		}
		if length := utf8.RuneCountInString(topic.Focus); length > 10 {
			t.Errorf("topic %d focus is too long (%d characters): %s", index+1, length, topic.Focus)
		}
		for pointIndex, point := range topic.Points {
			hasAllowedPrefix := false
			for _, prefix := range allowedPrefixes {
				if strings.HasPrefix(point, prefix) {
					hasAllowedPrefix = true
					break
				}
			}
			if !hasAllowedPrefix {
				t.Errorf("topic %d point %d does not start with a reminder word: %s", index+1, pointIndex+1, point)
			}
			if length := utf8.RuneCountInString(point); length > 20 {
				t.Errorf("topic %d point %d is too long (%d characters): %s", index+1, pointIndex+1, length, point)
			}
		}
		if length := utf8.RuneCountInString(topic.Slogan); length > 20 {
			t.Errorf("topic %d slogan is too long (%d characters): %s", index+1, length, topic.Slogan)
		}
	}
}

func TestDefaultTopicsDoNotRepeatWithinCycle(t *testing.T) {
	topics, err := LoadTopics("")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 11, 15, 0, 0, 0, 0, time.Local)
	seenFocuses := make(map[string]bool, len(topics))
	for offset := 0; offset < len(topics); offset++ {
		content, err := ContentForDate(start.AddDate(0, 0, offset), topics)
		if err != nil {
			t.Fatal(err)
		}
		if seenFocuses[content.Focus] {
			t.Fatalf("focus repeated before the 120-day cycle completed: %s", content.Focus)
		}
		seenFocuses[content.Focus] = true
	}
	first, _ := ContentForDate(start, topics)
	afterCycle, _ := ContentForDate(start.AddDate(0, 0, len(topics)), topics)
	if first.Focus != afterCycle.Focus {
		t.Fatalf("topic cycle mismatch: got %s, want %s", afterCycle.Focus, first.Focus)
	}
}

func TestRenderHTMLContainsDynamicContent(t *testing.T) {
	content := PosterContent{
		Date:   time.Date(2026, 7, 24, 0, 0, 0, 0, time.Local),
		Focus:  "消防安全",
		Points: [3]string{"第一条", "第二条", "第三条"},
		Slogan: "测试标语",
	}
	html, err := renderHTML(content)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"2026.07.24 星期五", "今日重点：消防安全", "第一条", "第二条", "第三条", "测试标语"} {
		if !strings.Contains(html, expected) {
			t.Fatalf("rendered HTML does not contain %q", expected)
		}
	}
}

func TestBackgroundVariantsRotateByDate(t *testing.T) {
	start := time.Date(2027, 12, 29, 0, 0, 0, 0, time.Local)
	seen := make(map[string]bool)
	for offset := 0; offset < posterBackgroundVariants; offset++ {
		asset := backgroundAssetForDate(start.AddDate(0, 0, offset))
		if seen[asset] {
			t.Fatalf("background repeated before all variants were used: %s", asset)
		}
		seen[asset] = true
	}
	if got, want := backgroundAssetForDate(start.AddDate(0, 0, posterBackgroundVariants)), backgroundAssetForDate(start); got != want {
		t.Fatalf("background cycle mismatch: got %s, want %s", got, want)
	}
}
