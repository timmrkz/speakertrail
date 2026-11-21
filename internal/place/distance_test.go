package place

import (
	"math"
	"testing"
)

func TestKm(t *testing.T) {
	for _, c := range NRWCities {
		if _, ok := centres[c]; !ok {
			t.Errorf("%s has no centre, so nobody there has a distance", c)
		}
	}
	// Straight lines between city centres, as a map measures them.
	for _, tc := range []struct {
		a, b string
		km   float64
	}{
		{"Köln", "Köln", 0},
		{"Köln", "Bonn", 25},
		{"Köln", "Düsseldorf", 34},
		{"Köln", "Dortmund", 73},
		{"Köln", "Münster", 123},
	} {
		got, ok := Km(tc.a, tc.b)
		if !ok || math.Abs(got-tc.km) > 3 {
			t.Errorf("Km(%s, %s) = %.1f, %v, want about %.0f", tc.a, tc.b, got, ok, tc.km)
		}
		if back, _ := Km(tc.b, tc.a); math.Abs(back-got) > 1e-9 {
			t.Errorf("Km(%s, %s) = %.1f, but back %.1f", tc.a, tc.b, got, back)
		}
	}
	for _, tc := range [][2]string{{"Köln", "Berlin"}, {"", "Köln"}, {"Ruhr Region", "Essen"}} {
		if _, ok := Km(tc[0], tc[1]); ok {
			t.Errorf("Km(%q, %q) knows a distance between places it does not know", tc[0], tc[1])
		}
	}
}
