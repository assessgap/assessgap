package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestClassify(t *testing.T) {
	tests := []struct{ path, want string }{
		{"internal/comps/select.go", kindProduction},
		{"internal/ingest/build.sql", kindProduction},
		{"internal/web/views/report.templ", kindProduction},
		{"internal/web/styles/input.css", kindProduction},
		{"scripts/check.sh", kindProduction},
		{"internal/comps/comps_test.go", kindTest},
		{"internal/golden/golden.go", kindTest},   // test-only package
		{"internal/fixture/fixture.go", kindTest}, // test-only package
		{"testdata/README.md", kindTest},
		{"internal/web/views/report_templ.go", kindGenerated},
		{"internal/web/static/app.css", kindGenerated},
		{"internal/web/static/htmx.min.js", kindGenerated},
		{"testdata/golden/check/in-line.txt", kindGenerated},
		{"testdata/fixture/sales/page-00000.csv", kindGenerated},
		{"go.sum", kindGenerated},
		{"docs/testing.md", kindOther},
		{".github/workflows/ci.yml", kindOther},
		{"go.mod", kindOther},
	}
	for _, tt := range tests {
		if got := classify(tt.path); got != tt.want {
			t.Errorf("classify(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestTable(t *testing.T) {
	files := []File{
		{"internal/comps/select.go", 10, 4},
		{"internal/cli/check.go", 30, 6},
		{"internal/comps/comps_test.go", 50, 20},
		{"internal/web/views/report_templ.go", 400, 300},
		{"testdata/golden/check/in-line.txt", 9, 9},
		{"README.md", 3, 1},
	}
	want := startMarker + `
### Lines changed: test vs production

| | Files | Added | Deleted | Net |
|---|--:|--:|--:|--:|
| Test | 1 | +50 | −20 | +30 |
| Production | 2 | +40 | −10 | +30 |
| Generated (excluded) | 2 | +409 | −309 | +100 |
| Docs and config (excluded) | 1 | +3 | −1 | +2 |

Test lines added per production line added: 1.25.

_Updated automatically on every push. Generated: ` + "`*_templ.go`, `app.css`, vendored `*.min.js`, `testdata/golden/`, `testdata/fixture/`, `go.sum`" + `._
` + endMarker
	if got := table(files); got != want {
		t.Errorf("table:\n%s\nwant:\n%s", got, want)
	}
	if got := table(nil); !strings.Contains(got, "| Production | 0 | +0 | −0 | +0 |") || strings.Contains(got, "per production line") {
		t.Errorf("empty table:\n%s", got)
	}
}

func TestSplice(t *testing.T) {
	section := startMarker + "\nNEW\n" + endMarker
	tests := []struct{ name, body, want string }{
		{"empty body", "", section + "\n"},
		{"appended after the description", "## Summary\nText.\n", "## Summary\nText.\n\n" + section + "\n"},
		{"replaces the old table in place",
			"## Summary\n" + startMarker + "\nOLD\n" + endMarker + "\n## Tests\n",
			"## Summary\n" + section + "\n## Tests\n"},
	}
	for _, tt := range tests {
		if got := splice(tt.body, section); got != tt.want {
			t.Errorf("%s:\n got %q\nwant %q", tt.name, got, tt.want)
		}
	}
	// Running twice changes nothing.
	once := splice("## Summary\n", section)
	if twice := splice(once, section); twice != once {
		t.Errorf("not idempotent:\n%q\n%q", once, twice)
	}
}

func TestParseNumstat(t *testing.T) {
	in := "10\t4\tinternal/comps/select.go\n" +
		"-\t-\tdocs/diagram.png\n" +
		"3\t1\tinternal/{old => new}/x.go\n" +
		"2\t0\told.go => new.go\n"
	got, err := parseNumstat(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	want := []File{
		{"internal/comps/select.go", 10, 4},
		{"docs/diagram.png", 0, 0},
		{"internal/new/x.go", 3, 1},
		{"new.go", 2, 0},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}
