package comps

import (
	"fmt"
	"math"
	"sort"
)

// Params are the rules for choosing similar homes. Every report uses the
// defaults (DefaultParams returns a copy). Other values exist so the tuning tool (tools/tune) measures
// a candidate rule with the same code a report would run.
type Params struct {
	// Steps are tried in order until one admits MinComps homes. Each must be a
	// superset of the one before it.
	Steps    []Search
	MinComps int // fewer than this triggers widening
	MaxComps int // keep the most similar N

	// Similarity weights. A distance of 1.0 ≈ "noticeably different on one
	// dimension".
	WSF       float64 // per 100% building sqft difference (±25% → 0.25 at the default 1)
	WAge      float64 // per AgeScale years
	AgeScale  float64
	WBath     float64 // per bathroom (half bath = 0.5)
	WLand     float64 // per 100% land sqft difference, capped at 1
	WTypeResd float64 // mismatch in residence type (stories)
	WExtWall  float64 // mismatch in exterior wall
	WBasement float64 // mismatch in basement type
	WClass    float64 // different class within the same group
	// WMiles is per mile between parcel centers, capped at CapMiles (an
	// unknown location counts as the cap).
	WMiles, CapMiles float64

	// Off by default; measured in the 2026-11-26 tuning run.
	WAir, WBsmtFin, WGarage float64 // mismatch penalties
	// MaxMiles, if > 0, admits only homes within that many miles (located
	// homes only) at every search step but the last, so a home always has the
	// widest search to fall back on.
	MaxMiles float64
	// SameTypeResd and SameBasement require a match at every step but the last.
	SameTypeResd, SameBasement bool
}

// defaults are the rules every report uses (docs/methodology.md).
//
// Within the hard filters (same neighborhood and class, size and age bands),
// nearness counts most: 4 per mile, capped at 1 mile, against 1 per 100% size
// difference, and a different basement type costs nothing. With 30 comps, this
// and ranking the subject among its comps (analyze.go) separated Board of
// Review reductions from no change best when tuned on tax years 2023–2024. On
// held-out 2025 pro se appeals it raised the AUC from 0.789 to 0.817 (95% CI
// of the gain +0.025 to +0.030); see docs/dev/2026-11-26-tune.md. The Board
// prefers comps on the same block or within ¼ mile.
var defaults = Params{
	Steps: []Search{
		{Level: 0, SameClass: true, SFTolerance: 0.25, AgeTol: 15},
		{Level: 1, SameClass: false, SFTolerance: 0.25, AgeTol: 15},
		{Level: 2, SameClass: false, SFTolerance: 0.35, AgeTol: 25},
		{Level: 3, SameClass: false, SFTolerance: 0.50, AgeTol: 40},
	},
	MinComps: 8,
	MaxComps: 30,
	WSF:      1.0, WAge: 1.0, AgeScale: 15, WBath: 0.5, WLand: 0.5,
	WTypeResd: 0.5, WExtWall: 0.5, WBasement: 0, WClass: 0.25,
	WMiles: 4.0, CapMiles: 1.0,
}

// DefaultParams returns a copy of the rules every report uses, for a caller
// to vary without touching them.
func DefaultParams() Params {
	p := defaults
	p.Steps = append([]Search(nil), defaults.Steps...)
	return p
}

// minComps is the default widening threshold; decide also uses it as the
// smallest group a top-tier report may rest on.
var minComps = defaults.MinComps

func (s Search) describe(class string) string {
	who := "class " + class
	if !s.SameClass {
		who = classGroup[class] + " homes"
	}
	age := fmt.Sprintf(", built within ±%d years", s.AgeTol)
	if s.AgeTol == 0 {
		age = ""
	}
	return fmt.Sprintf("same neighborhood, %s, building sqft within ±%.0f%%%s", who, 100*s.SFTolerance, age)
}

// distance scores how different c is from s; 0 = identical on every measure.
//
// The float64() conversions stop the compiler fusing "d += w * x" into one
// FMA instruction on arm64 but not amd64, which would make the same report
// differ in the last bits (and so in comp order) between a Mac and the server.
// Terms that are off by default are added only when their weight is set, so
// the default sum is computed exactly as before they existed.
func (p Params) distance(s, c Home) float64 {
	d := p.WSF * math.Abs(c.BldgSF-s.BldgSF) / s.BldgSF
	if s.YearBuilt > 0 && c.YearBuilt > 0 {
		d += p.WAge * math.Abs(float64(c.YearBuilt-s.YearBuilt)) / p.AgeScale
	}
	d += float64(p.WBath * math.Abs(c.Baths()-s.Baths()))
	if s.LandSF > 0 && c.LandSF > 0 {
		d += float64(p.WLand * math.Min(1, math.Abs(c.LandSF-s.LandSF)/s.LandSF))
	}
	if s.TypeResd != c.TypeResd {
		d += p.WTypeResd
	}
	if s.ExtWall != c.ExtWall {
		d += p.WExtWall
	}
	if s.Basement != c.Basement {
		d += p.WBasement
	}
	if s.Class != c.Class {
		d += p.WClass
	}
	// An unknown location counts as far, so it can't jump ahead of near homes.
	mi := miles(s, c)
	if mi < 0 {
		mi = p.CapMiles
	}
	d += float64(p.WMiles * math.Min(p.CapMiles, mi))
	for _, t := range []struct {
		w    float64
		a, b string
	}{{p.WAir, s.Air, c.Air}, {p.WBsmtFin, s.BsmtFin, c.BsmtFin}, {p.WGarage, s.Garage, c.Garage}} {
		if t.w != 0 && t.a != t.b {
			d += t.w
		}
	}
	return d
}

