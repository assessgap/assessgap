// Command build turns raw_* tables in data/cook.db into typed, indexed tables
// (parcel, assessment, res_char, condo_char, sale, bor_appeal, ccao_appeal, nbhd_bor_stats).
package main

import (
	_ "embed"
	"flag"
	"log"
	"strings"
	"time"

	"github.com/assessgap/assessgap/internal/store"
)

//go:embed build.sql
var buildSQL string

func main() {
	dbPath := flag.String("db", "data/cook.db", "database")
	flag.Parse()
	db, err := store.Open(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Execute statement-by-statement so failures point at the right step.
	for _, stmt := range splitSQL(buildSQL) {
		t := time.Now()
		if _, err := db.Exec(stmt); err != nil {
			log.Fatalf("%v\n--- statement ---\n%s", err, stmt)
		}
		if d := time.Since(t); d > time.Second {
			log.Printf("%-60.60s %s", firstLine(stmt), d.Round(time.Millisecond))
		}
	}
	for _, t := range []string{"parcel", "assessment", "res_char", "condo_char", "sale", "bor_appeal", "ccao_appeal", "nbhd_bor_stats"} {
		var n int
		db.QueryRow("SELECT COUNT(*) FROM " + t).Scan(&n)
		log.Printf("%-16s %d rows", t, n)
	}
}

// splitSQL splits on semicolons at line ends, dropping comment-only chunks.
// (build.sql has no semicolons inside string literals.)
func splitSQL(s string) []string {
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
