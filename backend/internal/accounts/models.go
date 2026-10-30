// Package accounts owns team-server identity: users, sessions, invitations,
// the workspace registry and the admin audit trail. It lives in its own
// SQLite file (platform.db) and never touches workspace kernel databases.
package accounts

import "time"

const (
	RoleAdmin  = "admin"
	RoleMember = "member"

	StatusActive   = "active"
	StatusDisabled = "disabled"
)

type User struct {
	ID           string     `gorm:"primaryKey;size:40" json:"id"`
	Username     string     `gorm:"uniqueIndex;size:64;not null" json:"username"`
	DisplayName  string     `gorm:"size:128;not null" json:"displayName"`
	PasswordHash string     `gorm:"size:255;not null" json:"-"`
	Role         string     `gorm:"size:16;not null" json:"role"`
	Status       string     `gorm:"size:16;not null" json:"status"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
	LastActiveAt *time.Time `json:"lastActiveAt"`
}

func (User) TableName() string { return "users" }

// Session stores only a SHA-256 of the cookie token.
type Session struct {
	TokenHash  string    `gorm:"primaryKey;size:64"`
	UserID     string    `gorm:"index;size:40;not null"`
	ExpiresAt  time.Time `gorm:"index;not null"`
	LastSeenAt time.Time `gorm:"not null"`
	IP         string    `gorm:"size:64"`
	UserAgent  string    `gorm:"size:255"`
	CreatedAt  time.Time
}

func (Session) TableName() string { return "sessions" }

type Invitation struct {
	TokenHash string    `gorm:"primaryKey;size:64"`
	Role      string    `gorm:"size:16;not null"`
	CreatedBy string    `gorm:"size:40;not null"`
	ExpiresAt time.Time `gorm:"not null"`
	UsedBy    *string   `gorm:"size:40"`
	UsedAt    *time.Time
	CreatedAt time.Time
}

func (Invitation) TableName() string { return "invitations" }

// Workspace registers one kernel data directory per user. DataDir is relative
// to the team-server data root so the volume can move.
type Workspace struct {
	ID                 string `gorm:"primaryKey;size:40"`
	OwnerUserID        string `gorm:"uniqueIndex;size:40;not null"`
	DataDir            string `gorm:"size:255;not null"`
	Status             string `gorm:"size:16;not null"`
	StorageQuotaBytes  int64  `gorm:"not null;default:0"`
	RelayKeyCiphertext string `gorm:"type:text"`
	RelayKeyHint       string `gorm:"size:16"`
	RelayKeyUpdatedAt  *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (Workspace) TableName() string { return "workspaces" }

// AuditEvent is append-only; nothing updates or deletes rows.
type AuditEvent struct {
	ID           uint      `gorm:"primaryKey;autoIncrement"`
	ActorUserID  string    `gorm:"size:40;not null;index"`
	Action       string    `gorm:"size:64;not null"`
	TargetUserID string    `gorm:"size:40;index"`
	Detail       string    `gorm:"size:512"`
	CreatedAt    time.Time `gorm:"index"`
}

func (AuditEvent) TableName() string { return "audit_events" }
