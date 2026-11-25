package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPerIP(t *testing.T) {
	l := newPerIP(2, time.Minute)
	t0 := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	for i, want := range []bool{true, true, false} {
		if got := l.allow("a", t0); got != want {
			t.Fatalf("request %d: got %v want %v", i, got, want)
		}
	}
	if !l.allow("b", t0) {
		t.Fatal("other IP should be allowed")
	}
	if l.allow("a", t0.Add(time.Minute-time.Nanosecond)) {
		t.Fatal("still inside the window: should be refused")
	}
	if !l.allow("a", t0.Add(time.Minute)) {
		t.Fatal("new window should reset")
	}
}

func TestBusy(t *testing.T) {
	ok := func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }
	tests := []struct {
		name       string
		taken      int // slots already in use
		wait       time.Duration
		status     int
		retryAfter string
	}{
		// With the only slot taken, nothing can free it, so the wait runs out.
		{"all slots taken", 1, time.Millisecond, http.StatusServiceUnavailable, "5"},
		// A free slot is taken at once, long before the wait.
		{"free slot", 0, time.Hour, http.StatusOK, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &busy{slots: make(chan struct{}, 1), wait: tt.wait}
			for range tt.taken {
				b.slots <- struct{}{}
			}
			rec := httptest.NewRecorder()
			b.limit(ok)(rec, httptest.NewRequest("GET", "/check/1", nil))
			if rec.Code != tt.status || rec.Header().Get("Retry-After") != tt.retryAfter {
				t.Errorf("status %d, Retry-After %q; want %d, %q", rec.Code, rec.Header().Get("Retry-After"), tt.status, tt.retryAfter)
			}
			if len(b.slots) != tt.taken {
				t.Errorf("%d slots in use after the request, want %d (a served request must release its slot)", len(b.slots), tt.taken)
			}
		})
	}
}

func TestClientIP(t *testing.T) {
	tests := []struct{ name, remote, xff, want string }{
		{"behind Caddy: last forwarded hop", "127.0.0.1:5000", "9.9.9.9, 1.2.3.4", "1.2.3.4"},
		{"behind Caddy over IPv6 loopback", "[::1]:5000", "1.2.3.4", "1.2.3.4"},
		{"loopback without the header", "127.0.0.1:5000", "", "127.0.0.1"},
		{"not from Caddy: header ignored", "5.6.7.8:5000", "1.2.3.4", "5.6.7.8"},
		{"no port", "5.6.7.8", "", "5.6.7.8"},
	}
	for _, tt := range tests {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = tt.remote
		r.Header.Set("X-Forwarded-For", tt.xff)
		if got := clientIP(r); got != tt.want {
			t.Errorf("%s: clientIP = %q, want %q", tt.name, got, tt.want)
		}
	}
}
