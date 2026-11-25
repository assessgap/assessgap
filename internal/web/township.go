package web

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/assessgap/assessgap/internal/bor"
	"github.com/assessgap/assessgap/internal/comps"
	"github.com/assessgap/assessgap/internal/web/views"
	"github.com/dustin/go-humanize"
)

// townshipSlug is a township's URL path segment: "Lake View" → "lake-view".
func townshipSlug(name string) string {
	return strings.ReplaceAll(strings.ToLower(name), " ", "-")
}

func townshipPath(name string) string { return "/townships/" + townshipSlug(name) }

// resolveTownship maps a path segment to a township. canonical is false when
// the segment is a variant ("lakeview", "Lake_View", "lake-view-township")
// that should redirect to the township's slug.
func resolveTownship(seg string) (name string, canonical, ok bool) {
	for _, t := range bor.Townships {
		if townshipSlug(t) == seg {
			return t, true, true
		}
	}
	typed := strings.NewReplacer("-", " ", "_", " ", "+", " ").Replace(seg)
	name, ok = bor.LookupTownship(typed)
	return name, false, ok
}

// appealTotals sums one filer type's neighborhood appeal statistics.
type appealTotals struct {
	appeals, reduced int
	cutWeighted      float64 // sum of avg_cut_pct × reductions, for a reduction-weighted mean
	cutReduced       int     // reductions in neighborhoods with an avg_cut_pct
	first, last      int
}

// townshipFacts are a township's numbers from the county data, read once at
// startup (the data changes only when the database is rebuilt).
type townshipFacts struct {
	homes            int
	proSe, attorney  appealTotals
	hasProSe, hasAtt bool
}

