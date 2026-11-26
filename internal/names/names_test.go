package names_test

import (
	"bufio"
	"os"
	"strings"
	"testing"

	"github.com/timmrkz/speakertrail/internal/names"
)

// row is one line of testdata/names.tsv.
type row struct {
	name, handle, first, family string
}

// rows reads the invented names in testdata: names.tsv, written with the
// rules, and blind.tsv and realistic.tsv, written blind to measure them.
func rows(t *testing.T) []row {
	t.Helper()
	var out []row
	for _, file := range []string{"testdata/names.tsv", "testdata/blind.tsv", "testdata/realistic.tsv"} {
		out = append(out, read(t, file)...)
	}
	if len(out) < 600 {
		t.Fatalf("only %d names", len(out))
	}
	return out
}

func read(t *testing.T, file string) []row {
	t.Helper()
	f, err := os.Open(file)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []row
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := s.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		c := strings.Split(line, "\t")
		if len(c) != 4 {
			t.Fatalf("%q has %d columns, want 4", line, len(c))
		}
		out = append(out, row{c[0], c[1], c[2], c[3]})
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func link(handle string) string {
	if handle == "" {
		return ""
	}
	return "https://www.linkedin.com/in/" + handle
}

func TestFirst(t *testing.T) {
	for _, r := range rows(t) {
		if got := names.First(r.name, link(r.handle)); got != r.first {
			t.Errorf("First(%q, %q) = %q, want %q", r.name, r.handle, got, r.first)
		}
	}
}

func TestFamily(t *testing.T) {
	for _, r := range rows(t) {
		if r.family == "?" {
			continue
		}
		if got := names.Family(r.name); got != r.family {
			t.Errorf("Family(%q) = %q, want %q", r.name, got, r.family)
		}
	}
}

// The link says the order of a name only where it joins two of its
// words, so a link made from a business that holds one of them changes
// nothing.
func TestALinkWithOneWordOfTheNameSaysNothing(t *testing.T) {
	for handle, want := range map[string]string{
		"beispiel-coaching":     "Mara",
		"coach-mara":            "Mara",
		"mara-beispiel":         "Mara",
		"beispiel-mara-1a2b3c4": "Beispiel",
		"%C3%B6rnek-mara":       "Mara",
	} {
		if got := names.First("Mara Beispiel", link(handle)); got != want {
			t.Errorf("Mara Beispiel at /in/%s: %q, want %q", handle, got, want)
		}
	}
	for _, l := range []string{"", "not a link", "https://www.linkedin.com/company/beispiel", "https://www.linkedin.com/in/"} {
		if got := names.First("Mara Beispiel", l); got != "Mara" {
			t.Errorf("Mara Beispiel at %q: %q", l, got)
		}
	}
}
