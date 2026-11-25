package web

import (
	"context"
	"testing"

	"github.com/assessgap/assessgap/internal/store"
)

func TestNormalizeQuery(t *testing.T) {
	tests := []struct{ in, want string }{
		{"5555 west belmont avenue", "5555 W BELMONT AVE"},
		{"5555 W. Belmont Ave, Chicago IL", "5555 W BELMONT AVE"},
		{"221 s evergreen ave apt b", "221 S EVERGREEN AVE UNIT B"},
		{"221 s evergreen #b", "221 S EVERGREEN UNIT B"},
		{"5555 w belm", "5555 W BELM"},
	}
	for _, tt := range tests {
		if got := normalizeQuery(tt.in); got != tt.want {
			t.Errorf("normalizeQuery(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestPinFromQuery(t *testing.T) {
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{"13-28-100-027-0000", "13281000270000", true},
		{"13281000270000", "13281000270000", true},
		{"13-28-100-027", "13281000270000", true},
		{"5555 W Belmont", "", false},
		{"1328", "", false},
	}
	for _, tt := range tests {
		got, ok := pinFromQuery(tt.in)
		if got != tt.want || ok != tt.ok {
			t.Errorf("pinFromQuery(%q) = %q, %v; want %q, %v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

func TestSearchSample(t *testing.T) {
	db, err := store.OpenReadOnly("../../data-sample/cook.db")
	if err != nil {
		t.Skip(err)
	}
	if db.Ping() != nil {
		t.Skip("no sample db")
	}
	ctx := context.Background()
	for _, q := range []string{"5555 w belmont", "5555 belmont", "13-28-100-027-0000"} {
		ms, err := search(ctx, db, q)
		if err != nil {
			t.Fatal(err)
		}
		if len(ms) == 0 {
			t.Errorf("search(%q): no matches", q)
		}
	}
}
