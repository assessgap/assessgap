package comps

// Residential classes the engine supports: single-family, townhome and
// split-level homes. Condos (299) and multi-family (211/212) come later.
//
// Groups are used when a neighborhood has too few same-class comps. They
// follow the Assessor's own split: one-story, two-or-more-story, townhome.
var classGroup = map[string]string{
	"202": "one-story", // < 1,000 sf
	"203": "one-story", // 1,000–1,800 sf
	"204": "one-story", // > 1,800 sf
	"234": "one-story", // split level (closest to one-story in practice)

	"205": "multi-story", // 62+ yrs, ≤ 2,200 sf
	"206": "multi-story", // 62+ yrs, 2,201–4,999 sf
	"207": "multi-story", // ≤ 62 yrs, ≤ 2,000 sf
	"208": "multi-story", // ≤ 62 yrs, 3,801–4,999 sf
	"209": "multi-story", // 5,000+ sf
	"278": "multi-story", // ≤ 62 yrs, 2,001–3,800 sf

	"210": "townhome", // townhome / row house
	"295": "townhome", // individually owned townhome / row house
}

// InScope reports whether a class is supported.
func InScope(class string) bool { _, ok := classGroup[class]; return ok }

// ScopeClasses lists the supported classes (for SQL IN clauses).
func ScopeClasses() []string {
	out := make([]string, 0, len(classGroup))
	for c := range classGroup {
		out = append(out, c)
	}
	return out
}

func unsupportedReason(class string) string {
	switch {
	case class == "299":
		return "Condominium units (class 299) are not supported yet."
	case class == "211" || class == "212":
		return "Multi-family buildings (class " + class + ") are not supported yet."
	case class == "":
		return "No property class on record for this PIN."
	case class[0] != '2':
		return "Class " + class + " is not residential; only homes (classes 202–210, 234, 278, 295) are supported."
	default:
		return "Class " + class + " is not supported; only homes (classes 202–210, 234, 278, 295) are supported."
	}
}
