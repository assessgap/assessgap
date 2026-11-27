package cli

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"reflect"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/assessgap/assessgap/internal/comps"
	"github.com/assessgap/assessgap/internal/live"
)

// runLiveEval compares live-mode reports with reports from a full local
// database for a stratified random sample of supported homes.
func runLiveEval(ctx context.Context, env *Env, args []string) error {
	fl := newFlags(env, "live-eval", "[-db full.db] [-n 200] [-seed 1]")
	dbFlag := fl.String("db", "", "full county database (default $ASSESSGAP_DB, else "+DefaultDB+")")
	n := fl.Int("n", 200, "homes to check")
	seed := fl.Uint64("seed", 1, "random seed")
	if _, err := parseArgs(fl, args); err != nil {
		return err
	}
	if *n < 1 { // nothing to compare; the summary would print NaN
		return &exitError{code: 2, msg: "-n must be at least 1"}
	}
	db, err := openCounty(dbPath(env, *dbFlag))
	if err != nil {
		return err
	}
	defer db.Close()
	w := env.Stdout
	logf := logger(env)

	pins, err := stratifiedPINs(ctx, db, *seed)
	if err != nil {
		return err
	}
	var retries atomic.Int64
	f := newFetcher(env)
	f.Log = func(format string, a ...any) {
		if strings.Contains(format, "retry") {
			retries.Add(1)
			logf(format, a...)
		}
	}
	local := comps.New(db)

	var (
		results  []liveResult
		skipped  int
		failures []string
	)
	for _, pin := range pins {
		if len(results) >= *n || ctx.Err() != nil {
			break
		}
		lr, err := local.Report(ctx, pin, 0)
		if err != nil || lr.Unsupported != "" {
			skipped++
			continue
		}
		res, err := f.Fetch(ctx, pin, 0)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", pin, err))
			logf("%s: %v", pin, err)
			continue
		}
		rr, err := comps.New(res.DB).Report(ctx, pin, 0)
		out := liveResult{pin: pin, township: lr.Township, stats: res.Stats}
		if err != nil {
			out.diffs = []string{"live report error: " + err.Error()}
		} else {
			out.diffs = compareReports(lr, rr)
			if len(out.diffs) > 0 {
				out.cause = explain(ctx, db, res.DB, lr)
			}
		}
		res.Close()
		results = append(results, out)
		logf("%3d %s %-14s %2d calls %6.2f MB %5.1fs %s", len(results), pin, lr.Township, res.Stats.Calls,
			float64(res.Stats.Bytes)/1e6, res.Stats.Duration.Seconds(), strings.Join(out.diffs, "; "))
	}
	printLiveEval(w, results, failures, skipped, retries.Load())
	return nil
}

type liveResult struct {
	pin, township string
	stats         live.Stats
	diffs         []string
	cause         string
}

