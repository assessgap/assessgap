package ingest

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"strings"
	"time"
)

//go:embed build.sql
var buildSQL string

// Tables are the typed tables Build creates.
var Tables = []string{"parcel", "assessment", "res_char", "condo_char", "sale", "bor_appeal", "ccao_appeal", "nbhd_bor_stats"}

// Build turns the raw_* tables in db into typed, indexed tables. It is
// idempotent: every table is dropped and rebuilt. Statements taking longer
// than a second are logged.
func Build(ctx context.Context, db *sql.DB, log func(string, ...any)) error {
	for _, stmt := range SplitSQL(buildSQL) {
		t := time.Now()
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("%w\n--- statement ---\n%s", err, stmt)
		}
		if d := time.Since(t); d > time.Second {
			log("%-60.60s %s", firstLine(stmt), d.Round(time.Millisecond))
		}
	}
	return nil
}

// Counts returns the row count of each typed table.
func Counts(ctx context.Context, db *sql.DB) (map[string]int, error) {
	out := map[string]int{}
	for _, t := range Tables {
		var n int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+t).Scan(&n); err != nil {
			return nil, err
		}
		out[t] = n
	}
	return out, nil
}

// SplitSQL splits on semicolons at line ends, dropping comment-only chunks.
// (build.sql has no semicolons inside string literals.)
func SplitSQL(s string) []string {
	var out []string
	var cur strings.Builder
	for _, line := range strings.Split(s, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "--") || trim == "" {
			continue
		}
		cur.WriteString(line)
		cur.WriteString("\n")
		if strings.HasSuffix(trim, ";") {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	if strings.TrimSpace(cur.String()) != "" {
		out = append(out, cur.String())
	}
	return out
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
