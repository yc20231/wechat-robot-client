package config

import "testing"

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
