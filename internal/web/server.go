// Package web is the AssessGap website: address search, the free appeal
// check, and the static pages.
package web

//go:generate go tool templ generate -path views
//go:generate ../../bin/tailwindcss -i styles/input.css -o static/app.css --minify

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"net/mail"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/assessgap/assessgap/internal/bor"
	"github.com/assessgap/assessgap/internal/comps"
	"github.com/assessgap/assessgap/internal/packet"
	"github.com/assessgap/assessgap/internal/web/views"
)

//go:embed static
var staticFS embed.FS

// Server serves the site. Cook is the read-only county data; App holds signups.
type Server struct {
	Cook *sql.DB
	App  *sql.DB
	Dev  bool
	Log  *slog.Logger

	eng     *comps.Engine
	site    views.Site
	towns   map[string]townshipFacts
	ogImage string // share image path with a cache-busting hash
}

// New prepares a server. It reads a few site-wide numbers once at startup.
func New(cook, app *sql.DB, dev bool, log *slog.Logger) (*Server, error) {
	s := &Server{Cook: cook, App: app, Dev: dev, Log: log, eng: comps.New(cook)}
	s.site = views.Site{Dev: dev, CSS: "/static/app.css?v=" + fileHash("static/app.css")}
	err := cook.QueryRow(`
		SELECT SUM(n_decrease) * 1.0 / SUM(n_appeals), SUM(n_appeals), MIN(first_year), MAX(last_year)
		FROM nbhd_bor_stats WHERE pro_se = 1`).
		Scan(&s.site.ProSeWinRate, &s.site.ProSeAppeals, &s.site.FirstYear, &s.site.LastYear)
	if err != nil {
		return nil, err
	}
	s.ogImage = "/static/og.png?v=" + fileHash("static/og.png")
	if s.towns, err = loadTownshipFacts(context.Background(), cook); err != nil {
		return nil, err
	}
	return s, nil
}

func fileHash(name string) string {
	b, err := staticFS.ReadFile(name)
	if err != nil {
		return "0"
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:4])
}

// Handler returns the site's routes wrapped in middleware.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	static, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /static/", cacheForever(http.StripPrefix("/static/", http.FileServerFS(static))))
	mux.HandleFunc("GET /robots.txt", s.handleRobots)
	mux.HandleFunc("GET /sitemap.xml", s.handleSitemap)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	mux.HandleFunc("GET /townships", s.handleTownships)
	mux.HandleFunc("GET /townships/{slug}", s.handleTownship)
	mux.Handle("GET /townships/{$}", http.RedirectHandler("/townships", http.StatusMovedPermanently))

	mux.HandleFunc("GET /{$}", s.page(func(r *http.Request) templ.Component { return views.Home(s.site) }))
	mux.HandleFunc("GET /how-it-works", s.page(func(r *http.Request) templ.Component { return views.HowItWorks(s.site) }))
	mux.HandleFunc("GET /terms", s.page(func(r *http.Request) templ.Component { return views.Terms(s.site) }))
	mux.HandleFunc("GET /privacy", s.page(func(r *http.Request) templ.Component { return views.Privacy(s.site) }))
	heavy := make(busy, 4)
	mux.HandleFunc("GET /search", heavy.limit(s.handleSearch))
	mux.HandleFunc("GET /check/{pin}", heavy.limit(s.handleCheck))
	mux.HandleFunc("GET /check/{pin}/report.pdf", heavy.limit(s.handleReportPDF))
	mux.HandleFunc("POST /notify", newPerIP(10, 10*time.Minute).limit(s.handleNotify))
	mux.HandleFunc("/", s.notFound)

	h := http.NewCrossOriginProtection().Handler(mux)
	h = s.headers(h)
	h = s.logRequests(h)
	return h
}

func (s *Server) page(c func(*http.Request) templ.Component) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { s.render(w, r, c(r)) }
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, c templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := c.Render(views.WithShare(r.Context(), s.share(r)), w); err != nil {
		s.Log.Error("render", "path", r.URL.Path, "err", err)
	}
}

func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	s.Log.Error("request", "path", r.URL.Path, "err", err)
	w.WriteHeader(http.StatusInternalServerError)
	s.render(w, r, views.Error(s.site))
}

