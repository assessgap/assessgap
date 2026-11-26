package main

import (
	"encoding/json"
	"fmt"
	"math"
	"runtime"
	"sort"
	"sync"

	"github.com/assessgap/assessgap/internal/comps"
)

// Trial is one candidate rule: overrides on rule25c plus the
// statistic that turns a comp set into a score.
type Trial struct {
	Name string `json:"name"`
	// Stat: "median" (the report's gap from the median, the default),
	// "mean", "trim20" (mean without the top and bottom 20%), "wmedian"
	// (similarity-weighted median, weight 1/(1+distance)) or "pctile" (the
	// subject's percentile rank among its comps).
	Stat    string          `json:"stat,omitempty"`
	SFTols  []float64       `json:"sf_tols,omitempty"`  // per search step, overrides Steps[i].SFTolerance
	AgeTols []int           `json:"age_tols,omitempty"` // per search step, overrides Steps[i].AgeTol
	Params  json.RawMessage `json:"params,omitempty"`   // comps.Params fields to override
}

// rule25c is the rule every trial was measured against: the engine's
// defaults from 2026-11-25c until this run changed them. Trials override it,
// not comps.DefaultParams, so the logged configs and the holdout keep their
// meaning after the engine moved to finalist 1.
var rule25c = func() comps.Params {
	p := comps.DefaultParams
	p.Steps = []comps.Search{
		{Level: 0, SameClass: true, SFTolerance: 0.25, AgeTol: 15},
		{Level: 1, SameClass: false, SFTolerance: 0.25, AgeTol: 15},
		{Level: 2, SameClass: false, SFTolerance: 0.35, AgeTol: 25},
		{Level: 3, SameClass: false, SFTolerance: 0.50, AgeTol: 40},
	}
	p.MinComps, p.MaxComps = 8, 20
	p.WSF, p.WAge, p.AgeScale, p.WBath, p.WLand = 4, 1, 15, 0.5, 0.5
	p.WTypeResd, p.WExtWall, p.WBasement, p.WClass = 0.5, 0.5, 0.5, 0.25
	p.WMiles, p.CapMiles = 1, 1
	p.WAir, p.WBsmtFin, p.WGarage, p.MaxMiles = 0, 0, 0, 0
	p.SameTypeResd, p.SameBasement = false, false
	return p
}()

func (t Trial) params() (comps.Params, error) {
	p := rule25c
	p.Steps = append([]comps.Search(nil), p.Steps...) // never write through to the base rule
	if len(t.Params) > 0 {
		if err := json.Unmarshal(t.Params, &p); err != nil {
			return p, fmt.Errorf("%s: params: %w", t.Name, err)
		}
	}
	if t.SFTols != nil && len(t.SFTols) != len(p.Steps) || t.AgeTols != nil && len(t.AgeTols) != len(p.Steps) {
		return p, fmt.Errorf("%s: need one tolerance per search step (%d)", t.Name, len(p.Steps))
	}
	for i := range p.Steps {
		if t.SFTols != nil {
			p.Steps[i].SFTolerance = t.SFTols[i]
		}
		if t.AgeTols != nil {
			p.Steps[i].AgeTol = t.AgeTols[i]
		}
	}
	return p, nil
}

func (t Trial) stat() string {
	if t.Stat == "" {
		return "median"
	}
	return t.Stat
}

func (t Trial) isGap() bool { return t.stat() != "pctile" && t.stat() != "pctile_gap" }

// Scored is one appeal's result under a trial.
type Scored struct {
	OK    bool    // supported subject
	N     int     // comps found
	Score float64 // the trial's statistic
	Gap   float64 // the report's gap from the median, for tiers and the logistic models
	Flags int     // data checks a report would show (bit 0 bldg_sf, 1 beds, 2 fbath, 3 yrblt, 4 land)
	Level int     // search step used
}

var flagBits = map[string]int{"bldg_sf": 1, "beds": 2, "fbath": 4, "yrblt": 8, "land": 16}

// score runs a trial on every appeal, in parallel across the CPU cores.
func score(d *Dataset, t Trial, withFlags bool) ([]Scored, error) {
	p, err := t.params()
	if err != nil {
		return nil, err
	}
	out := make([]Scored, len(d.Appeals))
	idx := make(chan int, 1024)
	var wg sync.WaitGroup
	for w := 0; w < runtime.NumCPU(); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range idx {
				out[i] = scoreOne(d, d.Appeals[i], p, t.stat(), withFlags)
			}
		}()
	}
	for i := range d.Appeals {
		idx <- i
	}
	close(idx)
	wg.Wait()
	return out, nil
}

func scoreOne(d *Dataset, a Appeal, p comps.Params, stat string, withFlags bool) Scored {
	if a.Subj < 0 {
		return Scored{}
	}
	pool := d.pool(a)
	s := pool[a.Subj]
	cs, search := p.SelectComps(s, pool)
	st := comps.ComputeStats(s, cs)
	r := Scored{OK: true, N: len(cs), Gap: st.PctAboveMedian, Level: search.Level}
	if withFlags {
		for _, f := range comps.DataFlags(s, pool, cs) {
			r.Flags |= flagBits[f.Field]
		}
	}
	if len(cs) == 0 {
		return r
	}
	vals := make([]float64, len(cs))
	for i, c := range cs {
		vals[i] = c.BldgAVPerSF
	}
	switch stat {
	case "median":
		r.Score = st.PctAboveMedian
	case "pctile":
		r.Score = st.SubjectPercentile
	case "pctile_gap":
		// The percentile first; the gap only breaks ties (it moves the score
		// by under 0.001, less than one percentile step).
		r.Score = st.SubjectPercentile + 0.0005*math.Max(-1, math.Min(1, st.PctAboveMedian))
	case "mean":
		r.Score = rel(s.BldgAVPerSF, mean(vals))
	case "trim20":
		sort.Float64s(vals)
		k := len(vals) / 5
		r.Score = rel(s.BldgAVPerSF, mean(vals[k:len(vals)-k]))
	case "wmedian":
		w := make([]float64, len(cs))
		for i, c := range cs {
			w[i] = 1 / (1 + c.Distance)
		}
		r.Score = rel(s.BldgAVPerSF, weightedMedian(vals, w))
	default:
		panic("unknown stat " + stat) // validated in main before any run
	}
	return r
}

var stats = map[string]bool{"median": true, "mean": true, "trim20": true, "wmedian": true, "pctile": true, "pctile_gap": true}

// rel is the subject's rate relative to a center, rounded like the report's gap.
func rel(subject, center float64) float64 {
	if center <= 0 {
		return 0
	}
	return math.Round((subject/center-1)*1e4) / 1e4
}

func mean(xs []float64) float64 {
	t := 0.0
	for _, x := range xs {
		t += x
	}
	return t / float64(len(xs))
}

// weightedMedian is the smallest value whose cumulative weight reaches half.
func weightedMedian(vals, w []float64) float64 {
	ix := make([]int, len(vals))
	total := 0.0
	for i := range ix {
		ix[i] = i
		total += w[i]
	}
	sort.Slice(ix, func(a, b int) bool { return vals[ix[a]] < vals[ix[b]] })
	cum := 0.0
	for _, i := range ix {
		cum += w[i]
		if cum >= total/2 {
			return vals[i]
		}
	}
	return vals[ix[len(ix)-1]]
}
