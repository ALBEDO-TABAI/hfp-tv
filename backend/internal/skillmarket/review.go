package skillmarket

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"infinite-canvas/backend/internal/accounts"
	"infinite-canvas/backend/internal/skillhub"

	"gorm.io/gorm"
)

// ReviewAuthor and SubmissionView are the admin review projections.
type ReviewAuthor struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
}

type SubmissionView struct {
	ID               string            `json:"id"`
	Status           string            `json:"status"`
	Revision         int64             `json:"revision"`
	PreviewDigest    string            `json:"previewDigest"`
	Author           ReviewAuthor      `json:"author"`
	Target           string            `json:"target"`
	SkillID          string            `json:"skillId"`
	CurrentSkillName *string           `json:"currentSkillName"`
	Metadata         skillhub.Metadata `json:"metadata"`
	FileCount        int               `json:"fileCount"`
	ArchiveBytes     int64             `json:"archiveBytes"`
	SubmittedAt      *time.Time        `json:"submittedAt"`
	ReviewedAt       *time.Time        `json:"reviewedAt"`
	ReviewReason     string            `json:"reviewReason"`
	Reviewer         *string           `json:"reviewer"`
}

type ReleaseFiles struct {
	ReleaseID    string          `json:"releaseId"`
	VersionLabel string          `json:"versionLabel"`
	Files        []skillhub.File `json:"files"`
}

type SubmissionDetail struct {
	Submission      SubmissionView  `json:"submission"`
	Files           []skillhub.File `json:"files"`
	PreviousRelease *ReleaseFiles   `json:"previousRelease"`
}

type FileContent struct {
	Path     string `json:"path"`
	MimeType string `json:"mimeType"`
	Encoding string `json:"encoding"`
	Content  string `json:"content"`
}

type SkillView struct {
	SkillID          string `json:"skillId"`
	Name             string `json:"name"`
	OwnerUsername    string `json:"ownerUsername"`
	Official         bool   `json:"official"`
	Listed           bool   `json:"listed"`
	CurrentReleaseID string `json:"currentReleaseId"`
	VersionLabel     string `json:"versionLabel"`
	PublishedAt      string `json:"publishedAt"`
}

func (s *Service) userNames(ctx context.Context) (map[string]accounts.User, error) {
	var users []accounts.User
	if err := s.db.WithContext(ctx).Select("id", "username", "display_name").Find(&users).Error; err != nil {
		return nil, err
	}
	byID := make(map[string]accounts.User, len(users))
	for _, user := range users {
		byID[user.ID] = user
	}
	return byID, nil
}

func (s *Service) view(ctx context.Context, row Submission, users map[string]accounts.User) (SubmissionView, error) {
	draft, err := s.draftOf(row)
	if err != nil {
		return SubmissionView{}, err
	}
	author := users[row.UserID]
	view := SubmissionView{ID: row.ID, Status: row.Status, Revision: row.Revision, PreviewDigest: row.PreviewDigest,
		Author: ReviewAuthor{ID: author.ID, Username: author.Username, DisplayName: author.DisplayName}, Target: row.Target, SkillID: row.SkillID,
		Metadata: draft.Metadata, FileCount: len(draft.Files), ArchiveBytes: row.ArchiveBytes, SubmittedAt: row.SubmittedAt, ReviewedAt: row.ReviewedAt,
		ReviewReason: row.ReviewReason}
	if row.ReviewerID != "" {
		name := users[row.ReviewerID].Username
		view.Reviewer = &name
	}
	var skill Skill
	if err := s.db.WithContext(ctx).Where("id = ?", row.SkillID).First(&skill).Error; err == nil {
		view.CurrentSkillName = &skill.Name
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return SubmissionView{}, err
	}
	return view, nil
}

// ReviewQueue lists "pending" (in review) or "processed" (published, rejected).
func (s *Service) ReviewQueue(ctx context.Context, status string) ([]SubmissionView, error) {
	query := s.db.WithContext(ctx).Model(&Submission{})
	if status == "processed" {
		query = query.Where("status IN ?", []string{StatusPublished, StatusRejected}).Order("reviewed_at desc").Limit(200)
	} else {
		query = query.Where("status = ?", StatusInReview).Order("submitted_at asc")
	}
	var rows []Submission
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	users, err := s.userNames(ctx)
	if err != nil {
		return nil, err
	}
	views := make([]SubmissionView, 0, len(rows))
	for _, row := range rows {
		view, err := s.view(ctx, row, users)
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, nil
}

func (s *Service) submissionByID(ctx context.Context, id string) (*Submission, error) {
	var row Submission
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrSubmissionNotFound
	}
	return &row, err
}

