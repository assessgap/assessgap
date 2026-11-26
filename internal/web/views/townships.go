package views

// TownshipWindow is a township's Board of Review filing window, formatted.
type TownshipWindow struct {
	State       string // open | upcoming | closed | unscheduled
	Label       string // "Open now", "Opening soon", ...
	Group       string // the Board's group number, "" when not posted
	Open, Close string // "November 9, 2026", "" when not posted
	EvidenceDue string
	DaysLeft    string // "14 days left" or "last day"; only when open
}

// TownshipAppeals is one filer type's Board of Review outcomes in a township.
type TownshipAppeals struct {
	Appeals, Reduced, AvgCut string // "1,234", "41%", "9%"
	Years                    string // "2023–2025"
}

// TownshipData is everything a township page shows.
type TownshipData struct {
	Name, Slug      string
	Window          TownshipWindow
	ScheduleAsOf    string // when the schedule was last checked against the Board's site
	Homes           string // supported homes in AssessGap's copy of the data
	ProSe, Attorney *TownshipAppeals
}

// TownshipLink is one township in the /townships list.
type TownshipLink struct {
	Name, Slug, Dates string
}

// TownshipGroup is one status section of the /townships list.
type TownshipGroup struct {
	State, Heading string
	Townships      []TownshipLink
}

// TownshipIndex is the /townships page.
type TownshipIndex struct {
	Groups       []TownshipGroup
	ScheduleAsOf string
}
