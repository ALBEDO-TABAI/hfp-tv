package accounts

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

const testPassword = "correct horse battery staple"

func testService(t *testing.T) *Service {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func testAdmin(t *testing.T, s *Service) (Identity, string) {
	t.Helper()
	identity, err := s.CreateAdmin(context.Background(), NewUser{Username: "admin", Password: testPassword})
	if err != nil {
		t.Fatal(err)
	}
	_, token, err := s.Login(context.Background(), "admin", testPassword, "192.0.2.1", "test")
	if err != nil {
		t.Fatal(err)
	}
	return identity, token
}

func TestPasswordHashAndVerify(t *testing.T) {
	hash, err := HashPassword(testPassword)
	if err != nil {
		t.Fatal(err)
	}
	second, err := HashPassword(testPassword)
	if err != nil {
		t.Fatal(err)
	}
	if hash == second || !strings.HasPrefix(hash, "$argon2id$") || strings.Contains(hash, testPassword) {
		t.Fatal("password hash is not salted argon2id")
	}
	if !VerifyPassword(hash, testPassword) || VerifyPassword(hash, "wrong password") {
		t.Fatal("password verification failed")
	}
	for _, invalid := range []string{"", "$argon2id$v=19$m=999999999,t=3,p=2$bad$bad", passwordPrefix() + "bad$bad"} {
		if VerifyPassword(invalid, testPassword) {
			t.Fatal("accepted invalid hash")
		}
	}
	for _, invalid := range []string{"short", strings.Repeat("a", 1025)} {
		if _, err := HashPassword(invalid); !errors.Is(err, ErrPassword) {
			t.Fatalf("invalid password: %v", err)
		}
	}
}

func TestSessionsCreationExpiryAndLogout(t *testing.T) {
	s := testService(t)
	admin, first := testAdmin(t, s)
	ctx := context.Background()
	_, second, err := s.Login(ctx, "admin", testPassword, "192.0.2.2", "test browser")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("session tokens must be random")
	}
	var stored Session
	if err := s.db.Where("token_hash = ?", tokenHash(second)).Take(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.TokenHash == second || len(stored.TokenHash) != 64 || stored.IP != "192.0.2.2" || stored.UserAgent != "test browser" {
		t.Fatal("invalid stored session")
	}
	me, err := s.Authenticate(ctx, second)
	if err != nil || me.User.ID != admin.User.ID || me.WorkspaceID != admin.WorkspaceID {
		t.Fatalf("identity: %+v %v", me, err)
	}
	if err := s.Logout(ctx, first); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(ctx, first); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("logged out token accepted: %v", err)
	}
	if _, err := s.Authenticate(ctx, second); err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return stored.ExpiresAt }
	if _, err := s.Authenticate(ctx, second); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("expired token accepted: %v", err)
	}
	if _, err := s.Authenticate(ctx, ""); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("empty token accepted: %v", err)
	}
}

