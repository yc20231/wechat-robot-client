package config

import "testing"

func TestNoticeAuthReuseRequiresOptIn(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("OWNER_WXIDS", "owner")
	t.Setenv("PRODUCTION_NOTICE_ENABLED", "false")
	t.Setenv("PRODUCTION_NOTICE_TOKEN", "")
	t.Setenv("PRODUCTION_NOTICE_ROBOT_TOKEN", "")
	t.Setenv("PRODUCTION_NOTICE_REUSE_EXISTING_AUTH", "false")
	cfg, err := Load()
	if err != nil || cfg.NoticeToken != "" || cfg.NoticeRobotToken != "" || cfg.NoticeEnabled {
		t.Fatal("must not implicitly enable notices or reuse tokens", err)
	}
	t.Setenv("PRODUCTION_NOTICE_REUSE_EXISTING_AUTH", "true")
	cfg, err = Load()
	if err != nil || cfg.NoticeToken != cfg.BotToken || cfg.NoticeRobotToken != cfg.InternalRouteToken || cfg.NoticeEnabled {
		t.Fatal("reuse must resolve both tokens without enabling worker", err)
	}
	t.Setenv("PRODUCTION_NOTICE_TOKEN", "dedicated-backend")
	t.Setenv("PRODUCTION_NOTICE_ROBOT_TOKEN", "dedicated-robot")
	cfg, err = Load()
	if err != nil || cfg.NoticeToken != "dedicated-backend" || cfg.NoticeRobotToken != "dedicated-robot" {
		t.Fatal("dedicated tokens must take precedence", err)
	}
}

func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("BACKEND_URL", "https://example.com")
	t.Setenv("BOT_TOKEN", "bot-token")
	t.Setenv("INTERNAL_ROUTE_TOKEN", "route-token")
	t.Setenv("WEBHOOK_TOKEN", "webhook-token")
	t.Setenv("ADMIN_TOKEN", "admin-token")
}

func TestLoadPrefersOwnerWxIDs(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("OWNER_WXIDS", " owner-one,owner-two ")
	t.Setenv("ADMIN_WXIDS", "legacy-owner")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.OwnerWxIDs) != 2 {
		t.Fatalf("owners = %#v", cfg.OwnerWxIDs)
	}
	if _, ok := cfg.OwnerWxIDs["owner-one"]; !ok {
		t.Fatalf("owner-one missing from %#v", cfg.OwnerWxIDs)
	}
	if _, ok := cfg.OwnerWxIDs["legacy-owner"]; ok {
		t.Fatalf("legacy owner unexpectedly merged into %#v", cfg.OwnerWxIDs)
	}
}

func TestLoadFallsBackToLegacyAdminWxIDs(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("OWNER_WXIDS", "")
	t.Setenv("ADMIN_WXIDS", "legacy-owner")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.OwnerWxIDs["legacy-owner"]; !ok {
		t.Fatalf("legacy owner missing from %#v", cfg.OwnerWxIDs)
	}
}

func TestLoadRequiresFixedOwner(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("OWNER_WXIDS", "")
	t.Setenv("ADMIN_WXIDS", "")

	if _, err := Load(); err == nil {
		t.Fatal("Load accepted configuration without a fixed owner")
	}
}
