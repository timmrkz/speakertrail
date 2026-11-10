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
