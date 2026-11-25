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
	p, linked := search.Pasted(page, "https://www.linkedin.com/in/joerg-probe-7a1b2c3")
	if !linked || p.Name != "Jörg Probe" || p.Headline != "Inhaber | Probe Möbel, Bielefeld" || p.Location != "Bielefeld, Nordrhein-Westfalen, Deutschland" {
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
	// A page of someone else is read all the same, by where LinkedIn
	// puts the name, but says it is not the one in the link.
	if q, linked := search.Pasted(page, "https://www.linkedin.com/in/ida-probe"); linked || q.Name != "Jörg Probe" || !strings.Contains(q.Text, "Probe Möbel") {
		t.Errorf("another's page %+v, linked %v", q, linked)
	}
	// A long page is cut to what a first message is written from.
	if long, _ := search.Pasted(page+strings.Repeat("\nEin Beitrag über Eiche", 1000), "https://www.linkedin.com/in/joerg-probe"); len([]rune(long.Text)) > search.TextChars {
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
		p, _ := search.Pasted(c.page, "https://www.linkedin.com/in/"+c.link)
		if p.Name != c.name || p.Headline != c.headline || p.Location != c.location {
			t.Errorf("%q: read %q, %q, %q", c.page, p.Name, p.Headline, p.Location)
		}
		if strings.HasPrefix(p.Text, "Menü") {
			t.Errorf("%q: the text keeps the menu", c.page)
		}
	}
}

// A page as Chrome copied it on Tim's Mac on 23 November 2026, in
// LinkedIn's English interface, with every person invented: the menu,
// the top with a middle initial the link leaves out, a block of other
// people before Highlights, the feed of posts, and Experience and
// Education only after it.
const realPage = `0 notifications

Search

Home
My Network
Jobs
Messaging
Notifications

Me

For Business
Try Premium for €0

Cover photo

Mara S. Beispiel
· 1st

Gründerin Probe Keramik | M.Sc. Design | Mentorin

Dortmund, North Rhine-Westphalia, Germany

·

Probe Keramik + 1 more

Fachhochschule Beispielstadt

500+ connections

Dr. Lena is a mutual connection

Save

People who follow Mara S. also follow

View profile
Otto Muster

Founder Muster Labs | Speaker

6,685 followers

Highlights

Mara S. is a new connection

“Hi Mara S., it’s great to connect with you.”

Say hello
About
Ich töpfere seit zehn Jahren und habe 2024 mein Studio eröffnet.
Activity
737 followers

Posts

Comments

Images
View Mara S. Beispiel’s profile
Mara S. Beispiel

• 1st

Gründerin Probe Keramik | M.Sc. Design | Mentorin

1mo •

` + "Heute haben wir den neuen Brennofen eingeweiht. Lange Rede, kurzer Sinn: es brennt.\n\nView image\n1/3\n\n12\n\n" + `Experience
Gründerin
Probe Keramik · Self-employed
Jan 2024 - Present · 1 yr 11 mos
Dortmund, North Rhine-Westphalia, Germany
Education
Fachhochschule Beispielstadt
M.Sc. Design
Interests
Top Voices
Otto Muster
People also viewed
Ida Probe
Coach`

func TestPastedAsChromeCopiesIt(t *testing.T) {
	p, linked := search.Pasted(realPage+strings.Repeat("\n\nLange Rede, kurzer Sinn, ein Beitrag mehr.", 400), "https://www.linkedin.com/in/marabeispiel")
	if !linked || p.Name != "Mara S. Beispiel" || p.Headline != "Gründerin Probe Keramik | M.Sc. Design | Mentorin" || p.Location != "Dortmund, North Rhine-Westphalia, Germany" {
		t.Errorf("read %q, %q, %q, linked %v", p.Name, p.Headline, p.Location, linked)
	}
	// Who they are comes first and is whole, the feed after it, cut.
	about, exp, edu, act := strings.Index(p.Text, "About\nIch töpfere"), strings.Index(p.Text, "Experience\nGründerin"), strings.Index(p.Text, "Education\nFachhochschule"), strings.Index(p.Text, "Activity\n")
	if !strings.HasPrefix(p.Text, "Mara S. Beispiel\n") || about < 0 || exp < about || edu < exp || act < edu || len([]rune(p.Text)) > search.TextChars {
		t.Errorf("the text is in the wrong order:\n%s", p.Text[:min(len(p.Text), 1500)])
	}
	if !strings.Contains(p.Text, "Brennofen") {
		t.Errorf("the feed is left out")
	}
	for _, left := range []string{"notifications", "Try Premium", "Cover photo", "Otto Muster", "Dr. Lena", "Ida Probe", "Highlights", "Say hello", "View image", "1/3", "\n12\n", "• 1st", "Top Voices", "\n\n\n"} {
		if strings.Contains(p.Text, left) {
			t.Errorf("the text keeps %q", left)
		}
	}
}

// A page as Chrome copied it on Tim's Mac on 24 November 2026, with
// every person invented: a link with a middle initial the page leaves
// out, LinkedIn's footer, and the message window Tim had open over the
// page, with his own name and theirs in it, before the sections after
// it. The footer and the message window say nothing about them, and Tim
// is not the person.
const footerPage = `Mara Beispiel
· 1st

Gründerin Probe Keramik | Abitur 2027

Düsseldorf, North Rhine-Westphalia, Germany

About
Ich töpfere seit zehn Jahren.

Top skills

Keramik • Social Media Marketing

Accessibility

Talent Solutions

Community Guidelines

Careers

Marketing Solutions

Privacy & Terms

LinkedIn Corporation © 2026

Select language

English (English)
Tom Beobachter
MessagingYou are on the messaging overlay. Press enter to open the list of conversations.

Compose message
New message
Enter message recipients

Mara Beispiel
Mara Beispiel Profile
1st degree connection
· 1st
Gründerin Probe Keramik | Abitur 2027

Attach an image for your draft conversation
Open Emoji Keyboard

Featured
Link

Probe Keramik | Töpfern für alle

Activity
67 followers

Heute haben wir den neuen Brennofen eingeweiht.

Show all

English

Deutsch`

func TestPastedWithFooterAndMessage(t *testing.T) {
	p, linked := search.Pasted(footerPage, "https://www.linkedin.com/in/mara-s-beispiel")
	if !linked || p.Name != "Mara Beispiel" || p.Headline != "Gründerin Probe Keramik | Abitur 2027" {
		t.Errorf("read %q, %q, linked %v", p.Name, p.Headline, linked)
	}
	for _, kept := range []string{"Ich töpfere", "Keramik • Social Media Marketing", "Töpfern für alle", "Brennofen"} {
		if !strings.Contains(p.Text, kept) {
			t.Errorf("the text lacks %q:\n%s", kept, p.Text)
		}
	}
	for _, left := range []string{"Accessibility", "Talent Solutions", "LinkedIn Corporation", "Tom Beobachter", "messaging overlay", "Compose message", "Emoji", "Mara Beispiel Profile", "1st degree"} {
		if strings.Contains(p.Text, left) {
			t.Errorf("the text keeps %q:\n%s", left, p.Text)
		}
	}
}
