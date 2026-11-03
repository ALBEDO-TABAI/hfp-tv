package errorlog

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"infinite-canvas/backend/internal/kernel"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "platform.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	store, err := Open(db)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestErrorLogKeepsTaskFailuresAndRepeatsWorkspaceErrorsOncePerMinute(t *testing.T) {
	store := openTestStore(t)
	now := time.Date(2026, 11, 3, 18, 5, 4, 0, time.UTC)
	store.now = func() time.Time { return now }
	if err := store.RecordTask("ws-alice", kernel.FailureReport{TaskID: "b81f", TaskType: "canvas_video", Model: "doubao-seedance-2.0-fast",
		Message: "声明式协议任务失败：上游返回失败状态", ProviderTaskID: "task_Cdcy", FailedAt: now, Log: []string{"17:52:56 INFO 后端任务开始处理", "17:53:30 INFO 上游任务状态：running"}}); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if err := store.RecordWorkspace("ws-bob", errors.New("准备工作区配置失败")); err != nil {
			t.Fatal(err)
		}
	}
	now = now.Add(2 * time.Minute)
	if err := store.RecordWorkspace("ws-bob", errors.New("准备工作区配置失败")); err != nil {
		t.Fatal(err)
	}
	events, err := store.List(Query{})
	if err != nil || len(events) != 3 {
		t.Fatalf("events = %d %v", len(events), err)
	}
	tasks, _ := store.List(Query{Source: SourceTask, WorkspaceIDs: []string{"ws-alice"}})
	if len(tasks) != 1 || tasks[0].ProviderTaskID != "task_Cdcy" || len(tasks[0].Log) != 2 || tasks[0].Source != SourceTask {
		t.Fatalf("task events = %+v", tasks)
	}
	older, _ := store.List(Query{Before: events[0].CreatedAt})
	if len(older) != 2 {
		t.Fatalf("paging = %d", len(older))
	}
}

func TestErrorLogPrunesToTheNewestEvents(t *testing.T) {
	store := openTestStore(t)
	store.keep = 30
	base := time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)
	for index := range 100 {
		if err := store.RecordTask("ws", kernel.FailureReport{TaskID: fmt.Sprint(index), Message: "x", FailedAt: base.Add(time.Duration(index) * time.Second)}); err != nil {
			t.Fatal(err)
		}
	}
	var count int64
	store.db.Model(&Event{}).Count(&count)
	newest, _ := store.List(Query{Limit: 1})
	if count != 30 || newest[0].TaskID != "99" {
		t.Fatalf("count=%d newest=%+v", count, newest)
	}
}
