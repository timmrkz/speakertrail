package extract

import (
	"strings"
	"testing"
)

// All companies, people and addresses here are invented.

func TestImprintLink(t *testing.T) {
	for name, tc := range map[string]struct{ body, want string }{
		"by text": {`<footer><a href="/datenschutz">Datenschutz</a><a href="/rechtliches/angaben">Impressum</a></footer>`,
			"https://beispiel.example/rechtliches/angaben"},
		"by path":   {`<a href="https://www.beispiel.example/de/impressum/">§</a>`, "https://www.beispiel.example/de/impressum/"},
		"english":   {`<a href="/about">About</a><a href="/notice">Legal notice</a>`, "https://beispiel.example/notice"},
		"elsewhere": {`<a href="https://baukasten.example/impressum">Impressum</a>`, ""},
		"none":      {`<a href="/team">Team</a>`, ""},
	} {
		if got := ImprintLink(tc.body, "https://beispiel.example/"); got != tc.want {
			t.Errorf("%s: %q, want %q", name, got, tc.want)
		}
	}
}

func TestParseImprint(t *testing.T) {
	for name, tc := range map[string]struct {
		text      string
		company   string
		form      string
		city      string
		directors []string
		young     bool
	}{
		"label and names on one line": {
			text: `Impressum
Angaben gemäß § 5 DDG
Backstube Muster GmbH
Musterstraße 12
50667 Köln
Geschäftsführer: Lena Musterfrau, Tom Testmann
Kontakt: Telefon: +49 221 000000, E-Mail: hallo@backstube.example
Registergericht: Amtsgericht Köln, HRB 000000`,
			company: "Backstube Muster GmbH", form: "GmbH", city: "Köln",
			directors: []string{"Lena Musterfrau", "Tom Testmann"}, young: true,
		},
		"vertreten durch with roles": {
			text: `Kontrolle Robotics UG (haftungsbeschränkt)
Beispielweg 3, 52062 Aachen
Vertreten durch die Geschäftsführer Karla Kontrolle (CEO) und Dr. Jonas Beispielmann (CTO)`,
			company: "Kontrolle Robotics UG (haftungsbeschränkt)", form: "UG (haftungsbeschränkt)", city: "Aachen",
			directors: []string{"Karla Kontrolle", "Dr. Jonas Beispielmann"}, young: true,
		},
		"label above the names": {
			text: `Probe Labs GmbH
44137 Dortmund

Geschäftsführung:
Mara Beispielfrau
Kofi Probemann

Umsatzsteuer-ID: DE000000000`,
			company: "Probe Labs GmbH", form: "GmbH", city: "Dortmund",
			directors: []string{"Mara Beispielfrau", "Kofi Probemann"}, young: true,
		},
		"english": {
			text: `Legal notice
Sample Health GmbH, Beispielallee 1, 40210 Düsseldorf
Managing Directors: Anna-Lena von Beispiel & Jürgen Weß`,
			company: "Sample Health GmbH", form: "GmbH", city: "Düsseldorf",
			directors: []string{"Anna-Lena von Beispiel", "Jürgen Weß"}, young: true,
		},
		"a savings bank": {
			text: `Sparkasse Musterstadt
Anstalt des öffentlichen Rechts
Vorstand: Erika Erfunden, Max Mustermann`,
			directors: []string{"Erika Erfunden", "Max Mustermann"}, young: false,
		},
		"a stock company": {
			text: `Beispiel Holding AG
Geschäftsführer: Lena Musterfrau`,
			company: "Beispiel Holding AG", form: "AG",
			directors: []string{"Lena Musterfrau"}, young: false,
		},
		"a company that runs it": {
			text: `Muster Projekt GmbH & Co. KG
Vertreten durch: Muster Verwaltungs GmbH`,
			company: "Muster Projekt GmbH & Co. KG", form: "GmbH & Co. KG", young: false,
		},
		"a photo credit is nobody": {
			text: `Beispiel Studio GmbH
Vertreten durch den Geschäftsführer: Lena Musterfrau
Inhaber der Bildrechte: Tom Testmann`,
			company: "Beispiel Studio GmbH", form: "GmbH",
			directors: []string{"Lena Musterfrau"}, young: true,
		},
		"a sole trader": {
			text: `Inhaberin: Karla Kontrolle
Kontrolle Keramik
33602 Bielefeld`,
			city: "Bielefeld", directors: []string{"Karla Kontrolle"}, young: true,
		},
	} {
		im := ParseImprint(tc.text)
		if im.Company != tc.company || im.LegalForm != tc.form || im.City != tc.city {
			t.Errorf("%s: company %q form %q city %q", name, im.Company, im.LegalForm, im.City)
		}
		if strings.Join(im.Directors, "|") != strings.Join(tc.directors, "|") {
			t.Errorf("%s: directors %q, want %q", name, im.Directors, tc.directors)
		}
		if im.Young() != tc.young {
			t.Errorf("%s: young %v", name, im.Young())
		}
		for _, d := range im.Directors {
			if strings.ContainsAny(d, "@0123456789+") {
				t.Errorf("%s: contact details in a name: %q", name, d)
			}
		}
	}
}

func TestLinksMarksChromeAndReadsLogos(t *testing.T) {
	body := `<header><a href="/">Home</a></header>
<main><a href="https://beispiel.example"><img src="logo.png" alt="Beispiel Robotics"></a>
<a href="https://probe.example/?utm_source=portfolio">Probe Labs</a></main>
<footer><a href="https://bank.example">Our partner</a></footer>`
	links := Links(body, "https://hub.example/portfolio")
	if len(links) != 4 {
		t.Fatalf("links %+v", links)
	}
	if !links[0].Chrome || links[1].Chrome || links[2].Chrome || !links[3].Chrome {
		t.Errorf("chrome %+v", links)
	}
	if links[1].Text != "Beispiel Robotics" || links[2].URL != "https://probe.example/" {
		t.Errorf("links %+v", links)
	}
}
