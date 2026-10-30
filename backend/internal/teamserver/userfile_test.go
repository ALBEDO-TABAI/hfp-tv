package teamserver

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"infinite-canvas/backend/internal/accounts"
)

func TestAccountFilesDriveLoginKeysAndDisable(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	dir := t.TempDir()
	files := AccountFiles{UsersPath: filepath.Join(dir, "users.txt"), KeysPath: filepath.Join(dir, "user-keys.txt")}
	if err := writeFile(files.UsersPath, "# 团队成员\nalice:alice-password\nbob:bob-password\n"); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(files.KeysPath, "alice:sk-alice-file-1111\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.server.SyncAccountFiles(ctx, files); err != nil {
		t.Fatal(err)
	}
	if _, err := h.users.CreateUser(ctx, "", accounts.CreateUserInput{Username: "admin", Password: "admin-password", Role: accounts.RoleAdmin}); err != nil {
		t.Fatal(err)
	}
	aliceCookie := h.login("alice", "alice-password")
	adminCookie := h.login("admin", "admin-password")
	config := h.do(call{method: http.MethodGet, path: "/api/workspace/model-config", cookie: aliceCookie}).Body.String()
	if !strings.Contains(config, "sk-alice-file-1111") {
		t.Fatalf("file key not provisioned into alice's workspace: %s", config)
	}

	// A key change reaches the open kernel without a restart.
	if err := writeFile(files.KeysPath, "alice:sk-alice-file-2222\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.server.SyncAccountFiles(ctx, files); err != nil {
		t.Fatal(err)
	}
	config = h.do(call{method: http.MethodGet, path: "/api/workspace/model-config", cookie: aliceCookie}).Body.String()
	if !strings.Contains(config, "sk-alice-file-2222") || strings.Contains(config, "sk-alice-file-1111") {
		t.Fatalf("changed file key not applied: %s", config)
	}

	// The console shows file accounts but cannot change what the file owns.
	list := h.do(call{method: http.MethodGet, path: "/api/platform/admin/users", cookie: adminCookie}).Body.String()
	if !strings.Contains(list, `"source":"file"`) || strings.Contains(list, "sk-alice-file") {
		t.Fatalf("admin user list = %s", list)
	}
	alice := mustFindUser(t, h, "alice")
	reset := h.do(call{method: http.MethodPost, path: "/api/platform/admin/users/" + alice + "/password", cookie: adminCookie, body: `{"password":"console-password"}`})
	if reset.Code != http.StatusConflict || reason(t, reset) != "account_managed_by_file" {
		t.Fatalf("console reset = %d %s", reset.Code, reset.Body.String())
	}

	// Removing alice's lines signs her out at once.
	if err := writeFile(files.UsersPath, "bob:bob-password\n"); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(files.KeysPath, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := h.server.SyncAccountFiles(ctx, files); err != nil {
		t.Fatal(err)
	}
	if response := h.do(call{method: http.MethodGet, path: "/api/canvas-projects", cookie: aliceCookie}); response.Code != http.StatusUnauthorized {
		t.Fatalf("removed account still served: %d", response.Code)
	}

	// A broken edit is rejected as a whole; bob keeps working.
	if err := writeFile(files.UsersPath, "bob:bob-password\ncarol:short\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.server.SyncAccountFiles(ctx, files); err == nil || strings.Contains(err.Error(), "short") {
		t.Fatalf("broken file = %v", err)
	}
	h.login("bob", "bob-password")
}

func TestWatchAccountFilesAppliesStableChanges(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	files := AccountFiles{UsersPath: filepath.Join(dir, "users.txt")}
	if err := writeFile(files.UsersPath, "alice:alice-password\n"); err != nil {
		t.Fatal(err)
	}
	fingerprint, _ := files.Fingerprint()
	if _, err := h.server.SyncAccountFiles(context.Background(), files); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go h.server.WatchAccountFiles(ctx, files, 20*time.Millisecond, fingerprint)
	if err := writeFile(files.UsersPath, "alice:alice-password\nbob:bob-password\n"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := h.users.Authenticate(context.Background(), "bob", "bob-password", "10.0.0.9"); err == nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("watcher did not apply the new account")
}

func mustFindUser(t *testing.T, h *harness, username string) string {
	t.Helper()
	views, err := h.users.ListUsers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, view := range views {
		if view.Username == username {
			return view.ID
		}
	}
	t.Fatalf("user %s not found", username)
	return ""
}
