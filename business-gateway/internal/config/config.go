package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ListenAddr         string
	BindingsFile       string
	BackendURL         string
	BotToken           string
	InternalRouteToken string
	WebhookToken       string
	AdminToken         string
	AdminWxIDs         map[string]struct{}
	BackendTimeout     time.Duration
	DedupTTL           time.Duration
	RequireAtMention   bool
}

func Load() (Config, error) {
	cfg := Config{
		ListenAddr:         envOrDefault("LISTEN_ADDR", ":8080"),
		BindingsFile:       envOrDefault("BINDINGS_FILE", "/data/groups.json"),
		BackendURL:         strings.TrimSpace(os.Getenv("BACKEND_URL")),
		BotToken:           strings.TrimSpace(os.Getenv("BOT_TOKEN")),
		InternalRouteToken: strings.TrimSpace(os.Getenv("INTERNAL_ROUTE_TOKEN")),
		WebhookToken:       strings.TrimSpace(os.Getenv("WEBHOOK_TOKEN")),
		AdminToken:         strings.TrimSpace(os.Getenv("ADMIN_TOKEN")),
		AdminWxIDs:         parseSet(os.Getenv("ADMIN_WXIDS")),
		BackendTimeout:     durationFromSeconds("BACKEND_TIMEOUT_SEC", 10*time.Second),
		DedupTTL:           durationFromSeconds("DEDUP_TTL_SEC", 24*time.Hour),
		RequireAtMention:   boolOrDefault("REQUIRE_AT_MENTION", true),
	}

	missing := make([]string, 0, 5)
	for _, item := range []struct {
		key   string
		value string
	}{
		{"BACKEND_URL", cfg.BackendURL},
		{"BOT_TOKEN", cfg.BotToken},
		{"INTERNAL_ROUTE_TOKEN", cfg.InternalRouteToken},
		{"WEBHOOK_TOKEN", cfg.WebhookToken},
		{"ADMIN_TOKEN", cfg.AdminToken},
	} {
		if item.value == "" {
			missing = append(missing, item.key)
		}
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("缺少必需环境变量: %s", strings.Join(missing, ", "))
	}
	if cfg.BackendTimeout <= 0 || cfg.DedupTTL <= 0 {
		return Config{}, errors.New("超时和去重 TTL 必须大于 0")
	}
	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func durationFromSeconds(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil {
		return -1
	}
	return time.Duration(seconds) * time.Second
}

func boolOrDefault(key string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return value
}

func parseSet(raw string) map[string]struct{} {
	result := make(map[string]struct{})
	for _, item := range strings.Split(raw, ",") {
		if value := strings.TrimSpace(item); value != "" {
			result[value] = struct{}{}
		}
	}
	return result
}
