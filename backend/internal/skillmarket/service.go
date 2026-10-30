package skillmarket

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"infinite-canvas/backend/internal/accounts"
	"infinite-canvas/backend/internal/skillhub"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	tokenTTL         = 30 * 24 * time.Hour
	deviceCodeTTL    = 10 * time.Minute
	devicePollPeriod = 5
	// VerificationPath is the in-app page where a logged-in member approves a
	// device code. It is relative so the browser resolves it on the team site.
	VerificationPath = "/skills/authorize"
)

// Directory is the slice of the account service the market needs.
type Directory interface {
	ActiveUser(ctx context.Context, id string) (*accounts.User, error)
	UserNumber(ctx context.Context, id string) (int, error)
	Record(ctx context.Context, actorID, action, targetUserID, detail string) error
}

type Service struct {
	db         *gorm.DB
	users      Directory
	signer     *Signer
	archiveDir string
	now        func() time.Time
}

// Open migrates the market tables in platform.db and prepares the archive store.
func Open(db *gorm.DB, users Directory, signer *Signer, dataRoot string) (*Service, error) {
	if err := db.AutoMigrate(&Skill{}, &Submission{}, &Release{}, &Token{}, &DeviceAuthorization{}, &IdempotentResponse{}); err != nil {
		return nil, fmt.Errorf("迁移技能市场数据表失败：%w", err)
	}
	archiveDir := filepath.Join(dataRoot, "market", "archives")
	if err := os.MkdirAll(archiveDir, 0o750); err != nil {
		return nil, err
	}
	return &Service{db: db, users: users, signer: signer, archiveDir: archiveDir, now: time.Now}, nil
}

func randomHex(bytes int) string {
	raw := make([]byte, bytes)
	if _, err := rand.Read(raw); err != nil {
		panic(err)
	}
	return hex.EncodeToString(raw)
}

