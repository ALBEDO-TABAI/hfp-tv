// Package errorlog keeps the team server's error log in platform.db: tasks that
// failed in a member's workspace kernel (pushed by the kernel, never read from
// its database) and workspaces that could not be opened. Administrators read it
// on the "错误记录" page instead of collecting screenshots from members.
package errorlog

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"infinite-canvas/backend/internal/kernel"

	"gorm.io/gorm"
)

const (
	SourceTask      = "task"
	SourceWorkspace = "workspace"

	// DefaultKeep bounds the table; older events are pruned.
	DefaultKeep = 5000
	// A workspace that cannot open fails every request; one event per minute is enough.
	workspaceRepeatWindow = time.Minute

	maxMessageRunes = 2000
	maxDetailRunes  = 600
	maxLogLines     = 16
	maxLogLineRunes = 400
)

type Event struct {
	ID             string     `gorm:"primaryKey;size:40" json:"id"`
	CreatedAt      time.Time  `gorm:"index;not null" json:"createdAt"`
	WorkspaceID    string     `gorm:"index;size:40;not null" json:"workspaceId"`
	Source         string     `gorm:"index;size:16;not null" json:"source"`
	TaskID         string     `gorm:"size:64" json:"taskId,omitempty"`
	TaskType       string     `gorm:"size:64" json:"taskType,omitempty"`
	Model          string     `gorm:"size:160" json:"model,omitempty"`
	Stage          string     `gorm:"size:160" json:"stage,omitempty"`
	Category       string     `gorm:"size:64" json:"category,omitempty"`
	Message        string     `gorm:"type:text;not null" json:"message"`
	ProviderTaskID string     `gorm:"size:160" json:"providerTaskId,omitempty"`
	HTTPStatus     int        `json:"httpStatus,omitempty"`
	UpstreamDetail string     `gorm:"type:text" json:"upstreamDetail,omitempty"`
	StartedAt      *time.Time `json:"startedAt,omitempty"`
	LogJSON        string     `gorm:"type:text" json:"-"`
	Log            []string   `gorm:"-" json:"log,omitempty"`
}

func (Event) TableName() string { return "error_events" }

type Store struct {
	db   *gorm.DB
	keep int

	mu          sync.Mutex
	inserts     int
	lastOpenErr map[string]time.Time
	now         func() time.Time
	newID       func() string
}

func Open(db *gorm.DB) (*Store, error) {
	if db == nil {
		return nil, errors.New("错误记录缺少平台数据库")
	}
	if err := db.AutoMigrate(&Event{}); err != nil {
		return nil, err
	}
	return &Store{db: db, keep: DefaultKeep, lastOpenErr: map[string]time.Time{}, now: time.Now, newID: kernel.NewID}, nil
}

// RecordTask stores a failed task reported by a workspace kernel.
func (s *Store) RecordTask(workspaceID string, report kernel.FailureReport) error {
	logLines := report.Log
	if len(logLines) > maxLogLines {
		logLines = logLines[len(logLines)-maxLogLines:]
	}
	trimmed := make([]string, 0, len(logLines))
	for _, line := range logLines {
		trimmed = append(trimmed, clip(line, maxLogLineRunes))
	}
	encoded, _ := json.Marshal(trimmed)
	failedAt := report.FailedAt
	if failedAt.IsZero() {
		failedAt = s.now()
	}
	source := strings.TrimSpace(report.Source)
	if source == "" {
		source = SourceTask
	}
	return s.insert(&Event{
		CreatedAt: failedAt, WorkspaceID: workspaceID, Source: source,
		TaskID: clip(report.TaskID, 64), TaskType: clip(report.TaskType, 64), Model: clip(report.Model, 160),
		Stage: clip(report.Stage, 160), Category: clip(report.Category, 64), Message: clip(report.Message, maxMessageRunes),
		ProviderTaskID: clip(report.ProviderTaskID, 160), HTTPStatus: report.HTTPStatus,
		UpstreamDetail: clip(report.UpstreamDetail, maxDetailRunes), StartedAt: report.StartedAt, LogJSON: string(encoded),
	})
}

// RecordWorkspace stores a workspace kernel that could not be opened.
func (s *Store) RecordWorkspace(workspaceID string, cause error) error {
	if cause == nil {
		return nil
	}
	message := clip(cause.Error(), maxMessageRunes)
	now := s.now()
	key := workspaceID + "\x00" + message
	s.mu.Lock()
	if last, seen := s.lastOpenErr[key]; seen && now.Sub(last) < workspaceRepeatWindow {
		s.mu.Unlock()
		return nil
	}
	s.lastOpenErr[key] = now
	if len(s.lastOpenErr) > 1000 {
		s.lastOpenErr = map[string]time.Time{key: now}
	}
	s.mu.Unlock()
	return s.insert(&Event{CreatedAt: now, WorkspaceID: workspaceID, Source: SourceWorkspace, Stage: "打开工作区", Message: message})
}

func (s *Store) insert(event *Event) error {
	event.ID = s.newID()
	if err := s.db.Create(event).Error; err != nil {
		return err
	}
	s.mu.Lock()
	s.inserts++
	prune := s.inserts%100 == 0
	s.mu.Unlock()
	if prune {
		return s.prune()
	}
	return nil
}

// prune keeps the newest s.keep events.
func (s *Store) prune() error {
	var cutoff Event
	err := s.db.Order("created_at desc").Offset(s.keep).Limit(1).Find(&cutoff).Error
	if err != nil || cutoff.ID == "" {
		return err
	}
	return s.db.Where("created_at <= ?", cutoff.CreatedAt).Delete(&Event{}).Error
}

type Query struct {
	// WorkspaceIDs limits the result; empty means every workspace.
	WorkspaceIDs []string
	Source       string
	// Before returns events older than this time, for paging.
	Before time.Time
	Limit  int
}

// List returns events newest first.
func (s *Store) List(query Query) ([]Event, error) {
	limit := query.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	db := s.db.Order("created_at desc").Limit(limit)
	if len(query.WorkspaceIDs) > 0 {
		db = db.Where("workspace_id IN ?", query.WorkspaceIDs)
	}
	if source := strings.TrimSpace(query.Source); source != "" {
		db = db.Where("source = ?", source)
	}
	if !query.Before.IsZero() {
		db = db.Where("created_at < ?", query.Before)
	}
	var events []Event
	if err := db.Find(&events).Error; err != nil {
		return nil, err
	}
	for index := range events {
		_ = json.Unmarshal([]byte(events[index].LogJSON), &events[index].Log)
	}
	return events, nil
}

func clip(value string, limit int) string {
	value = strings.TrimSpace(value)
	if runes := []rune(value); len(runes) > limit {
		return string(runes[:limit]) + "…"
	}
	return value
}
