// Package relaychannel keeps the company relay definition an administrator
// saved from the browser. It lives in platform.db and, while present, takes
// the place of the deployment file (TEAMSERVER_RELAY_CHANNEL_FILE).
package relaychannel

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"infinite-canvas/backend/internal/accounts"

	"gorm.io/gorm"
)

// recordID is the single row: a team server has one company relay.
const recordID = "company-relay"

// Record is the saved definition: a model channel without credentials.
type Record struct {
	ID           string    `gorm:"primaryKey;size:40"`
	TemplateJSON string    `gorm:"type:text;not null"`
	UpdatedBy    string    `gorm:"size:40"`
	UpdatedAt    time.Time `gorm:"not null"`
}

func (Record) TableName() string { return "relay_channel_settings" }

// Saved is a loaded definition and who saved it.
type Saved struct {
	Template  map[string]any
	UpdatedBy string
	UpdatedAt time.Time
}

type Store struct {
	db  *gorm.DB
	now func() time.Time
}

func Open(db *gorm.DB) (*Store, error) {
	if db == nil {
		return nil, errors.New("公司中转站设置需要平台数据库")
	}
	if err := db.AutoMigrate(&Record{}); err != nil {
		return nil, err
	}
	return &Store{db: db, now: time.Now}, nil
}

// Load returns the saved definition, or nil when none was saved.
func (s *Store) Load() (*Saved, error) {
	var record Record
	err := s.db.Where("id = ?", recordID).Limit(1).Find(&record).Error
	if err != nil {
		return nil, err
	}
	if record.ID == "" {
		return nil, nil
	}
	var template map[string]any
	if err := json.Unmarshal([]byte(record.TemplateJSON), &template); err != nil {
		return nil, fmt.Errorf("已保存的公司中转站定义无法读取：%w", err)
	}
	return &Saved{Template: template, UpdatedBy: record.UpdatedBy, UpdatedAt: record.UpdatedAt}, nil
}

// Save stores the definition and records who changed it in the audit log.
func (s *Store) Save(actorID string, template map[string]any) (*Saved, error) {
	body, err := json.Marshal(template)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	record := Record{ID: recordID, TemplateJSON: string(body), UpdatedBy: actorID, UpdatedAt: now}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&record).Error; err != nil {
			return err
		}
		return tx.Create(&accounts.AuditEvent{ActorUserID: actorID, Action: "relay_channel.update", Detail: auditDetail(template), CreatedAt: now}).Error
	})
	if err != nil {
		return nil, err
	}
	return &Saved{Template: template, UpdatedBy: actorID, UpdatedAt: now}, nil
}

// Delete drops the saved definition so the deployment file applies again.
func (s *Store) Delete(actorID string) error {
	now := s.now().UTC()
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", recordID).Delete(&Record{}).Error; err != nil {
			return err
		}
		return tx.Create(&accounts.AuditEvent{ActorUserID: actorID, Action: "relay_channel.reset", Detail: "source=file", CreatedAt: now}).Error
	})
}

func auditDetail(template map[string]any) string {
	models, _ := template["models"].([]any)
	host := ""
	if raw, _ := template["baseUrl"].(string); raw != "" {
		if parsed, err := url.Parse(raw); err == nil {
			host = parsed.Host
		}
	}
	detail := fmt.Sprintf("host=%s models=%d", host, len(models))
	names := make([]string, 0, len(models))
	for _, model := range models {
		if name, _ := model.(string); name != "" {
			names = append(names, name)
		}
	}
	if list := strings.Join(names, ","); list != "" {
		detail += " " + list
	}
	if runes := []rune(detail); len(runes) > 500 {
		detail = string(runes[:500]) + "…"
	}
	return detail
}
