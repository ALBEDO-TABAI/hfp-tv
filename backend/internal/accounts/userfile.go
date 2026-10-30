package accounts

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/gorm"
)

// fileActorID is the audit actor for changes made by the account files.
const fileActorID = "account-file"

// FileAccount is one account declared in the server-side account files.
// Password is either plaintext or an encoded $argon2id$ hash.
type FileAccount struct {
	Username string
	Password string
	RelayKey string
}

// ParseAccountFiles reads the users file ("username:password" per line) and the
// optional keys file ("username:relay-key" per line). Blank lines and lines
// starting with # are ignored. Errors name the file and line but never echo a
// password or key.
func ParseAccountFiles(usersPath, keysPath string) ([]FileAccount, error) {
	users, err := readColonFile(usersPath)
	if err != nil {
		return nil, err
	}
	var accounts []FileAccount
	seen := map[string]int{} // lower-case username -> index in accounts
	for _, entry := range users {
		where := fmt.Sprintf("账号文件 %s 第 %d 行", usersPath, entry.line)
		if !entry.hasColon {
			return nil, fmt.Errorf("%s：格式应为 用户名:密码", where)
		}
		if !usernamePattern.MatchString(entry.name) {
			return nil, fmt.Errorf("%s：用户名需为 3–32 位字母、数字、下划线、点或短横线", where)
		}
		if _, ok := seen[strings.ToLower(entry.name)]; ok {
			return nil, fmt.Errorf("%s：用户名 %s 重复", where, entry.name)
		}
		if isEncodedHash(entry.value) {
			if !validEncodedHash(entry.value) {
				return nil, fmt.Errorf("%s：%s 的密码哈希无法识别，请用 teamserver hash-password 重新生成", where, entry.name)
			}
		} else if validatePassword(entry.value) != nil {
			return nil, fmt.Errorf("%s：%s 的密码至少 8 个字符", where, entry.name)
		}
		seen[strings.ToLower(entry.name)] = len(accounts)
		accounts = append(accounts, FileAccount{Username: entry.name, Password: entry.value})
	}
	if len(accounts) == 0 {
		return nil, fmt.Errorf("账号文件 %s 里没有任何账号", usersPath)
	}
	if strings.TrimSpace(keysPath) == "" {
		return accounts, nil
	}
	keys, err := readColonFile(keysPath)
	if err != nil {
		return nil, err
	}
	keyed := map[string]bool{}
	for _, entry := range keys {
		where := fmt.Sprintf("密钥文件 %s 第 %d 行", keysPath, entry.line)
		name := strings.ToLower(entry.name)
		index, ok := seen[name]
		switch {
		case !entry.hasColon:
			return nil, fmt.Errorf("%s：格式应为 用户名:密钥", where)
		case !ok:
			return nil, fmt.Errorf("%s：用户 %s 不在账号文件里", where, entry.name)
		case keyed[name]:
			return nil, fmt.Errorf("%s：用户 %s 重复", where, entry.name)
		case entry.value == "" || len(entry.value) > maxRelayKeyLength || strings.ContainsAny(entry.value, " \t"):
			return nil, fmt.Errorf("%s：%s 的密钥为空或含空白字符", where, entry.name)
		}
		keyed[name] = true
		accounts[index].RelayKey = entry.value
	}
	return accounts, nil
}

type colonLine struct {
	line     int
	name     string
	value    string
	hasColon bool
}

// readColonFile splits each meaningful line at its first colon, so passwords
// and keys may themselves contain colons.
func readColonFile(path string) ([]colonLine, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取 %s 失败：%w", path, err)
	}
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
	var lines []colonLine
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 64*1024), 64*1024)
	for number := 1; scanner.Scan(); number++ {
		text := strings.TrimSpace(scanner.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		name, value, found := strings.Cut(text, ":")
		lines = append(lines, colonLine{line: number, name: strings.TrimSpace(name), value: strings.TrimSpace(value), hasColon: found})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取 %s 失败：%w", path, err)
	}
	return lines, nil
}

func isEncodedHash(value string) bool { return strings.HasPrefix(value, "$argon2id$") }

// validEncodedHash checks the structure verifyPassword expects.
func validEncodedHash(value string) bool {
	parts := strings.Split(value, "$")
	if len(parts) != 6 {
		return false
	}
	var version int
	var memory, iterations uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &threads); err != nil {
		return false
	}
	return memory > 0 && iterations > 0 && threads > 0 && len(parts[4]) > 0 && len(parts[5]) > 0
}

