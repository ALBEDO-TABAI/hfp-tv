package workshop

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"infinite-canvas/backend/internal/accounts"
	"infinite-canvas/backend/internal/projectsnapshot"

	"gorm.io/gorm"
)

// SnapshotPort is the workspace kernel's project export/import surface.
type SnapshotPort interface {
	Preflight(projectID string) (*projectsnapshot.Preflight, error)
	Export(projectID string, sink projectsnapshot.BlobSink) (*projectsnapshot.ExportResult, error)
	Import(snapshot *projectsnapshot.Snapshot, source projectsnapshot.BlobSource, options projectsnapshot.ImportOptions) (*projectsnapshot.ImportResult, error)
}

// Kernels opens the caller's own workspace kernel.
type Kernels func(ctx context.Context, workspace accounts.Workspace) (SnapshotPort, error)

type Auditor interface {
	Record(ctx context.Context, actorID, action, targetUserID, detail string) error
}

// Viewer is the authenticated caller of a workshop operation.
type Viewer struct {
	User      accounts.User
	Workspace accounts.Workspace
	Admin     bool
}

type Service struct {
	db       *gorm.DB
	blobs    *BlobStore
	kernels  Kernels
	audit    Auditor
	maxBytes int64
	now      func() time.Time
}

const DefaultMaxBytes int64 = 2 << 30

// UseKernels attaches the workspace kernel resolver; the team server sets it
// once its gateway exists.
func (s *Service) UseKernels(kernels Kernels) { s.kernels = kernels }

func Open(db *gorm.DB, dataRoot string, kernels Kernels, audit Auditor, maxBytes int64) (*Service, error) {
	if err := db.AutoMigrate(&Publication{}, &Version{}, &VersionFile{}, &CopyRecord{}); err != nil {
		return nil, fmt.Errorf("迁移创意工坊数据表失败：%w", err)
	}
	blobs, err := NewBlobStore(dataRoot)
	if err != nil {
		return nil, err
	}
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}
	return &Service{db: db, blobs: blobs, kernels: kernels, audit: audit, maxBytes: maxBytes, now: time.Now}, nil
}

func (s *Service) Blobs() *BlobStore { return s.blobs }

func newID(prefix string) string {
	raw := make([]byte, 10)
	if _, err := rand.Read(raw); err != nil {
		panic(err)
	}
	return prefix + hex.EncodeToString(raw)
}

// ---- errors ----

type Error struct {
	Status  int
	Reason  string
	Message string
	Details any
}

func (e *Error) Error() string { return e.Message }

func newError(status int, reason, message string) *Error {
	return &Error{Status: status, Reason: reason, Message: message}
}

var (
	ErrNotFound        = newError(http.StatusNotFound, "publication_not_found", "作品不存在或已下架")
	ErrVersionNotFound = newError(http.StatusNotFound, "version_not_found", "版本不存在")
	ErrForbidden       = newError(http.StatusForbidden, "forbidden", "没有操作权限")
	ErrVersionChanged  = newError(http.StatusConflict, "version_changed", "作者已更新或撤回稿件，请刷新后重新审核")
	ErrReasonRequired  = newError(http.StatusBadRequest, "reason_required", "请填写原因")
	ErrRightsRequired  = newError(http.StatusBadRequest, "rights_required", "请确认你拥有或有权分享这些素材")
	ErrNotListed       = newError(http.StatusConflict, "publication_not_listed", "作品还没有通过审核的版本")
	ErrRequestInvalid  = newError(http.StatusBadRequest, "request_invalid", "复制请求标识无效")
	ErrCopyInProgress  = newError(http.StatusConflict, "copy_in_progress", "这次复制仍在进行，请稍后查看")
)

func AsError(err error) *Error {
	var typed *Error
	if errors.As(err, &typed) {
		return typed
	}
	var blocked *projectsnapshot.ErrBlocked
	if errors.As(err, &blocked) {
		return &Error{Status: http.StatusConflict, Reason: "preflight_blocked", Message: blocked.Error(), Details: blocked.Preflight}
	}
	if errors.Is(err, projectsnapshot.ErrProjectNotFound) {
		return newError(http.StatusNotFound, "project_not_found", "项目不存在")
	}
	var account *accounts.Error
	if errors.As(err, &account) {
		return &Error{Status: account.Status, Reason: account.Reason, Message: account.Message}
	}
	return newError(http.StatusInternalServerError, "internal_error", "创意工坊内部错误："+err.Error())
}

