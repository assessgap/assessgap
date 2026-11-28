package format

import "testing"

func TestPIN(t *testing.T) {
	tests := []struct{ in, want string }{
		{"13281000270000", "13-28-100-027-0000"},
		{"1328", "1328"},
	}
	for _, tt := range tests {
		if got := PIN(tt.in); got != tt.want {
			t.Errorf("PIN(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
