package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Finalist is a rule (Trial) or a logistic model fitted on the tuning years
// (Model, one of modelFeatures' names), chosen before the holdout is opened.
type Finalist struct {
	Name  string `json:"name"`
	Why   string `json:"why"`
	Trial *Trial `json:"trial,omitempty"`
	Model string `json:"model,omitempty"`
}

const bootReps = 2000

// runHoldout scores the current rule and each finalist on the held-out year,
// once: a finalist already in the holdout log is refused.
//
// With dry set, the tuning years stand in for the holdout: it exercises the
// whole command without opening 2025 and writes nothing to the holdout log.
func runHoldout(cache, finalistsPath, outDir string, dry bool) error {
	b, err := os.ReadFile(finalistsPath)
	if err != nil {
		return err
	}
	var fs []Finalist
	if err := json.Unmarshal(b, &fs); err != nil {
		return err
	}
	if len(fs) == 0 || len(fs) > 3 {
		return fmt.Errorf("need 1–3 finalists, got %d", len(fs))
	}
	logPath := filepath.Join(outDir, "holdout-log.tsv")
	done := map[string]bool{}
	if lb, err := os.ReadFile(logPath); err == nil {
		for _, line := range strings.Split(string(lb), "\n") {
			if c := strings.Split(line, "\t"); len(c) > 1 {
				done[c[1]] = true
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, f := range fs {
		if done[f.Name] {
			return fmt.Errorf("finalist %q was already evaluated on the holdout (%s)", f.Name, logPath)
		}
		if (f.Trial == nil) == (f.Model == "") {
			return fmt.Errorf("finalist %q: set exactly one of trial and model", f.Name)
		}
	}

	tune, err := loadTuning(cache)
	if err != nil {
		return err
	}
	hold := tune
	if dry {
		outDir = filepath.Join(cache, "dry")
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			return err
		}
	} else {
		if hold, err = load(filepath.Join(cache, "holdout.gob")); err != nil {
			return err
		}
		lf, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		for _, f := range fs {
			fmt.Fprintf(lf, "%s\t%s\topened 2025 holdout\n", time.Now().UTC().Format(time.RFC3339), f.Name)
		}
		if err := lf.Close(); err != nil {
			return err
		}
	}

	cur := Trial{Name: "current"}
	needFlags := false
	for _, f := range fs {
		needFlags = needFlags || f.Model != ""
	}
	tuneBase, err := score(tune, cur, needFlags)
	if err != nil {
		return err
	}
	holdBase, err := score(hold, cur, needFlags)
	if err != nil {
		return err
	}
	pop := population(holdBase)

	type method struct {
		name   string
		tierOf func(float64) int // nil: no tiers (the logistic model)
		bands  []string
		vals   []float64 // per appeal in pop
		tune   tierTable // pro se tiers on the tuning years, same method
	}
	curTune := tierTable{}
	for _, i := range population(tuneBase) {
		if a := tune.Appeals[i]; a.Pro {
			addTier(&curTune, tierOf(value(cur, tuneBase[i])), a.Won)
		}
	}
	methods := []method{{name: "current", tierOf: tierOf, bands: tierNames, vals: values(cur, holdBase, pop), tune: curTune}}
	for _, f := range fs {
		m := method{name: f.Name}
		if f.Trial != nil {
			sc, err := score(hold, *f.Trial, false)
			if err != nil {
				return err
			}
			m.tierOf, m.bands = tierOf, tierNames
			if !f.Trial.isGap() {
				m.tierOf, m.bands = pctTierOf, pctTierNames
			}
			m.vals = values(*f.Trial, sc, pop)
			ts, err := score(tune, *f.Trial, false)
			if err != nil {
				return err
			}
			for _, i := range population(tuneBase) {
				if a := tune.Appeals[i]; a.Pro {
					addTier(&m.tune, m.tierOf(value(*f.Trial, ts[i])), a.Won)
				}
			}
		} else {
			// Fitted on every tuning-year pro se appeal, applied to 2025 as is.
			np := globalPrior(tune)
			tp := proPopulation(tune, tuneBase)
			X := make([][]float64, len(tp))
			y := make([]bool, len(tp))
			for k, i := range tp {
				X[k] = modelFeatures(f.Model, tune.Appeals[i], tuneBase[i], np)
				y[k] = tune.Appeals[i].Won
			}
			lm := fitLogit(X, y)
			// The same shrink target as in the fit, so the feature means the same thing.
			m.vals = make([]float64, len(pop))
			for k, i := range pop {
				m.vals[k] = lm.predict(modelFeatures(f.Model, hold.Appeals[i], holdBase[i], np))
			}
		}
		methods = append(methods, m)
	}

	// Cluster bootstrap by neighborhood: appeals in one neighborhood share
	// comps and a hearing officer's habits, so they aren't independent.
	clusterOf := make([]int, len(pop))
	cid := map[string]int{}
	for k, i := range pop {
		nb := hold.Appeals[i].Nbhd
		if _, ok := cid[nb]; !ok {
			cid[nb] = len(cid)
		}
		clusterOf[k] = cid[nb]
	}
	nClusters := len(cid)
	rng := rand.New(rand.NewPCG(2025, 1126))
	weights := make([][]float64, bootReps)
	for r := range weights {
		w := make([]float64, nClusters)
		for c := 0; c < nClusters; c++ {
			w[rng.IntN(nClusters)]++
		}
		weights[r] = w
	}

	var out strings.Builder
	w := io.MultiWriter(&out, os.Stdout)
	fmt.Fprintf(w, "2025 holdout: %d appeals scored by the current rule with 3+ comps (%d pro se), %d neighborhoods; bootstrap %d reps by neighborhood\n\n",
		len(pop), countPro(hold, pop, true), nClusters, bootReps)

	for _, filer := range []struct {
		name string
		pro  *bool
	}{{"pro se", ptr(true)}, {"attorney", ptr(false)}, {"all", nil}} {
		var sub []int // positions in pop
		for k, i := range pop {
			if filer.pro == nil || hold.Appeals[i].Pro == *filer.pro {
				sub = append(sub, k)
			}
		}
		boots := make([][]float64, len(methods))
		point := make([]float64, len(methods))
		for mi, m := range methods {
			xs := make([]bobs, len(sub))
			for j, k := range sub {
				xs[j] = bobs{m.vals[k], hold.Appeals[pop[k]].Won, clusterOf[k]}
			}
			sort.Slice(xs, func(a, b int) bool { return xs[a].s < xs[b].s })
			point[mi] = aucBoot(xs, nil)
			boots[mi] = make([]float64, bootReps)
			for r := range boots[mi] {
				boots[mi][r] = aucBoot(xs, weights[r])
			}
		}
		fmt.Fprintf(w, "**AUC, %s (n %d)**\n\n| method | AUC | 95%% CI | Δ vs current | 95%% CI of Δ |\n|---|--:|--:|--:|--:|\n", filer.name, len(sub))
		for mi, m := range methods {
			lo, hi := ci(boots[mi])
			if mi == 0 {
				fmt.Fprintf(w, "| %s | %.4f | %.4f – %.4f | | |\n", m.name, point[mi], lo, hi)
				continue
			}
			diff := make([]float64, bootReps)
			for r := range diff {
				diff[r] = boots[mi][r] - boots[0][r]
			}
			dlo, dhi := ci(diff)
			fmt.Fprintf(w, "| %s | %.4f | %.4f – %.4f | %+.4f | %+.4f – %+.4f |\n", m.name, point[mi], lo, hi, point[mi]-point[0], dlo, dhi)
		}
		fmt.Fprintln(w)
	}

	fmt.Fprintln(w, "**Other measures (pro se)**")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| method | top 25% by score: win rate (share) | Spearman, score vs % cut among winners | tier calibration error (2023–24 rates quoted for 2025) |")
	fmt.Fprintln(w, "|---|--:|--:|--:|")
	for _, m := range methods {
		var pro []obs
		var ws, wc []float64
		var tiers tierTable
		for k, i := range pop {
			a := hold.Appeals[i]
			if !a.Pro {
				continue
			}
			pro = append(pro, obs{m.vals[k], a.Won, 1})
			if a.Won && !math.IsNaN(a.Cut) {
				ws = append(ws, m.vals[k])
				wc = append(wc, a.Cut)
			}
			if m.tierOf != nil {
				addTier(&tiers, m.tierOf(m.vals[k]), a.Won)
			}
		}
		top, share := topShareWin(pro, 0.25)
		cal := "-"
		if m.tierOf != nil {
			cal = fmt.Sprintf("%.4f", calibErr(m.tune, tiers))
		}
		fmt.Fprintf(w, "| %s | %.1f%% (%.1f%%) | %.3f | %s |\n", m.name, 100*top, 100*share, spearman(ws, wc), cal)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "**Tier win rates, pro se: 2023–24 (tuning) and 2025, share of appeals / reduction rate**")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| method | years | band 1 | band 2 | band 3 | band 4 |")
	fmt.Fprintln(w, "|---|---|--:|--:|--:|--:|")
	for _, m := range methods {
		if m.tierOf == nil {
			continue
		}
		var tiers, att tierTable
		for k, i := range pop {
			if hold.Appeals[i].Pro {
				addTier(&tiers, m.tierOf(m.vals[k]), hold.Appeals[i].Won)
			} else {
				addTier(&att, m.tierOf(m.vals[k]), hold.Appeals[i].Won)
			}
		}
		fmt.Fprintf(w, "| %s | bands | %s |\n", m.name, strings.Join(m.bands, " | "))
		fmt.Fprintf(w, "| %s | 2023–24 pro se | %s |\n", m.name, tierCells(m.tune))
		fmt.Fprintf(w, "| %s | 2025 pro se | %s |\n", m.name, tierCells(tiers))
		fmt.Fprintf(w, "| %s | 2025 attorney | %s |\n", m.name, tierCells(att))
	}

	if err := os.WriteFile(filepath.Join(outDir, "holdout.md"), []byte(out.String()), 0o644); err != nil {
		return err
	}
	var names []string
	var cols [][]float64
	for _, m := range methods {
		names = append(names, m.name)
		cols = append(cols, m.vals)
	}
	return writeHoldoutTSV(filepath.Join(outDirOrCache(cache, outDir, dry), "holdout-2025.tsv"), hold, pop, names, cols)
}

// The per-appeal file names PINs, so it stays in the git-ignored cache.
func outDirOrCache(cache, outDir string, dry bool) string {
	if dry {
		return outDir
	}
	return cache
}

// writeHoldoutTSV writes one line per 2025 appeal with every method's score,
// for an independent recomputation. It stays in the git-ignored cache.
func writeHoldoutTSV(path string, d *Dataset, pop []int, names []string, cols [][]float64) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	bw := bufio.NewWriter(f)
	head := append([]string{"pin", "year", "nbhd", "pro_se", "won", "cut"}, names...)
	fmt.Fprintln(bw, strings.Join(head, "\t"))
	for k, i := range pop {
		a := d.Appeals[i]
		cut := ""
		if !math.IsNaN(a.Cut) {
			cut = fmt.Sprintf("%.6f", a.Cut)
		}
		row := []string{a.PIN, fmt.Sprint(a.Year), a.Nbhd, fmt.Sprint(b2i(a.Pro)), fmt.Sprint(b2i(a.Won)), cut}
		for mi := range cols {
			// Full precision: the percentile scores' tie-break is below 0.001.
			row = append(row, strconv.FormatFloat(cols[mi][k], 'g', -1, 64))
		}
		fmt.Fprintln(bw, strings.Join(row, "\t"))
	}
	if err := bw.Flush(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func ptr[T any](v T) *T { return &v }

func countPro(d *Dataset, pop []int, pro bool) int {
	n := 0
	for _, i := range pop {
		if d.Appeals[i].Pro == pro {
			n++
		}
	}
	return n
}

func values(t Trial, sc []Scored, pop []int) []float64 {
	v := make([]float64, len(pop))
	for k, i := range pop {
		v[k] = value(t, sc[i])
	}
	return v
}

func addTier(t *tierTable, k int, won bool) {
	t[k].n++
	if won {
		t[k].wins++
	}
}

func tierCells(t tierTable) string {
	var c []string
	for i := range t {
		c = append(c, fmt.Sprintf("%.1f%% / %.1f%%", 100*t.share(i), 100*t.rate(i)))
	}
	return strings.Join(c, " | ")
}

// bobs is an observation with its bootstrap cluster.
type bobs struct {
	s       float64
	y       bool
	cluster int
}

// aucBoot is aucSorted with each observation weighted by its cluster's draw
// count (nil weights = 1 each). xs must be sorted by score.
func aucBoot(xs []bobs, w []float64) float64 {
	o := make([]obs, len(xs))
	for i, x := range xs {
		wt := 1.0
		if w != nil {
			wt = w[x.cluster]
		}
		o[i] = obs{x.s, x.y, wt}
	}
	return aucSorted(o)
}

func ci(xs []float64) (lo, hi float64) {
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	return s[int(0.025*float64(len(s)))], s[int(math.Ceil(0.975*float64(len(s))))-1]
}
