package web

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// busy caps concurrent expensive requests (reports, search) so a scraper
// can't starve a 1-vCPU box. Requests wait up to wait for a slot, then get 503.
type busy struct {
	slots chan struct{}
	wait  time.Duration
}

// newBusy allows n concurrent requests, each waiting up to 5 s for a slot.
func newBusy(n int) *busy { return &busy{slots: make(chan struct{}, n), wait: 5 * time.Second} }

func (b *busy) limit(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t := time.NewTimer(b.wait)
		defer t.Stop()
		select {
		case b.slots <- struct{}{}:
			defer func() { <-b.slots }()
			next(w, r)
		case <-t.C:
			w.Header().Set("Retry-After", "5")
			http.Error(w, "Busy, please try again in a moment.", http.StatusServiceUnavailable)
		case <-r.Context().Done():
		}
	}
}

// perIP allows n requests per IP per window (fixed window). Good enough to
// stop form spam; not a security boundary.
type perIP struct {
	mu     sync.Mutex
	n      int
	window time.Duration
	start  time.Time
	seen   map[string]int
}

func newPerIP(n int, window time.Duration) *perIP {
	return &perIP{n: n, window: window, seen: map[string]int{}}
}

func (l *perIP) allow(ip string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.start) >= l.window {
		l.start, l.seen = now, map[string]int{}
	}
	l.seen[ip]++
	return l.seen[ip] <= l.n
}

func (l *perIP) limit(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !l.allow(clientIP(r), time.Now()) {
			w.Header().Set("Retry-After", "600")
			http.Error(w, "Too many requests, please try again later.", http.StatusTooManyRequests)
			return
		}
		next(w, r)
	}
}

// clientIP trusts X-Forwarded-For only from loopback, i.e. from Caddy on the
// same host. Caddy appends the real peer last.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			return strings.TrimSpace(parts[len(parts)-1])
		}
	}
	return host
}