var pinRe = regexp.MustCompile(`^\d{14}$`)

// loadReport validates the {pin} path value and builds its report. On failure
// it writes the response (404 for a malformed or unknown PIN) and returns nil.
func (s *Server) loadReport(w http.ResponseWriter, r *http.Request) *comps.Report {
	pin := strings.ReplaceAll(r.PathValue("pin"), "-", "")
	if !pinRe.MatchString(pin) {
		s.notFound(w, r)
		return nil
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	rep, err := s.eng.Report(ctx, pin, 0)
	if errors.Is(err, comps.ErrNotFound) {
		s.notFound(w, r)
		return nil
	}
	if err != nil {
		s.fail(w, r, err)
		return nil
	}
	return rep
}

func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	s.render(w, r, views.NotFound(s.site))
}

func (s *Server) handleCheck(w http.ResponseWriter, r *http.Request) {
	if rep := s.loadReport(w, r); rep != nil {
		v := toView(rep)
		if t, ok := bor.LookupTownship(rep.Township); ok && v.Window.Text != "" {
			v.Window.TownshipName, v.Window.TownshipURL = t, townshipPath(t)
		}
		s.render(w, r, views.Report(s.site, v))
	}
}

// handleReportPDF serves the free evidence report as a download. Unsupported
// properties have no comparison to print, so they get 404 like unknown PINs.
func (s *Server) handleReportPDF(w http.ResponseWriter, r *http.Request) {
	rep := s.loadReport(w, r)
	if rep == nil {
		return
	}
	var buf bytes.Buffer // render fully first so an error never sends half a PDF
	err := packet.Render(&buf, rep, time.Now())
	if errors.Is(err, packet.ErrUnsupported) {
		s.notFound(w, r)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "application/pdf")
	h.Set("Content-Disposition", `attachment; filename="assessgap-`+rep.PIN+`.pdf"`)
	h.Set("Content-Length", strconv.Itoa(buf.Len()))
	w.Write(buf.Bytes())
}

func (s *Server) handleNotify(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	email := strings.TrimSpace(r.PostForm.Get("email"))
	pin := r.PostForm.Get("pin")
	reason := r.PostForm.Get("reason")
	if !pinRe.MatchString(pin) {
		pin = ""
	}
	if reason != "condo-support" && reason != "unsupported" {
		reason = "township-opens"
	}
	// A township page's form has no PIN, only the township's slug.
	var township, slug string
	if t, _, ok := resolveTownship(r.PostForm.Get("township")); ok {
		township, slug = t, townshipSlug(t)
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || len(email) > 254 || addr.Address != email {
		s.render(w, r, s.notifyResult(r, views.NotifyFormFor(pin, slug, reason, "That email address doesn't look right.")))
		return
	}
	if pin != "" {
		_ = s.Cook.QueryRowContext(r.Context(), `SELECT COALESCE(township_name,'') FROM parcel WHERE pin = ?`, pin).Scan(&township)
	}
	_, err = s.App.ExecContext(r.Context(),
		`INSERT INTO signup (email, pin, township, reason) VALUES (?, ?, ?, ?)
		 ON CONFLICT (email, pin) DO NOTHING`, strings.ToLower(email), pin, township, reason)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.Log.Info("signup", "township", township, "reason", reason)
	s.render(w, r, s.notifyResult(r, views.NotifyThanks()))
}

// ---- middleware

func (s *Server) headers(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; "+
			"form-action 'self'; frame-ancestors 'none'; base-uri 'self'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		if s.Dev {
			h.Set("X-Robots-Tag", "noindex, nofollow")
		}
		next.ServeHTTP(w, r)
	})
}

func cacheForever(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) { w.status = code; w.ResponseWriter.WriteHeader(code) }

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: 200}
		defer func() {
			if p := recover(); p != nil {
				s.Log.Error("panic", "path", r.URL.Path, "panic", p)
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(sw, r)
		if !strings.HasPrefix(r.URL.Path, "/static/") {
			s.Log.Info("http", "method", r.Method, "path", r.URL.Path, "status", sw.status, "dur", time.Since(start).Round(time.Millisecond))
		}
	})
}
