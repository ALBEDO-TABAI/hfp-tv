package relaychannel

import (
	"testing"

	"infinite-canvas/backend/internal/accounts"
)

func TestSavedDefinitionSurvivesARestartAndIsAudited(t *testing.T) {
	root := t.TempDir()
	users, err := accounts.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := Open(users.DB())
	if err != nil {
		t.Fatal(err)
	}
	if saved, err := store.Load(); err != nil || saved != nil {
		t.Fatalf("empty store = %v %v", saved, err)
	}
	template := map[string]any{"name": "公司中转站", "baseUrl": "http://10.10.10.45:3000/v1", "models": []any{"gpt-image-2", "doubao-seedance-2.0-fast"}}
	if _, err := store.Save("admin-1", template); err != nil {
		t.Fatal(err)
	}
	if err := users.Close(); err != nil {
		t.Fatal(err)
	}

	users, err = accounts.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer users.Close()
	store, err = Open(users.DB())
	if err != nil {
		t.Fatal(err)
	}
	saved, err := store.Load()
	if err != nil || saved == nil || saved.UpdatedBy != "admin-1" || saved.Template["baseUrl"] != "http://10.10.10.45:3000/v1" {
		t.Fatalf("after restart = %+v %v", saved, err)
	}
	if err := store.Delete("admin-1"); err != nil {
		t.Fatal(err)
	}
	if saved, _ := store.Load(); saved != nil {
		t.Fatalf("definition survived delete: %+v", saved)
	}
	var actions []string
	if err := users.DB().Model(&accounts.AuditEvent{}).Order("id").Pluck("action", &actions).Error; err != nil {
		t.Fatal(err)
	}
	if len(actions) != 2 || actions[0] != "relay_channel.update" || actions[1] != "relay_channel.reset" {
		t.Fatalf("audit = %v", actions)
	}
}
