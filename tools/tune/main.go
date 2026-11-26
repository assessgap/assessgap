// Command tune measures candidate comparison rules against Board of Review
// outcomes, with an honest time split: rules are tuned on tax years
// 2023–2024 and checked once on 2025, which the search commands never open.
//
//	go run ./tools/tune prep    -db data/cook-c.db -cache data/tune        # cache appeals and pools (2 files)
//	go run ./tools/tune verify  -cache data/tune -tsv data/new-appeals.tsv # default rule = engine backtest
//	go run ./tools/tune trial   -cache data/tune -log trials.tsv -kind T trials.jsonl ... # one JSON Trial per line
//	go run ./tools/tune signals -cache data/tune -log trials.tsv           # context signals, grouped CV
//	go run ./tools/tune holdout -cache data/tune -finalists finalists.json # 2025, once per finalist
//
// See docs/dev/2026-11-26-tune.md.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

func main() {
	log.SetFlags(0)
	if len(os.Args) < 2 {
		log.Fatal("usage: tune prep|verify|trial|signals|holdout [flags]")
	}
	fl := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	cache := fl.String("cache", "data/tune", "cache directory (git-ignored)")
	dbPath := fl.String("db", "data/cook-c.db", "county database, opened read-only")
	logPath := fl.String("log", "docs/dev/tune-2026-11-26/trials.tsv", "trial log (appended)")
	kind := fl.String("kind", "select", "trial kind for the log")
	tsv := fl.String("tsv", "", "verify: the engine backtest's -tsv output")
	finalists := fl.String("finalists", "docs/dev/tune-2026-11-26/finalists.json", "holdout: finalists file")
	out := fl.String("out", "docs/dev/tune-2026-11-26", "holdout: output directory")
	dry := fl.Bool("dry", false, "holdout: run on the tuning years instead, to test the command without opening 2025")
	if err := fl.Parse(os.Args[2:]); err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()
	var err error
	switch os.Args[1] {
	case "prep":
		err = prep(ctx, *dbPath, *cache)
	case "verify":
		err = verify(*cache, *tsv)
	case "trial":
		var specs []string
		specs, err = trialSpecs(fl.Args())
		if err == nil {
			err = runTrials(*cache, *logPath, *kind, specs)
		}
	case "signals":
		err = runSignals(*cache, *logPath)
	case "holdout":
		err = runHoldout(*cache, *finalists, *out, *dry)
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		log.Fatal(err)
	}
}

// trialSpecs reads one JSON trial per non-empty line of each file; lines
// starting with # are comments.
func trialSpecs(files []string) ([]string, error) {
	var specs []string
	for _, p := range files {
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		for _, line := range strings.Split(string(b), "\n") {
			if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
				specs = append(specs, line)
			}
		}
	}
	return specs, nil
}

// verify checks that the default rule, run by this tool, gives exactly the
// engine backtest's comp count and gap for every appeal both cover.
func verify(cache, tsv string) error {
	d, err := loadTuning(cache)
	if err != nil {
		return err
	}
	sc, err := score(d, Trial{Name: "current"}, false)
	if err != nil {
		return err
	}
	f, err := os.Open(tsv)
	if err != nil {
		return err
	}
	defer f.Close()
	want := map[string][2]string{}
	r := bufio.NewScanner(f)
	for r.Scan() {
		c := strings.Split(r.Text(), "\t")
		if len(c) >= 6 && c[0] != "pin" {
			want[c[0]+"/"+c[1]] = [2]string{c[4], c[5]}
		}
	}
	var checked, mismatch, missing, extra int
	for i, a := range d.Appeals {
		w, ok := want[fmt.Sprintf("%s/%d", a.PIN, a.Year)]
		inTSV := ok
		if !sc[i].OK {
			if inTSV {
				extra++
			}
			continue
		}
		if !inTSV {
			if a.Year == 2024 {
				missing++
			}
			continue
		}
		checked++
		if w[0] != strconv.Itoa(sc[i].N) || w[1] != fmt.Sprintf("%.4f", sc[i].Gap) {
			mismatch++
			if mismatch <= 5 {
				log.Printf("mismatch %s/%d: engine n=%s gap=%s, tune n=%d gap=%.4f", a.PIN, a.Year, w[0], w[1], sc[i].N, sc[i].Gap)
			}
		}
	}
	fmt.Printf("checked %d appeals: %d mismatches; supported here but not scored by the engine: %d; scored by the engine but unsupported here: %d\n",
		checked, mismatch, missing, extra)
	if mismatch+missing+extra > 0 {
		return errors.New("verify failed")
	}
	return nil
}

