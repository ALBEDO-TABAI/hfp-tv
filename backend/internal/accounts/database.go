package accounts

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Open(dataRoot string) (*Service, error) {
	if strings.TrimSpace(dataRoot) == "" {
		return nil, fmt.Errorf("必须指定平台数据目录")
	}
	root, err := filepath.Abs(dataRoot)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(root, "platform.db")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return nil, err
	}
	dsn := (&url.URL{Scheme: "file", Path: path}).String() + "?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=on&_txlock=immediate"
	// SQL 参数可能包含凭据摘要；平台连接不输出 SQL。
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), TranslateError: true})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	if err := Migrate(db); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return &Service{db: db, root: root, now: time.Now, attempts: make(map[loginKey]loginAttempts)}, nil
}

// 平台迁移独立于会清理托管表的本地工作区迁移。
func Migrate(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.AutoMigrate(&User{}, &Session{}, &Invitation{}, &Workspace{}, &PlatformAuditEvent{}); err != nil {
			return err
		}
		for _, operation := range []string{"UPDATE", "DELETE"} {
			statement := fmt.Sprintf(`CREATE TRIGGER IF NOT EXISTS platform_audit_no_%s BEFORE %s ON platform_audit_events BEGIN SELECT RAISE(ABORT, 'audit events are append-only'); END`, strings.ToLower(operation), operation)
			if err := tx.Exec(statement).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Service) Close() error {
	db, err := s.db.DB()
	if err != nil {
		return err
	}
	return db.Close()
}
