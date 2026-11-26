package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/assessgap/assessgap/internal/fixture"
)

func TestMain(m *testing.M) { os.Exit(fixture.Main(m)) }

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		pos     []string
		out     string
		year    int
		wantErr error
	}{
		{"flags after a positional", []string{"13281000270000", "-o", "x.pdf"}, []string{"13281000270000"}, "x.pdf", 0, nil},
		{"flags between positionals", []string{"5555", "W", "-year", "2025", "Belmont", "-o=y.pdf"}, []string{"5555", "W", "Belmont"}, "y.pdf", 2025, nil},
		{"-- ends flags", []string{"-o", "x.pdf", "--", "-weird", "-year"}, []string{"-weird", "-year"}, "x.pdf", 0, nil},
		{"unknown flag", []string{"pin", "-nope"}, nil, "", 0, errUsage},
		{"help", []string{"pin", "-h"}, nil, "", 0, flag.ErrHelp},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := flag.NewFlagSet("t", flag.ContinueOnError)
			fs.SetOutput(io.Discard)
			out := fs.String("o", "", "")
			year := fs.Int("year", 0, "")
			pos, err := parseArgs(fs, tt.args)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && (!reflect.DeepEqual(pos, tt.pos) || *out != tt.out || *year != tt.year) {
				t.Errorf("= %q, -o %q, -year %d; want %q, %q, %d", pos, *out, *year, tt.pos, tt.out, tt.year)
			}
		})
	}
}

func TestDBPath(t *testing.T) {
	tests := []struct{ flag, env, want string }{
		{"x.db", "y.db", "x.db"},
		{"", "y.db", "y.db"},
		{"", "", DefaultDB},
	}
	for _, tt := range tests {
		env := &Env{Getenv: func(k string) string {
			if k == "ASSESSGAP_DB" {
				return tt.env
			}
			return ""
		}}
		if got := dbPath(env, tt.flag); got != tt.want {
			t.Errorf("dbPath(%q, env %q) = %q, want %q", tt.flag, tt.env, got, tt.want)
		}
	}
}

func TestUseLiveByDefault(t *testing.T) {
	existing := filepath.Join(t.TempDir(), "cook.db")
	if err := os.WriteFile(existing, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(t.TempDir(), "none.db")
	tests := []struct {
		name, flag, env, path, q string
		want                     bool
	}{
		{"no db anywhere, PIN", "", "", missing, "13281000270000", true},
		{"no db anywhere, address", "", "", missing, "3134 N Luna Ave", false},
		{"default db exists", "", "", existing, "13281000270000", false},
		{"-db given", missing, "", missing, "13281000270000", false},
		{"ASSESSGAP_DB set", "", missing, missing, "13281000270000", false},
	}
	for _, tt := range tests {
		env := &Env{Getenv: func(k string) string {
			if k == "ASSESSGAP_DB" {
				return tt.env
			}
			return ""
		}}
		if got := useLiveByDefault(env, tt.flag, tt.path, tt.q); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

// run executes the CLI with ASSESSGAP_DB set to db and returns status and output.
// With no database, check and report would fall back to the county's API,
// so they must be given one.
func run(t *testing.T, db string, args ...string) (int, string, string) {
	t.Helper()
	if db == "" && len(args) > 0 && (args[0] == "check" || args[0] == "report") {
		t.Fatalf("%s needs a database in tests (it would go live)", args[0])
	}
	var out, errb bytes.Buffer
	env := &Env{Stdout: &out, Stderr: &errb, Getenv: func(k string) string {
		if k == "ASSESSGAP_DB" {
			return db
		}
		return ""
	}}
	code := Run(context.Background(), env, args)
	return code, out.String(), errb.String()
}

// TestRunDispatch checks exit statuses. ASSESSGAP_DB points at a missing
// file, so a PIN must fail with the how-to-build message, never fall back to
// the county's API.
func TestRunDispatch(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "none.db")
	tests := []struct {
		name     string
		args     []string
		code     int
		contains string // in stderr, when it identifies the branch
	}{
		{"no command", nil, 2, ""},
		{"help", []string{"help"}, 0, ""},
		{"unknown command", []string{"bogus"}, 2, ""},
		{"check without a query", []string{"check"}, 2, ""},
		{"check -h", []string{"check", "-h"}, 0, ""},
		{"missing database is not live", []string{"check", "13281000270000"}, 1, "assessgap db sync"},
		{"report without -o", []string{"report", "13281000270000"}, 2, ""},
		{"db without a step", []string{"db"}, 2, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, _, stderr := run(t, missing, tt.args...)
			if code != tt.code || !strings.Contains(stderr, tt.contains) {
				t.Errorf("exit %d, stderr %q; want exit %d containing %q", code, stderr, tt.code, tt.contains)
			}
		})
	}
}

// TestTownship checks the filing-window command on fixed dates. The CLI
// treats -on as midday in Chicago; without that, Dec 8 would still be Dec 7
// in Chicago and show "1 days left" instead of the last day.
func TestTownship(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		code     int
		contains string // the line for the township, in stdout
	}{
		{"last day of the window", []string{"-on", "2026-12-08", "hyde park"}, 0, "File by Dec 8, 2026 (last day)"},
		{"bad date", []string{"-on", "2026-13-01", "hyde park"}, 2, ""},
		{"unknown township", []string{"Springfield"}, 1, ""},
		{"table: open", []string{"-on", "2026-11-24"}, 0, "Hyde Park      open (15 days left)  Nov 9, 2026   Dec 8, 2026   Dec 18, 2026"},
		{"table: not posted", []string{"-on", "2026-11-24"}, 0, "Jefferson      not posted"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, out, stderr := run(t, "", append([]string{"township"}, tt.args...)...)
			if code != tt.code || !strings.Contains(out, tt.contains) {
				t.Errorf("exit %d, stdout %q, stderr %q; want exit %d with %q", code, out, stderr, tt.code, tt.contains)
			}
		})
	}
}

