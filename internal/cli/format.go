package cli

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/assessgap/assessgap/internal/bor"
	"github.com/assessgap/assessgap/internal/comps"
	"github.com/dustin/go-humanize"
)

func money(v float64) string { return "$" + humanize.Comma(int64(math.Round(v))) }

func num(v float64) string { return humanize.Comma(int64(math.Round(v))) }

func perSF(v float64) string { return fmt.Sprintf("$%.2f", v) }

func pct(v float64) string { return fmt.Sprintf("%.0f%%", 100*v) }

// gapPhrase states the subject's position against the median as a fact.
func gapPhrase(g float64) string {
	switch {
	case comps.InLine(g):
		return "about equal to the median"
	case g > 0:
		return pct(g) + " above the median"
	}
	return pct(-g) + " below the median"
}

// milesText is a comp's distance from the subject, "?" when unknown.
func milesText(mi float64) string {
	switch {
	case mi < 0:
		return "?"
	case mi < 0.1:
		return "<0.1 mi"
	}
	return fmt.Sprintf("%.1f mi", mi)
}

func yearOr(y int) string {
	if y <= 0 {
		return "?"
	}
	return fmt.Sprint(y)
}

func baths(h comps.Home) string {
	b := h.Baths()
	if b == math.Trunc(b) {
		return fmt.Sprintf("%.0f", b)
	}
	return fmt.Sprintf("%.1f", b)
}

// titleCase renders "5555 W BELMONT AVE" as "5555 W Belmont Ave".
func titleCase(s string) string {
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

func day(t time.Time) string { return t.Format("Jan 2, 2006") }

// daysLeft is "N days left", or "last day" when the window closes today.
func daysLeft(st bor.Status) string {
	if st.DaysLeft == 1 {
		return "last day"
	}
	return fmt.Sprintf("%d days left", st.DaysLeft)
}

// windowText is one sentence on a township's Board of Review filing window.
func windowText(township string, now time.Time) string {
	st := bor.StatusOf(township, now)
	switch st.State {
	case bor.Open:
		return fmt.Sprintf("open now. File by %s (%s); evidence due %s. Portal: %s",
			day(st.Group.Close), daysLeft(st), day(st.Group.EvidenceDue), bor.PortalURL)
	case bor.Upcoming:
		return fmt.Sprintf("opens %s, closes %s; evidence due %s. Dates: %s",
			day(st.Group.Open), day(st.Group.Close), day(st.Group.EvidenceDue), bor.DatesURL)
	case bor.Closed:
		return fmt.Sprintf("the %d window closed %s. Dates: %s", bor.SessionYear, day(st.Group.Close), bor.DatesURL)
	}
	return fmt.Sprintf("dates not posted yet (as of %s). You can pre-register at %s", bor.ScheduleAsOf, bor.PortalURL)
}