// loadTownshipFacts aggregates nbhd_bor_stats (the neighborhood appeal
// statistics the reports show) to townships, and counts the homes in the
// classes AssessGap checks. A neighborhood belongs to the township whose
// Assessor township code is its first two digits; the 99999 catch-all
// neighborhood belongs to none.
func loadTownshipFacts(ctx context.Context, db *sql.DB) (map[string]townshipFacts, error) {
	classes := comps.ScopeClasses()
	args := make([]any, len(classes))
	for i, c := range classes {
		args[i] = c
	}
	rows, err := db.QueryContext(ctx, `
		SELECT township_code, township_name, SUM(class IN (?`+strings.Repeat(",?", len(classes)-1)+`))
		FROM parcel WHERE township_code <> '' AND township_name <> ''
		GROUP BY township_code, township_name`, args...)
	if err != nil {
		return nil, fmt.Errorf("township homes: %w", err)
	}
	defer rows.Close()
	facts := map[string]townshipFacts{}
	byCode := map[string]string{}
	for rows.Next() {
		var code, name string
		var homes int
		if err := rows.Scan(&code, &name, &homes); err != nil {
			return nil, err
		}
		byCode[code] = name
		f := facts[name]
		f.homes += homes
		facts[name] = f
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = db.QueryContext(ctx, `
		SELECT substr(nbhd, 1, 2), pro_se, n_appeals, n_decrease, avg_cut_pct, first_year, last_year
		FROM nbhd_bor_stats`)
	if err != nil {
		return nil, fmt.Errorf("township appeals: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var code string
		var proSe bool
		var n, reduced, first, last int
		var cut sql.NullFloat64
		if err := rows.Scan(&code, &proSe, &n, &reduced, &cut, &first, &last); err != nil {
			return nil, err
		}
		name, ok := byCode[code]
		if !ok {
			continue
		}
		f := facts[name]
		tot, has := &f.attorney, &f.hasAtt
		if proSe {
			tot, has = &f.proSe, &f.hasProSe
		}
		if !*has {
			tot.first, tot.last = first, last
		}
		*has = true
		tot.appeals += n
		tot.reduced += reduced
		if cut.Valid {
			tot.cutWeighted += cut.Float64 * float64(reduced)
			tot.cutReduced += reduced
		}
		tot.first, tot.last = min(tot.first, first), max(tot.last, last)
		facts[name] = f
	}
	return facts, rows.Err()
}

func appealsView(t appealTotals, has bool) *views.TownshipAppeals {
	if !has || t.appeals == 0 {
		return nil
	}
	v := &views.TownshipAppeals{
		Appeals: humanize.Comma(int64(t.appeals)),
		Reduced: pct(float64(t.reduced) / float64(t.appeals)),
		Years:   fmt.Sprintf("%d–%d", t.first, t.last),
	}
	if t.cutReduced > 0 {
		v.AvgCut = pct(t.cutWeighted / float64(t.cutReduced))
	}
	return v
}

func longDay(t time.Time) string { return t.Format("January 2, 2006") }

// townshipWindow formats bor.StatusOf for the township pages.
func townshipWindow(township string, now time.Time) views.TownshipWindow {
	st := bor.StatusOf(township, now)
	var w views.TownshipWindow
	if g := st.Group; g != nil {
		w.Group, w.Open, w.Close, w.EvidenceDue = g.Name, longDay(g.Open), longDay(g.Close), longDay(g.EvidenceDue)
	}
	switch st.State {
	case bor.Open:
		w.State, w.Label = "open", "Open now"
		w.DaysLeft = fmt.Sprintf("%d days left", st.DaysLeft)
		if st.DaysLeft == 1 {
			w.DaysLeft = "last day"
		}
	case bor.Upcoming:
		w.State, w.Label = "upcoming", "Opening soon"
	case bor.Closed:
		w.State, w.Label = "closed", fmt.Sprintf("Closed for the %d session", bor.SessionYear)
	default:
		w.State, w.Label = "unscheduled", "Dates not posted yet"
	}
	return w
}

func townshipView(name string, f townshipFacts, now time.Time) views.TownshipData {
	return views.TownshipData{
		Name: name, Slug: townshipSlug(name),
		Window:       townshipWindow(name, now),
		ScheduleAsOf: scheduleAsOf(),
		Homes:        humanize.Comma(int64(f.homes)),
		ProSe:        appealsView(f.proSe, f.hasProSe),
		Attorney:     appealsView(f.attorney, f.hasAtt),
	}
}

// scheduleAsOf renders bor.ScheduleAsOf ("2026-11-24") as "November 24, 2026".
func scheduleAsOf() string {
	t, err := time.Parse("2006-01-02", bor.ScheduleAsOf)
	if err != nil {
		return bor.ScheduleAsOf
	}
	return longDay(t)
}

// townshipIndex groups every township by its window's state at now.
func townshipIndex(now time.Time) views.TownshipIndex {
	groups := []views.TownshipGroup{
		{State: "open", Heading: "Open now"},
		{State: "upcoming", Heading: "Opening soon"},
		{State: "unscheduled", Heading: "Dates not posted yet"},
		{State: "closed", Heading: fmt.Sprintf("Closed for the %d session", bor.SessionYear)},
	}
	for _, t := range bor.Townships {
		w := townshipWindow(t, now)
		l := views.TownshipLink{Name: t, Slug: townshipSlug(t)}
		switch w.State {
		case "open":
			l.Dates = "Closes " + w.Close + " (" + w.DaysLeft + ")"
		case "upcoming":
			l.Dates = "Opens " + w.Open + ", closes " + w.Close
		case "closed":
			l.Dates = "Closed " + w.Close
		default:
			l.Dates = "The Board hasn't posted dates yet"
		}
		for i := range groups {
			if groups[i].State == w.State {
				groups[i].Townships = append(groups[i].Townships, l)
			}
		}
	}
	return views.TownshipIndex{Groups: groups, ScheduleAsOf: scheduleAsOf()}
}

func (s *Server) handleTownships(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, views.Townships(s.site, townshipIndex(time.Now())))
}

func (s *Server) handleTownship(w http.ResponseWriter, r *http.Request) {
	name, canonical, ok := resolveTownship(r.PathValue("slug"))
	if !ok {
		s.notFound(w, r)
		return
	}
	if !canonical {
		http.Redirect(w, r, (&url.URL{Path: townshipPath(name)}).String(), http.StatusMovedPermanently)
		return
	}
	s.render(w, r, views.Township(s.site, townshipView(name, s.towns[name], time.Now())))
}
