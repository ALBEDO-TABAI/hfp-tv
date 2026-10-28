package accounts

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	SessionLifetime    = 7 * 24 * time.Hour
	InvitationLifetime = 7 * 24 * time.Hour
	loginWindow        = 15 * time.Minute
	loginFailureLimit  = 5
	maxLoginBuckets    = 10000
)

type loginKey struct{ username, ip string }
type loginAttempts struct {
	count int
	until time.Time
}

type Service struct {
	db       *gorm.DB
	root     string
	now      func() time.Time
	loginMu  sync.Mutex
	attempts map[loginKey]loginAttempts
}

func normalizeUsername(value string) string { return strings.ToLower(strings.TrimSpace(value)) }

func prepareUser(input NewUser) (User, error) {
	input.Username = normalizeUsername(input.Username)
	if input.Username == "" || utf8.RuneCountInString(input.Username) > 64 || strings.ContainsFunc(input.Username, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
		return User{}, ErrInvalidInput
	}
	if input.Role == "" {
		input.Role = RoleMember
	}
	if input.Role != RoleMember && input.Role != RoleAdmin {
		return User{}, ErrInvalidInput
	}
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	if utf8.RuneCountInString(input.DisplayName) > 120 {
		return User{}, ErrInvalidInput
	}
	if input.DisplayName == "" {
		input.DisplayName = input.Username
	}
	hash, err := HashPassword(input.Password)
	if err != nil {
		return User{}, err
	}
	return User{ID: uuid.NewString(), Username: input.Username, DisplayName: input.DisplayName, PasswordHash: hash, Role: input.Role, Status: StatusActive}, nil
}

func (s *Service) createUser(tx *gorm.DB, user User) (Identity, error) {
	if err := tx.Create(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return Identity{}, ErrUsernameTaken
		}
		return Identity{}, err
	}
	workspace := Workspace{ID: uuid.NewString(), OwnerUserID: user.ID, Status: StatusActive}
	workspace.DataDir = filepath.Join(s.root, "workspaces", workspace.ID)
	if err := tx.Create(&workspace).Error; err != nil {
		return Identity{}, err
	}
	return Identity{User: user, WorkspaceID: workspace.ID}, nil
}

func (s *Service) audit(tx *gorm.DB, actorID, action, targetID string) error {
	return tx.Create(&PlatformAuditEvent{ID: uuid.NewString(), ActorUserID: actorID, Action: action, TargetID: targetID, CreatedAt: s.now().UTC()}).Error
}

// 仅供本机 admin create 命令使用；HTTP 创建用户必须走 CreateUser。
func (s *Service) CreateAdmin(ctx context.Context, input NewUser) (Identity, error) {
	input.Role = RoleAdmin
	user, err := prepareUser(input)
	if err != nil {
		return Identity{}, err
	}
	var identity Identity
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		identity, err = s.createUser(tx, user)
		if err != nil {
			return err
		}
		return s.audit(tx, user.ID, "admin.create", user.ID)
	})
	return identity, err
}

func tokenHash(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func newToken() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func (s *Service) allowLogin(key loginKey) bool {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	now := s.now()
	for key, entry := range s.attempts {
		if !now.Before(entry.until) {
			delete(s.attempts, key)
		}
	}
	entry, exists := s.attempts[key]
	if !exists {
		if len(s.attempts) >= maxLoginBuckets {
			return false
		}
		entry.until = now.Add(loginWindow)
	}
	if entry.count >= loginFailureLimit {
		return false
	}
	// 先占用次数，避免同时到达的失败请求绕过限速。
	entry.count++
	s.attempts[key] = entry
	return true
}

func (s *Service) Login(ctx context.Context, username, password, ip, userAgent string) (Identity, string, error) {
	key := loginKey{normalizeUsername(username), ip}
	if !s.allowLogin(key) {
		return Identity{}, "", ErrRateLimited
	}
	var user User
	err := s.db.WithContext(ctx).Where("username = ?", key.username).Take(&user).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return Identity{}, "", err
	}
	hash := user.PasswordHash
	if err != nil {
		hash = dummyPasswordHash()
	}
	valid := VerifyPassword(hash, password)
	if !valid || user.ID == "" || user.Status != StatusActive {
		return Identity{}, "", ErrCredentials
	}
	token, err := newToken()
	if err != nil {
		return Identity{}, "", err
	}
	var identity Identity
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := s.now().UTC()
		// 校验密码期间可能被停用或重置；发会话前在同一事务中重新确认。
		result := tx.Model(&User{}).Where("id = ? AND status = ? AND password_hash = ?", user.ID, StatusActive, hash).Update("last_active_at", now)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrCredentials
		}
		if err := tx.Create(&Session{TokenHash: tokenHash(token), UserID: user.ID, ExpiresAt: now.Add(SessionLifetime), IP: ip, UserAgent: userAgent}).Error; err != nil {
			return err
		}
		identity, err = s.authenticate(tx, token, false)
		return err
	})
	if err != nil {
		return Identity{}, "", err
	}
	s.loginMu.Lock()
	delete(s.attempts, key)
	s.loginMu.Unlock()
	return identity, token, nil
}

