package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/assessgap/assessgap/internal/fixture"
	"github.com/assessgap/assessgap/internal/golden"
)

// TestExitCodes pins the exit status of each way of calling the command:
// 0 for success and help, 1 for a failed lookup or a bad database, 2 for
// bad usage. Scripts (ops/refresh.sh, scripts/smoke.sh, CI) depend on these.
func TestExitCodes(t *testing.T) {
	db := fixture.Path(t)
	missing := filepath.Join(t.TempDir(), "missing.db")
	pdf := filepath.Join(t.TempDir(), "r.pdf")
	tests := []struct {
		name string
		args []string
		code int
	}{
		{"no arguments prints usage", nil, 2},
		{"help", []string{"help"}, 0},
		{"-h", []string{"-h"}, 0},
		{"unknown command", []string{"nope"}, 2},
		{"--version", []string{"--version"}, 0},
		{"version", []string{"version"}, 0},
		{"check -h", []string{"check", "-h"}, 0},
		{"report -h", []string{"report", "-h"}, 0},
		{"township -h", []string{"township", "-h"}, 0},
		{"db -h", []string{"db", "-h"}, 0},
		{"db sync -h", []string{"db", "sync", "-h"}, 0},
		{"db fetch -h", []string{"db", "fetch", "-h"}, 0},
		{"db load -h", []string{"db", "load", "-h"}, 0},
		{"db build -h", []string{"db", "build", "-h"}, 0},
		{"db validate -h", []string{"db", "validate", "-h"}, 0},
		{"db finalize -h", []string{"db", "finalize", "-h"}, 0},
		{"serve -h", []string{"serve", "-h"}, 0},
		{"backtest -h", []string{"backtest", "-h"}, 0},
		{"sample -h", []string{"sample", "-h"}, 0},
		{"live-eval -h", []string{"live-eval", "-h"}, 0},
		{"unknown flag", []string{"check", "-nope", "13331030130000"}, 2},
		{"flag without its value", []string{"check", "13331030130000", "-db"}, 2},
		{"check a PIN", []string{"check", "-db", db, "13331030130000"}, 0},
		{"check a dashed PIN, flags last", []string{"check", "13-33-103-013-0000", "-db", db}, 0},
		{"check an address", []string{"check", "-db", db, "2329", "N", "Lockwood", "Ave"}, 0},
		{"check an unknown PIN", []string{"check", "-db", db, "99999999999999"}, 1},
		{"check a malformed PIN", []string{"check", "-db", db, "123"}, 1},
		{"check with no PIN", []string{"check", "-db", db}, 2},
		{"check a missing database", []string{"check", "-db", missing, "2329 N Lockwood Ave"}, 1},
		{"report without -o", []string{"report", "-db", db, "13331030130000"}, 2},
		{"report a PIN", []string{"report", "-db", db, "-o", pdf, "13331030130000"}, 0},
		{"township, all", []string{"township"}, 0},
		{"township by name", []string{"township", "jefferson"}, 0},
		{"unknown township", []string{"township", "atlantis"}, 1},
		{"db unknown step", []string{"db", "nope"}, 2},
		// A negative sample size used to panic (slice bounds out of range).
		{"sample -n -1", []string{"sample", "-db", db, "-n", "-1"}, 2},
		{"live-eval -n 0", []string{"live-eval", "-db", db, "-n", "0"}, 2},
		{"serve on a bad address", []string{"serve", "-db", db, "-app", filepath.Join(t.TempDir(), "a.db"), "-addr", "127.0.0.1:99999"}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := assessgap(t, "", nil, tt.args...)
			if r.code != tt.code {
				t.Errorf("assessgap %s: exit %d, want %d\nstdout: %s\nstderr: %s",
					strings.Join(tt.args, " "), r.code, tt.code, r.stdout, r.stderr)
			}
			if strings.Contains(r.stderr, "panic:") { // a panic also exits 2
				t.Errorf("panicked:\n%s", r.stderr)
			}
		})
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Errorf("checking against a missing database created it (stat: %v)", err)
	}
}