// HashPassword exposes the stored hash format for `teamserver hash-password`.
func HashPassword(password string) (string, error) {
	if err := validatePassword(password); err != nil {
		return "", err
	}
	return hashPassword(password)
}

// FileSyncResult tells the caller which kernels need a follow-up: new relay
// keys to push, and disabled workspaces to close.
type FileSyncResult struct {
	Created            []string
	Updated            []string
	Disabled           []string
	RelayChanged       []Workspace
	DisabledWorkspaces []string
}

func (r *FileSyncResult) Changed() bool {
	return len(r.Created)+len(r.Updated)+len(r.Disabled)+len(r.RelayChanged) > 0
}

type fileAccountPlan struct {
	entry        FileAccount
	existing     *User
	workspace    *Workspace
	passwordHash string
	userUpdates  map[string]any
	details      []string
	relayChanged bool
	ciphertext   string
	hint         string
}

// SyncFileAccounts makes the database match the account files: listed accounts
// exist, are active and carry the listed password and key; file-managed
// accounts that are no longer listed are disabled with their data kept.
// Password hashing happens before the transaction because argon2id is slow and
// the platform database has a single connection.
func (s *Service) SyncFileAccounts(ctx context.Context, entries []FileAccount) (*FileSyncResult, error) {
	if len(entries) == 0 {
		return nil, errors.New("账号文件里没有任何账号")
	}
	var users []User
	if err := s.db.WithContext(ctx).Find(&users).Error; err != nil {
		return nil, err
	}
	var workspaces []Workspace
	if err := s.db.WithContext(ctx).Find(&workspaces).Error; err != nil {
		return nil, err
	}
	byName := make(map[string]*User, len(users))
	for index := range users {
		byName[strings.ToLower(users[index].Username)] = &users[index]
	}
	byOwner := make(map[string]*Workspace, len(workspaces))
	for index := range workspaces {
		byOwner[workspaces[index].OwnerUserID] = &workspaces[index]
	}

	listed := map[string]bool{}
	plans := make([]*fileAccountPlan, 0, len(entries))
	for _, entry := range entries {
		listed[strings.ToLower(entry.Username)] = true
		plan := &fileAccountPlan{entry: entry, existing: byName[strings.ToLower(entry.Username)], userUpdates: map[string]any{}}
		if err := s.planFileAccount(plan, byOwner); err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}

	result := &FileSyncResult{}
	now := s.now().UTC()
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, plan := range plans {
			if err := s.applyFileAccount(tx, plan, now, result); err != nil {
				return err
			}
		}
		for index := range users {
			user := &users[index]
			if user.Source != SourceFile || user.Status != StatusActive || listed[strings.ToLower(user.Username)] {
				continue
			}
			if err := tx.Model(&User{}).Where("id = ?", user.ID).Updates(map[string]any{"status": StatusDisabled, "updated_at": now}).Error; err != nil {
				return err
			}
			if err := s.revokeUserSessions(tx, user.ID, ""); err != nil {
				return err
			}
			if err := tx.Create(&AuditEvent{ActorUserID: fileActorID, Action: "user.update", TargetUserID: user.ID, Detail: "status=disabled source=file", CreatedAt: now}).Error; err != nil {
				return err
			}
			result.Disabled = append(result.Disabled, user.Username)
			if workspace := byOwner[user.ID]; workspace != nil {
				result.DisabledWorkspaces = append(result.DisabledWorkspaces, workspace.ID)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for index, workspace := range result.RelayChanged {
		if err := s.db.WithContext(ctx).Where("id = ?", workspace.ID).First(&result.RelayChanged[index]).Error; err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (s *Service) planFileAccount(plan *fileAccountPlan, byOwner map[string]*Workspace) error {
	entry, existing := plan.entry, plan.existing
	passwordChanged := existing == nil
	if existing != nil {
		if isEncodedHash(entry.Password) {
			passwordChanged = entry.Password != existing.PasswordHash
		} else {
			passwordChanged = !verifyPassword(existing.PasswordHash, entry.Password)
		}
	}
	if passwordChanged {
		plan.passwordHash = entry.Password
		if !isEncodedHash(entry.Password) {
			hash, err := hashPassword(entry.Password)
			if err != nil {
				return err
			}
			plan.passwordHash = hash
		}
	}
	currentKey := ""
	if existing != nil {
		plan.workspace = byOwner[existing.ID]
		if plan.workspace != nil && plan.workspace.RelayKeyCiphertext != "" {
			// An undecryptable key (lost secrets dir) is treated as different and resealed.
			currentKey, _ = s.RelayKey(*plan.workspace)
			if currentKey == "" {
				currentKey = "\x00unreadable"
			}
		}
		if existing.Source != SourceFile {
			plan.userUpdates["source"] = SourceFile
			plan.details = append(plan.details, "source=file")
		}
		if existing.Status != StatusActive {
			plan.userUpdates["status"] = StatusActive
			plan.details = append(plan.details, "status=active")
		}
		if passwordChanged {
			plan.userUpdates["password_hash"] = plan.passwordHash
			plan.details = append(plan.details, "password")
		}
	}
	if entry.RelayKey != currentKey {
		ciphertext, hint, err := s.sealRelayKey(entry.RelayKey)
		if err != nil {
			return err
		}
		plan.relayChanged, plan.ciphertext, plan.hint = true, ciphertext, hint
	}
	return nil
}

func (s *Service) applyFileAccount(tx *gorm.DB, plan *fileAccountPlan, now time.Time, result *FileSyncResult) error {
	entry := plan.entry
	if plan.existing == nil {
		user := &User{ID: newID("u_"), Username: entry.Username, DisplayName: entry.Username, PasswordHash: plan.passwordHash,
			Role: RoleMember, Status: StatusActive, Source: SourceFile, CreatedAt: now, UpdatedAt: now}
		workspaceID := newID("w_")
		workspace := &Workspace{ID: workspaceID, OwnerUserID: user.ID, DataDir: filepath.ToSlash(filepath.Join("workspaces", workspaceID)),
			Status: StatusActive, RelayKeyCiphertext: plan.ciphertext, RelayKeyHint: plan.hint, CreatedAt: now, UpdatedAt: now}
		if plan.ciphertext != "" {
			workspace.RelayKeyUpdatedAt = &now
		}
		if err := tx.Create(user).Error; err != nil {
			return err
		}
		if err := tx.Create(workspace).Error; err != nil {
			return err
		}
		detail := "role=member source=file"
		if plan.ciphertext != "" {
			detail += " relay=bound"
		}
		result.Created = append(result.Created, user.Username)
		return tx.Create(&AuditEvent{ActorUserID: fileActorID, Action: "user.create", TargetUserID: user.ID, Detail: detail, CreatedAt: now}).Error
	}
	user := plan.existing
	if len(plan.userUpdates) > 0 {
		plan.userUpdates["updated_at"] = now
		if err := tx.Model(&User{}).Where("id = ?", user.ID).Updates(plan.userUpdates).Error; err != nil {
			return err
		}
		if _, changed := plan.userUpdates["password_hash"]; changed {
			if err := s.revokeUserSessions(tx, user.ID, ""); err != nil {
				return err
			}
		}
		if err := tx.Create(&AuditEvent{ActorUserID: fileActorID, Action: "user.update", TargetUserID: user.ID, Detail: strings.Join(plan.details, " "), CreatedAt: now}).Error; err != nil {
			return err
		}
	}
	if plan.relayChanged && plan.workspace != nil {
		updates := map[string]any{"relay_key_ciphertext": plan.ciphertext, "relay_key_hint": plan.hint, "relay_key_updated_at": &now, "updated_at": now}
		action := "relay.bind"
		if plan.ciphertext == "" {
			updates["relay_key_updated_at"] = nil
			action = "relay.clear"
		}
		if err := tx.Model(&Workspace{}).Where("id = ?", plan.workspace.ID).Updates(updates).Error; err != nil {
			return err
		}
		if err := tx.Create(&AuditEvent{ActorUserID: fileActorID, Action: action, TargetUserID: user.ID, Detail: plan.hint, CreatedAt: now}).Error; err != nil {
			return err
		}
		result.RelayChanged = append(result.RelayChanged, *plan.workspace)
	}
	if len(plan.userUpdates) > 0 || plan.relayChanged {
		result.Updated = append(result.Updated, user.Username)
	}
	return nil
}