// population is the fixed set every trial is scored on: appeals the current
// rule supports with 3 or more comps (as the backtest's AUC). A trial that
// finds fewer than 3 comps for one gets a neutral score, so dropping hard
// cases can't raise a trial's AUC.
func population(base []Scored) []int {
	var ix []int
	for i, b := range base {
		if b.OK && b.N >= 3 {
			ix = append(ix, i)
		}
	}
	return ix
}

func value(t Trial, s Scored) float64 {
	if !s.OK || s.N < 3 {
		if t.isGap() {
			return 0
		}
		return 50
	}
	return s.Score
}

// folds assigns townships to 5 groups (sorted codes, round robin), for
// cross-validation grouped by place.
func folds(d *Dataset) map[string]int {
	seen := map[string]bool{}
	var towns []string
	for _, a := range d.Appeals {
		if !seen[a.Town] {
			seen[a.Town] = true
			towns = append(towns, a.Town)
		}
	}
	sort.Strings(towns)
	f := map[string]int{}
	for i, t := range towns {
		f[t] = i % 5
	}
	return f
}

const nFolds = 5

// metrics are one trial's measures on a dataset; the deltas are against the
// current rule on the same population.
type metrics struct {
	n                 int
	aucPro, aucAtt    float64
	aucAll            float64
	aucProYear        map[int]float64
	dPro              float64
	dProYear          map[int]float64
	foldsUp           int
	minFoldD          float64
	top25, top25Share float64
	top25Base         float64
	spearman          float64
	calib             float64 // tiers from the first year quoted for the second
	coverage          float64
	tiers             tierTable // pro se, all years
}

func measure(d *Dataset, base, sc []Scored, t Trial) metrics {
	pop := population(base)
	fold := folds(d)
	cur := Trial{Name: "current"}
	var pro, att, all, proBase []obs
	byYear, byYearBase := map[int][]obs{}, map[int][]obs{}
	var byFold, byFoldBase [nFolds][]obs
	var winScore, winCut []float64
	yearTiers := map[int]*tierTable{}
	m := metrics{n: len(pop), aucProYear: map[int]float64{}, dProYear: map[int]float64{}}
	covered := 0
	for _, i := range pop {
		a := d.Appeals[i]
		v, vb := value(t, sc[i]), value(cur, base[i])
		if sc[i].N >= 3 {
			covered++
		}
		o, ob := obs{v, a.Won, 1}, obs{vb, a.Won, 1}
		all = append(all, o)
		if !a.Pro {
			att = append(att, o)
			continue
		}
		pro = append(pro, o)
		proBase = append(proBase, ob)
		byYear[a.Year] = append(byYear[a.Year], o)
		byYearBase[a.Year] = append(byYearBase[a.Year], ob)
		f := fold[a.Town]
		byFold[f] = append(byFold[f], o)
		byFoldBase[f] = append(byFoldBase[f], ob)
		if a.Won && !math.IsNaN(a.Cut) {
			winScore = append(winScore, v)
			winCut = append(winCut, a.Cut)
		}
		if t.isGap() {
			if yearTiers[a.Year] == nil {
				yearTiers[a.Year] = &tierTable{}
			}
			k := tierOf(v)
			for _, tt := range []*tierTable{yearTiers[a.Year], &m.tiers} {
				tt[k].n++
				if a.Won {
					tt[k].wins++
				}
			}
		}
	}
	m.coverage = float64(covered) / float64(len(pop))
	m.aucPro, m.aucAtt, m.aucAll = auc(pro), auc(att), auc(all)
	m.dPro = m.aucPro - auc(proBase)
	for y, xs := range byYear {
		m.aucProYear[y] = auc(xs)
		m.dProYear[y] = m.aucProYear[y] - auc(byYearBase[y])
	}
	m.minFoldD = math.Inf(1)
	for f := 0; f < nFolds; f++ {
		dd := auc(byFold[f]) - auc(byFoldBase[f])
		if dd > 0 {
			m.foldsUp++
		}
		m.minFoldD = math.Min(m.minFoldD, dd)
	}
	m.top25, m.top25Share = topShareWin(pro, 0.25)
	m.top25Base, _ = topShareWin(proBase, 0.25)
	m.spearman = spearman(winScore, winCut)
	m.calib = math.NaN()
	if ys := sortedYears(yearTiers); len(ys) == 2 {
		m.calib = calibErr(*yearTiers[ys[0]], *yearTiers[ys[1]])
	}
	return m
}

