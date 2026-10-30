package accounts

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeAccountFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseAccountFilesRejectsBadLinesWithoutEchoingSecrets(t *testing.T) {
	dir := t.TempDir()
	keys := writeAccountFile(t, dir, "keys.txt", "alice:sk-alice\n")
	cases := map[string]string{
		"no colon":       "alice secret-password\n",
		"bad username":   "a b:secret-password\n",
		"short password": "alice:short\n",
		"duplicate":      "alice:secret-password\nALICE:other-password\n",
		"empty":          "# nobody yet\n\n",
	}
	for name, content := range cases {
		users := writeAccountFile(t, dir, "users.txt", content)
		_, err := ParseAccountFiles(users, keys)
		if err == nil {
			t.Fatalf("%s: accepted", name)
		}
		if strings.Contains(err.Error(), "-password") || strings.Contains(err.Error(), "short\n") {
			t.Fatalf("%s: error echoes the password: %v", name, err)
		}
	}
	users := writeAccountFile(t, dir, "users.txt", "alice:secret-password\n")
	for name, content := range map[string]string{
		"unknown user": "bob:sk-bob\n",
		"blank key":    "alice:\n",
		"spaced key":   "alice:sk with space\n",
		"duplicate":    "alice:sk-1\nalice:sk-2\n",
	} {
		keys := writeAccountFile(t, dir, "keys.txt", content)
		_, err := ParseAccountFiles(users, keys)
		if err == nil {
			t.Fatalf("keys %s: accepted", name)
		}
		if strings.Contains(err.Error(), "sk-") {
			t.Fatalf("keys %s: error echoes the key: %v", name, err)
		}
	}
}

func TestParseAccountFilesKeepsColonsAndComments(t *testing.T) {
	dir := t.TempDir()
	users := writeAccountFile(t, dir, "users.txt", "\xef\xbb\xbf# team\r\n alice : pass:with:colons \r\n\r\nbob:another-password\n")
	keys := writeAccountFile(t, dir, "keys.txt", "# keys\nalice:sk:alice\n")
	entries, err := ParseAccountFiles(users, keys)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Username != "alice" || entries[0].Password != "pass:with:colons" || entries[0].RelayKey != "sk:alice" || entries[1].RelayKey != "" {
		t.Fatalf("unexpected entries: %+v", entries)
	}
}