// stratifiedPINs returns in-scope PINs shuffled within each township and
// interleaved round-robin across townships.
func stratifiedPINs(ctx context.Context, db *sql.DB, seed uint64) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT pin, COALESCE(township_name, '') FROM parcel
		WHERE class IN (`+placeholders(len(comps.ScopeClasses()))+`) ORDER BY pin`, anys(comps.ScopeClasses())...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	by := map[string][]string{}
	for rows.Next() {
		var p, t string
		if err := rows.Scan(&p, &t); err != nil {
			return nil, err
		}
		by[t] = append(by[t], p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rng := rand.New(rand.NewPCG(seed, seed))
	towns := sortedKeys(by)
	rng.Shuffle(len(towns), func(i, j int) { towns[i], towns[j] = towns[j], towns[i] })
	for _, t := range towns {
		ps := by[t]
		rng.Shuffle(len(ps), func(i, j int) { ps[i], ps[j] = ps[j], ps[i] })
	}
	var out []string
	for i := 0; ; i++ {
		added := false
		for _, t := range towns {
			if i < len(by[t]) {
				out = append(out, by[t][i])
				added = true
			}
		}
		if !added {
			return out, nil
		}
	}
}

func compPINs(cs []comps.Comp) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.PIN
	}
	return out
}

// compareReports lists what differs between the local and live reports.
func compareReports(a, b *comps.Report) []string {
	var d []string
	if a.Year != b.Year || a.AVStage != b.AVStage {
		d = append(d, fmt.Sprintf("year/stage %d %s vs %d %s", a.Year, a.AVStage, b.Year, b.AVStage))
	}
	if b.Subject == nil || !reflect.DeepEqual(*a.Subject, *b.Subject) {
		d = append(d, "subject")
	}
	if b.Stats == nil || *a.Stats != *b.Stats {
		d = append(d, "stats")
	}
	if !reflect.DeepEqual(compPINs(a.Comps), compPINs(b.Comps)) {
		d = append(d, "comp PINs")
	} else if !reflect.DeepEqual(a.Comps, b.Comps) {
		d = append(d, "comp details")
	}
	if !reflect.DeepEqual(compPINs(a.Evidence), compPINs(b.Evidence)) {
		d = append(d, "lower-assessed PINs")
	}
	if !reflect.DeepEqual(a.NbhdAppeals, b.NbhdAppeals) {
		d = append(d, "neighborhood BOR stats")
	}
	if a.Verdict != b.Verdict {
		d = append(d, "tier")
	}
	if !reflect.DeepEqual(a.Sales, b.Sales) {
		d = append(d, "sales")
	}
	return d
}

// explain compares the comparison inputs (the neighborhood's assessed values
// and characteristics, and its appeal records) in the two databases.
func explain(ctx context.Context, local, remote *sql.DB, r *comps.Report) string {
	var parts []string
	q := `SELECT a.pin, a.class || '|' || COALESCE(a.mailed, '') || '|' || COALESCE(a.certified, '') ||
		'|' || COALESCE(c.bldg_sf, '') || '|' || COALESCE(c.yrblt, '') || '|' || COALESCE(c.num_cards, '') ||
		'|' || COALESCE(c.fbath, '') || '|' || COALESCE(c.land_sf, '')
		FROM assessment a LEFT JOIN res_char c ON c.pin = a.pin WHERE a.year = ? AND a.nbhd = ?`
	l, err1 := keyed(ctx, local, q, r.Year, r.Nbhd)
	m, err2 := keyed(ctx, remote, q, r.Year, r.Nbhd)
	if err1 != nil || err2 != nil {
		return fmt.Sprintf("compare failed: %v %v", err1, err2)
	}
	changed, onlyL, onlyR := 0, 0, 0
	for k, v := range l {
		if w, ok := m[k]; !ok {
			onlyL++
		} else if w != v {
			changed++
		}
	}
	for k := range m {
		if _, ok := l[k]; !ok {
			onlyR++
		}
	}
	if changed+onlyL+onlyR > 0 {
		parts = append(parts, fmt.Sprintf("neighborhood inputs differ: %d homes changed, %d only local, %d only in API", changed, onlyL, onlyR))
	}
	bq := `SELECT b.pin || '|' || b.tax_year, b.result || '|' || b.pro_se || '|' || COALESCE(b.av_after, '')
		FROM bor_appeal b JOIN parcel p ON p.pin = b.pin WHERE p.nbhd = ?`
	bl, err1 := keyed(ctx, local, bq, r.Nbhd)
	br, err2 := keyed(ctx, remote, bq, r.Nbhd)
	if err1 == nil && err2 == nil && !reflect.DeepEqual(bl, br) {
		parts = append(parts, fmt.Sprintf("appeal records differ (%d local, %d API)", len(bl), len(br)))
	}
	var ln, rn int
	errL := local.QueryRowContext(ctx, `SELECT COUNT(*) FROM parcel WHERE nbhd = ?`, r.Nbhd).Scan(&ln)
	errR := remote.QueryRowContext(ctx, `SELECT COUNT(*) FROM parcel WHERE nbhd = ?`, r.Nbhd).Scan(&rn)
	if errL == nil && errR == nil && ln != rn {
		parts = append(parts, fmt.Sprintf("neighborhood parcels %d local vs %d API", ln, rn))
	}
	if len(parts) == 0 {
		return "inputs identical (code difference)"
	}
	return strings.Join(parts, "; ")
}

func keyed(ctx context.Context, db *sql.DB, q string, args ...any) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v sql.NullString
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k.String] = v.String
	}
	return out, rows.Err()
}

func printLiveEval(w io.Writer, rs []liveResult, failures []string, skipped int, retries int64) {
	var calls, mb, secs []float64
	match, coreMatch := 0, 0
	towns := map[string]bool{}
	fieldCount := map[string]int{}
	for _, r := range rs {
		towns[r.township] = true
		calls = append(calls, float64(r.stats.Calls))
		mb = append(mb, float64(r.stats.Bytes)/1e6)
		secs = append(secs, r.stats.Duration.Seconds())
		if len(r.diffs) == 0 {
			match++
		}
		core := true
		for _, d := range r.diffs {
			fieldCount[strings.SplitN(d, " ", 2)[0]]++
			if d == "subject" || d == "stats" || d == "comp PINs" || strings.HasPrefix(d, "year/stage") {
				core = false
			}
		}
		if core {
			coreMatch++
		}
	}
	n := len(rs)
	fmt.Fprintf(w, "homes checked: %d in %d townships (skipped %d unsupported locally; %d live failures; %d HTTP retries)\n",
		n, len(towns), skipped, len(failures), retries)
	fmt.Fprintf(w, "identical reports: %d (%.1f%%)\n", match, pctOf(match, n))
	fmt.Fprintf(w, "identical subject, statistics and comparable PIN set: %d (%.1f%%)\n", coreMatch, pctOf(coreMatch, n))
	fmt.Fprintf(w, "API calls per check: p50 %.0f, p95 %.0f, max %.0f\n", quantile(calls, .5), quantile(calls, .95), quantile(calls, 1))
	fmt.Fprintf(w, "MB per check:        p50 %.2f, p95 %.2f, max %.2f\n", quantile(mb, .5), quantile(mb, .95), quantile(mb, 1))
	fmt.Fprintf(w, "seconds per check:   p50 %.1f, p95 %.1f, max %.1f\n", quantile(secs, .5), quantile(secs, .95), quantile(secs, 1))
	fmt.Fprintln(w, "differences by field:")
	for _, k := range sortedKeys(fieldCount) {
		fmt.Fprintf(w, "  %-12s %d\n", k, fieldCount[k])
	}
	fmt.Fprintln(w, "mismatches:")
	for _, r := range rs {
		if len(r.diffs) > 0 {
			fmt.Fprintf(w, "  %s (%s): %s. Cause: %s\n", r.pin, r.township, strings.Join(r.diffs, ", "), r.cause)
		}
	}
	for _, f := range failures {
		fmt.Fprintf(w, "  failure: %s\n", f)
	}
}

// quantile is the nearest-rank quantile of xs (q in [0,1]).
func quantile(xs []float64, q float64) float64 {
	if len(xs) == 0 {
		return math.NaN()
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	i := int(math.Ceil(q*float64(len(s)))) - 1
	return s[max(0, min(i, len(s)-1))]
}
