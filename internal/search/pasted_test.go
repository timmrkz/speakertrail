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
