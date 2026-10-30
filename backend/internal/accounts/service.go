package accounts

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"infinite-canvas/backend/internal/localcrypto"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	SessionTTL        = 14 * 24 * time.Hour
	sessionRefreshAge = 10 * time.Minute
	minPasswordLength = 8
	maxPasswordLength = 256
	maxRelayKeyLength = 512
)

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{3,32}$`)

type Service struct {
	db         *gorm.DB
	secretsDir string
	now        func() time.Time
	limiter    *loginLimiter
}

// Open creates or migrates <root>/platform.db and the secrets directory. The
// platform schema is migrated on its own and never shares a database with a
// workspace kernel, so MigrateLocalSchema can never drop these tables.
func Open(root string) (*Service, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("团队服务器数据目录不能为空")
	}
	if err := os.MkdirAll(root, 0o750); err != nil {
		return nil, err
	}
	secretsDir := filepath.Join(root, "secrets")
	if err := os.MkdirAll(secretsDir, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(secretsDir, 0o700); err != nil {
		return nil, err
	}
	dsn := filepath.Join(root, "platform.db") + "?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=on&_synchronous=NORMAL"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Warn)})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&User{}, &Session{}, &Invitation{}, &Workspace{}, &AuditEvent{}); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("迁移平台数据库失败：%w", err)
	}
	return &Service{db: db, secretsDir: secretsDir, now: time.Now, limiter: newLoginLimiter()}, nil
}

func (s *Service) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func newToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func newID(prefix string) string {
	raw := make([]byte, 10)
	if _, err := rand.Read(raw); err != nil {
		panic(err)
	}
	return prefix + hex.EncodeToString(raw)
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func validateCredentials(username, password string) error {
	if !usernamePattern.MatchString(username) {
		return ErrInvalidUsername
	}
	return validatePassword(password)
}

func validatePassword(password string) error {
	if len([]rune(password)) < minPasswordLength || len(password) > maxPasswordLength {
		return ErrInvalidPassword
	}
	return nil
}

func validRole(role string) bool { return role == RoleAdmin || role == RoleMember }

type CreateUserInput struct {
	Username    string
	DisplayName string
	Password    string
	Role        string
	RelayKey    string
}

// CreateUser registers the account and its workspace in one transaction. The
// workspace directory itself is created by the kernel pool on first open.
func (s *Service) CreateUser(ctx context.Context, actorID string, input CreateUserInput) (*User, error) {
	input.Username = strings.TrimSpace(input.Username)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	if input.DisplayName == "" {
		input.DisplayName = input.Username
	}
	if input.Role == "" {
		input.Role = RoleMember
	}
	if !validRole(input.Role) {
		return nil, ErrInvalidRole
	}
	if err := validateCredentials(input.Username, input.Password); err != nil {
		return nil, err
	}
	hash, err := hashPassword(input.Password)
	if err != nil {
		return nil, err
	}
	ciphertext, hint, err := s.sealRelayKey(input.RelayKey)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	user := &User{ID: newID("u_"), Username: input.Username, DisplayName: input.DisplayName, PasswordHash: hash,
		Role: input.Role, Status: StatusActive, CreatedAt: now, UpdatedAt: now}
	workspaceID := newID("w_")
	workspace := &Workspace{ID: workspaceID, OwnerUserID: user.ID, DataDir: filepath.ToSlash(filepath.Join("workspaces", workspaceID)),
		Status: StatusActive, RelayKeyCiphertext: ciphertext, RelayKeyHint: hint, CreatedAt: now, UpdatedAt: now}
	if ciphertext != "" {
		workspace.RelayKeyUpdatedAt = &now
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&User{}).Where("lower(username) = lower(?)", user.Username).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrUsernameTaken
		}
		if err := tx.Create(user).Error; err != nil {
			return err
		}
		if err := tx.Create(workspace).Error; err != nil {
			return err
		}
		detail := "role=" + user.Role
		if ciphertext != "" {
			detail += " relay=bound"
		}
		return tx.Create(&AuditEvent{ActorUserID: actorOrSelf(actorID, user.ID), Action: "user.create", TargetUserID: user.ID, Detail: detail, CreatedAt: now}).Error
	})
	if err != nil {
		return nil, err
	}
	return user, nil
}

func actorOrSelf(actorID, userID string) string {
	if strings.TrimSpace(actorID) == "" {
		return userID
	}
	return actorID
}

// HasUsers reports whether any account exists; the CLI uses it to seed the
// first administrator.
func (s *Service) HasUsers(ctx context.Context) (bool, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&User{}).Count(&count).Error
	return count > 0, err
}

// Authenticate verifies a password with rate limiting by username and client IP.
// Unknown users, disabled users and wrong passwords return the same error.
func (s *Service) Authenticate(ctx context.Context, username, password, clientIP string) (*User, error) {
	username = strings.TrimSpace(username)
	if !s.limiter.allow(username, clientIP, s.now()) {
		return nil, ErrTooManyAttempts
	}
	var user User
	err := s.db.WithContext(ctx).Where("lower(username) = lower(?)", username).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		verifyPassword(dummyHash, password)
		s.limiter.fail(username, clientIP, s.now())
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	if !verifyPassword(user.PasswordHash, password) || user.Status != StatusActive {
		s.limiter.fail(username, clientIP, s.now())
		return nil, ErrInvalidCredentials
	}
	s.limiter.succeed(username, clientIP)
	return &user, nil
}

// IssueSession returns the raw cookie token; only its hash is stored.
func (s *Service) IssueSession(ctx context.Context, userID, clientIP, userAgent string) (string, time.Time, error) {
	token, err := newToken()
	if err != nil {
		return "", time.Time{}, err
	}
	now := s.now().UTC()
	expires := now.Add(SessionTTL)
	if len(userAgent) > 255 {
		userAgent = userAgent[:255]
	}
	session := Session{TokenHash: hashToken(token), UserID: userID, ExpiresAt: expires, LastSeenAt: now, IP: clientIP, UserAgent: userAgent, CreatedAt: now}
	if err := s.db.WithContext(ctx).Create(&session).Error; err != nil {
		return "", time.Time{}, err
	}
	if err := s.db.WithContext(ctx).Model(&User{}).Where("id = ?", userID).Update("last_active_at", now).Error; err != nil {
		return "", time.Time{}, err
	}
	return token, expires, nil
}

// Principal is the authenticated caller resolved from a session cookie.
type Principal struct {
	User      User
	Workspace Workspace
	TokenHash string
}

func (p Principal) IsAdmin() bool { return p.User.Role == RoleAdmin }

// ResolveSession returns the session owner and their workspace. Expired
// sessions, disabled users and disabled workspaces are all unauthenticated.
func (s *Service) ResolveSession(ctx context.Context, token string) (*Principal, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrUnauthenticated
	}
	tokenHash := hashToken(token)
	var session Session
	err := s.db.WithContext(ctx).Where("token_hash = ?", tokenHash).First(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrUnauthenticated
	}
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	if !now.Before(session.ExpiresAt) {
		_ = s.db.WithContext(ctx).Delete(&Session{}, "token_hash = ?", tokenHash).Error
		return nil, ErrUnauthenticated
	}
	var user User
	if err := s.db.WithContext(ctx).Where("id = ?", session.UserID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUnauthenticated
		}
		return nil, err
	}
	if user.Status != StatusActive {
		return nil, ErrUnauthenticated
	}
	var workspace Workspace
	if err := s.db.WithContext(ctx).Where("owner_user_id = ?", user.ID).First(&workspace).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrWorkspaceMissing
		}
		return nil, err
	}
	if workspace.Status != StatusActive {
		return nil, ErrWorkspaceMissing
	}
	// Sliding expiry, written at most every few minutes to keep SQLite quiet.
	if now.Sub(session.LastSeenAt) > sessionRefreshAge {
		_ = s.db.WithContext(ctx).Model(&Session{}).Where("token_hash = ?", tokenHash).
			Updates(map[string]any{"last_seen_at": now, "expires_at": now.Add(SessionTTL)}).Error
		_ = s.db.WithContext(ctx).Model(&User{}).Where("id = ?", user.ID).Update("last_active_at", now).Error
	}
	return &Principal{User: user, Workspace: workspace, TokenHash: tokenHash}, nil
}

func (s *Service) RevokeSession(ctx context.Context, token string) error {
	return s.db.WithContext(ctx).Delete(&Session{}, "token_hash = ?", hashToken(strings.TrimSpace(token))).Error
}

func (s *Service) revokeUserSessions(tx *gorm.DB, userID, keepTokenHash string) error {
	query := tx.Where("user_id = ?", userID)
	if keepTokenHash != "" {
		query = query.Where("token_hash <> ?", keepTokenHash)
	}
	return query.Delete(&Session{}).Error
}

// ChangePassword requires the current password and signs out other devices.
func (s *Service) ChangePassword(ctx context.Context, principal *Principal, current, next string) error {
	if !verifyPassword(principal.User.PasswordHash, current) {
		return ErrInvalidCredentials
	}
	if err := validatePassword(next); err != nil {
		return err
	}
	hash, err := hashPassword(next)
	if err != nil {
		return err
	}
	now := s.now().UTC()
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&User{}).Where("id = ?", principal.User.ID).Updates(map[string]any{"password_hash": hash, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := s.revokeUserSessions(tx, principal.User.ID, principal.TokenHash); err != nil {
			return err
		}
		return tx.Create(&AuditEvent{ActorUserID: principal.User.ID, Action: "user.password.change", TargetUserID: principal.User.ID, CreatedAt: now}).Error
	})
}

// UserView is the admin projection: never includes the password hash or relay key.
type UserView struct {
	ID           string     `json:"id"`
	Username     string     `json:"username"`
	DisplayName  string     `json:"displayName"`
	Role         string     `json:"role"`
	Status       string     `json:"status"`
	CreatedAt    time.Time  `json:"createdAt"`
	LastActiveAt *time.Time `json:"lastActiveAt"`
	Relay        RelayView  `json:"relay"`
	WorkspaceID  string     `json:"workspaceId"`
}

type RelayView struct {
	Bound     bool       `json:"bound"`
	Hint      string     `json:"hint"`
	UpdatedAt *time.Time `json:"updatedAt"`
}

func relayView(workspace Workspace) RelayView {
	return RelayView{Bound: workspace.RelayKeyCiphertext != "", Hint: workspace.RelayKeyHint, UpdatedAt: workspace.RelayKeyUpdatedAt}
}

func viewOf(user User, workspace Workspace) UserView {
	return UserView{ID: user.ID, Username: user.Username, DisplayName: user.DisplayName, Role: user.Role, Status: user.Status,
		CreatedAt: user.CreatedAt, LastActiveAt: user.LastActiveAt, Relay: relayView(workspace), WorkspaceID: workspace.ID}
}

func (s *Service) RelayStatus(workspace Workspace) RelayView { return relayView(workspace) }

func (s *Service) ListUsers(ctx context.Context) ([]UserView, error) {
	var users []User
	if err := s.db.WithContext(ctx).Order("created_at asc").Find(&users).Error; err != nil {
		return nil, err
	}
	var workspaces []Workspace
	if err := s.db.WithContext(ctx).Find(&workspaces).Error; err != nil {
		return nil, err
	}
	byOwner := make(map[string]Workspace, len(workspaces))
	for _, workspace := range workspaces {
		byOwner[workspace.OwnerUserID] = workspace
	}
	views := make([]UserView, 0, len(users))
	for _, user := range users {
		views = append(views, viewOf(user, byOwner[user.ID]))
	}
	return views, nil
}

func (s *Service) userView(ctx context.Context, userID string) (*UserView, error) {
	var user User
	if err := s.db.WithContext(ctx).Where("id = ?", userID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	var workspace Workspace
	if err := s.db.WithContext(ctx).Where("owner_user_id = ?", userID).First(&workspace).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	view := viewOf(user, workspace)
	return &view, nil
}

type UserPatch struct {
	DisplayName *string
	Role        *string
	Status      *string
}

// UpdateUser applies an admin edit. Disabling revokes every session at once;
// the caller closes the kernel.
func (s *Service) UpdateUser(ctx context.Context, actor *Principal, userID string, patch UserPatch) (*UserView, error) {
	updates := map[string]any{"updated_at": s.now().UTC()}
	var details []string
	if patch.DisplayName != nil {
		name := strings.TrimSpace(*patch.DisplayName)
		if name == "" || len([]rune(name)) > 64 {
			return nil, newError(400, "invalid_display_name", "显示名称需为 1–64 个字符")
		}
		updates["display_name"] = name
		details = append(details, "displayName")
	}
	if patch.Role != nil {
		if !validRole(*patch.Role) {
			return nil, ErrInvalidRole
		}
		if userID == actor.User.ID && *patch.Role != RoleAdmin {
			return nil, ErrCannotModifySelf
		}
		updates["role"] = *patch.Role
		details = append(details, "role="+*patch.Role)
	}
	if patch.Status != nil {
		if *patch.Status != StatusActive && *patch.Status != StatusDisabled {
			return nil, ErrInvalidStatus
		}
		if userID == actor.User.ID && *patch.Status != StatusActive {
			return nil, ErrCannotModifySelf
		}
		updates["status"] = *patch.Status
		details = append(details, "status="+*patch.Status)
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&User{}).Where("id = ?", userID).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrUserNotFound
		}
		if patch.Status != nil && *patch.Status == StatusDisabled {
			if err := s.revokeUserSessions(tx, userID, ""); err != nil {
				return err
			}
		}
		return tx.Create(&AuditEvent{ActorUserID: actor.User.ID, Action: "user.update", TargetUserID: userID, Detail: strings.Join(details, " "), CreatedAt: s.now().UTC()}).Error
	})
	if err != nil {
		return nil, err
	}
	return s.userView(ctx, userID)
}

// SetPassword is the admin reset; it signs the user out everywhere.
func (s *Service) SetPassword(ctx context.Context, actor *Principal, userID, password string) error {
	if err := validatePassword(password); err != nil {
		return err
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	now := s.now().UTC()
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&User{}).Where("id = ?", userID).Updates(map[string]any{"password_hash": hash, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrUserNotFound
		}
		keep := ""
		if actor.User.ID == userID {
			keep = actor.TokenHash
		}
		if err := s.revokeUserSessions(tx, userID, keep); err != nil {
			return err
		}
		return tx.Create(&AuditEvent{ActorUserID: actor.User.ID, Action: "user.password.reset", TargetUserID: userID, CreatedAt: now}).Error
	})
}

func (s *Service) sealRelayKey(key string) (string, string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", "", nil
	}
	if len(key) > maxRelayKeyLength || strings.ContainsAny(key, " \t\r\n") {
		return "", "", ErrInvalidRelayKey
	}
	ciphertext, err := localcrypto.Encrypt(s.secretsDir, key)
	if err != nil {
		return "", "", err
	}
	hint := key
	if len(hint) > 4 {
		hint = hint[len(hint)-4:]
	}
	return ciphertext, "…" + hint, nil
}

// SetRelayKey binds (or with an empty key clears) the user's company relay key.
// Returns the workspace so the caller can push the channel into the kernel.
func (s *Service) SetRelayKey(ctx context.Context, actor *Principal, userID, key string) (*UserView, *Workspace, error) {
	ciphertext, hint, err := s.sealRelayKey(key)
	if err != nil {
		return nil, nil, err
	}
	now := s.now().UTC()
	var workspace Workspace
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("owner_user_id = ?", userID).First(&workspace).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrUserNotFound
			}
			return err
		}
		updates := map[string]any{"relay_key_ciphertext": ciphertext, "relay_key_hint": hint, "relay_key_updated_at": &now, "updated_at": now}
		if ciphertext == "" {
			updates["relay_key_updated_at"] = nil
		}
		if err := tx.Model(&Workspace{}).Where("id = ?", workspace.ID).Updates(updates).Error; err != nil {
			return err
		}
		action := "relay.bind"
		if ciphertext == "" {
			action = "relay.clear"
		}
		return tx.Create(&AuditEvent{ActorUserID: actor.User.ID, Action: action, TargetUserID: userID, Detail: hint, CreatedAt: now}).Error
	})
	if err != nil {
		return nil, nil, err
	}
	if err := s.db.WithContext(ctx).Where("id = ?", workspace.ID).First(&workspace).Error; err != nil {
		return nil, nil, err
	}
	view, err := s.userView(ctx, userID)
	return view, &workspace, err
}

// ResetPasswordByUsername is the command-line recovery path for a locked-out
// administrator; it runs without a session and signs the user out everywhere.
func (s *Service) ResetPasswordByUsername(ctx context.Context, username, password string) error {
	var user User
	if err := s.db.WithContext(ctx).Where("lower(username) = lower(?)", strings.TrimSpace(username)).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrUserNotFound
		}
		return err
	}
	return s.SetPassword(ctx, &Principal{User: user}, user.ID, password)
}

// RelayKey decrypts the bound key for a workspace; empty when unbound.
func (s *Service) RelayKey(workspace Workspace) (string, error) {
	if workspace.RelayKeyCiphertext == "" {
		return "", nil
	}
	return localcrypto.Decrypt(s.secretsDir, workspace.RelayKeyCiphertext)
}

func (s *Service) Workspace(ctx context.Context, id string) (*Workspace, error) {
	var workspace Workspace
	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&workspace).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrWorkspaceMissing
		}
		return nil, err
	}
	return &workspace, nil
}

// WorkspaceForUser returns the workspace owned by userID.
func (s *Service) WorkspaceForUser(ctx context.Context, userID string) (*Workspace, error) {
	var workspace Workspace
	if err := s.db.WithContext(ctx).Where("owner_user_id = ?", userID).First(&workspace).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrWorkspaceMissing
		}
		return nil, err
	}
	return &workspace, nil
}

// ActiveWorkspaces lists workspaces whose owner is active; the server opens
// them at startup so queued generation tasks resume after a restart.
func (s *Service) ActiveWorkspaces(ctx context.Context) ([]Workspace, error) {
	var workspaces []Workspace
	err := s.db.WithContext(ctx).Table("workspaces").Select("workspaces.*").
		Joins("JOIN users ON users.id = workspaces.owner_user_id").
		Where("workspaces.status = ? AND users.status = ?", StatusActive, StatusActive).
		Order("workspaces.created_at asc").Find(&workspaces).Error
	return workspaces, err
}

// CreateInvitation returns a single-use raw token.
func (s *Service) CreateInvitation(ctx context.Context, actor *Principal, role string, ttl time.Duration) (string, time.Time, error) {
	if !validRole(role) {
		return "", time.Time{}, ErrInvalidRole
	}
	if ttl <= 0 || ttl > 30*24*time.Hour {
		ttl = 72 * time.Hour
	}
	token, err := newToken()
	if err != nil {
		return "", time.Time{}, err
	}
	now := s.now().UTC()
	invitation := Invitation{TokenHash: hashToken(token), Role: role, CreatedBy: actor.User.ID, ExpiresAt: now.Add(ttl), CreatedAt: now}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&invitation).Error; err != nil {
			return err
		}
		return tx.Create(&AuditEvent{ActorUserID: actor.User.ID, Action: "invitation.create", Detail: "role=" + role, CreatedAt: now}).Error
	})
	return token, invitation.ExpiresAt, err
}

func (s *Service) Invitation(ctx context.Context, token string) (*Invitation, error) {
	var invitation Invitation
	err := s.db.WithContext(ctx).Where("token_hash = ?", hashToken(strings.TrimSpace(token))).First(&invitation).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrInvitationInvalid
	}
	if err != nil {
		return nil, err
	}
	if invitation.UsedBy != nil || !s.now().UTC().Before(invitation.ExpiresAt) {
		return nil, ErrInvitationInvalid
	}
	return &invitation, nil
}

// AcceptInvitation creates the account and consumes the token atomically.
func (s *Service) AcceptInvitation(ctx context.Context, token, username, displayName, password string) (*User, error) {
	invitation, err := s.Invitation(ctx, token)
	if err != nil {
		return nil, err
	}
	user, err := s.CreateUser(ctx, invitation.CreatedBy, CreateUserInput{Username: username, DisplayName: displayName, Password: password, Role: invitation.Role})
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	result := s.db.WithContext(ctx).Model(&Invitation{}).Where("token_hash = ? AND used_by IS NULL", invitation.TokenHash).
		Updates(map[string]any{"used_by": user.ID, "used_at": now})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		// Another request consumed the token first; undo this account.
		_ = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Delete(&Workspace{}, "owner_user_id = ?", user.ID).Error; err != nil {
				return err
			}
			return tx.Delete(&User{}, "id = ?", user.ID).Error
		})
		return nil, ErrInvitationInvalid
	}
	return user, nil
}

type AuditView struct {
	ID             uint      `json:"id"`
	ActorUsername  string    `json:"actorUsername"`
	Action         string    `json:"action"`
	TargetUsername string    `json:"targetUsername"`
	Detail         string    `json:"detail"`
	CreatedAt      time.Time `json:"createdAt"`
}

func (s *Service) Audit(ctx context.Context, limit int) ([]AuditView, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var events []AuditEvent
	if err := s.db.WithContext(ctx).Order("id desc").Limit(limit).Find(&events).Error; err != nil {
		return nil, err
	}
	var users []User
	if err := s.db.WithContext(ctx).Select("id", "username").Find(&users).Error; err != nil {
		return nil, err
	}
	names := make(map[string]string, len(users))
	for _, user := range users {
		names[user.ID] = user.Username
	}
	views := make([]AuditView, 0, len(events))
	for _, event := range events {
		views = append(views, AuditView{ID: event.ID, ActorUsername: names[event.ActorUserID], Action: event.Action,
			TargetUsername: names[event.TargetUserID], Detail: event.Detail, CreatedAt: event.CreatedAt})
	}
	return views, nil
}

// PurgeExpiredSessions keeps the sessions table small; called periodically.
func (s *Service) PurgeExpiredSessions(ctx context.Context) error {
	return s.db.WithContext(ctx).Where("expires_at <= ?", s.now().UTC()).Delete(&Session{}).Error
}
