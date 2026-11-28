package cli

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/assessgap/assessgap/internal/comps"
)

// assessgap backtest: the comparison's tiers and groups against actual Board
// of Review outcomes. docs/methodology.md quotes its tables.

// One row per PIN and tax year: residential home appeals with a clear result.
// If a PIN has several appeals in a year (rare), any Decrease counts as a win,
// and it counts as pro se if any filing was.
var backtestSQL = `
SELECT pin, tax_year,
       MAX(result = 'Decrease')                        AS won,
       MAX(pro_se)                                     AS pro_se,
       MAX(CASE WHEN result = 'Decrease' AND av_before > 0
                THEN (av_before - av_after) / av_before END) AS cut_pct
FROM bor_appeal
WHERE tax_year IN (%s)
  AND substr(class, 1, 3) IN (` + placeholders(len(comps.ScopeClasses())) + `)
  AND result IN ('Decrease', 'No Change')
GROUP BY pin, tax_year
ORDER BY tax_year, pin`

type outcome struct {
	n, wins int
	cutSum  float64
}

func (o *outcome) add(won bool, cut float64) {
	o.n++
	if won {
		o.wins++
		o.cutSum += cut
	}
}

func (o outcome) String() string {
	if o.n == 0 {
		return "0 | - | -"
	}
	avgCut := "-"
	if o.wins > 0 {
		avgCut = fmt.Sprintf("%.1f%%", 100*o.cutSum/float64(o.wins))
	}
	return fmt.Sprintf("%d | %.1f%% | %s", o.n, pctOf(o.wins, o.n), avgCut)
}

type appeal struct {
	pin      string
	year     int
	won, pro bool
	cut      sql.NullFloat64
}