// ---- views ----

type AuthorView struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
}

type DerivedView struct {
	PublicationID string `json:"publicationId"`
	VersionID     string `json:"versionId"`
	Title         string `json:"title"`
	AuthorName    string `json:"authorName"`
	Available     bool   `json:"available"`
}

type Stats struct {
	Canvases int   `json:"canvases"`
	Nodes    int   `json:"nodes"`
	Files    int   `json:"files"`
	Bytes    int64 `json:"bytes"`
}

type PublicationView struct {
	ID               string       `json:"id"`
	Title            string       `json:"title"`
	Summary          string       `json:"summary"`
	Tags             []string     `json:"tags"`
	Author           AuthorView   `json:"author"`
	Official         bool         `json:"official"`
	Featured         bool         `json:"featured"`
	Status           string       `json:"status"`
	CoverSHA256      string       `json:"coverSha256"`
	CurrentVersionID string       `json:"currentVersionId"`
	VersionNo        int          `json:"versionNo"`
	Stats            Stats        `json:"stats"`
	CopyCount        int64        `json:"copyCount"`
	AllowDownload    bool         `json:"allowDownload"`
	UnlistReason     string       `json:"unlistReason"`
	DerivedFrom      *DerivedView `json:"derivedFrom"`
	PublishedAt      *time.Time   `json:"publishedAt"`
	UpdatedAt        time.Time    `json:"updatedAt"`
	CanManage        bool         `json:"canManage"`
}

type VersionView struct {
	ID             string       `json:"id"`
	PublicationID  string       `json:"publicationId"`
	VersionNo      int          `json:"versionNo"`
	ReviewStatus   string       `json:"reviewStatus"`
	ReviewNote     string       `json:"reviewNote"`
	Title          string       `json:"title"`
	Summary        string       `json:"summary"`
	Tags           []string     `json:"tags"`
	CoverSHA256    string       `json:"coverSha256"`
	AllowDownload  bool         `json:"allowDownload"`
	SnapshotSHA256 string       `json:"snapshotSha256"`
	Stats          Stats        `json:"stats"`
	DerivedFrom    *DerivedView `json:"derivedFrom"`
	CreatedAt      time.Time    `json:"createdAt"`
	ReviewedAt     *time.Time   `json:"reviewedAt"`
}

func decodeTags(raw string) []string {
	tags := []string{}
	_ = json.Unmarshal([]byte(raw), &tags)
	return tags
}

func (s *Service) authors(ctx context.Context) (map[string]AuthorView, error) {
	var users []accounts.User
	if err := s.db.WithContext(ctx).Select("id", "username", "display_name").Find(&users).Error; err != nil {
		return nil, err
	}
	out := make(map[string]AuthorView, len(users))
	for _, user := range users {
		out[user.ID] = AuthorView{ID: user.ID, Username: user.Username, DisplayName: user.DisplayName}
	}
	return out, nil
}

func (s *Service) derived(ctx context.Context, publicationID, versionID string, authors map[string]AuthorView) *DerivedView {
	if publicationID == "" {
		return nil
	}
	view := &DerivedView{PublicationID: publicationID, VersionID: versionID}
	var source Publication
	if err := s.db.WithContext(ctx).Where("id = ?", publicationID).First(&source).Error; err == nil {
		view.Title = source.Title
		view.AuthorName = authors[source.AuthorUserID].DisplayName
		view.Available = source.Status == PublicationListed && source.CurrentVersionID != ""
	}
	return view
}

func versionStats(version Version) Stats {
	return Stats{Canvases: version.CanvasCount, Nodes: version.NodeCount, Files: version.FileCount, Bytes: version.TotalBytes}
}