func (s *Service) authenticate(db *gorm.DB, token string, admin bool) (Identity, error) {
	if token == "" {
		return Identity{}, ErrUnauthenticated
	}
	var identity Identity
	err := db.Table("users").Select("users.*, workspaces.id AS workspace_id").
		Joins("JOIN sessions ON sessions.user_id = users.id").
		Joins("JOIN workspaces ON workspaces.owner_user_id = users.id").
		Where("sessions.token_hash = ? AND sessions.expires_at > ? AND users.status = ? AND workspaces.status = ?", tokenHash(token), s.now().UTC(), StatusActive, StatusActive).
		Take(&identity).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Identity{}, ErrUnauthenticated
	}
	if err != nil {
		return Identity{}, err
	}
	if admin && identity.User.Role != RoleAdmin {
		return Identity{}, ErrForbidden
	}
	return identity, nil
}

func (s *Service) Authenticate(ctx context.Context, token string) (Identity, error) {
	return s.authenticate(s.db.WithContext(ctx), token, false)
}

func (s *Service) Logout(ctx context.Context, token string) error {
	return s.db.WithContext(ctx).Where("token_hash = ?", tokenHash(token)).Delete(&Session{}).Error
}

func (s *Service) adminWrite(ctx context.Context, token, action string, write func(*gorm.DB, Identity) (string, error)) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		actor, err := s.authenticate(tx, token, true)
		if err != nil {
			return err
		}
		targetID, err := write(tx, actor)
		if err != nil {
			return err
		}
		return s.audit(tx, actor.User.ID, action, targetID)
	})
}

func (s *Service) CreateUser(ctx context.Context, token string, input NewUser) (Identity, error) {
	if _, err := s.authenticate(s.db.WithContext(ctx), token, true); err != nil {
		return Identity{}, err
	}
	user, err := prepareUser(input)
	if err != nil {
		return Identity{}, err
	}
	var identity Identity
	err = s.adminWrite(ctx, token, "user.create", func(tx *gorm.DB, _ Identity) (string, error) {
		identity, err = s.createUser(tx, user)
		return user.ID, err
	})
	return identity, err
}

func (s *Service) ListUsers(ctx context.Context, token string) ([]Identity, error) {
	users := make([]Identity, 0)
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := s.authenticate(tx, token, true); err != nil {
			return err
		}
		return tx.Table("users").Select("users.*, workspaces.id AS workspace_id").Joins("JOIN workspaces ON workspaces.owner_user_id = users.id").Order("users.created_at, users.id").Scan(&users).Error
	})
	return users, err
}

func (s *Service) CreateInvitation(ctx context.Context, token, role string, expiresAt time.Time) (Invitation, string, error) {
	var invitation Invitation
	var raw string
	err := s.adminWrite(ctx, token, "invitation.create", func(tx *gorm.DB, actor Identity) (string, error) {
		if role == "" {
			role = RoleMember
		}
		if role != RoleMember && role != RoleAdmin {
			return "", ErrInvalidInput
		}
		if expiresAt.IsZero() {
			expiresAt = s.now().Add(InvitationLifetime)
		}
		if !expiresAt.After(s.now()) {
			return "", ErrInvalidInput
		}
		var err error
		raw, err = newToken()
		if err != nil {
			return "", err
		}
		invitation = Invitation{ID: uuid.NewString(), TokenHash: tokenHash(raw), Role: role, CreatedBy: actor.User.ID, ExpiresAt: expiresAt.UTC()}
		return invitation.ID, tx.Create(&invitation).Error
	})
	if err != nil {
		return Invitation{}, "", err
	}
	return invitation, raw, nil
}

func (s *Service) AcceptInvitation(ctx context.Context, token string, input NewUser) (Identity, error) {
	var invitation Invitation
	err := s.db.WithContext(ctx).Where("token_hash = ? AND used_by IS NULL AND expires_at > ?", tokenHash(token), s.now().UTC()).Take(&invitation).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Identity{}, ErrInvitation
	}
	if err != nil {
		return Identity{}, err
	}
	input.Role = invitation.Role
	user, err := prepareUser(input)
	if err != nil {
		return Identity{}, err
	}
	var identity Identity
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&Invitation{}).Where("id = ? AND used_by IS NULL AND expires_at > ?", invitation.ID, s.now().UTC()).Update("used_by", user.ID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrInvitation
		}
		identity, err = s.createUser(tx, user)
		return err
	})
	return identity, err
}

func (s *Service) DisableUser(ctx context.Context, token, userID string) error {
	return s.adminWrite(ctx, token, "user.disable", func(tx *gorm.DB, _ Identity) (string, error) {
		result := tx.Model(&User{}).Where("id = ?", userID).Update("status", StatusDisabled)
		if result.Error != nil {
			return "", result.Error
		}
		if result.RowsAffected != 1 {
			return "", ErrNotFound
		}
		if err := tx.Model(&Workspace{}).Where("owner_user_id = ?", userID).Update("status", StatusDisabled).Error; err != nil {
			return "", err
		}
		return userID, tx.Where("user_id = ?", userID).Delete(&Session{}).Error
	})
}

func (s *Service) ResetPassword(ctx context.Context, token, userID, password string) error {
	if _, err := s.authenticate(s.db.WithContext(ctx), token, true); err != nil {
		return err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	return s.adminWrite(ctx, token, "user.reset_password", func(tx *gorm.DB, _ Identity) (string, error) {
		result := tx.Model(&User{}).Where("id = ?", userID).Update("password_hash", hash)
		if result.Error != nil {
			return "", result.Error
		}
		if result.RowsAffected != 1 {
			return "", ErrNotFound
		}
		return userID, tx.Where("user_id = ?", userID).Delete(&Session{}).Error
	})
}
