package cli

import (
	"fmt"
	"time"

	"github.com/assessgap/assessgap/internal/bor"
	"github.com/assessgap/assessgap/internal/comps"
	"github.com/assessgap/assessgap/internal/format"
)

// gapPhrase states the subject's position against the median as a fact.
func gapPhrase(g float64) string { return format.Gap(g, comps.InLine(g)) + " the median" }

// milesText is a comp's distance from the subject, "?" when unknown.
func milesText(mi float64) string { return format.Miles(mi, "?") }

func yearOr(y int) string {
	if y <= 0 {
		return "?"
	}
	return fmt.Sprint(y)
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
