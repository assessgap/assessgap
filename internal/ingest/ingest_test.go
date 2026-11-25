package ingest

import (
	"strings"
	"testing"
)

func TestSelect(t *testing.T) {
	tests := []struct {
		names   []string
		prefix  string
		wantN   int
		wantErr bool
		where0  string
	}{
		{nil, "", len(Datasets), false, ""},
		{[]string{"sales"}, "", 1, false, "year >= '2018'"},
		{[]string{"parcel_universe"}, "1328", 1, false, "starts_with(pin, '1328')"},
		{[]string{"sales"}, "1328", 1, false, "(year >= '2018') AND (starts_with(pin, '1328'))"},
		{[]string{"nope"}, "", 0, true, ""},
		{nil, "13'; --", 0, true, ""},
	}
	for _, tt := range tests {
		got, err := Select(tt.names, tt.prefix)
		if (err != nil) != tt.wantErr {
			t.Fatalf("Select(%v, %q) err = %v", tt.names, tt.prefix, err)
		}
		if len(got) != tt.wantN {
			t.Fatalf("Select(%v, %q) = %d datasets, want %d", tt.names, tt.prefix, len(got), tt.wantN)
		}
		if tt.wantN > 0 && got[0].Where != tt.where0 {
			t.Errorf("Select(%v, %q)[0].Where = %q, want %q", tt.names, tt.prefix, got[0].Where, tt.where0)
		}
	}
	// Select must not modify the package-level list.
	if Datasets[0].Where != "" {
		t.Errorf("Datasets mutated: %q", Datasets[0].Where)
	}
}

func TestSplitSQL(t *testing.T) {
	stmts := SplitSQL("-- comment\nCREATE TABLE a (x);\n\nINSERT INTO a\nVALUES (1);\nANALYZE")
	if len(stmts) != 3 {
		t.Fatalf("%d statements: %q", len(stmts), stmts)
	}
	if !strings.HasPrefix(stmts[1], "INSERT") || !strings.Contains(stmts[1], "VALUES") {
		t.Errorf("statement 2 = %q", stmts[1])
	}
	if n := len(SplitSQL(buildSQL)); n < 20 {
		t.Errorf("build.sql splits into %d statements", n)
	}
}
