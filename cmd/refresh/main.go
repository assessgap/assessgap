// Command refresh holds the database steps of ops/refresh.sh.
//
//	refresh validate -old data/cook.db -new data/cook.db.new
//	    compare row counts and the latest assessment year; exit 1 if the new
//	    database must not replace the old one. A missing -old file means a first
//	    build: only the non-empty rule applies.
//	refresh finalize -db data/cook.db.new
//	    checkpoint the WAL and switch to rollback journaling, so the file can be
//	    renamed into place without -wal/-shm companions.
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"

	"github.com/assessgap/assessgap/internal/refresh"
	"github.com/assessgap/assessgap/internal/store"
)

func main() {
	log.SetFlags(0)
	if len(os.Args) < 2 {
		usage()
	}
	ctx := context.Background()
	var err error
	switch os.Args[1] {
	case "validate":
		err = validate(ctx, os.Args[2:])
	case "finalize":
		err = finalize(ctx, os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		log.Fatalf("refresh %s: %v", os.Args[1], err)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: refresh validate -old OLD.db -new NEW.db [-max-shrink 0.02]\n       refresh finalize -db NEW.db")
	os.Exit(2)
}

func validate(ctx context.Context, args []string) error {
	fl := flag.NewFlagSet("validate", flag.ExitOnError)
	oldPath := fl.String("old", "data/cook.db", "database currently in service")
	newPath := fl.String("new", "data/cook.db.new", "freshly built database")
	maxShrink := fl.Float64("max-shrink", 0.02, "largest allowed drop in any table's row count (fraction)")
	fl.Parse(args)

	cur, err := mustOpen(*newPath)
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

	var oldSnap *refresh.Snapshot
	old, err := mustOpen(*oldPath)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		log.Printf("no current database at %s; first build, skipping comparisons", *oldPath)
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

	fmt.Print(refresh.Summary(oldSnap, newSnap))
	if oldSnap != nil {
		sc, err := refresh.CertifiedSince(ctx, old, cur, newSnap.LatestYear)
		if err != nil {
			return fmt.Errorf("stage change: %w", err)
		}
		fmt.Println(sc)
	}

	if problems := refresh.Check(oldSnap, newSnap, *maxShrink); len(problems) > 0 {
		for _, p := range problems {
			log.Printf("FAIL %s", p)
		}
		return fmt.Errorf("%d validation problem(s); not swapping", len(problems))
	}
	fmt.Println("validation: ok")
	return nil
}

func finalize(ctx context.Context, args []string) error {
	fl := flag.NewFlagSet("finalize", flag.ExitOnError)
	path := fl.String("db", "data/cook.db.new", "database to finalize")
	fl.Parse(args)
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

// mustOpen opens an existing database read-only. Unlike sql.Open it fails
// when the file is missing instead of creating an empty database.
func mustOpen(path string) (*sql.DB, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	return store.OpenReadOnly(path)
}
