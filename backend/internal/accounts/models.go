package accounts

import "time"

const (
	RoleMember     = "member"
	RoleAdmin      = "admin"
	StatusActive   = "active"
	StatusDisabled = "disabled"
)

type User struct {
	ID           string     `json:"id" gorm:"primaryKey"`
	Username     string     `json:"username" gorm:"not null;uniqueIndex"`
	DisplayName  string     `json:"display_name"`
	PasswordHash string     `json:"-" gorm:"not null"`
	Role         string     `json:"role" gorm:"not null;check:role IN ('member','admin')"`
	Status       string     `json:"status" gorm:"not null;check:status IN ('active','disabled')"`
	LastActiveAt *time.Time `json:"last_active_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

type Session struct {
	TokenHash string    `json:"-" gorm:"primaryKey"`
	UserID    string    `json:"-" gorm:"not null;index"`
	ExpiresAt time.Time `json:"-" gorm:"not null;index"`
	IP        string    `json:"-"`
	UserAgent string    `json:"-"`
}

type Invitation struct {
	ID        string    `json:"id" gorm:"primaryKey"`
	TokenHash string    `json:"-" gorm:"not null;uniqueIndex"`
	Role      string    `json:"role" gorm:"not null;check:role IN ('member','admin')"`
	CreatedBy string    `json:"created_by" gorm:"not null"`
	ExpiresAt time.Time `json:"expires_at" gorm:"not null"`
	UsedBy    *string   `json:"used_by"`
	CreatedAt time.Time `json:"created_at"`
}

type Workspace struct {
	ID                 string `gorm:"primaryKey"`
	OwnerUserID        string `gorm:"not null;uniqueIndex"`
	DataDir            string `gorm:"not null;uniqueIndex"`
	Status             string `gorm:"not null;check:status IN ('active','disabled')"`
	StorageQuotaBytes  int64
	RelayKeyCiphertext []byte     `json:"-"`
	RelayKeyHint       string     `json:"-"`
	RelayKeyUpdatedAt  *time.Time `json:"-"`
}

type PlatformAuditEvent struct {
	ID          string    `gorm:"primaryKey"`
	ActorUserID string    `gorm:"not null;index"`
	Action      string    `gorm:"not null"`
	TargetID    string    `gorm:"not null"`
	CreatedAt   time.Time `gorm:"not null"`
}

type Identity struct {
	User        User   `json:"user" gorm:"embedded"`
	WorkspaceID string `json:"workspace_id"`
}

type NewUser struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
	Role        string `json:"role"`
}
