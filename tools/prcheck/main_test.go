package main

import (
	"os"
	"strings"
	"testing"
)

func TestProblems(t *testing.T) {
	filled := "## Summary\nX\n\n## Numbers impact\nNone, goldens unchanged.\n\n## Risk and rollback\nLow. Revert the commit.\n"
	golden := []string{"internal/comps/select.go", "testdata/golden/check/in-line.txt"}
	tests := []struct {
		name    string
		body    string
		changed []string
		want    []string // substrings, one per problem, in order
	}{
		{"filled in", filled, nil, nil},
		{"CRLF line endings", strings.ReplaceAll(filled, "\n", "\r\n"), nil, nil},
		{"left as the template (comments only)",
			"## Numbers impact\n<!-- Required. -->\n\n## Risk and rollback\n<!-- Required.\n Level... -->\n", nil,
			[]string{`"Numbers impact" is empty`, `"Risk and rollback" is empty`}},
		{"placeholder answers", "## Numbers impact\nTBD\n## Risk and rollback\n_N/A_\n", nil,
			[]string{`"Numbers impact" is empty`, `"Risk and rollback" is empty`}},
		{"section deleted", "## Numbers impact\nNone, goldens unchanged.\n", nil,
			[]string{`"Risk and rollback" is missing`}},
		{"no template at all", "Fixes a typo.", nil, []string{`"Numbers impact" is missing`, `"Risk and rollback" is missing`}},
		// A "###" sub-heading inside a section is part of its text, not a new section.
		{"sub-heading inside", "## Numbers impact\n### check/in-line\nmedian 18.95 → 19.10\n## Risk and rollback\nLow.\n", golden, nil},
		// The bot's line-count table, appended after the last section, is not an answer.
		{"empty last section followed by the bot table",
			"## Numbers impact\nNone.\n## Risk and rollback\n<!-- Required. -->\n\n<!-- prloc:start -->\n### Lines changed\n| x |\n<!-- prloc:end -->\n", nil,
			[]string{`"Risk and rollback" is empty`}},
		{"says none but goldens changed", filled, golden, []string{"testdata/golden/check/in-line.txt changed"}},
	}
	for _, tt := range tests {
		got := problems(tt.body, tt.changed)
		if len(got) != len(tt.want) {
			t.Errorf("%s: problems %q, want %d", tt.name, got, len(tt.want))
			continue
		}
		for i := range got {
			if !strings.Contains(got[i], tt.want[i]) {
				t.Errorf("%s: problem %d = %q, want it to mention %q", tt.name, i, got[i], tt.want[i])
			}
		}
	}
}

// TestTemplateFailsUnedited checks the real template, unedited, is rejected.
func TestTemplateFailsUnedited(t *testing.T) {
	b, err := os.ReadFile("../../.github/pull_request_template.md")
	if err != nil {
		t.Fatal(err)
	}
	if got := problems(string(b), nil); len(got) != 2 {
		t.Errorf("unedited template: %q, want both required sections reported", got)
	}
}
