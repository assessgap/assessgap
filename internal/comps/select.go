package comps

import (
	"fmt"
	"math"
	"sort"
)

const (
	minComps = 8  // fewer than this triggers widening
	maxComps = 20 // keep the most similar N
)

// searchSteps are tried in order until one yields minComps candidates.
// Each step is a superset of the one before it.
var searchSteps = []Search{
	{Level: 0, SameClass: true, SFTolerance: 0.25, AgeTol: 15},
	{Level: 1, SameClass: false, SFTolerance: 0.25, AgeTol: 15},
	{Level: 2, SameClass: false, SFTolerance: 0.35, AgeTol: 25},
	{Level: 3, SameClass: false, SFTolerance: 0.50, AgeTol: 40},
}

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

// Similarity weights. Distance 1.0 ≈ "noticeably different on one dimension".
const (
	wSF       = 4.0  // per 100% sqft difference (±25% → 1.0)
	wAge      = 1.0  // per 15 years
	wBath     = 0.5  // per bathroom (half bath = 0.5)
	wLand     = 0.5  // per 100% land sqft difference, capped at 1
	wCategory = 0.5  // each mismatch: type_resd, ext_wall, basement
	wClass    = 0.25 // different class within the same group
)

// distance scores how different c is from s; 0 = identical on every measure.
//
// The float64() conversions stop the compiler fusing "d += w * x" into one
// FMA instruction on arm64 but not amd64, which would make the same report
// differ in the last bits (and so in comp order) between a Mac and the server.
func distance(s, c Home) float64 {
	d := wSF * math.Abs(c.BldgSF-s.BldgSF) / s.BldgSF
	if s.YearBuilt > 0 && c.YearBuilt > 0 {
		d += wAge * math.Abs(float64(c.YearBuilt-s.YearBuilt)) / 15
	}
	d += float64(wBath * math.Abs(c.Baths()-s.Baths()))
	if s.LandSF > 0 && c.LandSF > 0 {
		d += float64(wLand * math.Min(1, math.Abs(c.LandSF-s.LandSF)/s.LandSF))
	}
	for _, pair := range [][2]string{{s.TypeResd, c.TypeResd}, {s.ExtWall, c.ExtWall}, {s.Basement, c.Basement}} {
		if pair[0] != pair[1] {
			d += wCategory
		}
	}
	if s.Class != c.Class {
		d += wClass
	}
	return d
}

// usable reports whether a home can serve as a comp at all.
func usable(subject, h Home) bool {
	return h.PIN != subject.PIN && h.NumCards == 1 && h.BldgSF > 0 && h.AV > 0
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

// selectComps widens the search until it has minComps candidates (or runs
// out of steps) and returns the maxComps most similar, nearest first.
func selectComps(subject Home, pool []Home) ([]Comp, Search) {
	var cands []Home
	var used Search
	for _, step := range searchSteps {
		cands = cands[:0]
		for _, h := range pool {
			if usable(subject, h) && step.admits(subject, h) {
				cands = append(cands, h)
			}
		}
		used = step
		if len(cands) >= minComps {
			break
		}
	}
	used.Candidates = len(cands)
	used.Description = used.describe(subject.Class)

	comps := make([]Comp, len(cands))
	for i, h := range cands {
		comps[i] = Comp{Home: h, Distance: round(distance(subject, h), 3)}
	}
	sort.Slice(comps, func(i, j int) bool {
		if comps[i].Distance != comps[j].Distance {
			return comps[i].Distance < comps[j].Distance
		}
		return comps[i].PIN < comps[j].PIN
	})
	if len(comps) > maxComps {
		comps = comps[:maxComps]
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