// loadAppeals reads the in-scope Board of Review appeals for the tax years
// before any reports run, so the per-appeal queries get the connection.
func loadAppeals(ctx context.Context, db *sql.DB, years []string) ([]appeal, error) {
	q := fmt.Sprintf(backtestSQL, placeholders(len(years)))
	rows, err := db.QueryContext(ctx, q, append(anys(years), anys(comps.ScopeClasses())...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var appeals []appeal
	for rows.Next() {
		var a appeal
		if err := rows.Scan(&a.pin, &a.year, &a.won, &a.pro, &a.cut); err != nil {
			return nil, err
		}
		appeals = append(appeals, a)
	}
	return appeals, rows.Err()
}

func evalBacktest(ctx context.Context, w io.Writer, logf func(string, ...any), db *sql.DB, e *comps.Engine, yearList string, perAppeal io.Writer) error {
	ys := strings.Split(yearList, ",")
	for _, y := range ys {
		if _, err := strconv.Atoi(y); err != nil {
			return &exitError{2, fmt.Sprintf("-years: %q is not a year", y)}
		}
	}
	appeals, err := loadAppeals(ctx, db, ys)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "in-scope BOR appeals (PIN-years) %s: %d\n", yearList, len(appeals))

	t := newBacktestTally()
	if perAppeal != nil {
		fmt.Fprintln(perAppeal, "pin\tyear\tpro_se\twon\tnum_comps\tgap\tverdict\tcomps_lower\tpercentile")
	}
	start := time.Now()
	for i, a := range appeals {
		if i > 0 && i%20000 == 0 {
			logf("  %d/%d (%s)", i, len(appeals), time.Since(start).Round(time.Second))
		}
		r, err := e.Report(ctx, a.pin, a.year)
		if err != nil {
			t.reportErrors++
			continue
		}
		if r.Unsupported != "" {
			t.skipped[firstSentence(r.Unsupported)]++
			continue
		}
		t.add(a, r)
		if perAppeal != nil {
			st := r.Stats
			fmt.Fprintf(perAppeal, "%s\t%d\t%t\t%t\t%d\t%.4f\t%s\t%d\t%.1f\n", a.pin, a.year, a.pro, a.won, st.NumComps, st.PctAboveMedian, r.Verdict,
				st.CompsLower, st.SubjectPercentile)
		}
	}
	fmt.Fprintf(w, "scored in %s; errors %d; skipped (unsupported) %d\n",
		time.Since(start).Round(time.Second), t.reportErrors, sum(t.skipped))
	for _, k := range sortedKeys(t.skipped) {
		fmt.Fprintf(w, "  %6d  %s\n", t.skipped[k], k)
	}
	t.print(w, ys)
	return nil
}

// backtestKey is one row of the per-year verdict tables; year 0 is all years.
type backtestKey struct {
	year    int
	verdict string
	proSe   bool
}

// backtestTally collects the backtest's tables as appeals are scored.
type backtestTally struct {
	byKey        map[backtestKey]*outcome
	byGap        map[string]map[bool]*outcome
	byTier       map[string]map[bool]*outcome
	byGroup      map[string]map[bool]*outcome
	gaps         map[bool][]scored
	ranks        map[bool][]scored // percentile among comps, the gap breaking ties
	skipped      map[string]int
	reportErrors int
}

func newBacktestTally() *backtestTally {
	return &backtestTally{
		byKey:   map[backtestKey]*outcome{},
		byGap:   map[string]map[bool]*outcome{},
		byTier:  map[string]map[bool]*outcome{},
		byGroup: map[string]map[bool]*outcome{},
		gaps:    map[bool][]scored{},
		ranks:   map[bool][]scored{},
		skipped: map[string]int{},
	}
}

// add counts one scored appeal in every table.
func (t *backtestTally) add(a appeal, r *comps.Report) {
	for _, yr := range []int{a.year, 0} { // 0 = all years
		k := backtestKey{yr, r.Verdict, a.pro}
		if t.byKey[k] == nil {
			t.byKey[k] = &outcome{}
		}
		t.byKey[k].add(a.won, a.cut.Float64)
	}
	st := r.Stats
	bucket(t.byGap, gapBucket(st.PctAboveMedian, st.NumComps))[a.pro].add(a.won, a.cut.Float64)
	bucket(t.byTier, tierBucket(st.PctAboveMedian, st.NumComps))[a.pro].add(a.won, a.cut.Float64)
	bucket(t.byGroup, groupBucket(*st))[a.pro].add(a.won, a.cut.Float64)
	if st.NumComps >= 3 {
		t.gaps[a.pro] = append(t.gaps[a.pro], scored{st.PctAboveMedian, a.won})
		t.ranks[a.pro] = append(t.ranks[a.pro], scored{comps.RankScore(*st), a.won})
	}
}

// bucket is m[k], created with a pro se and an attorney outcome if missing.
func bucket(m map[string]map[bool]*outcome, k string) map[bool]*outcome {
	if m[k] == nil {
		m[k] = map[bool]*outcome{false: {}, true: {}}
	}
	return m[k]
}

// print writes the tables as Markdown, for pasting into docs.
func (t *backtestTally) print(w io.Writer, ys []string) {
	for _, p := range append([]string{"all"}, ys...) {
		yr := 0
		if p != "all" {
			yr, _ = strconv.Atoi(p) // validated by evalBacktest
		}
		fmt.Fprintf(w, "\n**Tax year %s**\n\n", p)
		printOutcomeHeader(w, "verdict")
		for _, v := range verdicts {
			printOutcomeRow(w, v, deref(t.byKey[backtestKey{yr, v, true}]), deref(t.byKey[backtestKey{yr, v, false}]))
		}
	}
	fmt.Fprintf(w, "\n**By subject building AV/sf vs. comp median (all years)**\n\n")
	printOutcomeHeader(w, "gap")
	for _, g := range gapOrder {
		if m := t.byGap[g]; m != nil {
			printOutcomeRow(w, g, *m[true], *m[false])
		}
	}
	fmt.Fprintf(w, "\n**By the gap cut points the reports used until 2026-11-26 (all years)**\n\n")
	printOutcomeHeader(w, "gap")
	for _, g := range tierOrder {
		if m := t.byTier[g]; m != nil {
			printOutcomeRow(w, g, *m[true], *m[false])
		}
	}
	fmt.Fprintf(w, "\n**By the report's groups: gap from the median, then share of similar homes assessed lower (all years)**\n\n")
	printOutcomeHeader(w, "group")
	for _, g := range groupOrder {
		if m := t.byGroup[g]; m != nil {
			printOutcomeRow(w, g, *m[true], *m[false])
		}
	}
	all := append(append([]scored(nil), t.gaps[true]...), t.gaps[false]...)
	allRanks := append(append([]scored(nil), t.ranks[true]...), t.ranks[false]...)
	fmt.Fprintf(w, "\n**How well each score separates reductions from no change (AUC, 3+ comps)**\n\n")
	fmt.Fprintf(w, "Rank = the percentile among the comps, the gap breaking ties (docs/dev/2026-11-26-tune.md).\n\n")
	fmt.Fprintf(w, "| filer | n | AUC of the gap | AUC of the rank |\n|---|--:|--:|--:|\n")
	fmt.Fprintf(w, "| pro se | %d | %.4f | %.4f |\n", len(t.gaps[true]), auc(t.gaps[true]), auc(t.ranks[true]))
	fmt.Fprintf(w, "| attorney | %d | %.4f | %.4f |\n", len(t.gaps[false]), auc(t.gaps[false]), auc(t.ranks[false]))
	fmt.Fprintf(w, "| all | %d | %.4f | %.4f |\n", len(all), auc(all), auc(allRanks))
}

func printOutcomeHeader(w io.Writer, first string) {
	fmt.Fprintf(w, "| %s | pro se n | win | avg cut | attorney n | win | avg cut | all n | win | avg cut |\n", first)
	fmt.Fprintln(w, "|---|--:|--:|--:|--:|--:|--:|--:|--:|--:|")
}

func printOutcomeRow(w io.Writer, label string, pro, att outcome) {
	all := outcome{n: pro.n + att.n, wins: pro.wins + att.wins, cutSum: pro.cutSum + att.cutSum}
	fmt.Fprintf(w, "| %s | %s | %s | %s |\n", label, pro, att, all)
}

func deref(o *outcome) outcome {
	if o == nil {
		return outcome{}
	}
	return *o
}