func hashSecret(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// ---- archives ----

func (s *Service) archivePath(sha string) string { return filepath.Join(s.archiveDir, sha+".zip") }

// storeArchive writes content-addressed bytes once; existing files are reused.
func (s *Service) storeArchive(data []byte) (string, error) {
	sha := skillhub.Digest(data)
	path := s.archivePath(sha)
	if _, err := os.Stat(path); err == nil {
		return sha, nil
	}
	tmp, err := os.CreateTemp(s.archiveDir, ".upload-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	return sha, os.Rename(tmp.Name(), path)
}

func (s *Service) loadArchive(sha string) ([]byte, error) {
	if len(sha) != 64 {
		return nil, ErrNotFound
	}
	data, err := os.ReadFile(s.archivePath(sha))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil || skillhub.Digest(data) != sha {
		return nil, newError(http.StatusInternalServerError, "archive_corrupt", "技能包存储损坏")
	}
	return data, nil
}

// ---- device authorization and tokens ----

const userCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func newUserCode() string {
	raw := make([]byte, 8)
	if _, err := rand.Read(raw); err != nil {
		panic(err)
	}
	code := make([]byte, 0, 9)
	for i, b := range raw {
		if i == 4 {
			code = append(code, '-')
		}
		code = append(code, userCodeAlphabet[int(b)%len(userCodeAlphabet)])
	}
	return string(code)
}

func (s *Service) StartAuthorization(ctx context.Context, clientName string) (*skillhub.Authorization, error) {
	clientName = strings.TrimSpace(clientName)
	if utf8.RuneCountInString(clientName) > 60 {
		clientName = string([]rune(clientName)[:60])
	}
	deviceCode := randomHex(32)
	now := s.now().UTC()
	record := DeviceAuthorization{DeviceCodeHash: hashSecret(deviceCode), UserCode: newUserCode(), ClientName: clientName, ExpiresAt: now.Add(deviceCodeTTL), CreatedAt: now}
	_ = s.db.WithContext(ctx).Where("expires_at < ?", now).Delete(&DeviceAuthorization{}).Error
	if err := s.db.WithContext(ctx).Create(&record).Error; err != nil {
		return nil, err
	}
	return &skillhub.Authorization{DeviceCode: deviceCode, UserCode: record.UserCode,
		VerificationURL: VerificationPath + "?code=" + record.UserCode, ExpiresIn: int64(deviceCodeTTL.Seconds()), Interval: devicePollPeriod}, nil
}

func normalizeUserCode(code string) string { return strings.ToUpper(strings.TrimSpace(code)) }

// AuthorizationInfo backs the in-app approval page.
type AuthorizationInfo struct {
	UserCode   string    `json:"userCode"`
	ClientName string    `json:"clientName"`
	ExpiresAt  time.Time `json:"expiresAt"`
	Status     string    `json:"status"`
}

func (s *Service) pendingAuthorization(ctx context.Context, userCode string) (*DeviceAuthorization, error) {
	var record DeviceAuthorization
	err := s.db.WithContext(ctx).Where("user_code = ?", normalizeUserCode(userCode)).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && !s.now().UTC().Before(record.ExpiresAt)) {
		return nil, ErrAuthorizationInvalid
	}
	return &record, err
}

func (s *Service) AuthorizationInfo(ctx context.Context, userCode string) (*AuthorizationInfo, error) {
	record, err := s.pendingAuthorization(ctx, userCode)
	if err != nil {
		return nil, err
	}
	status := "pending"
	if record.ApprovedUserID != "" {
		status = "approved"
	}
	return &AuthorizationInfo{UserCode: record.UserCode, ClientName: record.ClientName, ExpiresAt: record.ExpiresAt, Status: status}, nil
}

// ApproveAuthorization binds a pending device code to the logged-in member.
func (s *Service) ApproveAuthorization(ctx context.Context, userID, userCode string) error {
	record, err := s.pendingAuthorization(ctx, userCode)
	if err != nil {
		return err
	}
	if record.ApprovedUserID != "" && record.ApprovedUserID != userID {
		return ErrAuthorizationInvalid
	}
	result := s.db.WithContext(ctx).Model(&DeviceAuthorization{}).Where("device_code_hash = ? AND (approved_user_id = '' OR approved_user_id = ?)", record.DeviceCodeHash, userID).
		Update("approved_user_id", userID)
	if result.Error != nil {
		return result.Error
	}
	return s.users.Record(ctx, userID, "market.authorize", userID, record.ClientName)
}

// ExchangeDeviceCode issues a market token once the code is approved.
func (s *Service) ExchangeDeviceCode(ctx context.Context, deviceCode string) (*skillhub.Token, error) {
	var record DeviceAuthorization
	err := s.db.WithContext(ctx).Where("device_code_hash = ?", hashSecret(strings.TrimSpace(deviceCode))).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAuthorizationExpired
	}
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	if !now.Before(record.ExpiresAt) {
		_ = s.db.WithContext(ctx).Delete(&record).Error
		return nil, ErrAuthorizationExpired
	}
	if record.ApprovedUserID == "" {
		return nil, ErrAuthorizationPending
	}
	token := randomHex(32)
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Where("device_code_hash = ?", record.DeviceCodeHash).Delete(&DeviceAuthorization{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrAuthorizationExpired
		}
		return tx.Create(&Token{TokenHash: hashSecret(token), UserID: record.ApprovedUserID, ExpiresAt: now.Add(tokenTTL), CreatedAt: now}).Error
	})
	if err != nil {
		return nil, err
	}
	return &skillhub.Token{AccessToken: token, ExpiresIn: int64(tokenTTL.Seconds())}, nil
}

// RevokeToken never fails for unknown tokens: the kernel keeps its stored
// token file when revoke errors.
func (s *Service) RevokeToken(ctx context.Context, token string) error {
	if strings.TrimSpace(token) == "" {
		return nil
	}
	return s.db.WithContext(ctx).Where("token_hash = ?", hashSecret(strings.TrimSpace(token))).Delete(&Token{}).Error
}

