// Package skillmarket is the team server's in-house skill market. It speaks the
// SkillHub v1 contract the workspace kernels already use (internal/skillhub),
// keeps submissions, reviews and releases in platform.db, stores canonical
// archives on the data volume and signs approved releases with a platform key.
package skillmarket

import "time"

const (
	StatusDraft         = "draft"
	StatusReadyToSubmit = "ready_to_submit"
	StatusInReview      = "in_review"
	StatusPublished     = "published"
	StatusRejected      = "rejected"

	TargetNew      = "new"
	TargetExisting = "existing"
)

// Skill is a published identity; it exists only after a first approval.
type Skill struct {
	ID               string `gorm:"primaryKey;size:80"`
	OwnerUserID      string `gorm:"size:40;not null;index"`
	Name             string `gorm:"size:160;not null"`
	Official         bool   `gorm:"not null;default:false"`
	Listed           bool   `gorm:"not null;default:true"`
	CurrentReleaseID string `gorm:"size:80"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (Skill) TableName() string { return "market_skills" }

// Submission is one author draft. SkillID is assigned at creation, so a new
// skill's ID is stable from the first upload to approval.
type Submission struct {
	ID            string `gorm:"primaryKey;size:40"`
	UserID        string `gorm:"size:40;not null;index"`
	SkillID       string `gorm:"size:80;not null;index"`
	Target        string `gorm:"size:16;not null"`
	Status        string `gorm:"size:24;not null;index"`
	Revision      int64  `gorm:"not null"`
	MetadataJSON  string `gorm:"type:text;not null"`
	ContentHash   string `gorm:"size:64"`
	ArchiveSHA256 string `gorm:"size:64"`
	ArchiveBytes  int64
	FilesJSON     string `gorm:"type:text"`
	PreviewDigest string `gorm:"size:64"`
	ReviewReason  string `gorm:"size:1024"`
	ReviewerID    string `gorm:"size:40"`
	ReleaseID     string `gorm:"size:80"`
	SubmittedAt   *time.Time
	ReviewedAt    *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (Submission) TableName() string { return "market_submissions" }

// Release stores the exact JSON that was signed; it is served back unchanged
// because clients compare the signed payload field by field.
type Release struct {
	ID              string `gorm:"primaryKey;size:80"`
	SkillID         string `gorm:"size:80;not null;index"`
	SubmissionID    string `gorm:"size:40;not null"`
	ReleaseJSON     string `gorm:"type:text;not null"`
	AttestationJSON string `gorm:"type:text;not null"`
	ArchiveSHA256   string `gorm:"size:64;not null"`
	PublishedAt     time.Time
}

func (Release) TableName() string { return "market_releases" }

type Token struct {
	TokenHash string    `gorm:"primaryKey;size:64"`
	UserID    string    `gorm:"size:40;not null;index"`
	ExpiresAt time.Time `gorm:"not null"`
	CreatedAt time.Time
}

func (Token) TableName() string { return "market_tokens" }

type DeviceAuthorization struct {
	DeviceCodeHash string    `gorm:"primaryKey;size:64"`
	UserCode       string    `gorm:"uniqueIndex;size:16;not null"`
	ClientName     string    `gorm:"size:80"`
	ExpiresAt      time.Time `gorm:"not null"`
	ApprovedUserID string    `gorm:"size:40"`
	CreatedAt      time.Time
}

func (DeviceAuthorization) TableName() string { return "market_device_authorizations" }

// IdempotentResponse replays create/submit responses for a retried key.
type IdempotentResponse struct {
	UserID       string `gorm:"primaryKey;size:40"`
	Key          string `gorm:"primaryKey;size:160"`
	Operation    string `gorm:"size:16;not null"`
	ResponseJSON string `gorm:"type:text;not null"`
	CreatedAt    time.Time
}

func (IdempotentResponse) TableName() string { return "market_idempotency" }
