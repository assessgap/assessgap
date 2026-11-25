// Package golden compares homeowner-facing output with committed files in
// testdata/golden, so any change to a number shows up as a reviewable diff.
// Only tests import it.
//
// Structured output (JSON) is flattened to one "path = value" line per field,
// with floats rounded to 6 decimal places, so a diff names the field that
// changed. Text output is compared line by line.
//
// Rewrite the files deliberately with
//
//	go test ./internal/golden ./internal/web -run Golden -update
package golden

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files in testdata/golden")

// Dir is testdata/golden at the repository root.
var Dir = func() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "golden")
}()

// maxDiffLines caps the differences printed per file.
const maxDiffLines = 40

// Flatten turns a JSON document into sorted "path = value" lines: object keys
// in sorted order, array elements by index, strings quoted, floats rounded to
// 6 decimal places.
func Flatten(doc []byte) ([]string, error) {
	dec := json.NewDecoder(bytes.NewReader(doc))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	var out []string
	flatten(&out, "", v)
	return out, nil
}

func flatten(out *[]string, path string, v any) {
	switch x := v.(type) {
	case map[string]any:
		if len(x) == 0 {
			*out = append(*out, path+" = {}")
		}
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			p := k
			if path != "" {
				p = path + "." + k
			}
			flatten(out, p, x[k])
		}
	case []any:
		if len(x) == 0 {
			*out = append(*out, path+" = []")
		}
		for i, e := range x {
			flatten(out, fmt.Sprintf("%s[%d]", path, i), e)
		}
	case json.Number:
		*out = append(*out, path+" = "+number(x))
	case string:
		*out = append(*out, path+" = "+strconv.Quote(x))
	case nil:
		*out = append(*out, path+" = null")
	default:
		*out = append(*out, fmt.Sprintf("%s = %v", path, x))
	}
}

// number prints integers as-is and rounds other numbers to 6 decimal places,
// so the last bits of a float can't make a golden flaky across platforms.
func number(n json.Number) string {
	s := n.String()
	if !strings.ContainsAny(s, ".eE") {
		return s
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return s
	}
	r := strconv.FormatFloat(f, 'f', 6, 64)
	r = strings.TrimRight(strings.TrimRight(r, "0"), ".")
	if r == "-0" {
		r = "0"
	}
	return r
}

// CheckFields compares "path = value" lines with the golden file at
// testdata/golden/name. On mismatch it reports each changed field as
// "label: path: old → new". header is written as a comment at the top.
func CheckFields(t *testing.T, name, label, header string, got []string) {
	t.Helper()
	path := filepath.Join(Dir, name)
	if *update {
		write(t, path, header, got)
		return
	}
	want := read(t, path)
	if diffs := fieldDiff(want, got); len(diffs) > 0 {
		t.Errorf("%s: %d field(s) differ from %s (rerun with -update if the change is intended):\n%s",
			label, len(diffs), rel(path), strings.Join(cap(diffs), "\n"))
	}
}

// CheckLines compares text with the golden file line by line.
func CheckLines(t *testing.T, name, label, header string, got []string) {
	t.Helper()
	path := filepath.Join(Dir, name)
	if *update {
		write(t, path, header, got)
		return
	}
	want := read(t, path)
	if diffs := lineDiff(want, got); len(diffs) > 0 {
		t.Errorf("%s: text differs from %s (rerun with -update if the change is intended):\n%s",
			label, rel(path), strings.Join(cap(diffs), "\n"))
	}
}

func rel(path string) string {
	if i := strings.Index(path, "testdata"); i >= 0 {
		return path[i:]
	}
	return path
}

func cap(lines []string) []string {
	if len(lines) <= maxDiffLines {
		return lines
	}
	return append(lines[:maxDiffLines:maxDiffLines], fmt.Sprintf("  … and %d more", len(lines)-maxDiffLines))
}

func write(t *testing.T, path, header string, lines []string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, h := range strings.Split(header, "\n") {
		b.WriteString("# " + h + "\n")
	}
	for _, l := range lines {
		b.WriteString(l + "\n")
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

// read returns the file's lines without "# " comment lines.
func read(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (create it with -update)", err)
	}
	var out []string
	for _, l := range strings.Split(strings.TrimSuffix(string(b), "\n"), "\n") {
		if !strings.HasPrefix(l, "# ") {
			out = append(out, l)
		}
	}
	return out
}

// fieldDiff lists fields whose value changed, appeared or disappeared.
func fieldDiff(want, got []string) []string {
	split := func(lines []string) (map[string]string, []string) {
		m := make(map[string]string, len(lines))
		var order []string
		for _, l := range lines {
			k, v, _ := strings.Cut(l, " = ")
			if _, dup := m[k]; !dup {
				order = append(order, k)
			}
			m[k] = v
		}
		return m, order
	}
	w, worder := split(want)
	g, gorder := split(got)
	var out []string
	for _, k := range worder {
		gv, ok := g[k]
		switch {
		case !ok:
			out = append(out, fmt.Sprintf("  %s: %s → (missing)", k, w[k]))
		case gv != w[k]:
			out = append(out, fmt.Sprintf("  %s: %s → %s", k, w[k], gv))
		}
	}
	for _, k := range gorder {
		if _, ok := w[k]; !ok {
			out = append(out, fmt.Sprintf("  %s: (new) → %s", k, g[k]))
		}
	}
	return out
}

// lineDiff is a longest-common-subsequence diff: "- " lines are only in want,
// "+ " lines only in got.
func lineDiff(want, got []string) []string {
	n, m := len(want), len(got)
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if want[i] == got[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var out []string
	i, j := 0, 0
	for i < n || j < m {
		switch {
		case i < n && j < m && want[i] == got[j]:
			i++
			j++
		case j < m && (i == n || lcs[i][j+1] >= lcs[i+1][j]):
			out = append(out, fmt.Sprintf("  line %d + %s", j+1, got[j]))
			j++
		default:
			out = append(out, fmt.Sprintf("  line %d - %s", i+1, want[i]))
			i++
		}
	}
	return out
}
