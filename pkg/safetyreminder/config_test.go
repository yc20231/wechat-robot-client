package safetyreminder

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigFileDefaultsWhenMissing(t *testing.T) {
	config, err := LoadConfigFile(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if config.Enabled || config.Cron != "0 8 * * *" || !config.SendOnWeekends {
		t.Fatalf("unexpected defaults: %+v", config)
	}
}

func TestLoadConfigFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	data := []byte(`{"enabled":true,"cron":"30 7 * * *","target_chat_room_id":"123@chatroom","send_on_weekends":false,"test_token":"secret"}`)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	config, err := LoadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !config.Enabled || config.Cron != "30 7 * * *" || config.TargetChatRoomID != "123@chatroom" || config.SendOnWeekends || config.TestToken != "secret" {
		t.Fatalf("unexpected config: %+v", config)
	}
	if err := config.ValidateForSend(); err != nil {
		t.Fatal(err)
	}
}

func TestInvalidConfigRemainsDisabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.json")
	if err := os.WriteFile(path, []byte(`{"enabled":true,"unknown":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	config, err := LoadConfigFile(path)
	if err == nil {
		t.Fatal("expected invalid config error")
	}
	if config.Enabled {
		t.Fatal("invalid configuration must not enable scheduled sending")
	}
}
