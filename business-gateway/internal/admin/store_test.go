package admin

import (
	"path/filepath"
	"testing"
)

func TestFileStorePersistsDynamicRolesAndProtectsOwner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "admins.json")
	owners := map[string]struct{}{"owner-wxid": {}}
	store, err := NewFileStore(path, owners)
	if err != nil {
		t.Fatal(err)
	}
	if !store.IsOwner("owner-wxid") || !store.IsRoot("owner-wxid") || !store.IsAdmin("owner-wxid") {
		t.Fatal("fixed owner roles were not applied")
	}
	if err := store.SetRole("root-wxid", RoleRoot); err != nil {
		t.Fatal(err)
	}
	if err := store.SetRole("admin-wxid", RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if err := store.SetRole("owner-wxid", RoleAdmin); err == nil {
		t.Fatal("fixed owner was modified")
	}

	reloaded, err := NewFileStore(path, owners)
	if err != nil {
		t.Fatal(err)
	}
	if !reloaded.IsRoot("root-wxid") || !reloaded.IsAdmin("admin-wxid") {
		t.Fatalf("dynamic roles were not persisted: %+v", reloaded.List())
	}
	if err := reloaded.DemoteRoot("root-wxid"); err != nil {
		t.Fatal(err)
	}
	role, ok := reloaded.RoleOf("root-wxid")
	if !ok || role != RoleAdmin {
		t.Fatalf("demoted role = %q, %t", role, ok)
	}
	if err := reloaded.Delete("root-wxid"); err != nil {
		t.Fatal(err)
	}
	if reloaded.IsAdmin("root-wxid") {
		t.Fatal("dynamic admin still exists after delete")
	}
}
