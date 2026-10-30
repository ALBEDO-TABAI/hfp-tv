// Package workshop is the team server's project workshop: members submit an
// immutable project snapshot, admins review it, every member can browse the
// approved version read-only and copy it into their own workspace.
package workshop

import "time"

const (
	ReviewPending   = "pending"
	ReviewApproved  = "approved"
	ReviewRejected  = "rejected"
	ReviewWithdrawn = "withdrawn"

	PublicationListed   = "listed"
	PublicationUnlisted = "unlisted"

	CopyCompleted = "completed"
	CopyFailed    = "failed"
)

// Publication is one shared work. It becomes visible once a version is
// approved; later versions replace it only after their own review.
type Publication struct {
	ID                       string `gorm:"primaryKey;size:40"`
	AuthorUserID             string `gorm:"size:40;not null;index;uniqueIndex:idx_workshop_publication_source"`
	SourceProjectID          string `gorm:"size:80;not null;uniqueIndex:idx_workshop_publication_source"`
	Status                   string `gorm:"size:16;not null;index"`
	CurrentVersionID         string `gorm:"size:40"`
	Title                    string `gorm:"size:160;not null"`
	Summary                  string `gorm:"type:text"`
	TagsJSON                 string `gorm:"type:text"`
	CoverSHA256              string `gorm:"size:64"`
	AllowDownload            bool   `gorm:"not null;default:false"`
	Official                 bool   `gorm:"not null;default:false"`
	Featured                 bool   `gorm:"not null;default:false"`
	CopyCount                int64  `gorm:"not null;default:0"`
	UnlistReason             string `gorm:"size:512"`
	DerivedFromPublicationID string `gorm:"size:40"`
	DerivedFromVersionID     string `gorm:"size:40"`
	PublishedAt              *time.Time
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

func (Publication) TableName() string { return "workshop_publications" }

// Version is one submission: an immutable snapshot plus the proposed public
// information. Review binds to SnapshotSHA256.
type Version struct {
	ID                       string `gorm:"primaryKey;size:40"`
	PublicationID            string `gorm:"size:40;not null;index"`
	VersionNo                int    `gorm:"not null"`
	SnapshotSHA256           string `gorm:"size:64;not null"`
	FileCount                int    `gorm:"not null"`
	TotalBytes               int64  `gorm:"not null"`
	CanvasCount              int    `gorm:"not null"`
	NodeCount                int    `gorm:"not null"`
	SourceWorkspaceID        string `gorm:"size:40;not null"`
	Title                    string `gorm:"size:160;not null"`
	Summary                  string `gorm:"type:text"`
	TagsJSON                 string `gorm:"type:text"`
	CoverSHA256              string `gorm:"size:64"`
	AllowDownload            bool   `gorm:"not null;default:false"`
	DerivedFromPublicationID string `gorm:"size:40"`
	DerivedFromVersionID     string `gorm:"size:40"`
	ReviewStatus             string `gorm:"size:16;not null;index"`
	ReviewerID               string `gorm:"size:40"`
	ReviewNote               string `gorm:"size:1024"`
	ReviewedAt               *time.Time
	CreatedAt                time.Time
}

func (Version) TableName() string { return "workshop_versions" }

// VersionFile authorizes file reads: a blob is served only through a version
// the reader may see.
type VersionFile struct {
	VersionID string `gorm:"primaryKey;size:40"`
	SHA256    string `gorm:"primaryKey;size:64"`
	Size      int64  `gorm:"not null"`
	MimeType  string `gorm:"size:128"`
	Kind      string `gorm:"size:16"`
	Name      string `gorm:"size:255"`
}

func (VersionFile) TableName() string { return "workshop_version_files" }

// CopyRecord makes copies idempotent per request and remembers where a copied
// project came from, so its later submission is labelled as derived.
type CopyRecord struct {
	ID                string `gorm:"primaryKey;size:40"`
	UserID            string `gorm:"size:40;not null;uniqueIndex:idx_workshop_copy_request"`
	RequestID         string `gorm:"size:80;not null;uniqueIndex:idx_workshop_copy_request"`
	PublicationID     string `gorm:"size:40;not null;index"`
	VersionID         string `gorm:"size:40;not null"`
	TargetWorkspaceID string `gorm:"size:40;not null;index"`
	TargetProjectID   string `gorm:"size:80;index"`
	Status            string `gorm:"size:16;not null"`
	Error             string `gorm:"size:512"`
	CreatedAt         time.Time
}

func (CopyRecord) TableName() string { return "workshop_copies" }
