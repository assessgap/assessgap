package comps

import (
	"fmt"
	"math"
	"strings"
)

// Plain words for the Assessor's characteristic values.
//
// The residential characteristics dataset (x54s-btds) publishes these as words,
// not the Assessor's numeric codes: "2 Story", "Frame + Masonry",
// "Formal Rec Room", "2 cars", "Central A/C" (value lists in
// docs/data-inventory.md, Part 1). We reword them for homeowners. A value we
// don't recognize is shown as recorded, except a bare number, which would be
// an undecoded code: that is hidden rather than shown.

var (
	storiesWords = map[string]string{
		"1 Story": "1-story", "1.5 Story": "1.5-story", "2 Story": "2-story",
		"3 Story +": "3+ story", "Split Level": "split-level",
	}
	wallWords = map[string]string{
		"Frame": "frame", "Masonry": "masonry", "Frame + Masonry": "frame and masonry", "Stucco": "stucco",
	}
	basementWords = map[string]string{
		"Full": "full basement", "Partial": "partial basement", "Slab": "slab", "Crawl": "crawl space",
	}
	finishWords = map[string]string{
		"Formal Rec Room": "finished", "Unfinished": "unfinished", "Apartment": "apartment",
	}
	airWords = map[string]string{"Central A/C": "yes", "No Central A/C": "no"}
)

func word(m map[string]string, v string) string {
	v = strings.TrimSpace(v)
	if w, ok := m[v]; ok {
		return w
	}
	if v == "" || strings.Trim(v, "0123456789.") == "" {
		return ""
	}
	return strings.ToLower(v)
}

// Stories is the residence type: "2-story", "split-level"; "" if unknown.
func (h Home) Stories() string { return word(storiesWords, h.TypeResd) }

// Construction is the exterior wall: "frame", "masonry"; "" if unknown.
func (h Home) Construction() string { return word(wallWords, h.ExtWall) }

// BasementType is "full basement", "slab", "crawl space"; "" if unknown.
func (h Home) BasementType() string { return word(basementWords, h.Basement) }

// BasementFinish is "finished", "unfinished" or "apartment" for homes with a
// basement; "" otherwise.
func (h Home) BasementFinish() string {
	if b := h.Basement; b != "Full" && b != "Partial" {
		return ""
	}
	return word(finishWords, h.BsmtFin)
}

// CentralAir is "yes", "no", or "" if unknown.
func (h Home) CentralAir() string { return word(airWords, h.Air) }

// GarageCars is the garage size in cars (0 = none), or -1 if unknown.
func (h Home) GarageCars() float64 {
	var n float64
	if _, err := fmt.Sscanf(strings.TrimSpace(h.Garage), "%g cars", &n); err != nil {
		return -1
	}
	return n
}

// GarageText is "none", "2-car", "1.5-car", or "" if unknown.
func (h Home) GarageText() string {
	switch n := h.GarageCars(); {
	case n < 0:
		return ""
	case n == 0:
		return "none"
	default:
		return fmt.Sprintf("%g-car", n)
	}
}

// Features are a home's characteristics as plain words for the evidence grid,
// capitalized; "" means not on record.
type Features struct {
	Stories, Construction, Basement, Air, Fireplaces, Garage string
}

// Features describes h for display.
func (h Home) Features() Features {
	bsmt := h.BasementType()
	if fin := h.BasementFinish(); bsmt != "" && fin != "" {
		bsmt = strings.TrimSuffix(bsmt, " basement") + ", " + fin
	}
	return Features{
		Stories:      capitalize(h.Stories()),
		Construction: capitalize(h.Construction()),
		Basement:     capitalize(bsmt),
		Air:          capitalize(h.CentralAir()),
		Fireplaces:   fireplaceCount(h.Fireplaces),
		Garage:       capitalize(h.GarageText()),
	}
}