func (s *Service) publicationView(ctx context.Context, publication Publication, viewer *Viewer, authors map[string]AuthorView) PublicationView {
	view := PublicationView{ID: publication.ID, Title: publication.Title, Summary: publication.Summary, Tags: decodeTags(publication.TagsJSON),
		Author: authors[publication.AuthorUserID], Official: publication.Official, Featured: publication.Featured, Status: publication.Status,
		CoverSHA256: publication.CoverSHA256, CurrentVersionID: publication.CurrentVersionID, CopyCount: publication.CopyCount,
		AllowDownload: publication.AllowDownload, UnlistReason: publication.UnlistReason, PublishedAt: publication.PublishedAt, UpdatedAt: publication.UpdatedAt,
		DerivedFrom: s.derived(ctx, publication.DerivedFromPublicationID, publication.DerivedFromVersionID, authors)}
	if viewer != nil {
		view.CanManage = viewer.Admin || viewer.User.ID == publication.AuthorUserID
	}
	if publication.CurrentVersionID != "" {
		var version Version
		if err := s.db.WithContext(ctx).Where("id = ?", publication.CurrentVersionID).First(&version).Error; err == nil {
			view.VersionNo = version.VersionNo
			view.Stats = versionStats(version)
		}
	}
	return view
}

func (s *Service) versionView(ctx context.Context, version Version, authors map[string]AuthorView) VersionView {
	return VersionView{ID: version.ID, PublicationID: version.PublicationID, VersionNo: version.VersionNo, ReviewStatus: version.ReviewStatus,
		ReviewNote: version.ReviewNote, Title: version.Title, Summary: version.Summary, Tags: decodeTags(version.TagsJSON), CoverSHA256: version.CoverSHA256,
		AllowDownload: version.AllowDownload, SnapshotSHA256: version.SnapshotSHA256, Stats: versionStats(version),
		DerivedFrom: s.derived(ctx, version.DerivedFromPublicationID, version.DerivedFromVersionID, authors), CreatedAt: version.CreatedAt, ReviewedAt: version.ReviewedAt}
}

// ---- submission ----

type SubmissionInput struct {
	ProjectID       string   `json:"projectId"`
	Title           string   `json:"title"`
	Summary         string   `json:"summary"`
	Tags            []string `json:"tags"`
	CoverResourceID string   `json:"coverResourceId"`
	AllowDownload   bool     `json:"allowDownload"`
	RightsConfirmed bool     `json:"rightsConfirmed"`
}

func (s *Service) Preflight(ctx context.Context, viewer Viewer, projectID string) (*projectsnapshot.Preflight, error) {
	kernel, err := s.kernels(ctx, viewer.Workspace)
	if err != nil {
		return nil, err
	}
	preflight, err := kernel.Preflight(projectID)
	if err != nil {
		return nil, err
	}
	if preflight.Stats.Bytes > s.maxBytes {
		preflight.Issues = append(preflight.Issues, projectsnapshot.Issue{Level: "error", Code: "too_large",
			Message: fmt.Sprintf("文件总大小超过 %d MB", s.maxBytes>>20)})
	}
	return preflight, nil
}

func normalizeSubmission(input *SubmissionInput) error {
	input.Title = strings.TrimSpace(input.Title)
	input.Summary = strings.TrimSpace(input.Summary)
	if !input.RightsConfirmed {
		return ErrRightsRequired
	}
	if count := utf8.RuneCountInString(input.Title); count == 0 || count > 80 {
		return newError(http.StatusBadRequest, "title_invalid", "标题需为 1–80 个字符")
	}
	if utf8.RuneCountInString(input.Summary) > 1000 {
		return newError(http.StatusBadRequest, "summary_invalid", "简介最多 1000 个字符")
	}
	tags := []string{}
	seen := map[string]bool{}
	for _, tag := range input.Tags {
		tag = strings.TrimSpace(tag)
		if tag == "" || seen[tag] {
			continue
		}
		if utf8.RuneCountInString(tag) > 20 || len(tags) >= 8 {
			return newError(http.StatusBadRequest, "tags_invalid", "最多 8 个标签，每个不超过 20 个字符")
		}
		seen[tag] = true
		tags = append(tags, tag)
	}
	input.Tags = tags
	return nil
}