// Authenticate resolves a bearer token to an active member.
func (s *Service) Authenticate(ctx context.Context, token string) (*accounts.User, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrUnauthorized
	}
	var record Token
	err := s.db.WithContext(ctx).Where("token_hash = ?", hashSecret(token)).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && !s.now().UTC().Before(record.ExpiresAt)) {
		return nil, ErrUnauthorized
	}
	if err != nil {
		return nil, err
	}
	user, err := s.users.ActiveUser(ctx, record.UserID)
	if errors.Is(err, accounts.ErrUnauthenticated) {
		return nil, ErrUnauthorized
	}
	return user, err
}

func (s *Service) Session(ctx context.Context, user *accounts.User) (*skillhub.PublishingAccount, error) {
	number, err := s.users.UserNumber(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	return &skillhub.PublishingAccount{UserID: number, Username: user.Username, DisplayName: user.DisplayName}, nil
}

// ---- metadata ----

var (
	skillIDPattern  = regexp.MustCompile(`^[A-Za-z0-9_-]{1,80}$`)
	versionPattern  = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
	allowedRequires = map[string]bool{"beeftv.canvas": true, "model.image": true, "model.video": true, "model.audio": true}
)

func runeLimit(value string, min, max int) bool {
	count := utf8.RuneCountInString(strings.TrimSpace(value))
	return count >= min && count <= max
}

func normalizeMetadata(metadata skillhub.Metadata) (skillhub.Metadata, error) {
	metadata.Name = strings.TrimSpace(metadata.Name)
	metadata.Description = strings.TrimSpace(metadata.Description)
	metadata.License = strings.TrimSpace(metadata.License)
	metadata.VersionLabel = strings.TrimSpace(metadata.VersionLabel)
	metadata.SourceURL = strings.TrimSpace(metadata.SourceURL)
	switch {
	case !runeLimit(metadata.Name, 1, 80):
		return metadata, invalid("metadata_invalid", "名称需为 1–80 个字符")
	case !runeLimit(metadata.Description, 1, 1024):
		return metadata, invalid("metadata_invalid", "简介需为 1–1024 个字符")
	case !runeLimit(metadata.License, 1, 64):
		return metadata, invalid("metadata_invalid", "请填写许可")
	case !runeLimit(metadata.VersionLabel, 1, 32):
		return metadata, invalid("metadata_invalid", "版本号需为 1–32 个字符")
	case len(metadata.SourceURL) > 512 || utf8.RuneCountInString(metadata.RequirementNotes) > 1000:
		return metadata, invalid("metadata_invalid", "来源地址或要求说明过长")
	case metadata.MinClientVersion != "" && !versionPattern.MatchString(metadata.MinClientVersion):
		return metadata, invalid("metadata_invalid", "最低客户端版本需为 x.y.z")
	case len(metadata.Showcase) > 8 || len(metadata.Tags) > 16 || len(metadata.Aliases) > 16:
		return metadata, invalid("metadata_invalid", "展示、标签或别名数量过多")
	}
	seen := map[string]bool{}
	requirements := make([]string, 0, len(metadata.Requirements))
	for _, requirement := range metadata.Requirements {
		if !allowedRequires[requirement] {
			return metadata, invalid("metadata_invalid", "不支持的能力要求："+requirement)
		}
		if !seen[requirement] {
			seen[requirement] = true
			requirements = append(requirements, requirement)
		}
	}
	metadata.Requirements = requirements
	if metadata.Showcase == nil {
		metadata.Showcase = []skillhub.Showcase{}
	}
	for _, item := range metadata.Showcase {
		if len(item.URL) > 512 || utf8.RuneCountInString(item.Caption) > 200 {
			return metadata, invalid("metadata_invalid", "展示链接或说明过长")
		}
	}
	for _, list := range [][]string{metadata.Tags, metadata.Aliases} {
		for _, item := range list {
			if utf8.RuneCountInString(item) > 80 {
				return metadata, invalid("metadata_invalid", "标签或别名需不超过 80 个字符")
			}
		}
	}
	return metadata, nil
}

// ---- submissions ----

func (s *Service) draftOf(row Submission) (*skillhub.Draft, error) {
	var metadata skillhub.Metadata
	if err := json.Unmarshal([]byte(row.MetadataJSON), &metadata); err != nil {
		return nil, err
	}
	files := []skillhub.File{}
	if row.FilesJSON != "" {
		if err := json.Unmarshal([]byte(row.FilesJSON), &files); err != nil {
			return nil, err
		}
	}
	return &skillhub.Draft{ReviewReason: row.ReviewReason, ContentHash: row.ContentHash, ArchiveSHA256: row.ArchiveSHA256, ArchiveBytes: row.ArchiveBytes,
		SubmissionID: row.ID, Revision: row.Revision, Status: row.Status, Metadata: metadata, Files: files, PreviewDigest: row.PreviewDigest, Warnings: []string{}}, nil
}

func (s *Service) replay(ctx context.Context, userID, key, operation string, out any) (bool, error) {
	if key == "" {
		return false, nil
	}
	var record IdempotentResponse
	err := s.db.WithContext(ctx).Where("user_id = ? AND key = ?", userID, key).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if record.Operation != operation {
		return false, ErrIdempotencyConflict
	}
	return true, json.Unmarshal([]byte(record.ResponseJSON), out)
}

func rememberTx(tx *gorm.DB, userID, key, operation string, value any) error {
	if key == "" {
		return nil
	}
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&IdempotentResponse{UserID: userID, Key: key, Operation: operation, ResponseJSON: string(body), CreatedAt: time.Now().UTC()}).Error
}

