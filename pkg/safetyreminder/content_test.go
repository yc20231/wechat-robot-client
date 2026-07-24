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
	for _, expected := range []string{"2026年7月24日 星期五", "今日重点：消防安全", "第一条", "第二条", "第三条", "测试标语"} {
		if !strings.Contains(html, expected) {
			t.Fatalf("rendered HTML does not contain %q", expected)
		}
	}
}
