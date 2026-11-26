package search

import (
	"context"
	"database/sql"
	"os"
	"reflect"
	"testing"

	"github.com/assessgap/assessgap/internal/fixture"
	_ "modernc.org/sqlite"
)

func TestMain(m *testing.M) { os.Exit(fixture.Main(m)) }

func TestNormalizeQuery(t *testing.T) {
	tests := []struct{ in, want string }{
		{"5555 west belmont avenue", "5555 W BELMONT AVE"},
		{"5555 W. Belmont Ave, Chicago IL", "5555 W BELMONT AVE"},
		{"221 s evergreen ave apt b", "221 S EVERGREEN AVE UNIT B"},
		{"221 s evergreen #b", "221 S EVERGREEN UNIT B"},
		{"5555 w belmont av", "5555 W BELMONT AVE"},
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
		{"\t13281000270000\n", "13281000270000", true},
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
	house := []Match{
		{PIN: "1", Address: "5555 W BELMONT AVE"},
		{PIN: "2", Address: "5555 W BELMONT AVE UNIT 2"},
	}
	// Two parcels with one address: picking either could show the wrong home.
	twins := []Match{
		{PIN: "1", Address: "2329 N LOCKWOOD AVE"},
		{PIN: "2", Address: "2329 N LOCKWOOD AVE"},
	}
	tests := []struct {
		name   string
		q      string
		ms     []Match
		want   string
		wantOK bool
	}{
		{"house over its unit", "5555 w belmont avenue", house, "1", true},
		{"prefix only", "5555 w belmont", house, "", false},
		{"same address twice", "2329 n lockwood ave", twins, "", false},
	}
	for _, tt := range tests {
		m, ok := Exact(tt.q, tt.ms)
		if ok != tt.wantOK || m.PIN != tt.want {
			t.Errorf("Exact(%q) = %q, %v; want %q, %v", tt.q, m.PIN, ok, tt.want, tt.wantOK)
		}
	}
}

// TestFindStreetNamedForADirection: the Assessor spells out North, West and
// so on when they are part of the street name, which the fixture has none of.
// Before the fix, "1921 W North Ave" became "1921 W N AVE" and found nothing.
func TestFindStreetNamedForADirection(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1) // one in-memory database
	if _, err := db.Exec(`CREATE TABLE parcel (pin TEXT, address TEXT, city TEXT, class TEXT);
		INSERT INTO parcel VALUES
			('17062000260000', '1921 W NORTH AVE', 'CHICAGO', '212'),
			('04213011170000', '3704 ARI WEST DR', 'GLENVIEW', '208'),
			('13331030130000', '2329 N LOCKWOOD AVE', 'CHICAGO', '203')`); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		q, want string
		exact   bool // the whole address was typed, so the CLI picks it
	}{
		{"1921 W North Ave", "17062000260000", true},
		{"1921 west north avenue", "17062000260000", true},
		{"1921 North Ave", "17062000260000", false}, // no direction typed: retried with W
		{"3704 Ari West Dr", "04213011170000", true},
		{"2329 North Lockwood", "13331030130000", false}, // the direction still abbreviates
	}
	for _, tt := range tests {
		ms, err := Find(context.Background(), db, tt.q)
		if err != nil {
			t.Fatal(err)
		}
		if len(ms) != 1 || ms[0].PIN != tt.want {
			t.Errorf("Find(%q) = %v, want PIN %s", tt.q, ms, tt.want)
			continue
		}
		if _, ok := Exact(tt.q, ms); ok != tt.exact {
			t.Errorf("Exact(%q) ok = %v, want %v", tt.q, ok, tt.exact)
		}
	}
}

// TestFind looks up fixture parcels. A wrong match here shows someone else's home.
func TestFind(t *testing.T) {
	db := fixture.Open(t)
	tests := []struct {
		name  string
		q     string
		want  []string // PINs, in order; nil for none
		count int      // when the exact PINs don't matter, just how many
	}{
		{"PIN with dashes", "13-33-103-013-0000", []string{"13331030130000"}, 0},
		{"address prefix", "2329 n lockwood", []string{"13331030130000"}, 0},
		{"no direction: retried with N, S, E, W", "2329 lockwood", []string{"13331030130000"}, 0},
		{"condo units in one building", "2106 n laramie", []string{"13331120411001", "13331120411002", "13331120411003"}, 0},
		{"too short", "ab", nil, 0},
		{"capped at MaxMatches", "210", nil, 8}, // 68 fixture addresses start "210"
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms, err := Find(context.Background(), db, tt.q)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, m := range ms {
				got = append(got, m.PIN)
			}
			if tt.count > 0 {
				if len(got) != tt.count {
					t.Errorf("%d matches, want %d", len(got), tt.count)
				}
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Find(%q) = %v, want %v", tt.q, got, tt.want)
			}
		})
	}
}