func validIdempotencyKey(key string) bool { return len(key) <= 160 }

// CreateSubmission starts a draft. A metadata.skillId means "new version of
// my skill"; without it the market assigns the future skill's ID now.
func (s *Service) CreateSubmission(ctx context.Context, user *accounts.User, key string, metadata skillhub.Metadata) (*skillhub.Draft, error) {
	if !validIdempotencyKey(key) {
		return nil, invalid("idempotency_key_invalid", "重复提交标识无效")
	}
	var replayed skillhub.Draft
	if ok, err := s.replay(ctx, user.ID, key, "create", &replayed); err != nil || ok {
		return &replayed, err
	}
	metadata, err := normalizeMetadata(metadata)
	if err != nil {
		return nil, err
	}
	target := TargetNew
	if metadata.SkillID != "" {
		if !skillIDPattern.MatchString(metadata.SkillID) {
			return nil, invalid("metadata_invalid", "技能 ID 无效")
		}
		var skill Skill
		if err := s.db.WithContext(ctx).Where("id = ?", metadata.SkillID).First(&skill).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, newError(http.StatusNotFound, "skill_not_found", "要更新的技能不存在")
			}
			return nil, err
		}
		if skill.OwnerUserID != user.ID {
			return nil, ErrSkillNotOwned
		}
		target = TargetExisting
	} else {
		metadata.SkillID = "s_" + randomHex(8)
	}
	body, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	row := Submission{ID: "d_" + randomHex(10), UserID: user.ID, SkillID: metadata.SkillID, Target: target, Status: StatusDraft, Revision: 1,
		MetadataJSON: string(body), CreatedAt: now, UpdatedAt: now}
	draft, err := s.draftOf(row)
	if err != nil {
		return nil, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		return rememberTx(tx, user.ID, key, "create", draft)
	})
	return draft, err
}

func (s *Service) ownSubmission(ctx context.Context, userID, id string) (*Submission, error) {
	var row Submission
	err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrSubmissionNotFound
	}
	return &row, err
}

func (s *Service) Submission(ctx context.Context, user *accounts.User, id string) (*skillhub.Draft, error) {
	row, err := s.ownSubmission(ctx, user.ID, id)
	if err != nil {
		return nil, err
	}
	return s.draftOf(*row)
}

