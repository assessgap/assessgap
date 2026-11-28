package packet

import (
	"fmt"
	"time"

	"github.com/assessgap/assessgap/internal/bor"
	"github.com/assessgap/assessgap/internal/format"
)

// Page 3: the filing steps, from the Board of Review's published rules.

func (p *pager) steps(now time.Time) {
	r := p.r
	p.newPage("Filing steps: Cook County Board of Review")
	p.para(margin, contentW, regular, 9.5,
		"How a homeowner files their own appeal, from the Board of Review's published instructions and rules (checked "+
			bor.ScheduleAsOf+"). Rules and dates change; the Board's website is the final word. This is general "+
			"information, not legal advice. You file, sign and present your own appeal.")
	p.y += 4
	steps := []string{
		windowStep(r.Township, now),
		"Take a color photo of the front of your home. The Board's Rule 14 requires one, taken within the last 12 " +
			"months, in the brief you upload. Mark the date it was taken. Photos of the comparable homes are encouraged.",
		"Check the details on page 2 (square feet, year built, bedrooms, baths) against your home. If the Assessor " +
			"has something wrong, say so in your brief.",
		"Go to " + bor.PortalURL + " and choose Submit an Appeal. You can create an account or continue as a guest " +
			"(guests get the decision by mail only; account holders also get it by email).",
		"Choose \"" + bor.AppealType + "\" as the type of appeal and enter PIN " + format.PIN(r.PIN) + ". When asked if " +
			"you are a Board of Review attorney, answer No. Choose your appellant type (owner).",
		"Decide whether you want a hearing. It is optional, held by phone for homeowners, and the Board says you are " +
			"not penalized for skipping it. You choose this when you file.",
		"Enter comparable properties if you like (optional: the Board's analysts also pull their own). The evidence " +
			"comparables on page 2 are similar homes in your Assessor neighborhood whose building is assessed lower per " +
			"square foot than yours; the other homes listed there are not.",
		"Review, initial, confirm and submit before the deadline. Save your complaint number.",
		"Upload your evidence after submitting: your brief with the photo, and this report if you wish, by the " +
			"evidence deadline for your township.",
		fmt.Sprintf("Watch for the decision, usually %s weeks after your township closes. You can check status at %s. "+
			"A re-review must be requested within %d days of the decision letter. If you disagree with the decision, "+
			"you can appeal to the state Property Tax Appeal Board within %d days, and you can appeal again next year.",
			bor.DecisionWeeks, bor.DecisionsURL, bor.ReReviewDays, bor.PTABDays),
	}
	for i, s := range steps {
		p.checkItem(i+1, s)
	}
	p.y += 8
	p.boxed(NotAffiliated + " " + r.Disclaimer)
}

// windowStep describes the township's filing window at time now.
func windowStep(township string, now time.Time) string {
	st := bor.StatusOf(township, now)
	day := format.LongDate
	switch st.State {
	case bor.Open:
		return fmt.Sprintf("File by %s. %s Township is open for Board of Review appeals now. Evidence is due by %s.",
			day(st.Group.Close), township, day(st.Group.EvidenceDue))
	case bor.Upcoming:
		return fmt.Sprintf("%s Township opens for Board of Review appeals on %s and closes on %s. Evidence is due by %s.",
			township, day(st.Group.Open), day(st.Group.Close), day(st.Group.EvidenceDue))
	case bor.Closed:
		return fmt.Sprintf("%s Township's %d Board of Review window closed on %s. Check %s for next year's dates.",
			township, bor.SessionYear, day(st.Group.Close), bor.DatesURL)
	default:
		return fmt.Sprintf("Check when %s Township opens. The Board hasn't posted its dates yet; each township is open "+
			"about 30 days. You can pre-register on the Board's portal now. Dates: %s", township, bor.DatesURL)
	}
}

func (p *pager) checkItem(n int, s string) {
	const box, indent = 9.0, 38.0
	p.strokeRect(margin, p.y-box+1, box, box, 0.7)
	p.text(margin+box+6, p.y, bold, bodySize, fmt.Sprintf("%d.", n))
	p.para(margin+indent, contentW-indent, regular, bodySize, s)
	p.y += 2
}
