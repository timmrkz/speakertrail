package rubric

import (
	"testing"
	"time"
)

func has(sigs []Signal, key string) (Signal, bool) {
	for _, s := range sigs {
		if s.Key == key {
			return s, true
		}
	}
	return Signal{}, false
}

// Tim, 22 November 2026: people high up said they founded something, but
// study, or work as an employee somewhere else. "I would like to connect
// with people who are actually doing this full time so that it pays the
// bills."
func TestStudiesOrEmployedElsewhere(t *testing.T) {
	for _, tc := range []struct {
		name              string
		headline, roles   string
		studies, employed bool
		passage           string
	}{
		{"a student who co-founded something", "In Ausbildung/Studium: Psychologie (B. Sc.)", "Mitgründerin, Beispielwerk", true, false, ""},
		{"a founder with a job in the headline", "Founder of Probe AI und Digitale Projekte beim 1. FC Beispielstadt", "", false, true, "Digitale Projekte beim 1. FC Beispielstadt"},
		{"a founder with a job in the roles", "Gründer | Probe Labs", "Co-Founder, Probe Labs · Projektmanager, Beispiel Versicherung AG", false, true, "Projektmanager, Beispiel Versicherung AG"},
		{"a working student", "Founder @ Probe Labs", "Founder, Probe Labs · Werkstudent, Beispiel GmbH", true, true, ""},
		{"an owner who also coaches at a gym", "Inhaberin | Studio Beispiel", "Inhaberin, Studio Beispiel · Personal Trainerin, FitBeispiel", false, false, ""},
		{"an owner with two roles at her studio", "Yoga", "Gründerin, Beispiel Yoga · Yogalehrerin, Beispiel Yoga · Office, Beispiel Yoga GmbH", false, false, ""},
		{"an owner on a board", "Geschäftsführerin, Beispiel Bäckerei", "Geschäftsführerin, Beispiel Bäckerei · Mitglied, Beispiel Netzwerk · Speakerin, Beispiel Konferenz", false, false, ""},
		{"an owner without a company's name", "Inhaberin | Fotografin", "Fotografin, Lichtprobe", false, false, ""},
		{"a headline that helps someone", "Founder of Probe Labs | Ich helfe dir bei Veränderungen", "", false, false, ""},
		{"a headline that names no organisation", "Selbstständige Fotografin, Gründerin von Bildprobe | Fotos bei Hochzeiten", "", false, false, ""},
		{"a coach for students", "Inhaber von Lernprobe | Coach für Studierende", "", false, false, ""},
		{"freelance at a company", "Gründer von Probe Labs", "Gründer, Probe Labs · Fotograf, Selbstständig", false, false, ""},
	} {
		sigs := Signals(Person{Name: "Mara Kowal", Headline: tc.headline, CurrentRoles: tc.roles})
		s, studies := has(sigs, "studies")
		e, employed := has(sigs, "employed")
		if studies != tc.studies || employed != tc.employed {
			t.Errorf("%s: studies %v %q, employed %v %q", tc.name, studies, s.Passage, employed, e.Passage)
		}
		if tc.passage != "" && e.Passage != tc.passage {
			t.Errorf("%s: shown by %q, want %q", tc.name, e.Passage, tc.passage)
		}
		// Neither leaves a person out, they only order.
		if ok, why := Guest(sigs); !ok {
			t.Errorf("%s: left out, %s", tc.name, why)
		}
	}
}

// The two Tim rejected stood at 33 and 32 for being near, young and a
// little active. A doubt takes them below an owner who shows nothing more.
func TestDoubtsComeAfterOwners(t *testing.T) {
	now := day("2026-11-22")
	base := FitPerson{BornAbout: 1999, AgeReason: "First job from 2021", LastActive: day("2026-10-23"),
		Home: "Leverkusen", City: "Köln", Km: 14, Near: true, Now: now}
	owner := FitPoints(base)
	for _, key := range []string{"studies", "employed"} {
		p := base
		p.Signals = []Signal{{Key: key}}
		fit, parts := Fit(p)
		if fit >= owner-10 {
			t.Errorf("%s: %d, an owner who shows nothing more %d", key, fit, owner)
		}
		if parts[0].Points >= 0 || parts[0].Why != byKey[key].Label {
			t.Errorf("%s: %+v", key, parts[0])
		}
	}
	// However much counts against, the fit stays 0 or more.
	p := FitPerson{Signals: []Signal{{Key: "studies"}, {Key: "employed"}, {Key: "funded"}}, Now: now, LastActive: now.Add(-400 * 24 * time.Hour)}
	if fit, parts := Fit(p); fit != 0 || FitPoints(p) != 0 {
		t.Errorf("fit %d, points %d, %v", fit, FitPoints(p), parts)
	}
}

// Tim, 22 November 2026, from a report: at the top stood a social media
// manager, studying and employed full time, whose own company was named
// after her. "I'm not interested in AI platforms and social media
// managers or social media anything."
func TestSocialMediaAndAI(t *testing.T) {
	sigs := Signals(Person{Name: "Mara Kowal", Headline: "Studium an der Hochschule Beispielstadt",
		CurrentRoles: "Social Media-Managerin, NetProbe GmbH · Content Creator, Unfallprobe · Content Creator / Social Media Manager, Mara Kowal"})
	for key, passage := range map[string]string{
		"studies":      "Studium an der Hochschule Beispielstadt",
		"employed":     "Social Media-Managerin, NetProbe GmbH",
		"social_media": "Social Media-Managerin, NetProbe GmbH",
	} {
		if s, ok := has(sigs, key); !ok || s.Passage != passage {
			t.Errorf("%s: %v %q, want %q", key, ok, s.Passage, passage)
		}
	}
	if ok, why := Guest(sigs); !ok {
		t.Errorf("left out, %s", why)
	}
	now := day("2026-11-22")
	fit, parts := Fit(FitPerson{Signals: sigs, BornAbout: 2000, AgeReason: "First job from 2022", LastActive: day("2026-11-03"),
		Home: "Köln", City: "Düsseldorf", Km: 36, Near: true, Now: now})
	if fit > 5 || parts[0].Why != "Studies or in training, Employed elsewhere, Social media work" {
		t.Errorf("fit %d, %+v", fit, parts[0])
	}

	for headline, want := range map[string]bool{
		"Founder of Probe AI":                      true,
		"Gründerin | KI-Plattform für Praxen":      true,
		"Co-Founder, Probe Labs (GenAI)":           true,
		"Yogalehrerin in Mainz und Kaiserslautern": false,
		"Inhaber, Probe Kampfsport":                false,
	} {
		if _, got := has(Signals(Person{Headline: headline}), "ai"); got != want {
			t.Errorf("%q builds on AI: %v, want %v", headline, got, want)
		}
	}
	for headline, want := range map[string]bool{
		"Content Creatorin | Selbstständig":   true,
		"Gründerin · Influencer Marketing":    true,
		"Community Managerin bei Probe":       true,
		"Inhaberin, Beispiel Yoga | Lehrerin": false,
		"Medienpädagogin, eigene Praxis":      false,
	} {
		if _, got := has(Signals(Person{Headline: headline}), "social_media"); got != want {
			t.Errorf("%q works in social media: %v, want %v", headline, got, want)
		}
	}
}