// miles is the great-circle distance between two homes' parcel centers
// (haversine), or -1 when either has no location on record.
func miles(a, b Home) float64 {
	if (a.Lat == 0 && a.Lon == 0) || (b.Lat == 0 && b.Lon == 0) {
		return -1
	}
	const earthMiles = 3958.8
	rad := math.Pi / 180
	dLat := (b.Lat - a.Lat) * rad
	dLon := (b.Lon - a.Lon) * rad
	// float64() on each product keeps arm64 from fusing it into the sum (see distance).
	h := float64(math.Sin(dLat/2)*math.Sin(dLat/2)) +
		float64(math.Cos(a.Lat*rad)*math.Cos(b.Lat*rad)*math.Sin(dLon/2)*math.Sin(dLon/2))
	return 2 * earthMiles * math.Asin(math.Min(1, math.Sqrt(h)))
}

// usable reports whether a home can serve as a comp at all. A home without a
// building/land split has no building rate, so it can't be one.
func usable(subject, h Home) bool {
	return h.PIN != subject.PIN && h.NumCards == 1 && h.BldgSF > 0 && h.AV > 0 && h.HasSplit
}

func (s Search) admits(subject, h Home) bool {
	if s.SameClass && h.Class != subject.Class {
		return false
	}
	if !s.SameClass && classGroup[h.Class] != classGroup[subject.Class] {
		return false
	}
	if math.Abs(h.BldgSF-subject.BldgSF) > s.SFTolerance*subject.BldgSF {
		return false
	}
	// Age is only applied when both years are known.
	if s.AgeTol > 0 && subject.YearBuilt > 0 && h.YearBuilt > 0 &&
		abs(h.YearBuilt-subject.YearBuilt) > s.AgeTol {
		return false
	}
	return true
}

// strictAdmits applies the optional hard rules (off by default), which hold
// at every step but the last.
func (p Params) strictAdmits(subject, h Home) bool {
	if p.SameTypeResd && h.TypeResd != subject.TypeResd {
		return false
	}
	if p.SameBasement && h.Basement != subject.Basement {
		return false
	}
	if p.MaxMiles > 0 {
		if mi := miles(subject, h); mi < 0 || mi > p.MaxMiles {
			return false
		}
	}
	return true
}

func (p Params) hasStrict() bool { return p.SameTypeResd || p.SameBasement || p.MaxMiles > 0 }

// selectComps chooses comps with the rules every report uses.
func selectComps(subject Home, pool []Home) ([]Comp, Search) {
	return defaults.SelectComps(subject, pool)
}

// SelectComps widens the search until it has MinComps candidates (or runs
// out of steps) and returns the MaxComps most similar, nearest first.
func (p Params) SelectComps(subject Home, pool []Home) ([]Comp, Search) {
	var cands []Home
	var used Search
	strict := p.hasStrict()
	for i, step := range p.Steps {
		cands = cands[:0]
		last := i == len(p.Steps)-1
		for _, h := range pool {
			if usable(subject, h) && step.admits(subject, h) && (!strict || last || p.strictAdmits(subject, h)) {
				cands = append(cands, h)
			}
		}
		used = step
		if len(cands) >= p.MinComps {
			break
		}
	}
	used.Candidates = len(cands)
	used.Description = used.describe(subject.Class)

	comps := make([]Comp, len(cands))
	for i, h := range cands {
		mi := miles(subject, h)
		if mi >= 0 {
			mi = round(mi, 2)
		}
		comps[i] = Comp{Home: h, Distance: round(p.distance(subject, h), 3), Miles: mi}
	}
	// Equally similar homes: the closer one first (unknown locations last).
	sort.Slice(comps, func(i, j int) bool {
		a, b := comps[i], comps[j]
		if a.Distance != b.Distance {
			return a.Distance < b.Distance
		}
		if a.Miles != b.Miles {
			if a.Miles < 0 || b.Miles < 0 {
				return b.Miles < 0
			}
			return a.Miles < b.Miles
		}
		return a.PIN < b.PIN
	})
	if len(comps) > p.MaxComps {
		comps = comps[:p.MaxComps]
	}
	return comps, used
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func round(x float64, places int) float64 {
	p := math.Pow(10, float64(places))
	return math.Round(x*p)/p + 0 // + 0 turns -0 into 0 for JSON
}
