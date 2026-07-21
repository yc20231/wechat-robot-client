package group

import (
	"path/filepath"
	"testing"
)

func TestFileStorePersistsBindings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "groups.json")
	store, err := NewFileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	binding := Binding{GroupID: "customer@chatroom", GroupName: "客户群", Type: TypeCustomer, CustomerCode: "270", Enabled: true}
	if err := store.Upsert(binding); err != nil {
		t.Fatal(err)
	}

	reloaded, err := NewFileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := reloaded.Get(binding.GroupID)
	if !ok || got != binding {
		t.Fatalf("reloaded binding = %+v, %t; want %+v", got, ok, binding)
	}
	if err := reloaded.Delete(binding.GroupID); err != nil {
		t.Fatal(err)
	}
	if _, ok := reloaded.Get(binding.GroupID); ok {
		t.Fatal("binding still exists after delete")
	}
}

func TestValidateRejectsUnsafeBindings(t *testing.T) {
	tests := []Binding{
		{GroupID: "customer@chatroom", Type: TypeCustomer, Enabled: true},
		{GroupID: "admin@chatroom", Type: TypeAdmin, CustomerCode: "*", Enabled: true},
		{GroupID: "other@chatroom", Type: "other", Enabled: true},
	}
	for _, binding := range tests {
		if err := Validate(binding); err == nil {
			t.Fatalf("Validate(%+v) unexpectedly succeeded", binding)
		}
	}
}