func (s *Service) ReviewDetail(ctx context.Context, id string) (*SubmissionDetail, error) {
	row, err := s.submissionByID(ctx, id)
	if err != nil {
		return nil, err
	}
	users, err := s.userNames(ctx)
	if err != nil {
		return nil, err
	}
	view, err := s.view(ctx, *row, users)
	if err != nil {
		return nil, err
	}
	draft, err := s.draftOf(*row)
	if err != nil {
		return nil, err
	}
	detail := &SubmissionDetail{Submission: view, Files: draft.Files}
	var skill Skill
	if err := s.db.WithContext(ctx).Where("id = ?", row.SkillID).First(&skill).Error; err == nil && skill.CurrentReleaseID != "" && skill.CurrentReleaseID != row.ReleaseID {
		var release Release
		if err := s.db.WithContext(ctx).Where("id = ?", skill.CurrentReleaseID).First(&release).Error; err == nil {
			previous, err := s.releaseOf(release)
			if err != nil {
				return nil, err
			}
			detail.PreviousRelease = &ReleaseFiles{ReleaseID: previous.ReleaseID, VersionLabel: previous.VersionLabel, Files: previous.Files}
		}
	}
	return detail, nil
}

// ReviewFile returns one submitted file for reading. Text stays text; the UI
// must never render it as HTML.
func (s *Service) ReviewFile(ctx context.Context, id, name string) (*FileContent, error) {
	row, err := s.submissionByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if row.ArchiveSHA256 == "" {
		return nil, ErrContentMissing
	}
	archive, err := s.loadArchive(row.ArchiveSHA256)
	if err != nil {
		return nil, err
	}
	data, err := readArchiveFile(archive, name)
	if err != nil {
		return nil, ErrNotFound
	}
	mimeType := mimeTypeFor(name)
	if strings.HasPrefix(mimeType, "image/") || !utf8.Valid(data) {
		return &FileContent{Path: name, MimeType: mimeType, Encoding: "base64", Content: base64.StdEncoding.EncodeToString(data)}, nil
	}
	return &FileContent{Path: name, MimeType: mimeType, Encoding: "utf-8", Content: string(data)}, nil
}

// Approve publishes the exact revision and preview the reviewer saw, signs the
// release and makes it the skill's current version.
func (s *Service) Approve(ctx context.Context, reviewer *accounts.User, id string, revision int64, digest string, official bool) (*SubmissionView, *skillhub.Release, error) {
	row, err := s.submissionByID(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if row.Status != StatusInReview || row.Revision != revision || row.PreviewDigest != digest {
		return nil, nil, ErrSubmissionChanged
	}
	draft, err := s.draftOf(*row)
	if err != nil {
		return nil, nil, err
	}
	author, err := s.users.ActiveUser(ctx, row.UserID)
	if err != nil {
		return nil, nil, newError(409, "author_inactive", "作者账号已停用，不能发布")
	}
	now := s.now().UTC()
	authorName := author.DisplayName
	if official {
		authorName = "官方"
	}
	releaseID := "r_" + now.Format("20060102") + "_" + randomHex(4)
	release := skillhub.Release{Metadata: draft.Metadata, ReleaseID: releaseID, Author: authorName, ContentHash: row.ContentHash,
		ArchiveSHA256: row.ArchiveSHA256, ArchiveBytes: row.ArchiveBytes, Files: draft.Files,
		DownloadURL: "skills/" + row.SkillID + "/releases/" + releaseID + "/archive", PublishedAt: now.Format(time.RFC3339),
		Review: &skillhub.ReviewSummary{ReviewedAt: now.Format(time.RFC3339), Revision: row.Revision, ContentHash: row.ContentHash,
			ArchiveSHA256: row.ArchiveSHA256, PreviewDigest: row.PreviewDigest, Trials: []skillhub.ReviewTrial{}}}
	release.SkillID = row.SkillID
	payload, attestation, err := s.signer.Sign(release)
	if err != nil {
		return nil, nil, err
	}
	attestationJSON, err := json.Marshal(attestation)
	if err != nil {
		return nil, nil, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&Submission{}).Where("id = ? AND status = ? AND revision = ? AND preview_digest = ?", row.ID, StatusInReview, revision, digest).
			Updates(map[string]any{"status": StatusPublished, "reviewer_id": reviewer.ID, "reviewed_at": now, "release_id": releaseID, "review_reason": "", "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrSubmissionChanged
		}
		if err := tx.Create(&Release{ID: releaseID, SkillID: row.SkillID, SubmissionID: row.ID, ReleaseJSON: string(payload),
			AttestationJSON: string(attestationJSON), ArchiveSHA256: row.ArchiveSHA256, PublishedAt: now}).Error; err != nil {
			return err
		}
		var skill Skill
		err := tx.Where("id = ?", row.SkillID).First(&skill).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return tx.Create(&Skill{ID: row.SkillID, OwnerUserID: row.UserID, Name: draft.Metadata.Name, Official: official, Listed: true,
				CurrentReleaseID: releaseID, CreatedAt: now, UpdatedAt: now}).Error
		}
		if err != nil {
			return err
		}
		if skill.OwnerUserID != row.UserID {
			return ErrSkillNotOwned
		}
		return tx.Model(&Skill{}).Where("id = ?", skill.ID).Updates(map[string]any{"name": draft.Metadata.Name, "current_release_id": releaseID,
			"official": skill.Official || official, "updated_at": now}).Error
	})
	if err != nil {
		return nil, nil, err
	}
	detail := "skill=" + row.SkillID + " release=" + releaseID
	if official {
		detail += " official"
	}
	_ = s.users.Record(ctx, reviewer.ID, "skill.approve", row.UserID, detail)
	updated, err := s.submissionByID(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	users, err := s.userNames(ctx)
	if err != nil {
		return nil, nil, err
	}
	view, err := s.view(ctx, *updated, users)
	if err != nil {
		return nil, nil, err
	}
	release.Attestation = attestation
	return &view, &release, nil
}

