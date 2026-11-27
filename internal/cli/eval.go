// Evaluation commands: measure the comparison engine on a full database.
//
//	assessgap sample   [-db path] [-n 2000] [-seed 1]   coverage + comparison mix in townships whose BOR dates aren't posted
//	assessgap backtest [-db path] [-years 2024,2025]    comparison tier vs. actual BOR outcome
//
// Output is plain text for pasting into docs. The commands are here; sample.go
// and backtest.go run them, and buckets.go holds the backtest's groupings.

package cli

import (
	"bufio"
	"context"
	"database/sql"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/assessgap/assessgap/internal/bor"
	"github.com/assessgap/assessgap/internal/comps"
)

var verdicts = []string{comps.Strong, comps.Possible, comps.Weak}

// unscheduledTownships are the townships with no Board of Review dates posted
// as of now: the ones still to open this session.
func unscheduledTownships(now time.Time) []string {
	var out []string
	for _, t := range bor.Townships {
		if bor.StatusOf(t, now).State == bor.Unscheduled {
			out = append(out, t)
		}
	}
	return out
}

func runSample(ctx context.Context, env *Env, args []string) error {
	fl := newFlags(env, "sample", "[-db path] [-n 2000] [-seed 1]")
	dbFlag := fl.String("db", "", "full county database (default $ASSESSGAP_DB, else "+DefaultDB+")")
	n := fl.Int("n", 2000, "sample size")
	seed := fl.Uint64("seed", 1, "random seed")
	if _, err := parseArgs(fl, args); err != nil {
		return err
	}
	if *n < 1 { // a negative n would slice pins[:n]
		return &exitError{code: 2, msg: "-n must be at least 1"}
	}
	db, e, err := evalEngine(env, *dbFlag)
	if err != nil {
		return err
	}
	defer db.Close()
	return evalSample(ctx, env.Stdout, logger(env), db, e, *n, *seed)
}

func runBacktest(ctx context.Context, env *Env, args []string) error {
	fl := newFlags(env, "backtest", "[-db path] [-years 2024,2025] [-tsv path]")
	dbFlag := fl.String("db", "", "full county database (default $ASSESSGAP_DB, else "+DefaultDB+")")
	years := fl.String("years", "2024,2025", "Board of Review tax years")
	tsv := fl.String("tsv", "", "also write one line per scored appeal (PIN, year, filer, outcome, gap) to this file")
	if _, err := parseArgs(fl, args); err != nil {
		return err
	}
	db, e, err := evalEngine(env, *dbFlag)
	if err != nil {
		return err
	}
	defer db.Close()
	if *tsv == "" {
		return evalBacktest(ctx, env.Stdout, logger(env), db, e, *years, nil)
	}
	f, err := os.Create(*tsv)
	if err != nil {
		return err
	}
	bw := bufio.NewWriter(f)
	if err := evalBacktest(ctx, env.Stdout, logger(env), db, e, *years, bw); err != nil {
		f.Close()
		return err
	}
	if err := bw.Flush(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// evalEngine opens the database read-only with neighborhood pools cached,
// since evaluations score many homes per neighborhood.
func evalEngine(env *Env, dbFlag string) (*sql.DB, *comps.Engine, error) {
	db, err := openCounty(dbPath(env, dbFlag))
	if err != nil {
		return nil, nil, err
	}
	e := comps.New(db)
	e.CachePools = true
	return db, e, nil
}

func placeholders(n int) string { return strings.TrimSuffix(strings.Repeat("?,", n), ",") }

func anys[T any](xs []T) []any {
	out := make([]any, len(xs))
	for i, x := range xs {
		out[i] = x
	}
	return out
}

func queryStrings(ctx context.Context, db *sql.DB, q string, args ...any) ([]string, error) {
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func firstSentence(s string) string {
	if i := strings.Index(s, ";"); i > 0 {
		return s[:i]
	}
	if i := strings.Index(s, ". "); i > 0 {
		return s[:i]
	}
	return s
}

func sum[K comparable](m map[K]int) int {
	t := 0
	for _, v := range m {
		t += v
	}
	return t
}

func pctOf(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return 100 * float64(a) / float64(b)
}

func sortedKeys[V any](m map[string]V) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
