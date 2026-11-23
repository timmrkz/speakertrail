package search_test

import (
	"strings"
	"testing"

	"github.com/timmrkz/speakertrail/internal/search"
)

// A LinkedIn page as Chrome copies it with ⌘A and ⌘C, of an invented
// person, in the German interface: the menu first, the name twice, and
// other people at its end.
const page = `Zum Hauptinhalt springen
Startseite
Mein Netzwerk
Nachrichten

Jörg Probe
Jörg Probe
Er/Ihm
· 1.
Inhaber | Probe Möbel, Bielefeld
Bielefeld, Nordrhein-Westfalen, Deutschland  Kontaktinfo
500+ Kontakte
Nachricht
Mehr
Info
Ich baue Möbel aus Eiche. Schreib mir: joerg@probe-moebel.example oder 0521 1234567
…mehr anzeigen
Berufserfahrung
Inhaber
Probe Möbel · Selbstständig
Jan. 2021–Heute · 5 J. 11 Mon.
Alle 4 Berufserfahrungen anzeigen
Weitere Profile für Sie
Lena Muster
Coach`

func TestPasted(t *testing.T) {
	p := search.Pasted(page, "https://www.linkedin.com/in/joerg-probe-7a1b2c3")
	if p.Name != "Jörg Probe" || p.Headline != "Inhaber | Probe Möbel, Bielefeld" || p.Location != "Bielefeld, Nordrhein-Westfalen, Deutschland" {
		t.Errorf("read %q, %q, %q", p.Name, p.Headline, p.Location)
	}
	if !strings.HasPrefix(p.Text, "Jörg Probe\nEr/Ihm") || !strings.Contains(p.Text, "Probe Möbel · Selbstständig") {
		t.Errorf("text %q", p.Text)
	}
	for _, left := range []string{"Startseite", "Lena Muster", "joerg@", "1234567", "Nachricht\n", "mehr anzeigen", "Alle 4"} {
		if strings.Contains(p.Text, left) {
			t.Errorf("the text keeps %q:\n%s", left, p.Text)
		}
	}
	// A page of someone else keeps its text but names nobody, so the
	// person keeps their name.
	if q := search.Pasted(page, "https://www.linkedin.com/in/ida-probe"); q.Name != "" || q.Headline != "" || !strings.Contains(q.Text, "Probe Möbel") {
		t.Errorf("another's page %+v", q)
	}
	// A long page is cut to what a first message is written from.
	if long := search.Pasted(page+strings.Repeat("\nEin Beitrag über Eiche", 1000), "https://www.linkedin.com/in/joerg-probe"); len([]rune(long.Text)) > search.TextChars {
		t.Errorf("kept %d characters", len([]rune(long.Text)))
	}
}

// A headline that names a city stays the headline, the place is the line
// after it, and a name with a title before it or more after it is still
// theirs.
func TestPastedHeadlinesAndNames(t *testing.T) {
	for _, c := range []struct{ page, link, name, headline, location string }{
		{"Menü\nMara Beispiel\nFotografin in Köln\nKöln, Nordrhein-Westfalen, Deutschland\nInfo", "mara-beispiel", "Mara Beispiel", "Fotografin in Köln", "Köln, Nordrhein-Westfalen, Deutschland"},
		{"Mara Beispiel\nInhaberin Studio Essen\nEssen, North Rhine-Westphalia, Germany", "mara-beispiel-71", "Mara Beispiel", "Inhaberin Studio Essen", "Essen, North Rhine-Westphalia, Germany"},
		{"Mara Beispiel\nSie/Ihr\nCoach für Teams im Ruhrgebiet\nRuhr Region", "mara-beispiel", "Mara Beispiel", "Coach für Teams im Ruhrgebiet", "Ruhr Region"},
		{"Menü\nDr. Mara Beispiel, MBA\nÄrztin\nBonn, Nordrhein-Westfalen, Deutschland", "mara-beispiel", "Dr. Mara Beispiel, MBA", "Ärztin", "Bonn, Nordrhein-Westfalen, Deutschland"},
		{"Menü\nMara Beispiel | Coach 🌱\nCoach\nBerlin, Berlin, Deutschland", "mara-beispiel", "Mara Beispiel", "Coach", "Berlin, Berlin, Deutschland"},
		{"Mara Beispiel\nBielefeld, Nordrhein-Westfalen, Deutschland\nInfo", "mara-beispiel", "Mara Beispiel", "", "Bielefeld, Nordrhein-Westfalen, Deutschland"},
	} {
		p := search.Pasted(c.page, "https://www.linkedin.com/in/"+c.link)
		if p.Name != c.name || p.Headline != c.headline || p.Location != c.location {
			t.Errorf("%q: read %q, %q, %q", c.page, p.Name, p.Headline, p.Location)
		}
		if strings.HasPrefix(p.Text, "Menü") {
			t.Errorf("%q: the text keeps the menu", c.page)
		}
	}
}
