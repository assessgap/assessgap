package web

import (
	"fmt"
	"time"

	"github.com/assessgap/assessgap/internal/bor"
	"github.com/assessgap/assessgap/internal/format"
	"github.com/assessgap/assessgap/internal/web/views"
)

// The filing window as the website words it. bor decides the state; the CLI
// (cli/format.go) and the PDF (packet's windowStep) word it their own way.

// window describes the township's filing window for the report banner.
func window(township string, now time.Time) views.Window {
	if township == "" {
		return views.Window{}
	}
	st := bor.StatusOf(township, now)
	day := func(t time.Time) string { return t.Format("January 2") }
	t := township + " Township"
	w := views.Window{State: st.State.Slug()}
	switch st.State {
	case bor.Open:
		w.Text = fmt.Sprintf("%s is open for Board of Review appeals. File by %s (%s).", t, day(st.Group.Close), format.DaysLeft(st.DaysLeft))
		w.LinkText, w.LinkURL = "Go to the Board's filing portal", bor.PortalURL
	case bor.Upcoming:
		w.Text = fmt.Sprintf("%s opens for Board of Review appeals on %s and closes on %s.", t, day(st.Group.Open), day(st.Group.Close))
		w.LinkText, w.LinkURL = "Board of Review dates", bor.DatesURL
	case bor.Closed:
		w.Text = fmt.Sprintf("%s's %d Board of Review window closed on %s. Leave your email below and we'll tell you when the next one opens.", t, bor.SessionYear, day(st.Group.Close))
		w.LinkText, w.LinkURL = "Board of Review dates", bor.DatesURL
	default:
		w.Text = fmt.Sprintf("The Board of Review hasn't posted %s's dates yet. You can pre-register on the Board's portal now, and we'll email you when it opens if you leave your email below.", t)
		w.LinkText, w.LinkURL = "Pre-register with the Board", bor.PortalURL
	}
	return w
}

// townshipWindow formats bor.StatusOf for the township pages.
func townshipWindow(township string, now time.Time) views.TownshipWindow {
	st := bor.StatusOf(township, now)
	w := views.TownshipWindow{State: st.State.Slug()}
	if g := st.Group; g != nil {
		w.Group, w.Open, w.Close, w.EvidenceDue = g.Name, format.LongDate(g.Open), format.LongDate(g.Close), format.LongDate(g.EvidenceDue)
	}
	switch st.State {
	case bor.Open:
		w.Label = "Open now"
		w.DaysLeft = format.DaysLeft(st.DaysLeft)
	case bor.Upcoming:
		w.Label = "Opening soon"
	case bor.Closed:
		w.Label = fmt.Sprintf("Closed for the %d session", bor.SessionYear)
	default:
		w.Label = "Dates not posted yet"
	}
	return w
}