func TestSyncFileAccountsCreatesUpdatesAndDisables(t *testing.T) {
	ctx := context.Background()
	service := openTestService(t)
	admin, err := service.CreateUser(ctx, "", CreateUserInput{Username: "admin", Password: "admin-password", Role: RoleAdmin})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.SyncFileAccounts(ctx, []FileAccount{{Username: "alice", Password: "alice-password", RelayKey: "sk-alice"}, {Username: "bob", Password: "bob-password"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Created) != 2 || len(result.RelayChanged) != 0 {
		t.Fatalf("first sync: %+v", result)
	}
	alice, err := service.Authenticate(ctx, "alice", "alice-password", "10.0.0.1")
	if err != nil || alice.Source != SourceFile || alice.Role != RoleMember {
		t.Fatalf("file account cannot log in: %+v %v", alice, err)
	}
	workspace, _ := service.WorkspaceForUser(ctx, alice.ID)
	if key, _ := service.RelayKey(*workspace); key != "sk-alice" {
		t.Fatalf("relay key = %q", key)
	}
	token, _, err := service.IssueSession(ctx, alice.ID, "10.0.0.1", "test")
	if err != nil {
		t.Fatal(err)
	}

	// Re-applying the same content changes nothing and keeps sessions.
	result, err = service.SyncFileAccounts(ctx, []FileAccount{{Username: "alice", Password: "alice-password", RelayKey: "sk-alice"}, {Username: "bob", Password: "bob-password"}})
	if err != nil || result.Changed() {
		t.Fatalf("idempotent sync changed something: %+v %v", result, err)
	}
	if _, err := service.ResolveSession(ctx, token); err != nil {
		t.Fatalf("unchanged sync revoked the session: %v", err)
	}

	// A new password signs the user out; a new key is reported for re-provisioning;
	// a removed line disables the account but keeps its workspace.
	result, err = service.SyncFileAccounts(ctx, []FileAccount{{Username: "alice", Password: "alice-new-password", RelayKey: "sk-alice-2"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.RelayChanged) != 1 || result.RelayChanged[0].OwnerUserID != alice.ID || len(result.Disabled) != 1 || result.Disabled[0] != "bob" || len(result.DisabledWorkspaces) != 1 {
		t.Fatalf("second sync: %+v", result)
	}
	if _, err := service.ResolveSession(ctx, token); err == nil {
		t.Fatal("password change kept the old session")
	}
	if _, err := service.Authenticate(ctx, "alice", "alice-password", "10.0.0.2"); err == nil {
		t.Fatal("old password still works")
	}
	if _, err := service.Authenticate(ctx, "bob", "bob-password", "10.0.0.3"); err == nil {
		t.Fatal("removed account can still log in")
	}
	if user, err := service.Authenticate(ctx, "admin", "admin-password", "10.0.0.4"); err != nil || user.Status != StatusActive {
		t.Fatal("console-created admin was touched by the file sync")
	}

	// Listing bob again re-enables him with his original workspace.
	bobBefore, _ := service.WorkspaceForUser(ctx, mustUserID(t, service, "bob"))
	if _, err := service.SyncFileAccounts(ctx, []FileAccount{{Username: "alice", Password: "alice-new-password", RelayKey: "sk-alice-2"}, {Username: "bob", Password: "bob-password"}}); err != nil {
		t.Fatal(err)
	}
	bob, err := service.Authenticate(ctx, "bob", "bob-password", "10.0.0.5")
	if err != nil {
		t.Fatalf("re-listed account cannot log in: %v", err)
	}
	if bobAfter, _ := service.WorkspaceForUser(ctx, bob.ID); bobAfter.ID != bobBefore.ID {
		t.Fatal("re-enabled account got a new workspace")
	}

	// The console cannot change what the file owns.
	principal := &Principal{User: *admin}
	if err := service.SetPassword(ctx, principal, bob.ID, "console-password"); !errors.Is(err, ErrManagedByFile) {
		t.Fatalf("console password reset = %v", err)
	}
	if _, _, err := service.SetRelayKey(ctx, principal, bob.ID, "sk-console"); !errors.Is(err, ErrManagedByFile) {
		t.Fatalf("console relay key = %v", err)
	}
	disabled := StatusDisabled
	if _, err := service.UpdateUser(ctx, principal, bob.ID, UserPatch{Status: &disabled}); !errors.Is(err, ErrManagedByFile) {
		t.Fatalf("console disable = %v", err)
	}
	name, role := "Bob", RoleAdmin
	if _, err := service.UpdateUser(ctx, principal, bob.ID, UserPatch{DisplayName: &name, Role: &role}); err != nil {
		t.Fatalf("display name and role should stay editable: %v", err)
	}
	if err := service.ChangePassword(ctx, &Principal{User: *bob}, "bob-password", "bob-own-password"); !errors.Is(err, ErrManagedByFile) {
		t.Fatalf("self password change = %v", err)
	}
	if err := service.ResetPasswordByUsername(ctx, "bob", "cli-password"); !errors.Is(err, ErrManagedByFile) {
		t.Fatalf("cli password reset = %v", err)
	}
}

func TestSyncFileAccountsAdoptsConsoleAccountAndAcceptsHashes(t *testing.T) {
	ctx := context.Background()
	service := openTestService(t)
	carol, err := service.CreateUser(ctx, "", CreateUserInput{Username: "carol", Password: "console-password", RelayKey: "sk-console"})
	if err != nil {
		t.Fatal(err)
	}
	hash, err := HashPassword("carol-file-password")
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.SyncFileAccounts(ctx, []FileAccount{{Username: "Carol", Password: hash}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Created) != 0 || len(result.Updated) != 1 || len(result.RelayChanged) != 1 {
		t.Fatalf("adoption: %+v", result)
	}
	user, err := service.Authenticate(ctx, "carol", "carol-file-password", "10.0.0.1")
	if err != nil || user.ID != carol.ID || user.Source != SourceFile {
		t.Fatalf("adopted account: %+v %v", user, err)
	}
	workspace, _ := service.WorkspaceForUser(ctx, carol.ID)
	if workspace.RelayKeyCiphertext != "" {
		t.Fatal("a file without a key line should clear the console-bound key")
	}
	// The same hash again is not a password change.
	if result, err := service.SyncFileAccounts(ctx, []FileAccount{{Username: "carol", Password: hash}}); err != nil || result.Changed() {
		t.Fatalf("hash re-sync changed something: %+v %v", result, err)
	}
	audit, _ := service.Audit(ctx, 10)
	if len(audit) == 0 || audit[0].ActorUsername != "账号文件" {
		t.Fatalf("file changes should be attributed to the account file: %+v", audit)
	}
}

func mustUserID(t *testing.T, service *Service, username string) string {
	t.Helper()
	var user User
	if err := service.db.Where("username = ?", username).First(&user).Error; err != nil {
		t.Fatal(err)
	}
	return user.ID
}