func capitalize(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// Thresholds for "Differences from your home", following what Property Tax
// Appeal Board decisions weigh when comparing homes, as reasons for less
// weight or as adjustments (see docs/dev/2026-11-25c.md): living area, age,
// location, design, construction, baths, basement, central air, fireplaces
// and garage. Decisions discounted comps about 6–19% smaller or ~20+ years
// apart when closer matches existed, so smaller gaps than ours are shown.
const (
	diffSizePct = 0.05 // living area differs by 5% or more
	diffYears   = 10   // built 10 or more years apart
)

// Differences lists, as facts, how comp c differs from subject s on the
// features the Board weighs: "0.3 mi away · 1 more full bath · built 12 years
// later · 2-car garage vs none". The comp's value comes first, then the
// subject's. Features unknown for either home are left out.
func Differences(s Home, c Comp) string {
	var out []string
	add := func(f string, a ...any) { out = append(out, fmt.Sprintf(f, a...)) }
	vs := func(theirs, yours, suffix string) {
		if theirs != "" && yours != "" && theirs != yours {
			add("%s%s vs %s", theirs, suffix, yours)
		}
	}

	switch {
	case c.Miles < 0:
	case c.Miles < 0.1:
		add("under 0.1 mi away")
	default:
		add("%.1f mi away", c.Miles)
	}
	if c.Class != s.Class {
		add("class %s vs %s", c.Class, s.Class)
	}
	vs(c.Stories(), s.Stories(), "")
	vs(c.Construction(), s.Construction(), "")
	if s.BldgSF > 0 {
		if d := c.BldgSF/s.BldgSF - 1; math.Abs(d) >= diffSizePct {
			add("%.0f%% %s", math.Abs(100*d), pick(d > 0, "larger", "smaller"))
		}
	}
	if s.YearBuilt > 0 && c.YearBuilt > 0 {
		if d := c.YearBuilt - s.YearBuilt; abs(d) >= diffYears {
			add("built %d years %s", abs(d), pick(d > 0, "later", "earlier"))
		}
	}
	count := func(d int, noun string) {
		if d != 0 {
			add("%d %s %s", abs(d), pick(d > 0, "more", "fewer"), plural(abs(d), noun))
		}
	}
	count(c.FullBaths-s.FullBaths, "full bath")
	count(c.HalfBaths-s.HalfBaths, "half bath")
	vs(c.BasementType(), s.BasementType(), "")
	if c.BasementType() == s.BasementType() {
		vs(c.BasementFinish(), s.BasementFinish(), " basement")
	}
	switch ca, sa := c.CentralAir(), s.CentralAir(); {
	case ca == "yes" && sa == "no":
		add("central air vs none")
	case ca == "no" && sa == "yes":
		add("no central air vs central air")
	}
	if c.Fireplaces >= 0 && s.Fireplaces >= 0 && c.Fireplaces != s.Fireplaces {
		yours := "none"
		if s.Fireplaces > 0 {
			yours = fmt.Sprint(s.Fireplaces)
		}
		add("%s vs %s", fireplaces(c.Fireplaces), yours)
	}
	if cg, sg := c.GarageCars(), s.GarageCars(); cg >= 0 && sg >= 0 && cg != sg {
		theirs := c.GarageText() + " garage"
		if cg == 0 {
			theirs = "no garage"
		}
		add("%s vs %s", theirs, s.GarageText())
	}
	if len(out) == 0 {
		return "No differences in the recorded features we compare"
	}
	return strings.Join(out, " · ")
}

// fireplaceCount is the grid's count; "" when not on record.
func fireplaceCount(n int) string {
	if n < 0 {
		return ""
	}
	return fmt.Sprint(n)
}

func fireplaces(n int) string {
	if n == 0 {
		return "no fireplace"
	}
	return fmt.Sprintf("%d %s", n, plural(n, "fireplace"))
}

func plural(n int, noun string) string {
	if n == 1 {
		return noun
	}
	return noun + "s"
}

func pick(b bool, yes, no string) string {
	if b {
		return yes
	}
	return no
}
