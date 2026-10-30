package workshop

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"
)

type ReviewItem struct {
	Version     VersionView     `json:"version"`
	Publication PublicationView `json:"publication"`
	Author      AuthorView      `json:"author"`
	Previous    *VersionView    `json:"previous"`
}

// ReviewQueue lists pending versions, or processed ones (approved, rejected).
func (s *Service) ReviewQueue(ctx context.Context, viewer Viewer, status string) ([]ReviewItem, error) {
	query := s.db.WithContext(ctx).Model(&Version{})
	if status == "processed" {
		query = query.Where("review_status IN ?", []string{ReviewApproved, ReviewRejected}).Order("reviewed_at desc").Limit(200)
	} else {
		query = query.Where("review_status = ?", ReviewPending).Order("created_at asc")
	}
	var versions []Version
	if err := query.Find(&versions).Error; err != nil {
		return nil, err
	}
	authors, err := s.authors(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]ReviewItem, 0, len(versions))
	for _, version := range versions {
		item, err := s.reviewItem(ctx, viewer, version, authors)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, nil
}

func (s *Service) reviewItem(ctx context.Context, viewer Viewer, version Version, authors map[string]AuthorView) (*ReviewItem, error) {
	var publication Publication
	if err := s.db.WithContext(ctx).Where("id = ?", version.PublicationID).First(&publication).Error; err != nil {
		return nil, err
	}
	item := &ReviewItem{Version: s.versionView(ctx, version, authors), Publication: s.publicationView(ctx, publication, &viewer, authors),
		Author: authors[publication.AuthorUserID]}
	if publication.CurrentVersionID != "" && publication.CurrentVersionID != version.ID {
		var previous Version
		if err := s.db.WithContext(ctx).Where("id = ?", publication.CurrentVersionID).First(&previous).Error; err == nil {
			view := s.versionView(ctx, previous, authors)
			item.Previous = &view
		}
	}
	return item, nil
}

func (s *Service) ReviewDetail(ctx context.Context, viewer Viewer, versionID string) (*ReviewItem, error) {
	version, _, err := s.loadVersion(ctx, versionID)
	if err != nil {
		return nil, err
	}
	authors, err := s.authors(ctx)
	if err != nil {
		return nil, err
	}
	return s.reviewItem(ctx, viewer, *version, authors)
}

// Approve publishes exactly the snapshot the reviewer looked at.
func (s *Service) Approve(ctx context.Context, viewer Viewer, versionID, snapshotSHA string, official bool) (*ReviewItem, error) {
	version, publication, err := s.loadVersion(ctx, versionID)
	if err != nil {
		return nil, err
	}
	if version.ReviewStatus != ReviewPending || version.SnapshotSHA256 != snapshotSHA {
		return nil, ErrVersionChanged
	}
	now := s.now().UTC()
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&Version{}).Where("id = ? AND review_status = ? AND snapshot_sha256 = ?", version.ID, ReviewPending, snapshotSHA).
			Updates(map[string]any{"review_status": ReviewApproved, "reviewer_id": viewer.User.ID, "reviewed_at": now, "review_note": ""})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrVersionChanged
		}
		updates := map[string]any{"status": PublicationListed, "current_version_id": version.ID, "title": version.Title, "summary": version.Summary,
			"tags_json": version.TagsJSON, "cover_sha256": version.CoverSHA256, "allow_download": version.AllowDownload, "unlist_reason": "",
			"official": publication.Official || official, "updated_at": now}
		if publication.PublishedAt == nil {
			updates["published_at"] = now
		}
		if publication.DerivedFromPublicationID == "" && version.DerivedFromPublicationID != "" {
			updates["derived_from_publication_id"] = version.DerivedFromPublicationID
			updates["derived_from_version_id"] = version.DerivedFromVersionID
		}
		return tx.Model(&Publication{}).Where("id = ?", publication.ID).Updates(updates).Error
	})
	if err != nil {
		return nil, err
	}
	detail := fmt.Sprintf("publication=%s version=%s", publication.ID, version.ID)
	if official {
		detail += " official"
	}
	_ = s.audit.Record(ctx, viewer.User.ID, "workshop.approve", publication.AuthorUserID, detail)
	return s.ReviewDetail(ctx, viewer, versionID)
}

func (s *Service) Reject(ctx context.Context, viewer Viewer, versionID, reason string) (*ReviewItem, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, ErrReasonRequired
	}
	if utf8.RuneCountInString(reason) > 500 {
		reason = string([]rune(reason)[:500])
	}
	version, publication, err := s.loadVersion(ctx, versionID)
	if err != nil {
		return nil, err
	}
	result := s.db.WithContext(ctx).Model(&Version{}).Where("id = ? AND review_status = ?", version.ID, ReviewPending).
		Updates(map[string]any{"review_status": ReviewRejected, "reviewer_id": viewer.User.ID, "reviewed_at": s.now().UTC(), "review_note": reason})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, ErrVersionChanged
	}
	_ = s.audit.Record(ctx, viewer.User.ID, "workshop.reject", publication.AuthorUserID, fmt.Sprintf("version=%s %s", version.ID, reason))
	return s.ReviewDetail(ctx, viewer, versionID)
}

// AdminPublications lists every work, including unlisted ones.
func (s *Service) AdminPublications(ctx context.Context, viewer Viewer) ([]PublicationView, error) {
	var publications []Publication
	if err := s.db.WithContext(ctx).Where("current_version_id <> ''").Order("updated_at desc").Find(&publications).Error; err != nil {
		return nil, err
	}
	authors, err := s.authors(ctx)
	if err != nil {
		return nil, err
	}
	views := make([]PublicationView, 0, len(publications))
	for _, publication := range publications {
		views = append(views, s.publicationView(ctx, publication, &viewer, authors))
	}
	return views, nil
}

// Moderate applies an admin action: unlist (reason required), relist,
// feature, unfeature, official, unofficial.
func (s *Service) Moderate(ctx context.Context, viewer Viewer, publicationID, action, reason string) (*PublicationView, error) {
	var publication Publication
	if err := s.db.WithContext(ctx).Where("id = ?", publicationID).First(&publication).Error; err != nil {
		return nil, ErrNotFound
	}
	updates := map[string]any{"updated_at": s.now().UTC()}
	switch action {
	case "unlist":
		reason = strings.TrimSpace(reason)
		if reason == "" {
			return nil, ErrReasonRequired
		}
		updates["status"], updates["unlist_reason"] = PublicationUnlisted, reason
	case "relist":
		if publication.CurrentVersionID == "" {
			return nil, ErrNotListed
		}
		updates["status"], updates["unlist_reason"] = PublicationListed, ""
	case "feature", "unfeature":
		updates["featured"] = action == "feature"
	case "official", "unofficial":
		updates["official"] = action == "official"
	default:
		return nil, newError(400, "action_invalid", "不支持的操作")
	}
	if err := s.db.WithContext(ctx).Model(&Publication{}).Where("id = ?", publication.ID).Updates(updates).Error; err != nil {
		return nil, err
	}
	_ = s.audit.Record(ctx, viewer.User.ID, "workshop."+action, publication.AuthorUserID, "publication="+publication.ID+" "+reason)
	if err := s.db.WithContext(ctx).Where("id = ?", publication.ID).First(&publication).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	authors, err := s.authors(ctx)
	if err != nil {
		return nil, err
	}
	view := s.publicationView(ctx, publication, &viewer, authors)
	return &view, nil
}