func (s *Service) Submissions(ctx context.Context, user *accounts.User) ([]skillhub.Draft, error) {
	var rows []Submission
	if err := s.db.WithContext(ctx).Where("user_id = ?", user.ID).Order("updated_at desc").Limit(200).Find(&rows).Error; err != nil {
		return nil, err
	}
	drafts := make([]skillhub.Draft, 0, len(rows))
	for _, row := range rows {
		draft, err := s.draftOf(row)
		if err != nil {
			return nil, err
		}
		drafts = append(drafts, *draft)
	}
	return drafts, nil
}

func editable(status string) bool {
	return status == StatusDraft || status == StatusReadyToSubmit || status == StatusRejected
}

// Upload replaces the draft content; a rejected draft returns to draft.
func (s *Service) Upload(ctx context.Context, user *accounts.User, id string, revision int64, raw []byte) (*skillhub.Draft, error) {
	row, err := s.ownSubmission(ctx, user.ID, id)
	if err != nil {
		return nil, err
	}
	if !editable(row.Status) {
		return nil, ErrSubmissionLocked
	}
	if revision != row.Revision {
		return nil, ErrRevisionConflict
	}
	archive, err := canonicalize(raw)
	if err != nil {
		return nil, err
	}
	sha, err := s.storeArchive(archive.Data)
	if err != nil {
		return nil, err
	}
	files, err := json.Marshal(archive.Files)
	if err != nil {
		return nil, err
	}
	updates := map[string]any{"content_hash": archive.ContentHash, "archive_sha256": sha, "archive_bytes": int64(len(archive.Data)),
		"files_json": string(files), "revision": row.Revision + 1, "status": StatusDraft, "preview_digest": "", "updated_at": s.now().UTC()}
	result := s.db.WithContext(ctx).Model(&Submission{}).Where("id = ? AND revision = ?", row.ID, row.Revision).Updates(updates)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, ErrRevisionConflict
	}
	return s.Submission(ctx, user, id)
}

func previewDigest(row Submission) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d\x00%s\x00%s", row.ID, row.Revision, row.ContentHash, row.MetadataJSON)))
	return hex.EncodeToString(sum[:])
}

// Validate marks uploaded content ready to submit; repeating it is harmless.
func (s *Service) Validate(ctx context.Context, user *accounts.User, id string, revision int64) (*skillhub.Draft, error) {
	row, err := s.ownSubmission(ctx, user.ID, id)
	if err != nil {
		return nil, err
	}
	if row.Status != StatusDraft && row.Status != StatusReadyToSubmit {
		return nil, ErrSubmissionLocked
	}
	if revision != row.Revision {
		return nil, ErrRevisionConflict
	}
	if row.ContentHash == "" {
		return nil, ErrContentMissing
	}
	digest := previewDigest(*row)
	result := s.db.WithContext(ctx).Model(&Submission{}).Where("id = ? AND revision = ?", row.ID, row.Revision).
		Updates(map[string]any{"status": StatusReadyToSubmit, "preview_digest": digest, "updated_at": s.now().UTC()})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, ErrRevisionConflict
	}
	return s.Submission(ctx, user, id)
}

func (s *Service) Preview(ctx context.Context, user *accounts.User, id string) (*skillhub.Preview, error) {
	row, err := s.ownSubmission(ctx, user.ID, id)
	if err != nil {
		return nil, err
	}
	if row.ContentHash == "" || row.PreviewDigest == "" {
		return nil, ErrContentMissing
	}
	draft, err := s.draftOf(*row)
	if err != nil {
		return nil, err
	}
	return &skillhub.Preview{Author: user.DisplayName, SubmissionID: row.ID, Revision: row.Revision, PreviewDigest: row.PreviewDigest,
		Files: draft.Files, Metadata: draft.Metadata, Warnings: []string{}}, nil
}