func TestAdminLifecycleAndServiceAuthorization(t *testing.T) {
	s := testService(t)
	_, admin := testAdmin(t, s)
	ctx := context.Background()
	member, err := s.CreateUser(ctx, admin, NewUser{Username: "member", Password: testPassword})
	if err != nil {
		t.Fatal(err)
	}
	var workspace Workspace
	if err := s.db.First(&workspace, "id = ?", member.WorkspaceID).Error; err != nil {
		t.Fatal(err)
	}
	if workspace.DataDir != filepath.Join(s.root, "workspaces", workspace.ID) || workspace.OwnerUserID != member.User.ID {
		t.Fatal("workspace registration mismatch")
	}
	if _, err := os.Stat(workspace.DataDir); !os.IsNotExist(err) {
		t.Fatal("creating an account must not start or create a kernel")
	}
	_, token, err := s.Login(ctx, "member", testPassword, "192.0.2.1", "test")
	if err != nil {
		t.Fatal(err)
	}
	_, otherToken, err := s.Login(ctx, "member", testPassword, "192.0.2.2", "test")
	if err != nil {
		t.Fatal(err)
	}
	_, listErr := s.ListUsers(ctx, token)
	_, createErr := s.CreateUser(ctx, token, NewUser{})
	_, _, inviteErr := s.CreateInvitation(ctx, token, RoleAdmin, time.Time{})
	for _, err := range []error{listErr, createErr, inviteErr, s.DisableUser(ctx, token, member.User.ID), s.ResetPassword(ctx, token, member.User.ID, testPassword)} {
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("member admin operation: %v", err)
		}
	}
	users, err := s.ListUsers(ctx, admin)
	if err != nil || len(users) != 2 {
		t.Fatalf("list users: %v %v", users, err)
	}
	const changed = "replacement password 123"
	if err := s.ResetPassword(ctx, admin, member.User.ID, changed); err != nil {
		t.Fatal(err)
	}
	for _, old := range []string{token, otherToken} {
		if _, err := s.Authenticate(ctx, old); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("reset retained session: %v", err)
		}
	}
	if _, _, err := s.Login(ctx, "member", testPassword, "192.0.2.1", "test"); !errors.Is(err, ErrCredentials) {
		t.Fatalf("old password accepted: %v", err)
	}
	_, token, err = s.Login(ctx, "member", changed, "192.0.2.1", "test")
	if err != nil {
		t.Fatal(err)
	}
	_, otherToken, err = s.Login(ctx, "member", changed, "192.0.2.2", "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DisableUser(ctx, admin, member.User.ID); err != nil {
		t.Fatal(err)
	}
	for _, old := range []string{token, otherToken} {
		if _, err := s.Authenticate(ctx, old); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("disabled user retained session: %v", err)
		}
	}
	if _, _, err := s.Login(ctx, "member", changed, "192.0.2.1", "test"); !errors.Is(err, ErrCredentials) {
		t.Fatalf("disabled login: %v", err)
	}
	if err := s.DisableUser(ctx, admin, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing target: %v", err)
	}
	var actions []string
	if err := s.db.Model(&PlatformAuditEvent{}).Order("created_at").Pluck("action", &actions).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actions, []string{"admin.create", "user.create", "user.reset_password", "user.disable"}) {
		t.Fatalf("audit actions: %v", actions)
	}
	var count int64
	if err := s.db.Model(&Session{}).Where("user_id = ?", member.User.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("sessions not deleted: %d %v", count, err)
	}
}

