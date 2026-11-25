package bor

import (
	"testing"
	"time"
)

func TestStatusOf(t *testing.T) {
	at := func(s string) time.Time { return d(s).Add(15 * time.Hour) }
	tests := []struct {
		town string
		now  string
		want State
		days int
	}{
		{"Hyde Park", "2026-11-24", Open, 15},
		{"hyde park", "2026-12-08", Open, 1},
		{"Hyde Park", "2026-12-09", Closed, 0},
		{"Hyde Park", "2026-11-08", Upcoming, 0},
		{"Jefferson", "2026-11-24", Unscheduled, 0},
		{"Lake View", "2026-11-24", Closed, 0},
	}
	for _, tt := range tests {
		got := StatusOf(tt.town, at(tt.now))
		if got.State != tt.want || got.DaysLeft != tt.days {
			t.Errorf("StatusOf(%q, %s) = %v/%d, want %v/%d", tt.town, tt.now, got.State, got.DaysLeft, tt.want, tt.days)
		}
	}
}

func TestScheduleTownshipsUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, g := range Schedule {
		if g.Close.Before(g.Open) || g.EvidenceDue.Before(g.Close) {
			t.Errorf("group %s: bad dates", g.Name)
		}
		for _, n := range g.Townships {
			if seen[n] {
				t.Errorf("%s listed twice", n)
			}
			seen[n] = true
			if _, ok := LookupTownship(n); !ok {
				t.Errorf("scheduled township %q is not in Townships", n)
			}
		}
	}
}

func TestLookupTownship(t *testing.T) {
	if len(Townships) != 38 {
		t.Errorf("%d townships, want 38", len(Townships))
	}
	tests := []struct {
		in, want string
		ok       bool
	}{
		{"Lake View", "Lake View", true},
		{"lakeview", "Lake View", true},
		{"Lake", "Lake", true},
		{"new trier township", "New Trier", true},
		{" JEFFERSON ", "Jefferson", true},
		{"Springfield", "", false},
	}
	for _, tt := range tests {
		got, ok := LookupTownship(tt.in)
		if got != tt.want || ok != tt.ok {
			t.Errorf("LookupTownship(%q) = %q, %v; want %q, %v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}
