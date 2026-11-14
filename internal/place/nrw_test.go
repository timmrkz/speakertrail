package place

import "testing"

func TestInNRW(t *testing.T) {
	for pc, want := range map[string]bool{
		"50667": true, "40210": true, "44137": true, "48143": true, "52062": true, "53111": true, "33602": true,
		"32423": true, "57072": true, "59065": true, "34414": true, "37671": true, "49477": true, "53721": true,
		"53604": true, "48477": true,
		// Next door: Lower Saxony, Hesse, Rhineland-Palatinate.
		"49074": false, "31134": false, "34117": false, "37073": false, "53424": false, "53474": false,
		"53619": false, "57518": false, "48455": false, "48529": false, "54290": false, "80331": false,
		"": false, "5066": false, "abcde": false,
	} {
		if got := InNRW(pc); got != want {
			t.Errorf("InNRW(%q) = %v, want %v", pc, got, want)
		}
	}
}

// Places as LinkedIn profiles give them, from the test with Exa.
func TestPlaceInNRW(t *testing.T) {
	for place, want := range map[string]string{
		"Cologne, North Rhine-Westphalia, Germany": "Köln",
		"Pulheim, North Rhine-Westphalia, Germany": "Pulheim",
		"Kamen, North Rhine-Westphalia, Germany":   "Kamen",
		"Münster, North Rhine-Westphalia, Germany": "Münster",
		"Cologne Bonn Region":                      "Köln",
		"Greater Dusseldorf Area":                  "Düsseldorf",
		"Ruhr Region":                              "",
		"North Rhine-Westphalia, Germany":          "",
		"Düsseldorf, Germany":                      "Düsseldorf",
		"Berlin Metropolitan Area":                 "-",
		"Munich, Bavaria, Germany":                 "-",
		"Münster, Hesse, Germany":                  "-",
		"Fulda, Hesse, Germany":                    "-",
		"Osnabrück Land":                           "-",
		"Vienna, Vienna, Austria":                  "-",
		"Blumenstein, Berne, Switzerland":          "-",
		"Germany":                                  "-",
		"":                                         "-",
		"Greater Copenhagen Region":                "-",
		"Hessen, Germany":                          "-",
		"Hamburg, Hamburg, Germany":                "-",
		"Wittlich, Rhineland-Palatinate, Germany":  "-",
		"Greater Munich Metropolitan Area":         "-",
		"Greater Hamburg Area":                     "-",
	} {
		city, ok := PlaceInNRW(place)
		got := city
		if !ok {
			got = "-"
		}
		if got != want {
			t.Errorf("PlaceInNRW(%q) = %q, %v, want %q", place, city, ok, want)
		}
	}
}