func TestInvitationSingleUseExpiryAndRollback(t *testing.T) {
	s := testService(t)
	_, admin := testAdmin(t, s)
	ctx := context.Background()
	invitation, token, err := s.CreateInvitation(ctx, admin, RoleMember, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if invitation.TokenHash != tokenHash(token) || invitation.TokenHash == token {
		t.Fatal("invitation stored in plaintext")
	}
	if _, err := s.AcceptInvitation(ctx, token, NewUser{Username: "admin", Password: testPassword}); !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("duplicate username: %v", err)
	}
	member, err := s.AcceptInvitation(ctx, token, NewUser{Username: "invited", Password: testPassword, Role: RoleAdmin})
	if err != nil || member.User.Role != RoleMember {
		t.Fatalf("invitation was consumed on failure or role escalated: %+v %v", member, err)
	}
	if _, err := s.AcceptInvitation(ctx, token, NewUser{Username: "reused", Password: testPassword}); !errors.Is(err, ErrInvitation) {
		t.Fatalf("reused invitation: %v", err)
	}
	_, token, err = s.CreateInvitation(ctx, admin, RoleMember, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, username := range []string{"racer-one", "racer-two"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.AcceptInvitation(ctx, token, NewUser{Username: username, Password: testPassword})
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	succeeded := 0
	for err := range results {
		if err == nil {
			succeeded++
		} else if !errors.Is(err, ErrInvitation) {
			t.Fatal(err)
		}
	}
	if succeeded != 1 {
		t.Fatalf("concurrent accept succeeded %d times", succeeded)
	}
	expires := s.now().Add(time.Minute)
	_, expired, err := s.CreateInvitation(ctx, admin, RoleAdmin, expires)
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return expires }
	if _, err := s.AcceptInvitation(ctx, expired, NewUser{Username: "expired", Password: testPassword}); !errors.Is(err, ErrInvitation) {
		t.Fatalf("expired invitation: %v", err)
	}
}

func TestLoginRateLimitByUsernameAndIP(t *testing.T) {
	s := testService(t)
	testAdmin(t, s)
	ctx := context.Background()
	for i := 0; i < loginFailureLimit; i++ {
		if _, _, err := s.Login(ctx, " ADMIN ", "incorrect", "192.0.2.10", "test"); !errors.Is(err, ErrCredentials) {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	if _, _, err := s.Login(ctx, "admin", testPassword, "192.0.2.10", "test"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("limit bypass: %v", err)
	}
	if _, _, err := s.Login(ctx, "admin", testPassword, "192.0.2.11", "test"); err != nil {
		t.Fatalf("other IP blocked: %v", err)
	}
	if _, _, err := s.Login(ctx, "unknown", testPassword, "192.0.2.10", "test"); !errors.Is(err, ErrCredentials) {
		t.Fatalf("unknown user: %v", err)
	}
	if ErrCredentials.Message != ErrRateLimited.Message {
		t.Fatal("login failure reveals account state")
	}
	later := s.now().Add(loginWindow)
	s.now = func() time.Time { return later }
	if _, _, err := s.Login(ctx, "admin", testPassword, "192.0.2.10", "test"); err != nil {
		t.Fatalf("window did not expire: %v", err)
	}
}

func TestPlatformMigrationAuditAndAtomicWrites(t *testing.T) {
	s := testService(t)
	_, token := testAdmin(t, s)
	var tables []string
	if err := s.db.Raw("SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'").Scan(&tables).Error; err != nil {
		t.Fatal(err)
	}
	sort.Strings(tables)
	if !reflect.DeepEqual(tables, []string{"invitations", "platform_audit_events", "sessions", "users", "workspaces"}) {
		t.Fatalf("unexpected schema: %v", tables)
	}
	if err := s.db.Exec("CREATE TABLE unrelated_data (value TEXT)").Error; err != nil {
		t.Fatal(err)
	}
	if err := s.db.Exec("INSERT INTO unrelated_data VALUES ('keep')").Error; err != nil {
		t.Fatal(err)
	}
	if err := Migrate(s.db); err != nil {
		t.Fatal(err)
	}
	var value string
	if err := s.db.Raw("SELECT value FROM unrelated_data").Scan(&value).Error; err != nil || value != "keep" {
		t.Fatalf("migration deleted unrelated data: %v", err)
	}
	for _, sql := range []string{"UPDATE platform_audit_events SET action='changed'", "DELETE FROM platform_audit_events"} {
		if err := s.db.Exec(sql).Error; err == nil {
			t.Fatal("audit mutation allowed")
		}
	}
	if err := s.db.Exec("CREATE TRIGGER fail_audit BEFORE INSERT ON platform_audit_events BEGIN SELECT RAISE(ABORT, 'test failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser(context.Background(), token, NewUser{Username: "rollback", Password: testPassword}); err == nil {
		t.Fatal("unlogged admin mutation succeeded")
	}
	for _, table := range []string{"users", "workspaces", "platform_audit_events"} {
		var count int64
		if err := s.db.Table(table).Count(&count).Error; err != nil || count != 1 {
			t.Fatalf("partial write to %s: %d %v", table, count, err)
		}
	}
	info, err := os.Stat(filepath.Join(s.root, "platform.db"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("database permissions: %v %v", info, err)
	}
}
