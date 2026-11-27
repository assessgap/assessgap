// Package format writes the figures and names a homeowner reads the same way
// on every surface: the report page, the evidence report PDF and the CLI.
//
// It is a leaf: it imports no other package of this module, so the engine's
// thresholds (what counts as "about equal") are passed in by the caller. What
// a surface shows for an unknown value ("?", "–" or nothing) stays with that
// surface, and is passed in where a function needs one.
package format

import (
	"fmt"
	"math"
	"strings"

	"github.com/dustin/go-humanize"
)

// Money is a dollar amount rounded to the dollar: "$12,345".
func Money(v float64) string { return "$" + humanize.Comma(int64(math.Round(v))) }

// Num is a count or area rounded to a whole number: "1,234".
func Num(v float64) string { return humanize.Comma(int64(math.Round(v))) }

// PerSF is a rate per square foot, to the cent: "$12.34".
func PerSF(v float64) string { return fmt.Sprintf("$%.2f", v) }

// Pct is a fraction as a whole percent: 0.123 is "12%".
func Pct(v float64) string { return fmt.Sprintf("%.0f%%", 100*v) }

// Gap is a rate's position against the median: "about equal to" when inLine,
// otherwise "12% above" or "12% below".
func Gap(g float64, inLine bool) string {
	switch {
	case inLine:
		return "about equal to"
	case g > 0:
		return Pct(g) + " above"
	}
	return Pct(-g) + " below"
}

// Baths is a bathroom count, with a half bath as ".5": "2" or "1.5".
func Baths(b float64) string {
	if b == math.Trunc(b) {
		return fmt.Sprintf("%.0f", b)
	}
	return fmt.Sprintf("%.1f", b)
}

// Miles is a distance from the subject: "<0.1 mi" or "0.4 mi", and unknown
// when the distance isn't known (negative).
func Miles(mi float64, unknown string) string {
	switch {
	case mi < 0:
		return unknown
	case mi < 0.1:
		return "<0.1 mi"
	}
	return fmt.Sprintf("%.1f mi", mi)
}

// TitleCase renders "5555 W BELMONT AVE" as "5555 W Belmont Ave".
func TitleCase(s string) string {
	ws := strings.Fields(strings.ToLower(s))
	for i, w := range ws {
		if len(w) == 1 && strings.Contains("nsew", w) {
			ws[i] = strings.ToUpper(w)
			continue
		}
		ws[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(ws, " ")
}

// PIN renders a 14-digit PIN as 13-28-100-027-0000, and anything else as is.
func PIN(p string) string {
	if len(p) != 14 {
		return p
	}
	return p[0:2] + "-" + p[2:4] + "-" + p[4:7] + "-" + p[7:10] + "-" + p[10:14]
}
