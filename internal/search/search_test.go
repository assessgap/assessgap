package search

import (
	"context"
	"os"
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
		if got := NormalizeQuery(tt.in); got != tt.want {
			t.Errorf("NormalizeQuery(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParsePIN(t *testing.T) {
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{"13-28-100-027-0000", "13281000270000", true},
		{"13281000270000", "13281000270000", true},
		{" 13281000270000 ", "13281000270000", true},
		{"13-28-100-027", "13281000270000", true},
		{"5555 W Belmont", "", false},
		{"1328", "", false},
	}
	for _, tt := range tests {
		got, ok := ParsePIN(tt.in)
		if got != tt.want || ok != tt.ok {
			t.Errorf("ParsePIN(%q) = %q, %v; want %q, %v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

func TestFormatPIN(t *testing.T) {
	tests := []struct{ in, want string }{
		{"13281000270000", "13-28-100-027-0000"},
		{"1328", "1328"},
	}
	for _, tt := range tests {
		if got := FormatPIN(tt.in); got != tt.want {
			t.Errorf("FormatPIN(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestExact(t *testing.T) {
	ms := []Match{
		{PIN: "1", Address: "5555 W BELMONT AVE"},
		{PIN: "2", Address: "5555 W BELMONT AVE UNIT 2"},
	}
	tests := []struct {
		q      string
		want   string
		wantOK bool
	}{
		{"5555 w belmont avenue", "1", true},
		{"5555 w belmont", "", false},
	}
	for _, tt := range tests {
		m, ok := Exact(tt.q, ms)
		if ok != tt.wantOK || m.PIN != tt.want {
			t.Errorf("Exact(%q) = %q, %v; want %q, %v", tt.q, m.PIN, ok, tt.want, tt.wantOK)
		}
	}
}

func TestFindSample(t *testing.T) {
	const path = "../../data-sample/cook.db"
	if _, err := os.Stat(path); err != nil {
		t.Skipf("sample DB missing (%v)", err)
	}
	db, err := store.OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	for _, q := range []string{"5555 w belmont", "5555 belmont", "13-28-100-027-0000"} {
		ms, err := Find(ctx, db, q)
		if err != nil {
			t.Fatal(err)
		}
		if len(ms) == 0 {
			t.Errorf("Find(%q): no matches", q)
		}
	}
}
