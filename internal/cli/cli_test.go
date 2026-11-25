package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseArgs(t *testing.T) {
	tests := []struct {
		args    []string
		pos     []string
		out     string
		year    int
		wantErr bool
	}{
		{[]string{"13281000270000", "-o", "x.pdf"}, []string{"13281000270000"}, "x.pdf", 0, false},
		{[]string{"-o", "x.pdf", "13281000270000"}, []string{"13281000270000"}, "x.pdf", 0, false},
		{[]string{"5555", "W", "-year", "2025", "Belmont", "-o=y.pdf"}, []string{"5555", "W", "Belmont"}, "y.pdf", 2025, false},
		{[]string{"-o", "x.pdf", "--", "-weird", "-year"}, []string{"-weird", "-year"}, "x.pdf", 0, false},
		{[]string{}, nil, "", 0, false},
		{[]string{"pin", "-nope"}, nil, "", 0, true},
		{[]string{"pin", "-year", "abc"}, nil, "", 0, true},
	}
	for _, tt := range tests {
		fs := flag.NewFlagSet("t", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		out := fs.String("o", "", "")
		year := fs.Int("year", 0, "")
		pos, err := parseArgs(fs, tt.args)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseArgs(%q) err = %v, wantErr %v", tt.args, err, tt.wantErr)
			continue
		}
		if tt.wantErr {
			continue
		}
		if !reflect.DeepEqual(pos, tt.pos) || *out != tt.out || *year != tt.year {
			t.Errorf("parseArgs(%q) = %q, -o %q, -year %d; want %q, %q, %d", tt.args, pos, *out, *year, tt.pos, tt.out, tt.year)
		}
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

// run executes the CLI with ASSESSGAP_DB set to db and returns status and output.
func run(t *testing.T, db string, args ...string) (int, string, string) {
	t.Helper()
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

func TestRunWithoutDB(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "none.db")
	tests := []struct {
		args     []string
		code     int
		contains string // in stderr
	}{
		{nil, 2, "Commands:"},
		{[]string{"bogus"}, 2, "unknown command"},
		{[]string{"check"}, 2, "usage: assessgap check"},
		{[]string{"check", "13281000270000"}, 1, "assessgap db sync"},
		{[]string{"report", "13281000270000"}, 2, "usage: assessgap report"},
		{[]string{"township", "Springfield"}, 1, "no Cook County township"},
		{[]string{"db"}, 2, "Steps:"},
	}
	for _, tt := range tests {
		code, _, stderr := run(t, missing, tt.args...)
		if code != tt.code || !strings.Contains(stderr, tt.contains) {
			t.Errorf("assessgap %q: exit %d, stderr %q; want exit %d containing %q", tt.args, code, stderr, tt.code, tt.contains)
		}
	}
}

func TestTownship(t *testing.T) {
	code, out, _ := run(t, "", "township", "-on", "2026-11-24", "hyde park")
	if code != 0 || !strings.Contains(out, "Hyde Park Township") || !strings.Contains(out, "open now") {
		t.Errorf("exit %d: %q", code, out)
	}
	code, out, _ = run(t, "", "township", "-on", "2026-11-24")
	if code != 0 || strings.Count(out, "\n") < 39 || !strings.Contains(out, "Jefferson") {
		t.Errorf("exit %d: %q", code, out)
	}
}

const sampleDB = "../../data-sample/cook.db"

func needSample(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(sampleDB); err != nil {
		t.Skipf("sample DB missing (%v); see README to build it", err)
	}
}

func TestCheckSample(t *testing.T) {
	needSample(t)
	tests := []struct {
		args     []string
		code     int
		contains []string // in stdout, or stderr when code != 0
	}{
		{[]string{"check", "13281000290000"}, 0, []string{
			"3134 N Luna Ave", "PIN 13-28-100-029-0000", "Jefferson Township", "Board of Review:", "per sq ft",
			"Similar homes assessed lower", "Filed by homeowners", "not an appraisal"}},
		{[]string{"check", "13-28-100-029-0000", "-year", "2026"}, 0, []string{"tax year 2026"}},
		{[]string{"check", "3134", "n", "luna", "ave"}, 0, []string{"PIN 13-28-100-029-0000"}},
		{[]string{"check", "5555 w belmont"}, 1, []string{"matches several", "13-28-100-001-0000"}},
		{[]string{"check", "99999 Nowhere"}, 1, []string{"no Cook County address"}},
		{[]string{"check", "13281010421001"}, 0, []string{"can't compare", "Condominium"}},
	}
	for _, tt := range tests {
		code, out, stderr := run(t, sampleDB, tt.args...)
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

func TestCheckJSON(t *testing.T) {
	needSample(t)
	code, out, stderr := run(t, sampleDB, "check", "13281000290000", "-json", "-brief")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var r struct {
		PIN      string            `json:"pin"`
		Verdict  string            `json:"verdict"`
		Evidence []json.RawMessage `json:"evidence_comps"`
		Comps    []json.RawMessage `json:"comps"`
	}
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatal(err)
	}
	if r.PIN != "13281000290000" || r.Verdict == "" || len(r.Evidence) == 0 || r.Comps != nil {
		t.Errorf("got %+v", r)
	}
}

func TestReportSample(t *testing.T) {
	needSample(t)
	out := filepath.Join(t.TempDir(), "x.pdf")
	code, _, stderr := run(t, sampleDB, "report", "13281000270000", "-o", out) // flags after the PIN
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	b, err := os.ReadFile(out)
	if err != nil || !bytes.HasPrefix(b, []byte("%PDF-")) {
		t.Fatalf("not a PDF (%v)", err)
	}
	code, _, stderr = run(t, sampleDB, "report", "-o", out, "13281010421001")
	if code != 1 || !strings.Contains(stderr, "no evidence report") {
		t.Errorf("condo: exit %d, %q", code, stderr)
	}
}
