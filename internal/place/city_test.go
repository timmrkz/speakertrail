package place

import "testing"

func TestCityOf(t *testing.T) {
	for in, want := range map[string]string{
		"Im Mediapark 5, 50670 Köln":     "Köln",
		"Cologne, Germany":               "Köln",
		"Friedrich-Ebert-Str. 18, Essen": "Essen",
		"Hauptstr. 1, 12345 Berlin":      "Berlin",
		"Mülheim an der Ruhr":            "Mülheim an der Ruhr",
		"Kölner Straße 5, Hürth":         "Hürth",
		"Yogalehrerin Bonn":              "Bonn",
		"":                               "",
	} {
		if got := CityOf(in); got != want {
			t.Errorf("CityOf(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSplit(t *testing.T) {
	for _, tc := range []struct{ query, kind, city string }{
		{"Yogalehrerin Köln", "Yogalehrerin", "Köln"},
		{"Gründerin direkt nach dem Studium  NRW", "Gründerin direkt nach dem Studium", "NRW"},
		{"founder under 30 cologne", "founder under 30", "Köln"},
		{"Personal Trainer Mülheim an der Ruhr", "Personal Trainer", "Mülheim an der Ruhr"},
		{"Coach Bergisch Gladbach", "Coach", "Bergisch Gladbach"},
		{"Physiotherapie eigene Praxis", "Physiotherapie eigene Praxis", ""},
		// A place alone is no kind of person.
		{"Köln", "Köln", ""},
		// Only the end counts.
		{"Kölner Gründer Bonn", "Kölner Gründer", "Bonn"},
	} {
		kind, city := Split(tc.query)
		if kind != tc.kind || city != tc.city {
			t.Errorf("%q: %q %q, want %q %q", tc.query, kind, city, tc.kind, tc.city)
		}
	}
}

func TestBigCitiesLie(t *testing.T) {
	for _, c := range NRWCities[:BigCities] {
		if _, ok := Km(c, "Köln"); !ok {
			t.Errorf("%s has no centre", c)
		}
	}
}