// TestDatabaseFromEnvironment checks $ASSESSGAP_DB is honored and that,
// with no database anywhere, an address lookup fails offline (only a PIN
// may fall back to the county API).
func TestDatabaseFromEnvironment(t *testing.T) {
	db := fixture.Path(t)
	if r := assessgap(t, "", []string{"ASSESSGAP_DB=" + db}, "check", "13331030130000"); r.code != 0 {
		t.Errorf("with $ASSESSGAP_DB: exit %d\n%s", r.code, r.stderr)
	}
	r := assessgap(t, t.TempDir(), nil, "check", "2329 N Lockwood Ave")
	if r.code != 1 || !strings.Contains(r.stderr, "data/cook.db") {
		t.Errorf("no database: exit %d, want 1 naming data/cook.db\n%s", r.code, r.stderr)
	}
}

// TestBinaryMatchesGoldens runs every golden case through the built binary
// and compares it with testdata/golden/check, as internal/golden does
// in-process. The shipped program must say exactly what the goldens say.
func TestBinaryMatchesGoldens(t *testing.T) {
	db := fixture.Path(t)
	for _, c := range golden.Cases {
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			r := assessgap(t, "", nil, append([]string{"check", "-json", "-db", db}, c.Args...)...)
			got := []string{"exit = " + strconv.Itoa(r.code)}
			if s := strings.TrimSpace(r.stderr); s != "" {
				got = append(got, "stderr = "+strconv.Quote(s))
			}
			if r.stdout != "" {
				fields, err := golden.Flatten([]byte(r.stdout))
				if err != nil {
					t.Fatalf("stdout is not JSON: %v", err)
				}
				got = append(got, fields...)
			}
			want := goldenLines(t, filepath.Join(golden.Dir, "check", c.Name+".txt"))
			if strings.Join(got, "\n") != strings.Join(want, "\n") {
				t.Errorf("%s (%s): the binary's output differs from the golden; run go test ./internal/golden for the fields",
					c.Name, c.Args[0])
			}
		})
	}
}

// goldenLines reads a golden file without its "# " comment lines.
func goldenLines(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, l := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
		if !strings.HasPrefix(l, "# ") {
			out = append(out, l)
		}
	}
	return out
}

// TestReportPDF writes the evidence report with the binary and checks the
// file is a well-formed PDF (header, xref offsets, trailer) of whole pages.
func TestReportPDF(t *testing.T) {
	out := filepath.Join(t.TempDir(), "report.pdf")
	r := assessgap(t, "", nil, "report", "-db", fixture.Path(t), "-o", out, "13331030130000")
	if r.code != 0 {
		t.Fatalf("exit %d\n%s", r.code, r.stderr)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	pages, err := parsePDF(b)
	if err != nil {
		t.Fatalf("report.pdf: %v", err)
	}
	if pages < 1 {
		t.Errorf("report.pdf has %d pages", pages)
	}
}

// TestCheckJSONBrief checks -brief is wired: it drops the full list of
// similar homes and keeps the rest of the report.
func TestCheckJSONBrief(t *testing.T) {
	db := fixture.Path(t)
	var full, brief map[string]any
	for _, x := range []struct {
		args []string
		into *map[string]any
	}{{[]string{"check", "-json", "-db", db, "13331030130000"}, &full}, {[]string{"check", "-json", "-brief", "-db", db, "13331030130000"}, &brief}} {
		r := assessgap(t, "", nil, x.args...)
		if r.code != 0 {
			t.Fatalf("%v: exit %d\n%s", x.args, r.code, r.stderr)
		}
		if err := json.Unmarshal([]byte(r.stdout), x.into); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok := full["comps"]; !ok {
		t.Fatal("full report has no comps")
	}
	if _, ok := brief["comps"]; ok {
		t.Error("-brief kept comps")
	}
	if full["pin"] != brief["pin"] || full["pin"] != "13331030130000" {
		t.Errorf("pin: full %v, brief %v", full["pin"], brief["pin"])
	}
}
