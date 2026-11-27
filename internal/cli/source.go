package cli

import (
	"database/sql"
	"fmt"
	"os"

	"github.com/assessgap/assessgap/internal/store"
)

// DefaultDB is where the database lives when neither -db nor $ASSESSGAP_DB says.
const DefaultDB = "data/cook.db"

// dbPath resolves the database path: the -db flag, else $ASSESSGAP_DB, else DefaultDB.
func dbPath(env *Env, flagVal string) string {
	if flagVal != "" {
		return flagVal
	}
	if p := env.Getenv("ASSESSGAP_DB"); p != "" {
		return p
	}
	return DefaultDB
}

// errNoDB explains how to get a database when there isn't one at path.
func errNoDB(path string) error {
	return fmt.Errorf("no county database at %s.\n"+
		"  Build the whole county (about 45 minutes, 9 GB):   assessgap db sync\n"+
		"  Or a small sample (one area of Jefferson):          assessgap db sync -data data-sample -pin-prefix 1328\n"+
		"  Then pass -db PATH or set ASSESSGAP_DB=PATH if it isn't at %s.", path, DefaultDB)
}

// openCounty opens an existing county database read-only. Unlike sql.Open it
// fails when the file is missing instead of creating an empty database.
func openCounty(path string) (*sql.DB, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, errNoDB(path)
	}
	db, err := store.OpenReadOnly(path)
	if err != nil { // SQLite's own message ("file is not a database") names no file
		return nil, fmt.Errorf("county database %s: %w", path, err)
	}
	return db, nil
}
