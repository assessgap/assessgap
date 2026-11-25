package web

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/assessgap/assessgap/internal/bor"
	"github.com/assessgap/assessgap/internal/web/views"
)

// indexedPaths are the pages listed in the sitemap. Report pages (/check/{pin})
// stay out: there are over a million, generated on demand, and near-identical.
var indexedPaths = func() []string {
	ps := []string{"/", "/how-it-works", "/terms", "/privacy", "/townships"}
	for _, t := range bor.Townships {
		ps = append(ps, townshipPath(t))
	}
	return ps
}()

// share is the request's link-preview and canonical-URL data. Only indexed
// pages get their own preview. Every other page (reports, and 404s, which echo
// whatever path was typed, PINs included) gets a generic preview pointing at
// the home page, so no address or PIN ends up in a third party's preview cache.
func (s *Server) share(r *http.Request) views.Share {
	o := origin(r)
	sh := views.Share{Origin: o, URL: o + "/", Image: o + s.ogImage, Private: true}
	if slices.Contains(indexedPaths, r.URL.Path) {
		sh.URL, sh.Private = o+r.URL.Path, false
		sh.Canonical = sh.URL
	}
	return sh
}

// origin is the public scheme and host for absolute URLs, taken from the request
// (Caddy passes the Host through). Local hosts get http.
func origin(r *http.Request) string {
	host := r.Host
	if strings.HasPrefix(host, "localhost") || strings.HasPrefix(host, "127.") || strings.HasSuffix(strings.Split(host, ":")[0], ".local") {
		return "http://" + host
	}
	return "https://" + host
}

func (s *Server) handleRobots(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if s.site.Dev {
		fmt.Fprint(w, "User-agent: *\nDisallow: /\n")
		return
	}
	fmt.Fprintf(w, "User-agent: *\nDisallow: /search\nDisallow: /notify\n\nSitemap: %s/sitemap.xml\n", origin(r))
}

func (s *Server) handleSitemap(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	o := origin(r)
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
	for _, p := range indexedPaths {
		fmt.Fprintf(&b, "  <url><loc>%s%s</loc></url>\n", o, p)
	}
	b.WriteString("</urlset>\n")
	w.Write([]byte(b.String()))
}