// TestCheckText covers the terminal report and how a query resolves to one
// PIN. The report's numbers are in the goldens (internal/golden).
func TestCheckText(t *testing.T) {
	db := fixture.Path(t)
	tests := []struct {
		args     []string
		code     int
		contains []string // in stdout, or stderr when code != 0
	}{
		// Figures the terminal report formats itself (the report's numbers are in the goldens).
		{[]string{"check", "13331030130000"}, 0, []string{
			"2329 N Lockwood Ave, Chicago", "(mailed value, not yet certified)", "30 of 30 similar homes",
			"Filed by homeowners: 61% got a reduction (170 appeals, 2023–2025); average cut 9%"}},
		// Two comps tie the subject exactly; only the 7 strictly lower count.
		{[]string{"check", "13331050170000"}, 0, []string{"Assessed lower         7 of 9 similar homes"}},
		{[]string{"check", "13-33-103-013-0000", "-year", "2025"}, 0, []string{"tax year 2025 (certified value)"}},
		// Address resolution.
		{[]string{"check", "2329", "n", "lockwood", "ave"}, 0, []string{"PIN 13-33-103-013-0000"}},
		{[]string{"check", "2106 n laramie"}, 1, []string{"13-33-112-041-1001", "13-33-112-041-1002", "13-33-112-041-1003"}},
		{[]string{"check", "99999 Nowhere"}, 1, []string{"no Cook County address"}},
	}
	for _, tt := range tests {
		code, out, stderr := run(t, db, tt.args...)
		text := out
		if tt.code != 0 {
			text = stderr
		}
		if code != tt.code {
			t.Errorf("%q: exit %d, want %d (stderr %q)", tt.args, code, tt.code, stderr)
			continue
		}
		for _, c := range tt.contains {
			if !strings.Contains(text, c) {
				t.Errorf("%q: output lacks %q:\n%s", tt.args, c, text)
			}
		}
	}
}

// TestCheckJSONBrief checks that -brief drops the full comp list but keeps
// the lower-assessed homes. Full -json output is in the goldens.
func TestCheckJSONBrief(t *testing.T) {
	code, out, stderr := run(t, fixture.Path(t), "check", "13331030130000", "-json", "-brief")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var r map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatal(err)
	}
	var evidence []json.RawMessage
	if err := json.Unmarshal(r["evidence_comps"], &evidence); err != nil {
		t.Fatal(err)
	}
	if _, ok := r["comps"]; ok || len(evidence) != 6 {
		t.Errorf("comps present = %v, %d lower-assessed homes; want no comps and 6", ok, len(evidence))
	}
}

func TestReportCommand(t *testing.T) {
	db := fixture.Path(t)
	dir := t.TempDir()
	out := filepath.Join(dir, "x.pdf")
	code, stdout, stderr := run(t, db, "report", "13331030130000", "-o", out) // flags after the PIN
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	b, err := os.ReadFile(out)
	if err != nil || !bytes.HasPrefix(b, []byte("%PDF-")) {
		t.Fatalf("not a PDF (%v)", err)
	}
	if stdout != out+"\n" {
		t.Errorf("stdout %q, want the output path", stdout)
	}
	// An unsupported property gets exit 1 and no file.
	condo := filepath.Join(dir, "condo.pdf")
	code, _, _ = run(t, db, "report", "-o", condo, "13331120411001")
	if _, err := os.Stat(condo); code != 1 || !errors.Is(err, os.ErrNotExist) {
		t.Errorf("condo: exit %d, file stat %v; want exit 1 and no file", code, err)
	}
}
