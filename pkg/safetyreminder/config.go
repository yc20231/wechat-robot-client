package safetyreminder

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

const (
	DefaultConfigPath = "/data/skills/.safety-reminder.json"
	ConfigPathEnv     = "SAFETY_REMINDER_CONFIG_FILE"
)

type Config struct {
	Enabled           bool     `json:"enabled"`
	Cron              string   `json:"cron"`
	TargetChatRoomID  string   `json:"target_chat_room_id,omitempty"`
	TargetChatRoomIDs []string `json:"target_chat_room_ids,omitempty"`
	SendOnWeekends    bool     `json:"send_on_weekends"`
	TestToken         string   `json:"test_token"`
	TopicsFile        string   `json:"topics_file"`
}

func DefaultConfig() Config {
	return Config{
		Cron:           "0 8 * * *",
		SendOnWeekends: true,
	}
}

func ConfigPath() string {
	if path := strings.TrimSpace(os.Getenv(ConfigPathEnv)); path != "" {
		return path
	}
	return DefaultConfigPath
}

// LoadConfig returns a disabled default configuration when the file does not exist.
func LoadConfig() (Config, error) {
	return LoadConfigFile(ConfigPath())
}

func LoadConfigFile(path string) (Config, error) {
	config := DefaultConfig()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return config, nil
	}
	if err != nil {
		return DefaultConfig(), fmt.Errorf("读取安全提醒配置失败: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return DefaultConfig(), fmt.Errorf("解析安全提醒配置失败: %w", err)
	}
	config.Cron = strings.TrimSpace(config.Cron)
	config.TargetChatRoomID = strings.TrimSpace(config.TargetChatRoomID)
	for index := range config.TargetChatRoomIDs {
		config.TargetChatRoomIDs[index] = strings.TrimSpace(config.TargetChatRoomIDs[index])
	}
	config.TestToken = strings.TrimSpace(config.TestToken)
	config.TopicsFile = strings.TrimSpace(config.TopicsFile)
	if config.Cron == "" {
		config.Cron = DefaultConfig().Cron
	}
	return config, nil
}

func (c Config) ValidateForSend() error {
	targets := c.Targets()
	if len(targets) == 0 {
		return errors.New("安全提醒目标群 ID 未配置")
	}
	for _, target := range targets {
		if !strings.HasSuffix(target, "@chatroom") {
			return fmt.Errorf("安全提醒目标 %q 必须是以 @chatroom 结尾的群 ID", target)
		}
	}
	return nil
}

// Targets returns the configured groups in order without empty or duplicate IDs.
// TargetChatRoomID is retained so existing single-group configurations keep working.
func (c Config) Targets() []string {
	targets := make([]string, 0, len(c.TargetChatRoomIDs)+1)
	seen := make(map[string]struct{}, len(c.TargetChatRoomIDs)+1)
	appendTarget := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, exists := seen[value]; exists {
			return
		}
		seen[value] = struct{}{}
		targets = append(targets, value)
	}
	appendTarget(c.TargetChatRoomID)
	for _, target := range c.TargetChatRoomIDs {
		appendTarget(target)
	}
	return targets
}
