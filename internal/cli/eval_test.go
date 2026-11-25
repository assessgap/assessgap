package cli

import "testing"

func TestAUC(t *testing.T) {
	w := func(g float64) scored { return scored{g, true} }
	l := func(g float64) scored { return scored{g, false} }
	tests := []struct {
		name string
		in   []scored
		want float64
	}{
		// Every winner above every loser.
		{"perfect", []scored{l(0.1), l(0.2), w(0.3), w(0.4)}, 1},
		{"reversed", []scored{w(0.1), w(0.2), l(0.3), l(0.4)}, 0},
		// Winner/loser pairs: (0.1,0.2)=0, (0.1,0.3)=0, (0.3,0.2)=1, (0.3,0.3)=0.5 → 1.5/4.
		{"tie counts half", []scored{w(0.1), l(0.2), w(0.3), l(0.3)}, 0.375},
		{"no losers", []scored{w(0.1), w(0.2)}, 0.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := auc(tt.in); got != tt.want {
				t.Errorf("auc = %v, want %v", got, tt.want)
			}
		})
	}
}
