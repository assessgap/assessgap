package web

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestPerIP(t *testing.T) {
	l := newPerIP(2, time.Minute)
	t0 := time.Now()
	for i, want := range []bool{true, true, false} {
		if got := l.allow("a", t0); got != want {
			t.Fatalf("request %d: got %v want %v", i, got, want)
		}
	}
	if !l.allow("b", t0) {
		t.Fatal("other IP should be allowed")
	}
	if !l.allow("a", t0.Add(time.Minute)) {
		t.Fatal("new window should reset")
	}
}

func TestClientIP(t *testing.T) {
	tests := []struct{ remote, xff, want string }{
		{"127.0.0.1:5000", "9.9.9.9, 1.2.3.4", "1.2.3.4"},
		{"127.0.0.1:5000", "", "127.0.0.1"},
		{"5.6.7.8:5000", "1.2.3.4", "5.6.7.8"}, // not from Caddy: ignore header
	}
	for _, tt := range tests {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = tt.remote
		if tt.xff != "" {
			r.Header.Set("X-Forwarded-For", tt.xff)
		}
		if got := clientIP(r); got != tt.want {
			t.Errorf("clientIP(%q, %q) = %q, want %q", tt.remote, tt.xff, got, tt.want)
		}
	}
}
