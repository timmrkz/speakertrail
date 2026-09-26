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

func TestStartupLinks(t *testing.T) {
	body := `<header><nav><a href="https://partner.example/">Partner</a></nav></header>
<main>
<h2>Our startups</h2>
<a href="https://www.beispiel-robotics.example/?utm_source=hub"><img alt="Beispiel Robotics" src="a.png"></a>
<a href="https://probe.example/en/">Website</a>
<a href="https://probe.example/team">Probe Labs team</a>
<a href="https://blog.hub.example/story">Our story</a>
<a href="/startups/probe-labs">Probe Labs</a>
<a href="https://www.linkedin.com/company/beispiel">LinkedIn</a>
<a href="https://muster.example/news/2026/award">An award</a>
</main>
<footer><a href="https://bank.example/">Sponsored by a bank</a></footer>`
	got := StartupLinks(body, "https://hub.example/portfolio")
	want := []Startup{
		{Name: "Beispiel Robotics", Website: "https://www.beispiel-robotics.example/"},
		{Name: "Probe", Website: "https://probe.example/"},
	}
	if len(got) != len(want) {
		t.Fatalf("startups %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("startup %d: %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestPortfolioWithPagesAboutEachStartup(t *testing.T) {
	body := `<html><body><header><nav>
<a href="/en/program/incubator">Incubator</a><a href="/en/program/mentors">Mentors</a><a href="/en/program/prototyping">Prototyping</a>
</nav></header><main>
<a href="/en/startups/beispiel-robotics">Beispiel Robotics</a>
<a href="/en/startups/probe-labs"><img alt="" src="p.png"></a>
<a href="/en/startups/muster-health">Muster Health</a>
<a href="/en/news/2026">News</a>
<a href="/en/our-startups/p2">2</a>
<a href="/en/our-startups/p2">Next</a>
</main><footer><a href="/en/privacy">Privacy</a></footer></body></html>`
	pp := Portfolio(body, "https://hub.example/en/our-startups")
	var got []string
	for _, s := range pp.Startups {
		got = append(got, s.Name+" "+s.Page+" "+s.Website)
	}
	want := []string{
		"Beispiel Robotics https://hub.example/en/startups/beispiel-robotics ",
		"Probe Labs https://hub.example/en/startups/probe-labs ",
		"Muster Health https://hub.example/en/startups/muster-health ",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("startups:\n%s", strings.Join(got, "\n"))
	}
	if strings.Join(pp.More, " ") != "https://hub.example/en/our-startups/p2" {
		t.Errorf("more pages %q", pp.More)
	}
	// The second page links back to the first, which is not another page.
	if more := Portfolio(`<a href="/en/our-startups">1</a><a href="/en/our-startups/p3">3</a>`, "https://hub.example/en/our-startups/p2").More; strings.Join(more, " ") != "https://hub.example/en/our-startups/p3" {
		t.Errorf("more pages from page 2 %q", more)
	}

	about := `<header><a href="https://www.linkedin.com/company/hub">LinkedIn</a></header>
<main><a href="https://www.instagram.com/probe">Instagram</a><a href="https://blog.example/probe">A story</a>
<a href="https://probe-labs.example/de">Website</a></main>`
	if got := StartupWebsite(about, "https://hub.example/en/startups/probe-labs"); got != "https://probe-labs.example/" {
		t.Errorf("website %q", got)
	}
}