// Submit exports an immutable snapshot of the project and queues it for
// review. A pending version of the same project is replaced by the new one.
func (s *Service) Submit(ctx context.Context, viewer Viewer, input SubmissionInput) (*PublicationView, *VersionView, error) {
	if err := normalizeSubmission(&input); err != nil {
		return nil, nil, err
	}
	preflight, err := s.Preflight(ctx, viewer, input.ProjectID)
	if err != nil {
		return nil, nil, err
	}
	if preflight.Blocked() {
		return nil, nil, &projectsnapshot.ErrBlocked{Preflight: *preflight}
	}
	kernel, err := s.kernels(ctx, viewer.Workspace)
	if err != nil {
		return nil, nil, err
	}
	exported, err := kernel.Export(input.ProjectID, s.blobs)
	if err != nil {
		return nil, nil, err
	}
	snapshotJSON, err := json.Marshal(exported.Snapshot)
	if err != nil {
		return nil, nil, err
	}
	snapshotSHA, _, err := s.blobs.Put(bytes.NewReader(snapshotJSON))
	if err != nil {
		return nil, nil, err
	}
	cover := ""
	if input.CoverResourceID != "" {
		cover = exported.Resources[input.CoverResourceID]
		if cover == "" {
			return nil, nil, newError(http.StatusBadRequest, "cover_invalid", "封面需从作品里的图片中选择")
		}
	} else if len(preflight.Covers) > 0 {
		cover = exported.Resources[preflight.Covers[0].ResourceID]
	}
	var copied CopyRecord
	derivedPublication, derivedVersion := "", ""
	if err := s.db.WithContext(ctx).Where("user_id = ? AND target_project_id = ? AND status = ?", viewer.User.ID, input.ProjectID, CopyCompleted).
		Order("created_at desc").First(&copied).Error; err == nil {
		derivedPublication, derivedVersion = copied.PublicationID, copied.VersionID
	}
	tags, _ := json.Marshal(input.Tags)
	now := s.now().UTC()
	var publication Publication
	var version Version
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Where("author_user_id = ? AND source_project_id = ?", viewer.User.ID, input.ProjectID).First(&publication).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			publication = Publication{ID: newID("p_"), AuthorUserID: viewer.User.ID, SourceProjectID: input.ProjectID, Status: "draft",
				Title: input.Title, Summary: input.Summary, TagsJSON: string(tags), CoverSHA256: cover, AllowDownload: input.AllowDownload,
				DerivedFromPublicationID: derivedPublication, DerivedFromVersionID: derivedVersion, CreatedAt: now, UpdatedAt: now}
			err = tx.Create(&publication).Error
		}
		if err != nil {
			return err
		}
		if err := tx.Model(&Version{}).Where("publication_id = ? AND review_status = ?", publication.ID, ReviewPending).
			Updates(map[string]any{"review_status": ReviewWithdrawn, "review_note": "已被新的投稿替代"}).Error; err != nil {
			return err
		}
		var latest int
		if err := tx.Model(&Version{}).Where("publication_id = ?", publication.ID).Select("COALESCE(MAX(version_no), 0)").Scan(&latest).Error; err != nil {
			return err
		}
		stats := exported.Snapshot.Stats
		version = Version{ID: newID("v_"), PublicationID: publication.ID, VersionNo: latest + 1, SnapshotSHA256: snapshotSHA, FileCount: len(exported.Snapshot.Files),
			TotalBytes: stats.Bytes, CanvasCount: stats.Canvases, NodeCount: stats.Nodes, SourceWorkspaceID: viewer.Workspace.ID, Title: input.Title,
			Summary: input.Summary, TagsJSON: string(tags), CoverSHA256: cover, AllowDownload: input.AllowDownload, DerivedFromPublicationID: derivedPublication,
			DerivedFromVersionID: derivedVersion, ReviewStatus: ReviewPending, CreatedAt: now}
		if err := tx.Create(&version).Error; err != nil {
			return err
		}
		files := make([]VersionFile, 0, len(exported.Snapshot.Files))
		for _, file := range exported.Snapshot.Files {
			files = append(files, VersionFile{VersionID: version.ID, SHA256: file.SHA256, Size: file.Size, MimeType: file.MimeType, Kind: file.Kind, Name: file.Name})
		}
		if len(files) > 0 {
			if err := tx.CreateInBatches(files, 200).Error; err != nil {
				return err
			}
		}
		return tx.Model(&Publication{}).Where("id = ?", publication.ID).Update("updated_at", now).Error
	})
	if err != nil {
		return nil, nil, err
	}
	_ = s.audit.Record(ctx, viewer.User.ID, "workshop.submit", viewer.User.ID, fmt.Sprintf("publication=%s version=%s", publication.ID, version.ID))
	authors, err := s.authors(ctx)
	if err != nil {
		return nil, nil, err
	}
	pub := s.publicationView(ctx, publication, &viewer, authors)
	ver := s.versionView(ctx, version, authors)
	return &pub, &ver, nil
}

