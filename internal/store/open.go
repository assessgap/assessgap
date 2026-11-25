package store

import (
	"database/sql"

	_ "modernc.org/sqlite"
)

// OpenReadOnly opens a database for concurrent reads (the website's view of cook.db).
func OpenReadOnly(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_pragma=query_only(1)&_pragma=busy_timeout(5000)&_pragma=cache_size(-200000)&_pragma=mmap_size(1073741824)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(8)
	return db, db.Ping()
}

// OpenApp opens (or creates) the small read-write app database: signups, orders.
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
