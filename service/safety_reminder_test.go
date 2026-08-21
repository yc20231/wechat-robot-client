package service

import (
	"bytes"
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"wechat-robot-client/pkg/safetyreminder"
)

func TestPrepareSafetyReminderPosterUsesApprovedStaticFile(t *testing.T) {
	directory := t.TempDir()
	date := time.Date(2026, 8, 22, 0, 0, 0, 0, time.Local)
	want := append([]byte(nil), []byte("\x89PNG\r\n\x1a\n")...)
	want = append(want, []byte("approved-poster")...)
	if err := os.WriteFile(filepath.Join(directory, "2026-08-22.png"), want, 0600); err != nil {
		t.Fatal(err)
	}

	got, focus, err := NewSafetyReminderService(context.Background()).PreparePoster(date, safetyreminder.Config{StaticPostersDir: directory})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("approved static poster bytes changed")
	}
	if focus != "静态审核海报" {
		t.Fatalf("unexpected static poster focus: %q", focus)
	}
}

func TestPrepareSafetyReminderPosterFallsBackWhenDateIsMissing(t *testing.T) {
	date := time.Date(2026, 9, 20, 0, 0, 0, 0, time.Local)
	got, focus, err := NewSafetyReminderService(context.Background()).PreparePoster(date, safetyreminder.Config{StaticPostersDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(got, []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatal("dynamic fallback did not return a PNG")
	}
	if focus == "" || focus == "静态审核海报" {
		t.Fatalf("unexpected dynamic focus: %q", focus)
	}
}

func TestPrepareSafetyReminderPosterLogsInvalidStaticFileAndFallsBack(t *testing.T) {
	directory := t.TempDir()
	date := time.Date(2026, 8, 22, 0, 0, 0, 0, time.Local)
	if err := os.WriteFile(filepath.Join(directory, "2026-08-22.png"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(previous) })

	got, focus, err := NewSafetyReminderService(context.Background()).PreparePoster(date, safetyreminder.Config{StaticPostersDir: directory})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(got, []byte("\x89PNG\r\n\x1a\n")) || focus == "" || focus == "静态审核海报" {
		t.Fatalf("unexpected dynamic fallback: bytes=%d focus=%q", len(got), focus)
	}
	if !strings.Contains(output.String(), "静态海报读取失败，回退动态生成") {
		t.Fatalf("fallback log missing: %s", output.String())
	}
}
