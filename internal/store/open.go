package store

import (
	"database/sql"

	_ "modernc.org/sqlite"
)

// OpenReadOnly opens a database for concurrent reads (the website's view of cook.db).
//
// The page cache is per connection and the Go SQLite driver keeps what it
// grows, so it is sized for one request, not for the server: the largest
// neighborhood's report reads about 14 MB of pages, and the OS file cache and
// the mmap window hold the rest. At 200 MB each, the 2 idle connections alone
// could keep 400 MB on the 1 GB server (docs/dev/2026-11-26-audit.md).
func OpenReadOnly(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_pragma=query_only(1)&_pragma=busy_timeout(5000)&_pragma=cache_size(-32000)&_pragma=mmap_size(1073741824)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(2) // database/sql's default, stated: idle connections keep their cache
	return db, db.Ping()
}

// OpenApp opens (or creates) the small read-write app database (email signups).
func OpenApp(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		return nil, err
	}
	_, err = db.Exec(appSchema)
	return db, err
}

const appSchema = `
CREATE TABLE IF NOT EXISTS signup (
  id         INTEGER PRIMARY KEY,
  email      TEXT NOT NULL,
  pin        TEXT NOT NULL DEFAULT '',
  township   TEXT NOT NULL DEFAULT '',
  reason     TEXT NOT NULL DEFAULT '',   -- e.g. "township-opens", "condo-support"
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
  UNIQUE (email, pin)
);`