func sortedYears[V any](m map[int]V) []int {
	var ys []int
	for y := range m {
		ys = append(ys, y)
	}
	sort.Ints(ys)
	return ys
}

var logHeader = []string{"id", "kind", "name", "config", "n", "coverage",
	"auc_pro", "auc_pro_2023", "auc_pro_2024", "d_auc_pro", "d_2023", "d_2024", "folds_up", "min_fold_d",
	"auc_att", "auc_all", "top25_win_pro", "top25_share", "top25_win_current", "spearman_cut_winners", "calib_err_2023to2024",
	"tiers_pro(share/win)", "seconds"}

// appendLog writes one row per trial, numbering rows after those already there.
func appendLog(path string, row func(id int) []string) error {
	n := 0
	if b, err := os.ReadFile(path); err == nil {
		n = strings.Count(string(b), "\n")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if n == 0 {
		fmt.Fprintln(f, strings.Join(logHeader, "\t"))
		n = 1
	}
	fmt.Fprintln(f, strings.Join(row(n), "\t"))
	return f.Close()
}

func f3(x float64) string {
	if math.IsNaN(x) {
		return "-"
	}
	return strconv.FormatFloat(x, 'f', 4, 64)
}

func tierString(t tierTable) string {
	if t.total() == 0 {
		return "-"
	}
	var parts []string
	for i, name := range tierNames {
		parts = append(parts, fmt.Sprintf("%s %.1f%%/%.1f%%", name, 100*t.share(i), 100*t.rate(i)))
	}
	return strings.Join(parts, "; ")
}

func metricCells(m metrics) []string {
	return []string{strconv.Itoa(m.n), f3(m.coverage),
		f3(m.aucPro), f3(m.aucProYear[2023]), f3(m.aucProYear[2024]), f3(m.dPro), f3(m.dProYear[2023]), f3(m.dProYear[2024]),
		fmt.Sprintf("%d/%d", m.foldsUp, nFolds), f3(m.minFoldD),
		f3(m.aucAtt), f3(m.aucAll), f3(m.top25), f3(m.top25Share), f3(m.top25Base), f3(m.spearman), f3(m.calib),
		tierString(m.tiers)}
}

func runTrials(cache, logPath, kind string, specs []string) error {
	var trials []Trial
	for _, s := range specs {
		var t Trial
		dec := json.NewDecoder(strings.NewReader(s))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&t); err != nil {
			return fmt.Errorf("trial %q: %w", s, err)
		}
		if err := t.validate(); err != nil {
			return err
		}
		trials = append(trials, t)
	}
	d, err := loadTuning(cache)
	if err != nil {
		return err
	}
	base, err := score(d, Trial{Name: "current"}, false)
	if err != nil {
		return err
	}
	for _, t := range trials {
		start := time.Now()
		sc, err := score(d, t, false)
		if err != nil {
			return err
		}
		m := measure(d, base, sc, t)
		secs := time.Since(start).Seconds()
		cfg, _ := json.Marshal(t)
		printTrial(os.Stdout, t.Name, m)
		if err := appendLog(logPath, func(id int) []string {
			return append(append([]string{strconv.Itoa(id), kind, t.Name, string(cfg)}, metricCells(m)...), fmt.Sprintf("%.0f", secs))
		}); err != nil {
			return err
		}
	}
	return nil
}

func printTrial(w io.Writer, name string, m metrics) {
	fmt.Fprintf(w, "%-28s pro %.4f (23 %.4f, 24 %.4f) Δ %+.4f (23 %+.4f, 24 %+.4f) folds %d/5 min %+.4f | att %.4f | top25 %.3f vs %.3f | rho %.3f | calib %s | cov %.4f\n",
		name, m.aucPro, m.aucProYear[2023], m.aucProYear[2024], m.dPro, m.dProYear[2023], m.dProYear[2024],
		m.foldsUp, m.minFoldD, m.aucAtt, m.top25, m.top25Base, m.spearman, f3(m.calib), m.coverage)
}
