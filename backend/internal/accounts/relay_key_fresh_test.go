package accounts

import (
	"context"
	"testing"
)

func TestRelayKeyReadsLatestBinding(t *testing.T) {
	s := openTestService(t)
	ctx := context.Background()
	admin, err := s.CreateUser(ctx, "", CreateUserInput{Username: "admin", Password: "admin-password", Role: RoleAdmin, RelayKey: "SYNTHETIC_OLD_KEY"})
	if err != nil {
		t.Fatal(err)
	}
	stale, err := s.WorkspaceForUser(ctx, admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	actor := &Principal{User: *admin}
	for _, key := range []string{"SYNTHETIC_NEW_KEY", ""} {
		if _, _, err := s.SetRelayKey(ctx, actor, admin.ID, key); err != nil {
			t.Fatal(err)
		}
		got, err := s.RelayKey(*stale)
		if err != nil || got != key {
			t.Fatalf("stale workspace did not resolve current binding: %v", err)
		}
	}
}

func TestRelayKeyRejectsAValueTooShortToBeAKey(t *testing.T) {
	s := openTestService(t)
	ctx := context.Background()
	admin, err := s.CreateUser(ctx, "", CreateUserInput{Username: "admin", Password: "admin-password", Role: RoleAdmin})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.SetRelayKey(ctx, &Principal{User: *admin}, admin.ID, "sk-1234"); err == nil {
		t.Fatal("a seven-character relay key was accepted")
	}
}
