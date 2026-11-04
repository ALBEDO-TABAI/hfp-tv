package database

import (
	"bytes"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// A setting nobody has saved yet is looked up with First at every start; the
// miss must not be printed as an error, while real failures still are.
func TestLoggerSkipsMissingRowsButKeepsRealErrors(t *testing.T) {
	var out bytes.Buffer
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/log.db"), &gorm.Config{Logger: newLogger(&out)})
	if err != nil {
		t.Fatal(err)
	}
	type setting struct {
		Key   string `gorm:"primaryKey"`
		Value string
	}
	if err := db.AutoMigrate(&setting{}); err != nil {
		t.Fatal(err)
	}
	var found setting
	if err := db.First(&found, "key = ?", "appearance").Error; err != gorm.ErrRecordNotFound {
		t.Fatalf("lookup = %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("a missing row was logged: %s", out.String())
	}
	if err := db.Exec("SELECT * FROM missing_table").Error; err == nil {
		t.Fatal("query on a missing table succeeded")
	}
	if !strings.Contains(out.String(), "missing_table") {
		t.Fatalf("a real error was not logged: %q", out.String())
	}
}
