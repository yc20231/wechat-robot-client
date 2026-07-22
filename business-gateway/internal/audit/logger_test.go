package audit

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestFileLoggerWritesPrivateJSONLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "audit.jsonl")
	logger := NewFileLogger(path)
	if err := logger.Record(Event{Action: "add_admin", ActorWxID: "owner", TargetWxID: "target", GroupID: "group"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("audit permissions = %o", info.Mode().Perm())
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var event Event
	if !bufio.NewScanner(file).Scan() {
		t.Fatal("missing audit line")
	}
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	if err := json.NewDecoder(file).Decode(&event); err != nil {
		t.Fatal(err)
	}
	if event.Action != "add_admin" || event.TargetWxID != "target" || event.Time.IsZero() {
		t.Fatalf("unexpected audit event: %+v", event)
	}
}
