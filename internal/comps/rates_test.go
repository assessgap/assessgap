package comps

import (
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// TestQuotedRatesMatch ties the backtest rates the reasons text quotes
// (groupRates) to the two other places a homeowner or reviewer reads them:
// the How it works page and docs/methodology.md. All three are typed by hand,
// so a re-run of the backtest that updates one and not the others would show
// different rates on different pages.
func TestQuotedRatesMatch(t *testing.T) {
	want := groupRates[:]

	page := read(t, filepath.Join("..", "web", "views", "pages.templ"))
	tested := between(t, page, `id="tested"`, `id="if-not"`)
	if got := ints(regexp.MustCompile(`about (\d+)%`).FindAllStringSubmatch(tested, -1)); !slices.Equal(got, want) {
		t.Errorf("pages.templ (How we tested it) quotes %v, groupRates is %v", got, want)
	}

	// The table under "By the report's groups": the third column is the
	// self-filed reduction rate, to one decimal.
	section := between(t, read(t, filepath.Join("..", "..", "docs", "methodology.md")), "By the report's groups", "\n## ")
	table := "|" + between(t, section+"\n\n", "\n\n|", "\n\n")
	var got []int
	for _, row := range strings.Split(table, "\n") {
		cells := strings.Split(row, "|")
		if len(cells) < 4 || strings.HasPrefix(strings.TrimSpace(cells[1]), "Building") || strings.HasPrefix(cells[1], "---") {
			continue
		}
		v, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(cells[3]), "%"), 64)
		if err != nil {
			t.Fatalf("methodology.md row %q: %v", row, err)
		}
		got = append(got, int(math.Round(v)))
	}
	if !slices.Equal(got, want) {
		t.Errorf("methodology.md (by the report's groups) rounds to %v, groupRates is %v", got, want)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// between returns the text of s after the first from and before the next to.
func between(t *testing.T, s, from, to string) string {
	t.Helper()
	_, after, ok := strings.Cut(s, from)
	if !ok {
		t.Fatalf("%q not found", from)
	}
	before, _, ok := strings.Cut(after, to)
	if !ok {
		t.Fatalf("%q not found after %q", to, from)
	}
	return before
}

func ints(ms [][]string) []int {
	var out []int
	for _, m := range ms {
		n, _ := strconv.Atoi(m[1])
		out = append(out, n)
	}
	return out
}
