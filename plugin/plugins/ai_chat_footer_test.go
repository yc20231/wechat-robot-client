package plugins

import "testing"

func TestAppendAIReplyFooterDefault(t *testing.T) {
	t.Setenv("AI_REPLY_FOOTER_ENABLED", "true")
	t.Setenv("AI_REPLY_FOOTER", "")

	want := "你好\n\n本消息由AI生成回复"
	if got := appendAIReplyFooter("你好"); got != want {
		t.Fatalf("appendAIReplyFooter() = %q, want %q", got, want)
	}
}

func TestAppendAIReplyFooterDisabled(t *testing.T) {
	t.Setenv("AI_REPLY_FOOTER_ENABLED", "false")
	if got := appendAIReplyFooter("你好"); got != "你好" {
		t.Fatalf("disabled footer changed reply: %q", got)
	}
}

func TestAppendAIReplyFooterCustomAndIdempotent(t *testing.T) {
	t.Setenv("AI_REPLY_FOOTER_ENABLED", "true")
	t.Setenv("AI_REPLY_FOOTER", "AI回复")

	if got := appendAIReplyFooter("你好"); got != "你好\n\nAI回复" {
		t.Fatalf("custom footer = %q", got)
	}
	if got := appendAIReplyFooter("你好\n\nAI回复"); got != "你好\n\nAI回复" {
		t.Fatalf("footer was duplicated: %q", got)
	}
}
