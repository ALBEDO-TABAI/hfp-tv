package accounts

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func openTestService(t *testing.T) *Service {
	t.Helper()
	service, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })
	return service
}

func TestPasswordHashRoundTrip(t *testing.T) {
	hash, err := hashPassword("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$") || !verifyPassword(hash, "correct horse") || verifyPassword(hash, "wrong horse") {
		t.Fatal("argon2id hash does not round-trip")
	}
}

func TestLoginSessionAndDisable(t *testing.T) {
	ctx := context.Background()
	service := openTestService(t)
	admin, err := service.CreateUser(ctx, "", CreateUserInput{Username: "admin", Password: "admin-pass-1", Role: RoleAdmin})
	if err != nil {
		t.Fatal(err)
	}
	member, err := service.CreateUser(ctx, admin.ID, CreateUserInput{Username: "member", Password: "member-pass", RelayKey: "sk-relay-abcd1234"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateUser(ctx, admin.ID, CreateUserInput{Username: "MEMBER", Password: "member-pass"}); !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("case-insensitive duplicate allowed: %v", err)
	}
	if _, err := service.Authenticate(ctx, "member", "nope-nope", "10.0.0.1"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong password: %v", err)
	}
	if _, err := service.Authenticate(ctx, "ghost", "whatever1", "10.0.0.1"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("unknown user must look like a wrong password: %v", err)
	}
	user, err := service.Authenticate(ctx, "member", "member-pass", "10.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := service.IssueSession(ctx, user.ID, "10.0.0.1", "test")
	if err != nil {
		t.Fatal(err)
	}
	principal, err := service.ResolveSession(ctx, token)
	if err != nil || principal.User.ID != member.ID || principal.Workspace.OwnerUserID != member.ID {
		t.Fatalf("session did not resolve to the member's workspace: %+v %v", principal, err)
	}
	if principal.IsAdmin() {
		t.Fatal("member resolved as admin")
	}
	key, err := service.RelayKey(principal.Workspace)
	if err != nil || key != "sk-relay-abcd1234" {
		t.Fatalf("relay key not recoverable: %q %v", key, err)
	}
	if principal.Workspace.RelayKeyHint != "…1234" || strings.Contains(principal.Workspace.RelayKeyCiphertext, "abcd1234") {
		t.Fatal("relay key stored in clear or hint wrong")
	}

	adminToken, _, _ := service.IssueSession(ctx, admin.ID, "10.0.0.2", "test")
	adminPrincipal, err := service.ResolveSession(ctx, adminToken)
	if err != nil {
		t.Fatal(err)
	}
	disabled := StatusDisabled
	if _, err := service.UpdateUser(ctx, adminPrincipal, admin.ID, UserPatch{Status: &disabled}); !errors.Is(err, ErrCannotModifySelf) {
		t.Fatalf("admin disabled self: %v", err)
	}
	if _, err := service.UpdateUser(ctx, adminPrincipal, member.ID, UserPatch{Status: &disabled}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ResolveSession(ctx, token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("disabled user's session still valid: %v", err)
	}
	if _, err := service.Authenticate(ctx, "member", "member-pass", "10.0.0.1"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("disabled user can log in: %v", err)
	}
}

func TestSessionExpiresAndPasswordChangeRevokesOthers(t *testing.T) {
	ctx := context.Background()
	service := openTestService(t)
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	user, err := service.CreateUser(ctx, "", CreateUserInput{Username: "solo", Password: "first-pass"})
	if err != nil {
		t.Fatal(err)
	}
	current, _, _ := service.IssueSession(ctx, user.ID, "", "")
	other, _, _ := service.IssueSession(ctx, user.ID, "", "")
	principal, err := service.ResolveSession(ctx, current)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ChangePassword(ctx, principal, "wrong-pass", "second-pass"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("password changed without the current one: %v", err)
	}
	if err := service.ChangePassword(ctx, principal, "first-pass", "second-pass"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ResolveSession(ctx, other); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("other device kept its session after a password change")
	}
	if _, err := service.ResolveSession(ctx, current); err != nil {
		t.Fatal("current device was signed out by its own password change")
	}
	now = now.Add(SessionTTL + time.Minute)
	if _, err := service.ResolveSession(ctx, current); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("expired session still valid")
	}
}

func TestLoginRateLimit(t *testing.T) {
	ctx := context.Background()
	service := openTestService(t)
	if _, err := service.CreateUser(ctx, "", CreateUserInput{Username: "target", Password: "real-password"}); err != nil {
		t.Fatal(err)
	}
	for range loginFailuresPerUser {
		_, _ = service.Authenticate(ctx, "target", "bad-password", "198.51.100.9")
	}
	if _, err := service.Authenticate(ctx, "target", "real-password", "198.51.100.9"); !errors.Is(err, ErrTooManyAttempts) {
		t.Fatalf("limiter did not engage: %v", err)
	}
	if _, err := service.Authenticate(ctx, "target", "real-password", "198.51.100.10"); err != nil {
		t.Fatalf("another client was locked out: %v", err)
	}
}

func TestInvitationIsSingleUse(t *testing.T) {
	ctx := context.Background()
	service := openTestService(t)
	admin, _ := service.CreateUser(ctx, "", CreateUserInput{Username: "root", Password: "root-pass1", Role: RoleAdmin})
	token, _, _ := service.IssueSession(ctx, admin.ID, "", "")
	principal, _ := service.ResolveSession(ctx, token)
	invite, _, err := service.CreateInvitation(ctx, principal, RoleMember, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	user, err := service.AcceptInvitation(ctx, invite, "newbie", "新人", "newbie-pass")
	if err != nil || user.Role != RoleMember {
		t.Fatalf("accept failed: %v", err)
	}
	if _, err := service.AcceptInvitation(ctx, invite, "second", "", "second-pass"); !errors.Is(err, ErrInvitationInvalid) {
		t.Fatalf("invitation reused: %v", err)
	}
	if _, err := service.WorkspaceForUser(ctx, user.ID); err != nil {
		t.Fatal("invited user has no workspace")
	}
	audit, err := service.Audit(ctx, 10)
	if err != nil || len(audit) == 0 {
		t.Fatalf("audit empty: %v", err)
	}
}