func (s *Service) Reject(ctx context.Context, reviewer *accounts.User, id string, revision int64, reason string) (*SubmissionView, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, ErrReasonRequired
	}
	if utf8.RuneCountInString(reason) > 500 {
		reason = string([]rune(reason)[:500])
	}
	row, err := s.submissionByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if row.Status != StatusInReview || row.Revision != revision {
		return nil, ErrSubmissionChanged
	}
	now := s.now().UTC()
	result := s.db.WithContext(ctx).Model(&Submission{}).Where("id = ? AND status = ? AND revision = ?", row.ID, StatusInReview, revision).
		Updates(map[string]any{"status": StatusRejected, "reviewer_id": reviewer.ID, "reviewed_at": now, "review_reason": reason, "updated_at": now})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, ErrSubmissionChanged
	}
	_ = s.users.Record(ctx, reviewer.ID, "skill.reject", row.UserID, "submission="+row.ID+" "+reason)
	updated, err := s.submissionByID(ctx, id)
	if err != nil {
		return nil, err
	}
	users, err := s.userNames(ctx)
	if err != nil {
		return nil, err
	}
	view, err := s.view(ctx, *updated, users)
	return &view, err
}

func (s *Service) Skills(ctx context.Context) ([]SkillView, error) {
	var skills []Skill
	if err := s.db.WithContext(ctx).Order("updated_at desc").Find(&skills).Error; err != nil {
		return nil, err
	}
	users, err := s.userNames(ctx)
	if err != nil {
		return nil, err
	}
	views := make([]SkillView, 0, len(skills))
	for _, skill := range skills {
		view := SkillView{SkillID: skill.ID, Name: skill.Name, OwnerUsername: users[skill.OwnerUserID].Username, Official: skill.Official,
			Listed: skill.Listed, CurrentReleaseID: skill.CurrentReleaseID}
		var row Release
		if err := s.db.WithContext(ctx).Where("id = ?", skill.CurrentReleaseID).First(&row).Error; err == nil {
			if release, err := s.releaseOf(row); err == nil {
				view.VersionLabel, view.PublishedAt = release.VersionLabel, release.PublishedAt
			}
		}
		views = append(views, view)
	}
	return views, nil
}

// SetListed hides or restores a skill; already installed copies stay with
// their owners.
func (s *Service) SetListed(ctx context.Context, actor *accounts.User, skillID string, listed bool) (*SkillView, error) {
	result := s.db.WithContext(ctx).Model(&Skill{}).Where("id = ?", skillID).Updates(map[string]any{"listed": listed, "updated_at": s.now().UTC()})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, ErrNotFound
	}
	action := "skill.relist"
	if !listed {
		action = "skill.unlist"
	}
	_ = s.users.Record(ctx, actor.ID, action, "", "skill="+skillID)
	views, err := s.Skills(ctx)
	if err != nil {
		return nil, err
	}
	for index := range views {
		if views[index].SkillID == skillID {
			return &views[index], nil
		}
	}
	return nil, ErrNotFound
}
