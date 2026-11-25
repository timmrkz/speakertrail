package names_test

import (
	"slices"
	"testing"

	"github.com/timmrkz/speakertrail/internal/names"
)

// The names are invented, the titles, degrees and marks around them are
// the kinds LinkedIn shows.
var cases = []struct {
	name, first, family string
}{
	{"Mara Beispiel", "Mara", "Beispiel"},
	{"Dr. med. Louis Beispiel", "Louis", "Beispiel"},
	{"Dr. Mara Beispiel", "Mara", "Beispiel"},
	{"Dr Mara Beispiel", "Mara", "Beispiel"},
	{"Prof. Dr. Mara Beispiel", "Mara", "Beispiel"},
	{"Prof. Dr.-Ing. Mara Beispiel", "Mara", "Beispiel"},
	{"Dr.-Ing Mara Beispiel", "Mara", "Beispiel"},
	{"Dipl.-Ing. Mara Beispiel", "Mara", "Beispiel"},
	{"Dipl.-Kfm. Tom Testmann", "Tom", "Testmann"},
	{"Dr. rer. nat. Tom Testmann", "Tom", "Testmann"},
	{"Dr. med. dent. Tom Testmann", "Tom", "Testmann"},
	{"Univ.-Prof. Dr. Tom Testmann", "Tom", "Testmann"},
	{"Mag. Tom Testmann", "Tom", "Testmann"},
	{"Mag.a Mara Beispiel", "Mara", "Beispiel"},
	{"Mara Beispiel, MBA", "Mara", "Beispiel"},
	{"Mara Beispiel MBA", "Mara", "Beispiel"},
	{"Mara Beispiel, PhD", "Mara", "Beispiel"},
	{"Tom Testmann M.Sc.", "Tom", "Testmann"},
	{"Tom Testmann, LL.M.", "Tom", "Testmann"},
	{"Mara Beispiel (sie/ihr)", "Mara", "Beispiel"},
	{"Mara Beispiel | Coach für Stimme", "Mara", "Beispiel"},
	{"Mara Beispiel – Coach", "Mara", "Beispiel"},
	{"🎤 Mara Beispiel 🚀", "Mara", "Beispiel"},
	{"Hans-Peter Testmann", "Hans-Peter", "Testmann"},
	{"Mara von Beispiel", "Mara", "Beispiel"},
	{"Mara B.", "Mara", ""},
	{"Mara", "Mara", ""},
	{"  Mara   Beispiel  ", "Mara", "Beispiel"},
	{"Mara\nBeispiel", "Mara", "Beispiel"},
	// A title and one name leave no first name to be sure of, only the
	// family's.
	{"Dr. Beispiel", "", "Beispiel"},
	{"Dr. med.", "", ""},
	{"", "", ""},
}

func TestFirst(t *testing.T) {
	for _, c := range cases {
		if got := names.First(c.name); got != c.first {
			t.Errorf("First(%q) = %q, want %q", c.name, got, c.first)
		}
	}
}

func TestFamily(t *testing.T) {
	for _, c := range cases {
		if got := names.Family(c.name); got != c.family {
			t.Errorf("Family(%q) = %q, want %q", c.name, got, c.family)
		}
	}
}

func TestWords(t *testing.T) {
	if got := names.Words("Prof. Dr. Mara von Beispiel, MBA (sie/ihr)"); !slices.Equal(got, []string{"Mara", "von", "Beispiel"}) {
		t.Errorf("Words = %q", got)
	}
}