type MySubmission struct {
	Publication PublicationView `json:"publication"`
	Latest      VersionView     `json:"latest"`
}

func (s *Service) MySubmissions(ctx context.Context, viewer Viewer) ([]MySubmission, error) {
	var publications []Publication
	if err := s.db.WithContext(ctx).Where("author_user_id = ?", viewer.User.ID).Order("updated_at desc").Find(&publications).Error; err != nil {
		return nil, err
	}
	authors, err := s.authors(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]MySubmission, 0, len(publications))
	for _, publication := range publications {
		var latest Version
		if err := s.db.WithContext(ctx).Where("publication_id = ?", publication.ID).Order("version_no desc").First(&latest).Error; err != nil {
			continue
		}
		items = append(items, MySubmission{Publication: s.publicationView(ctx, publication, &viewer, authors), Latest: s.versionView(ctx, latest, authors)})
	}
	return items, nil
}

func (s *Service) loadVersion(ctx context.Context, id string) (*Version, *Publication, error) {
	var version Version
	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&version).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrVersionNotFound
		}
		return nil, nil, err
	}
	var publication Publication
	if err := s.db.WithContext(ctx).Where("id = ?", version.PublicationID).First(&publication).Error; err != nil {
		return nil, nil, err
	}
	return &version, &publication, nil
}

func (s *Service) Withdraw(ctx context.Context, viewer Viewer, versionID string) error {
	version, publication, err := s.loadVersion(ctx, versionID)
	if err != nil {
		return err
	}
	if publication.AuthorUserID != viewer.User.ID {
		return ErrForbidden
	}
	result := s.db.WithContext(ctx).Model(&Version{}).Where("id = ? AND review_status = ?", version.ID, ReviewPending).
		Updates(map[string]any{"review_status": ReviewWithdrawn, "review_note": "作者撤回"})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrVersionChanged
	}
	return nil
}

// ---- visibility ----

// visible reports whether viewer may read a version: everyone sees the
// current approved version of a listed work; authors and admins see theirs.
func visible(viewer Viewer, version Version, publication Publication) bool {
	if viewer.Admin || publication.AuthorUserID == viewer.User.ID {
		return true
	}
	return publication.Status == PublicationListed && publication.CurrentVersionID == version.ID && version.ReviewStatus == ReviewApproved
}

type ListQuery struct {
	Query    string
	Source   string
	Sort     string
	Tag      string
	Page     int
	PageSize int
}

func (s *Service) List(ctx context.Context, viewer Viewer, query ListQuery) ([]PublicationView, int64, error) {
	if query.Page < 1 {
		query.Page = 1
	}
	if query.PageSize < 1 || query.PageSize > 60 {
		query.PageSize = 24
	}
	db := s.db.WithContext(ctx).Model(&Publication{}).Where("status = ? AND current_version_id <> ''", PublicationListed)
	if text := strings.TrimSpace(query.Query); text != "" {
		like := "%" + strings.ReplaceAll(strings.ReplaceAll(text, "%", ""), "_", "") + "%"
		db = db.Where("title LIKE ? OR summary LIKE ? OR tags_json LIKE ?", like, like, like)
	}
	if tag := strings.TrimSpace(query.Tag); tag != "" {
		encoded, _ := json.Marshal(tag)
		db = db.Where("tags_json LIKE ?", "%"+string(encoded)+"%")
	}
	switch query.Source {
	case "official":
		db = db.Where("official = ?", true)
	case "colleague":
		db = db.Where("official = ?", false)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	switch query.Sort {
	case "latest":
		db = db.Order("published_at desc")
	case "copies":
		db = db.Order("copy_count desc").Order("published_at desc")
	default:
		db = db.Order("featured desc").Order("official desc").Order("published_at desc")
	}
	var publications []Publication
	if err := db.Offset((query.Page - 1) * query.PageSize).Limit(query.PageSize).Find(&publications).Error; err != nil {
		return nil, 0, err
	}
	authors, err := s.authors(ctx)
	if err != nil {
		return nil, 0, err
	}
	views := make([]PublicationView, 0, len(publications))
	for _, publication := range publications {
		views = append(views, s.publicationView(ctx, publication, &viewer, authors))
	}
	return views, total, nil
}

func (s *Service) Publication(ctx context.Context, viewer Viewer, id string) (*PublicationView, error) {
	var publication Publication
	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&publication).Error; err != nil {
		return nil, ErrNotFound
	}
	canManage := viewer.Admin || publication.AuthorUserID == viewer.User.ID
	if !canManage && (publication.Status != PublicationListed || publication.CurrentVersionID == "") {
		return nil, newError(http.StatusGone, "publication_unlisted", "该作品已下架")
	}
	authors, err := s.authors(ctx)
	if err != nil {
		return nil, err
	}
	view := s.publicationView(ctx, publication, &viewer, authors)
	return &view, nil
}

