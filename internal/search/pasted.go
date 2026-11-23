package search

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/timmrkz/speakertrail/internal/place"
)

// TextChars is how much of a profile's text the app keeps, the most Exa
// is asked for and the most of a page Tim pastes. A first message is
// written from it by a model that reads 8192 tokens.
const TextChars = 8000

// Pasted reads a LinkedIn profile page as Tim copied it in Chrome, all of
// it with ⌘A and ⌘C, for the person whose link is link, for when Exa's
// index has no copy of it. The app never requests LinkedIn: Tim opened
// the page himself. The name is the line that spells the name in the
// link, the headline the line after it, the place the first line after
// it that names one. What comes before the name is LinkedIn's own menu,
// and what the page shows of other people at its end is left out, like
// LinkedIn's buttons, so the text holds the profile and nothing else.
// Email addresses and phone numbers are taken out, see WithoutContact.
func Pasted(text, link string) Profile {
	var lines []string
	for _, l := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		l = strings.TrimSpace(l)
		if l == "" {
			if len(lines) > 0 && lines[len(lines)-1] != "" {
				lines = append(lines, "")
			}
			continue
		}
		if others(l) {
			break
		}
		// LinkedIn writes many lines twice, once for the eye and once for
		// a screen reader.
		if button(l) || (len(lines) > 0 && lines[len(lines)-1] == l) {
			continue
		}
		lines = append(lines, l)
	}
	var p Profile
	start := 0
	if name := spelled(linkName(link)); name != "" {
		for i, l := range lines {
			if spelled(l) == name {
				p.Name, start = l, i
				break
			}
		}
	}
	lines = lines[start:]
	if p.Name != "" {
		for i, l := range lines[1:min(len(lines), 8)] {
			l = strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(l, "Contact info"), "Kontaktinfo"))
			l = strings.TrimSpace(strings.TrimSuffix(l, "·"))
			if l == "" || pronouns.MatchString(l) || degree.MatchString(l) {
				continue
			}
			if _, inNRW := place.PlaceInNRW(l); inNRW || germany.MatchString(l) {
				p.Location = l
				break
			}
			if p.Headline == "" && i < 4 {
				p.Headline = l
			}
		}
	}
	out := WithoutContact(strings.TrimSpace(strings.Join(lines, "\n")))
	if r := []rune(out); len(r) > TextChars {
		out = strings.TrimSpace(string(r[:TextChars]))
	}
	p.Text = out
	return p
}

var (
	// Pronouns stand under the name, like "She/Her" or "(Sie/Ihr)".
	pronouns = regexp.MustCompile(`^\(?\p{L}{2,4}/\p{L}{2,4}\)?$`)
	// How near Tim is to them, like "· 1st", "2nd" or "· 1."
	degree = regexp.MustCompile(`^·?\s*(?:1st|2nd|3rd\+?|[123]\.)(?:\s.*)?$`)
	// A place outside NRW still says the country.
	germany = regexp.MustCompile(`, (?:Germany|Deutschland)$`)
)

// spelled is the letters of a name or a link name, small, with ä, ö, ü
// and ß written out, and without the numbers LinkedIn adds to a link, so
// "Jörg Probe" and jorg-probe-7a1b or joerg-probe have the same.
func spelled(s string) string {
	s = strings.NewReplacer("ä", "a", "ö", "o", "ü", "u", "ß", "ss", "ae", "a", "oe", "o", "ue", "u").Replace(strings.ToLower(s))
	var b strings.Builder
	for w := range strings.FieldsFuncSeq(s, func(r rune) bool { return r == '-' || r == '_' || unicode.IsSpace(r) }) {
		if strings.ContainsFunc(w, unicode.IsDigit) {
			continue
		}
		for _, r := range w {
			if unicode.IsLetter(r) {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

// button is a line that is one of LinkedIn's buttons or links, which say
// nothing about the person.
func button(l string) bool {
	lower := strings.ToLower(strings.Trim(l, "…. "))
	switch lower {
	case "message", "nachricht", "more", "mehr", "follow", "folgen", "connect", "vernetzen",
		"contact info", "kontaktinfo", "like", "comment", "kommentieren", "repost", "send", "senden",
		"see more", "mehr anzeigen", "see less", "weniger anzeigen", "show translation", "übersetzung anzeigen":
		return true
	}
	return strings.HasPrefix(lower, "show all ") || (strings.HasPrefix(lower, "alle ") && strings.HasSuffix(lower, " anzeigen"))
}

// others is the heading of the part of the page that shows other people,
// where the profile ends.
func others(l string) bool {
	lower := strings.ToLower(l)
	for _, h := range []string{"people also viewed", "people you may know", "others named ", "you might like", "more profiles for you",
		"weitere profile", "personen, die sie kennen könnten", "andere personen namens ", "das könnte sie auch interessieren"} {
		if strings.HasPrefix(lower, h) {
			return true
		}
	}
	return false
}
