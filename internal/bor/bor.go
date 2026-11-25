// Package bor holds Cook County Board of Review facts: the township filing
// calendar and official links. Sources and check dates are in
// docs/bor-filing-facts.md. Update Schedule when the Board posts new groups.
package bor

import (
	"strings"
	"time"
)

const (
	PortalURL     = "https://appeals.cookcountyboardofreview.com/"
	DatesURL      = "https://www.cookcountyboardofreview.com/dates-and-deadlines"
	DecisionsURL  = "https://apps.cookcountyil.gov/BORDecisionSearch/"
	ScheduleAsOf  = "2026-11-24"
	SessionYear   = 2026
	AppealType    = "Property Over-Assessed"
	PTABDays      = 30
	ReReviewDays  = 3
	DecisionWeeks = "8–16"
)

// Group is one block of townships that open and close together.
type Group struct {
	Name        string
	Townships   []string
	Open, Close time.Time // filing window, inclusive dates
	EvidenceDue time.Time
}

var chicago = mustLoc("America/Chicago")

func mustLoc(name string) *time.Location {
	l, err := time.LoadLocation(name)
	if err != nil {
		return time.FixedZone("CST", -6*3600)
	}
	return l
}

func d(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02", s, chicago)
	if err != nil {
		panic(err)
	}
	return t
}

// Schedule is the 2026 session as posted in 2026TOWNSHIPOPEN-CLOSE.pdf.
var Schedule = []Group{
	{"1", []string{"Evanston", "New Trier", "Norwood Park", "Oak Park", "River Forest", "Riverside", "Rogers Park"},
		d("2026-08-03"), d("2026-09-01"), d("2026-09-11")},
	{"2", []string{"Berwyn", "Cicero", "Elk Grove", "Lake View", "Maine", "Palos"},
		d("2026-08-31"), d("2026-09-29"), d("2026-10-09")},
	{"3", []string{"Lyons", "Northfield", "Stickney", "West Chicago"},
		d("2026-10-08"), d("2026-11-06"), d("2026-11-16")},
	{"4", []string{"Barrington", "Bremen", "Calumet", "Lemont", "Hyde Park"},
		d("2026-11-09"), d("2026-12-08"), d("2026-12-18")},
}

// Townships are Cook County's 38 assessment townships, spelled as the
// Assessor's parcel data spells them (the city's are "North Chicago",
// "South Chicago" and so on).
var Townships = []string{
	"Barrington", "Berwyn", "Bloom", "Bremen", "Calumet", "Cicero", "Elk Grove", "Evanston",
	"Hanover", "Hyde Park", "Jefferson", "Lake", "Lake View", "Lemont", "Leyden", "Lyons",
	"Maine", "New Trier", "Niles", "North Chicago", "Northfield", "Norwood Park", "Oak Park", "Orland",
	"Palatine", "Palos", "Proviso", "Rich", "River Forest", "Riverside", "Rogers Park", "Schaumburg",
	"South Chicago", "Stickney", "Thornton", "West Chicago", "Wheeling", "Worth",
}

// LookupTownship matches a typed name ("lakeview", "new trier") to one of
// Townships, ignoring case, spaces and a trailing "Township".
func LookupTownship(name string) (string, bool) {
	key := func(s string) string {
		s = strings.ToLower(strings.TrimSpace(s))
		s = strings.TrimSuffix(s, "township")
		return strings.ReplaceAll(s, " ", "")
	}
	k := key(name)
	for _, t := range Townships {
		if key(t) == k {
			return t, true
		}
	}
	return "", false
}

// State of a township's filing window.
type State int

const (
	Unscheduled State = iota // dates not posted yet: owners can pre-register
	Upcoming
	Open
	Closed
)

// Status is a township's window relative to a moment in time.
type Status struct {
	Township string
	State    State
	Group    *Group
	DaysLeft int // Open: days until close, counting today
}

// StatusOf reports where township t's window stands at now.
func StatusOf(t string, now time.Time) Status {
	s := Status{Township: t}
	now = now.In(chicago)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, chicago)
	for i := range Schedule {
		g := &Schedule[i]
		for _, name := range g.Townships {
			if !strings.EqualFold(name, t) {
				continue
			}
			s.Group = g
			switch {
			case today.Before(g.Open):
				s.State = Upcoming
			case !today.After(g.Close):
				s.State = Open
				s.DaysLeft = int(g.Close.Sub(today).Hours()/24) + 1
			default:
				s.State = Closed
			}
			return s
		}
	}
	return s
}
