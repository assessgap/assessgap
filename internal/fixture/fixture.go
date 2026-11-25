// Package fixture builds the frozen county test database from the CSV pages
// in testdata/fixture, through the same code as a real build (ingest.Load,
// then ingest.Build). Only tests import it.
//
// A test package that uses it runs its tests through Main, which removes the
// built database afterwards:
//
//	func TestMain(m *testing.M) { os.Exit(fixture.Main(m)) }
package fixture

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/assessgap/assessgap/internal/ingest"
	"github.com/assessgap/assessgap/internal/store"
)

// Dir is testdata/fixture at the repository root.
var Dir = func() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "fixture")
}()

var (
	once    sync.Once
	tmpDir  string
	dbPath  string
	initErr error
)

// Main runs m and then deletes the database built for it.
func Main(m *testing.M) int {
	code := m.Run()
	if tmpDir != "" {
		if err := os.RemoveAll(tmpDir); err != nil {
			fmt.Fprintln(os.Stderr, "fixture:", err)
		}
	}
	return code
}

// Path returns the path of the built fixture database, building it on the
// first call in the test binary. Treat the file as read-only.
func Path(tb testing.TB) string {
	tb.Helper()
	once.Do(func() { dbPath, initErr = build() })
	if initErr != nil {
		tb.Fatalf("fixture: %v", initErr)
	}
	return dbPath
}

// Open opens the fixture database read-only, closing it when tb ends.
func Open(tb testing.TB) *sql.DB {
	tb.Helper()
	db, err := store.OpenReadOnly(Path(tb))
	if err != nil {
		tb.Fatalf("fixture: %v", err)
	}
	tb.Cleanup(func() { db.Close() })
	return db
}

func build() (string, error) {
	dir, err := os.MkdirTemp("", "assessgap-fixture-")
	if err != nil {
		return "", err
	}
	tmpDir = dir
	path := filepath.Join(dir, "cook.db")
	db, err := store.Open(path)
	if err != nil {
		return "", err
	}
	quiet := func(string, ...any) {}
	if err := ingest.Load(db, ingest.Datasets, Dir, quiet); err != nil {
		db.Close()
		return "", err
	}
	if err := ingest.Build(context.Background(), db, quiet); err != nil {
		db.Close()
		return "", err
	}
	// Fold the WAL into the main file so read-only connections see everything.
	if _, err := db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		db.Close()
		return "", err
	}
	return path, db.Close()
}
