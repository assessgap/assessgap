package cli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"github.com/assessgap/assessgap/internal/ingest"
	"github.com/assessgap/assessgap/internal/refresh"
	"github.com/assessgap/assessgap/internal/store"
)

const dbUsage = `usage: assessgap db <step> [flags]

Steps:
  sync      fetch, load and build in one go (what you usually want)
  fetch     download dataset pages into <data>/raw/<dataset>/
  load      (re)build raw_<dataset> tables from downloaded pages
  build     turn raw_* tables into the typed, indexed tables the engine reads
  validate  compare a freshly built database with the one in service (ops/refresh.sh)
  finalize  checkpoint and switch a built database to a single file (ops/refresh.sh)

Run "assessgap db <step> -h" for its flags.`

func runDB(ctx context.Context, env *Env, args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "-help" || args[0] == "--help" {
		fmt.Fprintln(env.Stderr, dbUsage)
		if len(args) == 0 {
			return errUsage
		}
		return nil
	}
	step, args := args[0], args[1:]
	switch step {
	case "sync", "fetch", "load":
		return dbPipeline(ctx, env, step, args)
	case "build":
		return dbBuild(ctx, env, args)
	case "validate":
		return dbValidate(ctx, env, args)
	case "finalize":
		return dbFinalize(ctx, env, args)
	}
	fmt.Fprintf(env.Stderr, "unknown step %q\n\n%s\n", step, dbUsage)
	return errUsage
}

func logger(env *Env) func(string, ...any) {
	l := log.New(env.Stderr, "", log.LstdFlags)
	return l.Printf
}

// dbPipeline runs fetch, load, or (sync) fetch + load + build.
func dbPipeline(ctx context.Context, env *Env, step string, args []string) error {
	fl := newFlags(env, "db "+step, "[-data dir] [-raw dir] [-db path] [-pin-prefix P] [dataset...]")
	dataDir := fl.String("data", "data", "data directory")
	rawFlag := fl.String("raw", "", "directory for downloaded pages (default <data>/raw). Fetch skips pages already there, so use a fresh dir for a refresh")
	dbFlag := fl.String("db", "", "database to load into (default <data>/cook.db)")
	pageSize := fl.Int("page", 50000, "rows per request")
	pinPrefix := fl.String("pin-prefix", "", "only rows whose PIN starts with this (for small samples)")
	names, err := parseArgs(fl, args)
	if err != nil {
		return err
	}
	sel, err := ingest.Select(names, *pinPrefix)
	if err != nil {
		return &exitError{2, err.Error()}
	}
	rawDir, path := *rawFlag, *dbFlag
	if rawDir == "" {
		rawDir = filepath.Join(*dataDir, "raw")
	}
	if path == "" {
		path = filepath.Join(*dataDir, "cook.db")
	}
	logf := logger(env)

	if step == "fetch" || step == "sync" {
		if err := ingest.Fetch(ctx, ingest.NewClient(*pageSize, logf), sel, rawDir, logf); err != nil {
			return err
		}
	}
	if step == "fetch" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	db, err := store.Open(path)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := ingest.Load(db, sel, rawDir, logf); err != nil {
		return err
	}
	if step == "load" {
		return nil
	}
	if err := ingest.Build(ctx, db, logf); err != nil {
		return err
	}
	return logCounts(ctx, db, logf, path)
}

func dbBuild(ctx context.Context, env *Env, args []string) error {
	fl := newFlags(env, "db build", "[-db path]")
	path := fl.String("db", DefaultDB, "database holding raw_* tables")
	if _, err := parseArgs(fl, args); err != nil {
		return err
	}
	if _, err := os.Stat(*path); err != nil {
		return err // store.Open would silently create an empty database
	}
	db, err := store.Open(*path)
	if err != nil {
		return err
	}
	defer db.Close()
	logf := logger(env)
	if err := ingest.Build(ctx, db, logf); err != nil {
		return err
	}
	return logCounts(ctx, db, logf, *path)
}

func logCounts(ctx context.Context, db *sql.DB, logf func(string, ...any), path string) error {
	n, err := ingest.Counts(ctx, db)
	if err != nil {
		return err
	}
	for _, t := range ingest.Tables {
		logf("%-16s %d rows", t, n[t])
	}
	logf("database ready: %s", path)
	return nil
}

// dbValidate compares row counts and the latest assessment year of a new
// database with the one in service, and fails if the new one must not
// replace it. A missing -old file means a first build: only the non-empty
// rule applies.
func dbValidate(ctx context.Context, env *Env, args []string) error {
	fl := newFlags(env, "db validate", "-old OLD.db -new NEW.db [-max-shrink 0.02]")
	oldPath := fl.String("old", DefaultDB, "database currently in service")
	newPath := fl.String("new", DefaultDB+".new", "freshly built database")
	maxShrink := fl.Float64("max-shrink", 0.02, "largest allowed drop in any table's row count (fraction)")
	if _, err := parseArgs(fl, args); err != nil {
		return err
	}
	cur, err := openExisting(*newPath)
	if err != nil {
		return err
	}
	defer cur.Close()
	if err := refresh.QuickCheck(ctx, cur); err != nil {
		return fmt.Errorf("%s: %w", *newPath, err)
	}
	newSnap, err := refresh.Take(ctx, cur)
	if err != nil {
		return fmt.Errorf("%s: %w", *newPath, err)
	}

	logf := logger(env)
	var oldSnap *refresh.Snapshot
	old, err := openExisting(*oldPath)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		logf("no current database at %s; first build, skipping comparisons", *oldPath)
	case err != nil:
		return err
	default:
		defer old.Close()
		s, err := refresh.Take(ctx, old)
		if err != nil {
			return fmt.Errorf("%s: %w", *oldPath, err)
		}
		oldSnap = &s
	}

	fmt.Fprint(env.Stdout, refresh.Summary(oldSnap, newSnap))
	if oldSnap != nil {
		sc, err := refresh.CertifiedSince(ctx, old, cur, newSnap.LatestYear)
		if err != nil {
			return fmt.Errorf("stage change: %w", err)
		}
		fmt.Fprintln(env.Stdout, sc)
	}
	if problems := refresh.Check(oldSnap, newSnap, *maxShrink); len(problems) > 0 {
		for _, p := range problems {
			logf("FAIL %s", p)
		}
		return fmt.Errorf("%d validation problem(s); not swapping", len(problems))
	}
	fmt.Fprintln(env.Stdout, "validation: ok")
	return nil
}

// dbFinalize checkpoints the WAL and switches to rollback journaling, so the
// file can be renamed into place without -wal/-shm companions.
func dbFinalize(ctx context.Context, env *Env, args []string) error {
	fl := newFlags(env, "db finalize", "-db NEW.db")
	path := fl.String("db", DefaultDB+".new", "database to finalize")
	if _, err := parseArgs(fl, args); err != nil {
		return err
	}
	if _, err := os.Stat(*path); err != nil {
		return err
	}
	db, err := store.Open(*path)
	if err != nil {
		return err
	}
	if err := refresh.Finalize(ctx, db); err != nil {
		db.Close()
		return err
	}
	return db.Close()
}

// openExisting opens a database read-only, failing (with fs.ErrNotExist)
// when the file is missing instead of creating an empty one.
func openExisting(path string) (*sql.DB, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	return store.OpenReadOnly(path)
}