func (s *Service) SubmissionArchive(ctx context.Context, user *accounts.User, id string) ([]byte, error) {
	row, err := s.ownSubmission(ctx, user.ID, id)
	if err != nil {
		return nil, err
	}
	if row.ArchiveSHA256 == "" {
		return nil, ErrContentMissing
	}
	return s.loadArchive(row.ArchiveSHA256)
}

// Submit sends the exact previewed revision to the review queue.
func (s *Service) Submit(ctx context.Context, user *accounts.User, id, key string, revision int64, digest string) (*skillhub.Draft, error) {
	if !validIdempotencyKey(key) {
		return nil, invalid("idempotency_key_invalid", "重复提交标识无效")
	}
	var replayed skillhub.Draft
	if ok, err := s.replay(ctx, user.ID, key, "submit", &replayed); err != nil || ok {
		return &replayed, err
	}
	row, err := s.ownSubmission(ctx, user.ID, id)
	if err != nil {
		return nil, err
	}
	if row.Status != StatusReadyToSubmit {
		return nil, ErrSubmissionLocked
	}
	if revision != row.Revision || digest == "" || digest != row.PreviewDigest {
		return nil, ErrPreviewChanged
	}
	now := s.now().UTC()
	var draft *skillhub.Draft
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&Submission{}).Where("id = ? AND revision = ? AND status = ? AND preview_digest = ?", row.ID, row.Revision, StatusReadyToSubmit, digest).
			Updates(map[string]any{"status": StatusInReview, "submitted_at": now, "review_reason": "", "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrPreviewChanged
		}
		var updated Submission
		if err := tx.Where("id = ?", row.ID).First(&updated).Error; err != nil {
			return err
		}
		var err error
		if draft, err = s.draftOf(updated); err != nil {
			return err
		}
		return rememberTx(tx, user.ID, key, "submit", draft)
	})
	return draft, err
}

// ---- published catalog ----

type publishedRelease struct {
	skill   Skill
	release skillhub.Release
}

func (s *Service) releaseOf(row Release) (skillhub.Release, error) {
	var release skillhub.Release
	if err := json.Unmarshal([]byte(row.ReleaseJSON), &release); err != nil {
		return release, err
	}
	var attestation skillhub.Attestation
	if err := json.Unmarshal([]byte(row.AttestationJSON), &attestation); err != nil {
		return release, err
	}
	release.Attestation = &attestation
	return release, nil
}

func (s *Service) listed(ctx context.Context) ([]publishedRelease, error) {
	var skills []Skill
	if err := s.db.WithContext(ctx).Where("listed = ? AND current_release_id <> ''", true).Find(&skills).Error; err != nil {
		return nil, err
	}
	out := make([]publishedRelease, 0, len(skills))
	for _, skill := range skills {
		var row Release
		if err := s.db.WithContext(ctx).Where("id = ?", skill.CurrentReleaseID).First(&row).Error; err != nil {
			return nil, err
		}
		release, err := s.releaseOf(row)
		if err != nil {
			return nil, err
		}
		out = append(out, publishedRelease{skill: skill, release: release})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].skill.Official != out[j].skill.Official {
			return out[i].skill.Official
		}
		return out[i].release.PublishedAt > out[j].release.PublishedAt
	})
	return out, nil
}

func matches(release skillhub.Release, query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return true
	}
	fields := append([]string{release.SkillID, release.Name, release.Description, release.Author}, release.Tags...)
	fields = append(fields, release.Aliases...)
	for _, field := range fields {
		if strings.Contains(strings.ToLower(field), query) {
			return true
		}
	}
	return false
}

func pageBounds(page, size, total int) (int, int, int, int) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 50 {
		size = 20
	}
	start := (page - 1) * size
	if start > total {
		start = total
	}
	end := start + size
	if end > total {
		end = total
	}
	return page, size, start, end
}

