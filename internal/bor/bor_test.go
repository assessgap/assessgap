package bor

import (
	"testing"
	"time"
)

// TestStatusOf uses Hyde Park's 2026 window (Nov 9 – Dec 8, 2026). Update the
// dates with the schedule.
func TestStatusOf(t *testing.T) {
	at := func(s string) time.Time { return d(s).Add(15 * time.Hour) } // 3 pm in Chicago
	tests := []struct {
		name string
		town string
		now  time.Time
		want State
		days int
	}{
		{"first day", "Hyde Park", at("2026-11-09"), Open, 30},
		{"mid-window", "Hyde Park", at("2026-11-24"), Open, 15},
		{"last day, any case", "hyde park", at("2026-12-08"), Open, 1},
		// 03:00 UTC on Dec 9 is still 9 pm on Dec 8 in Chicago.
		{"UTC instant on the last evening", "Hyde Park", time.Date(2026, 12, 9, 3, 0, 0, 0, time.UTC), Open, 1},
		{"day after", "Hyde Park", at("2026-12-09"), Closed, 0},
		{"day before", "Hyde Park", at("2026-11-08"), Upcoming, 0},
		{"not posted", "Jefferson", at("2026-11-24"), Unscheduled, 0},
	}
	for _, tt := range tests {
		got := StatusOf(tt.town, tt.now)
		if got.State != tt.want || got.DaysLeft != tt.days {
			t.Errorf("%s: StatusOf(%q, %s) = %v/%d, want %v/%d", tt.name, tt.town, tt.now, got.State, got.DaysLeft, tt.want, tt.days)
		}
	}
}

// TestDaysLeftAcrossDST uses a made-up window, Feb 22 – Mar 23, 2027, that
// spans the spring daylight-saving change (Mar 14, 2027). On Mar 1 the days
// Mar 1..23 are 23 days, counting today. Dividing hours by 24 gave 22.
func TestDaysLeftAcrossDST(t *testing.T) {
	saved := Schedule
	t.Cleanup(func() { Schedule = saved })
	Schedule = []Group{{"test", []string{"Worth"}, d("2027-02-22"), d("2027-03-23"), d("2027-04-02")}}
	got := StatusOf("Worth", d("2027-03-01").Add(15*time.Hour))
	if got.State != Open || got.DaysLeft != 23 {
		t.Errorf("StatusOf on 2027-03-01 = %v/%d, want Open/23", got.State, got.DaysLeft)
	}
}

// TestScheduleValid checks the hand-edited schedule: dates in order, no
// township twice, and every name spelled exactly as in Townships (StatusOf
// matches names exactly, so "Lakeview" would silently read as not posted).
func TestScheduleValid(t *testing.T) {
	known := map[string]bool{}
	for _, n := range Townships {
		known[n] = true
	}
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
			if !known[n] {
				t.Errorf("scheduled township %q is not spelled as in Townships", n)
			}
		}
	}
}

func TestLookupTownship(t *testing.T) {
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
