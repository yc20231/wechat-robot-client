package safetyreminder

import (
	"strings"
	"testing"
	"time"
)

func TestDefaultTopicsAndDailySelection(t *testing.T) {
	topics, err := LoadTopics("")
	if err != nil {
		t.Fatal(err)
	}
	if len(topics) < 31 {
		t.Fatalf("expected at least 31 topics, got %d", len(topics))
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
	start := time.Date(2026, 8, 6, 0, 0, 0, 0, time.Local)
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
