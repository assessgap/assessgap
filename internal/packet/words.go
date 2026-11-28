package packet

import (
	"fmt"

	"github.com/assessgap/assessgap/internal/comps"
	"github.com/assessgap/assessgap/internal/format"
)

// Figures use internal/format, as the web page and the CLI do. What is here
// is the PDF's own wording and placeholders.

func gapPhrase(g float64) string { return format.Gap(g, comps.InLine(g)) }

// milesText is the distance from the subject; "–" when unknown (and for the
// subject itself).
func milesText(mi float64) string { return format.Miles(mi, "–") }

// dash shows a characteristic that isn't on record as "–".
func dash(s string) string {
	if s == "" {
		return "–"
	}
	return s
}

func landSF(v float64) string {
	if v <= 0 {
		return "–"
	}
	return format.Num(v)
}

func plural(n, noun string) string {
	if n == "1" {
		return n + " " + noun
	}
	return n + " " + noun + "s"
}

func yearOr(y int) string {
	if y <= 0 {
		return "unknown"
	}
	return fmt.Sprint(y)
}

func ordinal(n int) string {
	suffix := "th"
	switch {
	case n%100 >= 11 && n%100 <= 13:
	case n%10 == 1:
		suffix = "st"
	case n%10 == 2:
		suffix = "nd"
	case n%10 == 3:
		suffix = "rd"
	}
	return fmt.Sprintf("%d%s", n, suffix)
}
