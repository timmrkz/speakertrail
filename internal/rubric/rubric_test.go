package rubric_test

import (
	"strings"
	"testing"

	"github.com/timmrkz/speakertrail/internal/rubric"
)

// Every person, company and passage here is invented.
func TestSignals(t *testing.T) {
	for _, c := range []struct {
		name   string
		person rubric.Person
		want   []string
	}{{
		name: "a coach who hosts her own workshops",
		person: rubric.Person{
			Headline: "Life Coach und Autorin, Praxis Musterfrau",
			Appearances: []rubric.Appearance{
				{Role: "host", Event: "Atem-Workshop im Beispielhaus", Evidence: "Durch den Abend führt Lena Musterfrau, die den Workshop seit 2019 anbietet."},
				{Role: "host", Event: "Atem-Workshop im Beispielhaus", Evidence: ""},
			},
		},
		want: []string{
			"+works_with_people: Life Coach und Autorin, Praxis Musterfrau",
			"+runs_events: Durch den Abend führt Lena Musterfrau, die den Workshop seit 2019 anbietet.",
			"+author: Life Coach und Autorin, Praxis Musterfrau",
		},
	}, {
		name: "the owner of a jiu-jitsu gym, by its imprint, whose site changed lately",
		person: rubric.Person{
			Headline:    "Owner, Beispiel BJJ Academy",
			FitEvidence: "Owner of Beispiel BJJ Academy, by its imprint",
			Companies:   []rubric.Company{{Name: "Beispiel BJJ Academy", Activity: "active", ActivityNote: "the website changed August 2026, by its sitemap", Founders: 1}},
		},
		want: []string{
			"+works_with_people: Owner, Beispiel BJJ Academy",
			"+owner_operator: Owner, Beispiel BJJ Academy",
			"+still_active: Beispiel BJJ Academy: the website changed August 2026, by its sitemap",
		},
	}, {
		name: "a managing director of a portfolio startup that raised money",
		person: rubric.Person{
			Headline:    "Managing director, Probe Robotics GmbH",
			FitEvidence: "Managing director of Probe Robotics GmbH, by its imprint. In the portfolio of Beispiel Hub",
			Appearances: []rubric.Appearance{{Role: "pitch", Event: "Pitch Abend", Evidence: "Probe Robotics hat eine Seed-Runde über 2 Millionen Euro abgeschlossen und verkauft SaaS an Logistiker."}},
			Companies:   []rubric.Company{{Name: "Probe Robotics GmbH", Founders: 2, Portfolio: "Beispiel Hub"}},
		},
		want: []string{
			"+owner_operator: Managing director of Probe Robotics GmbH, by its imprint. In the portfolio of Beispiel Hub",
			"-funded: Probe Robotics hat eine Seed-Runde über 2 Millionen Euro abgeschlossen und verkauft SaaS an Logistiker.",
			"-enterprise: Probe Robotics hat eine Seed-Runde über 2 Millionen Euro abgeschlossen und verkauft SaaS an Logistiker.",
			"-backed: In the portfolio of Beispiel Hub",
		},
	}, {
		name: "a corporate title, and a company that is gone",
		person: rubric.Person{
			Headline:  "Head of Innovation, Beispiel Versicherung",
			Companies: []rubric.Company{{Name: "Kontrolle Labs GmbH", Activity: "gone", ActivityNote: "the website no longer answers"}},
		},
		want: []string{
			"-corporate: Head of Innovation, Beispiel Versicherung",
			"-inactive: Kontrolle Labs GmbH: the website no longer answers",
		},
	}, {
		name: "a psychotherapist is out for now, a physiotherapist is not",
		person: rubric.Person{
			Headline: "Psychologische Psychotherapeutin in eigener Praxis",
		},
		want: []string{
			"+owner_operator: Psychologische Psychotherapeutin in eigener Praxis",
			"-therapist: Psychologische Psychotherapeutin in eigener Praxis",
		},
	}, {
		name:   "a physiotherapist",
		person: rubric.Person{Headline: "Physiotherapeut und Triathlet"},
		want: []string{
			"+works_with_people: Physiotherapeut und Triathlet",
			"+athlete: Physiotherapeut und Triathlet",
		},
	}, {
		name: "pitching to investors is not raising money, a head coach is no corporate title",
		person: rubric.Person{
			Headline:    "Head Coach, TV Beispielstadt",
			Appearances: []rubric.Appearance{{Role: "pitch", Event: "Investor Night", Evidence: "Tom Testmann pitcht vor Investoren und Business-Angel-Netzwerken."}},
		},
		want: []string{
			"+works_with_people: Head Coach, TV Beispielstadt",
		},
	}, {
		name: "a career switch and a bootstrapped company",
		person: rubric.Person{
			Appearances: []rubric.Appearance{{Role: "speaker", Event: "Gründergeschichten", Evidence: "Mara Beispielfrau, Quereinsteigerin aus der IT, hat ihr Putzteam ohne Investoren aufgebaut."}},
		},
		want: []string{
			"+bootstrapped: Mara Beispielfrau, Quereinsteigerin aus der IT, hat ihr Putzteam ohne Investoren aufgebaut.",
			"+career_switch: Mara Beispielfrau, Quereinsteigerin aus der IT, hat ihr Putzteam ohne Investoren aufgebaut.",
		},
	}, {
		name: "the model's signals count when their passage is there, once per signal",
		person: rubric.Person{
			Headline: "Coach",
			Model: []rubric.Signal{
				{Key: "works_with_people", Passage: "begleitet Menschen durch Umbrüche", Where: rubric.FromModel},
				{Key: "author", Passage: "ihr erstes Buch erschien 2024", Where: rubric.FromModel},
				{Key: "no_such_signal", Passage: "irgendwas", Where: rubric.FromModel},
				{Key: "athlete", Passage: "", Where: rubric.FromModel},
			},
		},
		want: []string{
			"+works_with_people: Coach",
			"+author: ihr erstes Buch erschien 2024",
		},
	}, {
		name:   "nobody known yet",
		person: rubric.Person{},
		want:   nil,
	}} {
		t.Run(c.name, func(t *testing.T) {
			var got []string
			for _, s := range rubric.Signals(c.person) {
				sign := "+"
				if !s.For {
					sign = "-"
				}
				got = append(got, sign+s.Key+": "+s.Passage)
			}
			if strings.Join(got, "\n") != strings.Join(c.want, "\n") {
				t.Errorf("signals:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(c.want, "\n"))
			}
		})
	}
}

func TestScoreCountsReasons(t *testing.T) {
	signals := rubric.Signals(rubric.Person{Headline: "Yoga-Lehrerin und Inhaberin, Studio Beispiel", FitEvidence: "Series A"})
	if got := rubric.Score(signals); got != 1 {
		t.Errorf("score %d, want two for and one against", got)
	}
}

// A long passage is cut around what shows the signal, and stays a quote
// from the text.
func TestPassagesStayShortQuotes(t *testing.T) {
	long := strings.Repeat("Viel Text über das Programm des Abends und die Sponsoren. ", 3)
	text := strings.TrimSpace(long) + " Am Ende spricht eine Autorin über ihren Weg, " + strings.Repeat("mit vielen Umwegen und Geschichten ", 6)
	s := rubric.Signals(rubric.Person{Appearances: []rubric.Appearance{{Role: "speaker", Evidence: text}}})
	if len(s) != 1 || s[0].Key != "author" {
		t.Fatalf("signals %+v", s)
	}
	p := strings.Trim(s[0].Passage, "…")
	if len([]rune(s[0].Passage)) > 170 || !strings.Contains(text, p) || !strings.Contains(p, "Autorin") {
		t.Errorf("passage %q", s[0].Passage)
	}
}
