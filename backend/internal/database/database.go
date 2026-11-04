package database

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Config struct {
	Driver  string
	DSN     string
	DataDir string
}

func Open(config Config) (*gorm.DB, error) {
	driver := strings.ToLower(strings.TrimSpace(config.Driver))
	if driver == "" {
		driver = "sqlite"
	}
	switch driver {
	case "sqlite":
		dsn := strings.TrimSpace(config.DSN)
		if dsn == "" {
			if err := os.MkdirAll(config.DataDir, 0o755); err != nil {
				return nil, err
			}
			dsn = config.DataDir + "/open_ai_canvas.db?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=on&_synchronous=NORMAL"
		}
		return gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: Logger()})
	default:
		return nil, fmt.Errorf("不支持的数据库驱动：%s", driver)
	}
}

// Logger is GORM's default logger (warnings, errors and queries slower than
// 200 ms) except that a lookup finding no row is not printed as an error:
// optional rows such as a setting nobody has saved yet are read with First,
// and their absence is an answer the caller handles.
func Logger() logger.Interface {
	return newLogger(os.Stdout)
}

func newLogger(out io.Writer) logger.Interface {
	return logger.New(log.New(out, "\r\n", log.LstdFlags), logger.Config{
		SlowThreshold:             200 * time.Millisecond,
		LogLevel:                  logger.Warn,
		IgnoreRecordNotFoundError: true,
		Colorful:                  true,
	})
}

func ConfigurePool(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	// SQLite serializes writers. A single shared connection avoids intermittent
	// SQLITE_BUSY failures under concurrent autosave/task updates while WAL
	// still keeps reads cheap for this single-process desktop application.
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	return nil
}
