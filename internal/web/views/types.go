// Package views holds the site's templ components. Components take plain,
// pre-formatted data; package web does the formatting.
package views

import "fmt"

// Site is shared by every page.
type Site struct {
	Dev          bool
	CSS          string // stylesheet URL with cache-busting hash
	ProSeWinRate float64
	ProSeAppeals int
	FirstYear    int
	LastYear     int
}

func (s Site) WinRate() string { return fmt.Sprintf("%.0f%%", 100*s.ProSeWinRate) }

func (s Site) Years() string { return fmt.Sprintf("%d–%d", s.FirstYear, s.LastYear) }

// Match is a search result.
type Match struct {
	PIN, PINFmt, Address, City, Kind string
}

// ReportData is everything the report page shows.
type ReportData struct {
	PIN, PINFmt, Address, Location string
	Year                           int
	Unsupported                    string
	IsCondo                        bool

	Verdict    string // strong | possible | weak
	Window     Window
	Headline   string
	Badge      string
	Reasons    []string
	StageLabel string
	Mailed     bool

	SubjectAVPerSF, MedianAVPerSF string
	GapLabel                      string // "12% above" / "4% below"
	NumComps                      int
	SearchDesc                    string
	SubjectAV, UniformityAV       string
	Reduction                     string // "" when there is none
	Subject                       HomeRow
	Evidence                      []HomeRow
	Comps                         []HomeRow
	Flags                         []string
	Sales                         []SaleRow
	ProSe, Attorney               *NbhdRow
	SmallSample                   bool
	Chart, ChartNarrow            Chart
	Disclaimer                    string
}

type HomeRow struct {
	Address, PIN, PINFmt                    string
	SqFt, YearBuilt, Beds, Baths, AV, PerSF string
	Evidence                                bool
}

type NbhdRow struct {
	WinRate, AvgCut, Years string
	Appeals                int
}

type SaleRow struct{ Date, Price string }

// ChartLayout sizes a strip chart in viewBox units.
type ChartLayout struct {
	W, H, Left, Right, Base, Top float64
	DotR, RowGap, Font           float64
	Ticks                        int
}

func (l ChartLayout) ViewBox() string { return fmt.Sprintf("0 0 %.0f %.0f", l.W, l.H) }

// Chart is a strip plot of AV per square foot.
type Chart struct {
	L                  ChartLayout
	Dots               []Dot
	SubjectX, MedianX  float64
	Ticks              []Tick
	SubjectLbl, MedLbl string
	MedAnchor          string // text-anchor for the median label
}

type Dot struct {
	X, Y     float64
	Evidence bool
	Title    string
}

type Tick struct {
	X     float64
	Label string
}

func px(v float64) string { return fmt.Sprintf("%.1f", v) }

// AppealsShort renders the self-filed appeal count, e.g. "82k".
func (s Site) AppealsShort() string {
	if s.ProSeAppeals < 1000 {
		return fmt.Sprint(s.ProSeAppeals)
	}
	return fmt.Sprintf("%dk", s.ProSeAppeals/1000)
}

// Window is the township's Board of Review filing window.
type Window struct {
	State    string // open | upcoming | closed | unscheduled
	Text     string
	LinkText string
	LinkURL  string

	TownshipName, TownshipURL string // the township's page on this site, "" if unknown
}
