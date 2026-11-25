package main

import (
	"os"
	"strings"
	"testing"
)

func TestProblems(t *testing.T) {
	filled := "## Summary\nX\n\n## Numbers impact\nNone, goldens unchanged.\n\n## Risk and rollback\nLow. Revert the commit.\n"
	tests := []struct {
		name string
		body string
		want []string // substrings, one per problem, in order
	}{
		{"filled in", filled, nil},
		{"CRLF line endings", strings.ReplaceAll(filled, "\n", "\r\n"), nil},
		{"left as the template (comments only)",
			"## Numbers impact\n<!-- Required. -->\n\n## Risk and rollback\n<!-- Required.\n Level... -->\n",
			[]string{`"Numbers impact" is empty`, `"Risk and rollback" is empty`}},
		{"placeholder answers", "## Numbers impact\nTBD\n## Risk and rollback\n_N/A_\n",
			[]string{`"Numbers impact" is empty`, `"Risk and rollback" is empty`}},
		{"section deleted", "## Numbers impact\nNone, goldens unchanged.\n",
			[]string{`"Risk and rollback" is missing`}},
		{"no template at all", "Fixes a typo.", []string{`"Numbers impact" is missing`, `"Risk and rollback" is missing`}},
		// A "###" sub-heading inside a section is part of its text, not a new section.
		{"sub-heading inside", "## Numbers impact\n### check/in-line\nmedian 18.95 → 19.10\n## Risk and rollback\nLow.\n", nil},
	}
	for _, tt := range tests {
		got := problems(tt.body)
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
	if got := problems(string(b)); len(got) != 2 {
		t.Errorf("unedited template: %q, want both required sections reported", got)
	}
}
