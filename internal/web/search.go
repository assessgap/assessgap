package web

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/assessgap/assessgap/internal/search"
	"github.com/assessgap/assessgap/internal/web/views"
)

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) > 120 {
		q = q[:120]
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	ms, err := search.Find(ctx, s.Cook, q)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	results := make([]views.Match, len(ms))
	for i, m := range ms {
		results[i] = views.Match{PIN: m.PIN, Address: titleCase(m.Address), City: titleCase(m.City),
			Kind: m.Kind(), PINFmt: formatPIN(m.PIN)}
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