// Snapshot returns the stored snapshot JSON of a version the viewer may read.
func (s *Service) Snapshot(ctx context.Context, viewer Viewer, versionID string) ([]byte, error) {
	version, publication, err := s.loadVersion(ctx, versionID)
	if err != nil {
		return nil, err
	}
	if !visible(viewer, *version, *publication) {
		return nil, ErrVersionNotFound
	}
	body, err := s.blobs.Open(version.SnapshotSHA256)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	return io.ReadAll(body)
}

type FileAccess struct {
	Path          string
	MimeType      string
	Name          string
	AllowDownload bool
}

// File authorizes one blob through a version the viewer may read.
func (s *Service) File(ctx context.Context, viewer Viewer, versionID, sha string) (*FileAccess, error) {
	version, publication, err := s.loadVersion(ctx, versionID)
	if err != nil {
		return nil, err
	}
	if !visible(viewer, *version, *publication) {
		return nil, ErrVersionNotFound
	}
	var file VersionFile
	if err := s.db.WithContext(ctx).Where("version_id = ? AND sha256 = ?", version.ID, sha).First(&file).Error; err != nil {
		return nil, newError(http.StatusNotFound, "file_not_found", "文件不存在")
	}
	path, err := s.blobs.FilePath(sha)
	if err != nil {
		return nil, newError(http.StatusNotFound, "file_not_found", "文件不存在")
	}
	allow := version.AllowDownload || viewer.Admin || publication.AuthorUserID == viewer.User.ID
	return &FileAccess{Path: path, MimeType: file.MimeType, Name: file.Name, AllowDownload: allow}, nil
}

// ---- copy ----

type CopyView struct {
	ID              string    `json:"id"`
	RequestID       string    `json:"requestId"`
	Status          string    `json:"status"`
	Error           string    `json:"error"`
	PublicationID   string    `json:"publicationId"`
	VersionID       string    `json:"versionId"`
	TargetProjectID string    `json:"targetProjectId"`
	CreatedAt       time.Time `json:"createdAt"`
}

func copyView(record CopyRecord) CopyView {
	return CopyView{ID: record.ID, RequestID: record.RequestID, Status: record.Status, Error: record.Error, PublicationID: record.PublicationID,
		VersionID: record.VersionID, TargetProjectID: record.TargetProjectID, CreatedAt: record.CreatedAt}
}

var requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,80}$`)

// Copy imports the current approved version into the viewer's workspace. The
// same request ID always returns the same copy.
func (s *Service) Copy(ctx context.Context, viewer Viewer, publicationID, requestID, title string) (*CopyView, error) {
	if !requestIDPattern.MatchString(requestID) {
		return nil, ErrRequestInvalid
	}
	var existing CopyRecord
	if err := s.db.WithContext(ctx).Where("user_id = ? AND request_id = ?", viewer.User.ID, requestID).First(&existing).Error; err == nil {
		if existing.Status != CopyCompleted {
			return nil, ErrCopyInProgress
		}
		view := copyView(existing)
		return &view, nil
	}
	var publication Publication
	if err := s.db.WithContext(ctx).Where("id = ?", publicationID).First(&publication).Error; err != nil {
		return nil, ErrNotFound
	}
	if publication.CurrentVersionID == "" || (publication.Status != PublicationListed && !viewer.Admin && publication.AuthorUserID != viewer.User.ID) {
		return nil, ErrNotListed
	}
	version, _, err := s.loadVersion(ctx, publication.CurrentVersionID)
	if err != nil {
		return nil, err
	}
	record := CopyRecord{ID: newID("c_"), UserID: viewer.User.ID, RequestID: requestID, PublicationID: publication.ID, VersionID: version.ID,
		TargetWorkspaceID: viewer.Workspace.ID, Status: "running", CreatedAt: s.now().UTC()}
	if err := s.db.WithContext(ctx).Create(&record).Error; err != nil {
		return nil, ErrCopyInProgress
	}
	result, err := s.importVersion(ctx, viewer, *version, publication, title)
	if err != nil {
		_ = s.db.WithContext(ctx).Delete(&CopyRecord{}, "id = ?", record.ID).Error
		return nil, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&CopyRecord{}).Where("id = ?", record.ID).Updates(map[string]any{"status": CopyCompleted, "target_project_id": result.ProjectID}).Error; err != nil {
			return err
		}
		return tx.Model(&Publication{}).Where("id = ?", publication.ID).Update("copy_count", gorm.Expr("copy_count + 1")).Error
	})
	if err != nil {
		return nil, err
	}
	record.Status, record.TargetProjectID = CopyCompleted, result.ProjectID
	view := copyView(record)
	return &view, nil
}

func (s *Service) importVersion(ctx context.Context, viewer Viewer, version Version, publication Publication, title string) (*projectsnapshot.ImportResult, error) {
	body, err := s.blobs.Open(version.SnapshotSHA256)
	if err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(body)
	_ = body.Close()
	if err != nil {
		return nil, err
	}
	var snapshot projectsnapshot.Snapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return nil, err
	}
	kernel, err := s.kernels(ctx, viewer.Workspace)
	if err != nil {
		return nil, err
	}
	title = strings.TrimSpace(title)
	if title == "" {
		title = publication.Title + "（副本）"
	}
	if utf8.RuneCountInString(title) > 120 {
		title = string([]rune(title)[:120])
	}
	return kernel.Import(&snapshot, s.blobs, projectsnapshot.ImportOptions{Title: title})
}

func (s *Service) CopyStatus(ctx context.Context, viewer Viewer, requestID string) (*CopyView, error) {
	var record CopyRecord
	if err := s.db.WithContext(ctx).Where("user_id = ? AND request_id = ?", viewer.User.ID, requestID).First(&record).Error; err != nil {
		return nil, newError(http.StatusNotFound, "copy_not_found", "没有这次复制记录")
	}
	view := copyView(record)
	return &view, nil
}

// Origin tells the canvas page where a copied project came from.
func (s *Service) Origin(ctx context.Context, viewer Viewer, projectID string) (*DerivedView, error) {
	var record CopyRecord
	if err := s.db.WithContext(ctx).Where("user_id = ? AND target_project_id = ? AND status = ?", viewer.User.ID, projectID, CopyCompleted).
		Order("created_at desc").First(&record).Error; err != nil {
		return nil, newError(http.StatusNotFound, "origin_not_found", "不是从创意工坊复制的项目")
	}
	authors, err := s.authors(ctx)
	if err != nil {
		return nil, err
	}
	return s.derived(ctx, record.PublicationID, record.VersionID, authors), nil
}

// AuthorUnlist lets an author take their own work down; only admins relist.
func (s *Service) AuthorUnlist(ctx context.Context, viewer Viewer, publicationID string) error {
	result := s.db.WithContext(ctx).Model(&Publication{}).Where("id = ? AND author_user_id = ?", publicationID, viewer.User.ID).
		Updates(map[string]any{"status": PublicationUnlisted, "unlist_reason": "作者下架", "updated_at": s.now().UTC()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrForbidden
	}
	_ = s.audit.Record(ctx, viewer.User.ID, "workshop.unlist", viewer.User.ID, "publication="+publicationID)
	return nil
}
