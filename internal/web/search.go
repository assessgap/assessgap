package web

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/assessgap/assessgap/internal/bor"
	"github.com/assessgap/assessgap/internal/search"
	"github.com/assessgap/assessgap/internal/web/views"
)

// maxQueryBytes caps the search text; no address is longer.
const maxQueryBytes = 120

// truncate cuts s to at most n bytes without splitting a UTF-8 character.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// townshipsOf maps each PIN to its township in bor.Townships' spelling, so a
// search result can say which township a home is in and link to its page.
// PINs whose township isn't one of the 38 are left out.
func (s *Server) townshipsOf(ctx context.Context, pins []string) (map[string]string, error) {
	out := map[string]string{}
	if len(pins) == 0 {
		return out, nil
	}
	args := make([]any, len(pins))
	for i, p := range pins {
		args[i] = p
	}
	rows, err := s.Cook.QueryContext(ctx, `SELECT pin, COALESCE(township_name, '') FROM parcel WHERE pin IN (?`+
		strings.Repeat(",?", len(pins)-1)+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("search townships: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var pin, name string
		if err := rows.Scan(&pin, &name); err != nil {
			return nil, err
		}
		if t, ok := bor.LookupTownship(name); ok {
			out[pin] = t
		}
	}
	return out, rows.Err()
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := truncate(strings.TrimSpace(r.URL.Query().Get("q")), maxQueryBytes)
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	ms, err := search.Find(ctx, s.Cook, q)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	pins := make([]string, len(ms))
	for i, m := range ms {
		pins[i] = m.PIN
	}
	towns, err := s.townshipsOf(ctx, pins)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	results := make([]views.Match, len(ms))
	for i, m := range ms {
		results[i] = views.Match{PIN: m.PIN, Address: titleCase(m.Address), City: titleCase(m.City),
			Kind: m.Kind(), PINFmt: formatPIN(m.PIN)}
		if t, ok := towns[m.PIN]; ok {
			results[i].Township, results[i].TownshipURL = t, townshipPath(t)
		}
	}
	if r.Header.Get("HX-Request") == "true" {
		s.render(w, r, views.SearchResults(q, results))
		return
	}
	// A full-page submit of an exact match goes straight to the report.
	if len(ms) == 1 {
		http.Redirect(w, r, "/check/"+ms[0].PIN, http.StatusSeeOther)
		return
	}
	s.render(w, r, views.SearchPage(s.site, q, results))
}
