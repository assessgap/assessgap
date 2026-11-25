package comps

import "testing"

func TestEstTaxChange(t *testing.T) {
	// Illustrative numbers only; they are not real Cook County rates.
	a := TaxInputs{Equalizer: 3.0, Rate: 0.07}
	b := TaxInputs{Equalizer: 3.0, Rate: 0.06}
	tests := []struct {
		name      string
		av, cut   float64
		scenarios []TaxInputs
		want      float64
	}{
		{"simple: 1000 × 3 × 7%", 30000, 1000, []TaxInputs{a}, 210},
		{"rounds to nearest $10", 30000, 1234, []TaxInputs{a}, 260}, // 259.14
		{"rounds to nearest $10 (175 → 180)", 30000, 1000, []TaxInputs{{Equalizer: 2.5, Rate: 0.07}}, 180},
		{"low end of range", 30000, 1000, []TaxInputs{a, b}, 180},
		{"exemption leaves taxable EAV positive: no effect", 30000, 1000, []TaxInputs{{3.0, 0.07, 10000}}, 210},
		{"fully exempt: no change", 4000, 1000, []TaxInputs{{3.0, 0.07, 20000}}, 0},
		// before = 6000×3 − 16000 = 2000; after = max(0, 5000×3 − 16000) = 0 → 2000 × 7%
		{"exemption floors taxable EAV at zero", 6000, 1000, []TaxInputs{{3.0, 0.07, 16000}}, 140},
		{"negative cut: no change", 30000, -1000, []TaxInputs{a}, 0},
		{"no scenarios", 30000, 1000, nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EstTaxChange(tt.av, tt.cut, tt.scenarios); got != tt.want {
				t.Errorf("EstTaxChange(%v, %v) = %v, want %v", tt.av, tt.cut, got, tt.want)
			}
		})
	}
}