// Compatibility follows the SkillHub rule: supported only when every declared
// requirement is among the caller's capabilities.
func Compatibility(requirements, capabilities []string) skillhub.Compatibility {
	have := map[string]bool{}
	for _, capability := range capabilities {
		have[capability] = true
	}
	reasons := []string{}
	for _, requirement := range requirements {
		if !have[requirement] {
			reasons = append(reasons, "missing_capability:"+requirement)
		}
	}
	if len(reasons) > 0 {
		return skillhub.Compatibility{Status: "unsupported", Reasons: reasons}
	}
	return skillhub.Compatibility{Status: "supported", Reasons: []string{}}
}

func (s *Service) Search(ctx context.Context, query, target string, capabilities []string, page, size int) (*skillhub.SearchResult, error) {
	all, err := s.listed(ctx)
	if err != nil {
		return nil, err
	}
	filtered := make([]skillhub.Release, 0, len(all))
	for _, item := range all {
		if matches(item.release, query) {
			filtered = append(filtered, item.release)
		}
	}
	page, size, start, end := pageBounds(page, size, len(filtered))
	items := make([]skillhub.SearchItem, 0, end-start)
	for _, release := range filtered[start:end] {
		requirements := release.Requirements
		if requirements == nil {
			requirements = []string{}
		}
		items = append(items, skillhub.SearchItem{Kind: "skill", SkillID: release.SkillID, ReleaseID: release.ReleaseID, VersionLabel: release.VersionLabel,
			Name: release.Name, Description: release.Description, Author: release.Author, License: release.License, SourceURL: release.SourceURL,
			Requirements: requirements, Compatibility: Compatibility(requirements, capabilities), Verification: "unverified"})
	}
	return &skillhub.SearchResult{Items: items, Total: len(filtered), Page: page, PageSize: size, Target: target}, nil
}

func (s *Service) Catalog(ctx context.Context, query string, page, size int) (*skillhub.Catalog, error) {
	all, err := s.listed(ctx)
	if err != nil {
		return nil, err
	}
	filtered := make([]skillhub.Release, 0, len(all))
	for _, item := range all {
		if matches(item.release, query) {
			filtered = append(filtered, item.release)
		}
	}
	page, size, start, end := pageBounds(page, size, len(filtered))
	return &skillhub.Catalog{Items: filtered[start:end], Total: int64(len(filtered)), Page: page, PageSize: size}, nil
}

// Release returns a published version. Older versions stay downloadable for
// rollback; an unlisted skill serves nothing.
func (s *Service) Release(ctx context.Context, skillID, releaseID string) (*skillhub.Release, error) {
	var skill Skill
	if err := s.db.WithContext(ctx).Where("id = ? AND listed = ?", skillID, true).First(&skill).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var row Release
	if err := s.db.WithContext(ctx).Where("id = ? AND skill_id = ?", releaseID, skillID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	release, err := s.releaseOf(row)
	return &release, err
}

func (s *Service) ReleaseArchive(ctx context.Context, skillID, releaseID string) ([]byte, error) {
	release, err := s.Release(ctx, skillID, releaseID)
	if err != nil {
		return nil, err
	}
	return s.loadArchive(release.ArchiveSHA256)
}

func (s *Service) ReleaseFile(ctx context.Context, skillID, releaseID, name string) ([]byte, error) {
	release, err := s.Release(ctx, skillID, releaseID)
	if err != nil {
		return nil, err
	}
	var listed *skillhub.File
	for index := range release.Files {
		if release.Files[index].Path == name {
			listed = &release.Files[index]
		}
	}
	if listed == nil {
		return nil, ErrNotFound
	}
	archive, err := s.loadArchive(release.ArchiveSHA256)
	if err != nil {
		return nil, err
	}
	data, err := readArchiveFile(archive, name)
	if err != nil || int64(len(data)) != listed.Size || skillhub.Digest(data) != listed.SHA256 {
		return nil, newError(http.StatusInternalServerError, "archive_corrupt", "技能文件校验失败")
	}
	return data, nil
}
