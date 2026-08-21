package safetyreminder

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadStaticPoster(t *testing.T) {
	date := time.Date(2026, 8, 22, 0, 0, 0, 0, time.Local)
	directory := t.TempDir()
	want := append([]byte(nil), []byte("\x89PNG\r\n\x1a\n")...)
	want = append(want, []byte("approved-poster")...)
	path := filepath.Join(directory, "2026-08-22.png")
	if err := os.WriteFile(path, want, 0600); err != nil {
		t.Fatal(err)
	}

	got, found, err := LoadStaticPoster(date, directory)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected static poster to be found")
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("static poster bytes changed: got %q, want %q", got, want)
	}
}

func TestLoadStaticPosterMissesUnconfiguredOrMissingDate(t *testing.T) {
	date := time.Date(2026, 9, 20, 0, 0, 0, 0, time.Local)
	for _, directory := range []string{"", t.TempDir()} {
		got, found, err := LoadStaticPoster(date, directory)
		if err != nil || found || got != nil {
			t.Fatalf("unexpected miss result for %q: bytes=%v found=%v err=%v", directory, got, found, err)
		}
	}
}

func TestLoadStaticPosterRejectsInvalidTargets(t *testing.T) {
	date := time.Date(2026, 8, 22, 0, 0, 0, 0, time.Local)
	tests := []struct {
		name  string
		setup func(string) error
	}{
		{name: "empty file", setup: func(path string) error { return os.WriteFile(path, nil, 0600) }},
		{name: "wrong signature", setup: func(path string) error { return os.WriteFile(path, []byte("not-png"), 0600) }},
		{name: "directory", setup: func(path string) error { return os.Mkdir(path, 0700) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			path := filepath.Join(directory, "2026-08-22.png")
			if err := test.setup(path); err != nil {
				t.Fatal(err)
			}
			got, found, err := LoadStaticPoster(date, directory)
			if err == nil || found || got != nil {
				t.Fatalf("expected invalid target error: bytes=%v found=%v err=%v", got, found, err)
			}
			if !strings.Contains(err.Error(), path) {
				t.Fatalf("error does not identify target path: %v", err)
			}
		})
	}
}
