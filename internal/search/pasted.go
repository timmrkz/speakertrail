package search

import (
	"regexp"
	"slices"
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
// the page himself.
//
// Such a page is LinkedIn's menu, the top of the profile with the name,
// the headline and the place, and then its sections, each under a
// heading of its own, like About, Activity and Experience, with blocks
// of other people between them (Tim's report of 23 November 2026). The
// menu, LinkedIn's buttons and the blocks of other people are left out.
// The sections a first message is written from come first, the feed of
// posts last, so the room of TextChars goes to who they are before what
// they shared lately. Email addresses and phone numbers are taken out,
// see WithoutContact. linked says whether the name on the page is the
// one in the link, which makes sure the page is theirs.
func Pasted(text, link string) (p Profile, linked bool) {
	top, sections := sectioned(text)
	start := 0
	i, linked := nameLine(top, link)
	if i >= 0 {
		p.Name, start = nameIn(top[i], linkNames(link)), i
		top = top[start:]
		p.Headline, p.Location = topLines(top[1:])
	}
	parts := []string{strings.Join(top, "\n")}
	for _, h := range order {
		if body, ok := sections[h]; ok {
			parts = append(parts, h+"\n"+strings.Join(body, "\n"))
		}
	}
	p.Text = Kept(blankless.ReplaceAllString(strings.Join(parts, "\n\n"), "\n\n"))
	return p, linked
}

// order is the sections of a profile kept, in the order a first message
// needs them, in the words of LinkedIn's English interface. A German
// heading is kept under its English one, see headings.
var order = []string{"About", "Experience", "Education", "Featured", "Skills", "Languages", "Licenses & certifications",
	"Volunteering", "Honors & awards", "Projects", "Publications", "Courses", "Services", "Organizations", "Causes", "Activity"}

// headings are LinkedIn's section headings, in English and German, and
// the section each is kept as. An empty one is a block left out: what it
// shows is other people, or LinkedIn talking to Tim.
var headings = map[string]string{
	"about": "About", "info": "About", "experience": "Experience", "berufserfahrung": "Experience",
	"education": "Education", "ausbildung": "Education", "featured": "Featured", "im fokus": "Featured",
	"skills": "Skills", "kenntnisse": "Skills", "languages": "Languages", "sprachen": "Languages",
	"licenses & certifications": "Licenses & certifications", "bescheinigungen und zertifikate": "Licenses & certifications",
	"volunteering": "Volunteering", "volunteer experience": "Volunteering", "ehrenamt": "Volunteering",
	"honors & awards": "Honors & awards", "auszeichnungen": "Honors & awards", "projects": "Projects", "projekte": "Projects",
	"publications": "Publications", "veröffentlichungen": "Publications", "courses": "Courses", "kurse": "Courses",
	"services": "Services", "dienstleistungen": "Services", "organizations": "Organizations", "organisationen": "Organizations",
	"causes": "Causes", "activity": "Activity", "aktivitäten": "Activity",
	"highlights": "", "interests": "", "interessen": "", "recommendations": "", "empfehlungen": "",
	"people also viewed": "", "people you may know": "", "you might like": "", "more profiles for you": "",
	"weitere profile für sie": "", "personen, die sie kennen könnten": "", "das könnte sie auch interessieren": "",
}

// otherPeople are the starts of headings of blocks of other people that
// carry a name, like "People who follow Mara also follow".
var otherPeople = []string{"people who follow ", "others named ", "explore premium profiles", "andere personen namens ", "personen, die "}

// sectioned splits a page into its top, everything before the first
// section, and its sections by heading, each without LinkedIn's buttons
// and the lines a screen reader hears twice.
func sectioned(text string) (top []string, sections map[string][]string) {
	sections = map[string][]string{}
	cur, skip := "", false
	add := func(l string) {
		at := &top
		if cur != "" {
			b := sections[cur]
			at = &b
			defer func() { sections[cur] = b }()
		}
		if n := len(*at); n > 0 && (*at)[n-1] == l || l == "" && n == 0 {
			return
		}
		*at = append(*at, l)
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for i, l := range lines {
		l = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(l), "… more"))
		lower := strings.ToLower(l)
		if h, ok := headings[lower]; ok {
			cur, skip = h, h == ""
			continue
		}
		// LinkedIn's footer, and the message window over the page, are
		// left out up to the next heading. Tim's own name stands in the
		// message window, and theirs as someone to write to.
		if footer(lower, lines[i+1:]) || strings.Contains(lower, "you are on the messaging overlay") {
			skip = true
			continue
		}
		if slices.ContainsFunc(otherPeople, func(h string) bool { return strings.HasPrefix(lower, h) }) {
			skip = true
			continue
		}
		if skip || button(l) {
			continue
		}
		add(l)
	}
	return top, sections
}

// footer says whether a line starts LinkedIn's footer: Accessibility,
// and Talent Solutions as the next line that is not empty. A skill
// called Accessibility has no Talent Solutions after it.
func footer(lower string, rest []string) bool {
	if lower != "accessibility" && lower != "barrierefreiheit" {
		return false
	}
	for _, l := range rest {
		if l = strings.TrimSpace(l); l != "" {
			return strings.EqualFold(l, "Talent Solutions")
		}
	}
	return false
}

// nameLine is where the name stands in the top of a page: the line that
// spells the name in the link, also without initials, as "Mara S.
// Beispiel" under /in/marabeispiel, or that holds it among more, like
// "Dr. Mara Beispiel, MBA". linked says it is one of these. Failing all,
// it is the line right above how near Tim is to them, "· 1st", where
// LinkedIn puts the name. -1 when it is none of these.
func nameLine(top []string, link string) (at int, linked bool) {
	for _, name := range linkNames(link) {
		for i, l := range top {
			if spelled(l) == name || spelled(withoutInitials(l)) == name {
				return i, true
			}
		}
		for i, l := range top {
			if len([]rune(l)) <= 80 && strings.Contains(spelled(l), name) {
				return i, true
			}
		}
	}
	for i, l := range top {
		if !degree.MatchString(l) {
			continue
		}
		for j := i - 1; j >= 0; j-- {
			if top[j] != "" && !pronouns.MatchString(top[j]) {
				return j, false
			}
		}
	}
	return -1, false
}

// linkNames are the ways the name in a link is spelled, see spelled:
// whole, and without the initials in it, so mara-s-beispiel is "Mara
// Beispiel" on her page too (Tim's report of 24 November 2026). None
// for a link without a name.
func linkNames(link string) []string {
	whole := spelled(linkName(link))
	if whole == "" {
		return nil
	}
	names := []string{whole}
	if short := spelled(withoutInitials(strings.ReplaceAll(linkName(link), "-", " "))); short != "" && short != whole {
		names = append(names, short)
	}
	return names
}

// withoutInitials is a name without its initials, "Mara Beispiel" for
// "Mara S. Beispiel".
func withoutInitials(name string) string {
	var words []string
	for w := range strings.FieldsSeq(name) {
		if len([]rune(strings.TrimSuffix(w, "."))) > 1 {
			words = append(words, w)
		}
	}
	return strings.Join(words, " ")
}

// topLines reads the headline and the place from the lines under the
// name. The headline is the first line, unless that is already the
// place. The place is the first line that has the shape of one, like
// "Köln, Nordrhein-Westfalen, Deutschland".
func topLines(lines []string) (headline, location string) {
	first := true
	for _, l := range lines[:min(len(lines), 8)] {
		l = strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(l, "Contact info"), "Kontaktinfo"))
		l = strings.TrimSpace(strings.TrimSuffix(l, "·"))
		if l == "" || pronouns.MatchString(l) || degree.MatchString(l) {
			continue
		}
		if isPlace(l) {
			return headline, l
		}
		if first {
			headline = l
		}
		first = false
	}
	return headline, ""
}

// Kept is a profile's text as the app keeps it: without email addresses
// and phone numbers, and at most TextChars long.
func Kept(text string) string {
	out := WithoutContact(strings.TrimSpace(text))
	if r := []rune(out); len(r) > TextChars {
		out = strings.TrimSpace(string(r[:TextChars]))
	}
	return out
}

// isPlace is a line that gives a place the way a profile's top does: a
// city with its state or country, or a region, never a headline that
// names a city, like "Fotografin in Köln".
func isPlace(l string) bool {
	if regionLine.MatchString(l) {
		return true
	}
	if !strings.Contains(l, ",") {
		return false
	}
	_, inNRW := place.PlaceInNRW(l)
	return inNRW || germany.MatchString(l)
}

// nameIn is the name in a line that holds more, like "Mara Beispiel |
// Coach 🌱": the part before a mark that holds one of names, or any
// part before a mark for a link without a name, without the marks and
// pictures after it.
func nameIn(l string, names []string) string {
	for _, sep := range []string{" | ", " · ", " – ", " - "} {
		if before, _, ok := strings.Cut(l, sep); ok && (len(names) == 0 || slices.ContainsFunc(names, func(n string) bool { return strings.Contains(spelled(before), n) })) {
			l = before
		}
	}
	return strings.TrimRightFunc(l, func(r rune) bool { return !unicode.IsLetter(r) && r != '.' && r != ')' })
}

var (
	// Pronouns stand under the name, like "She/Her" or "(Sie/Ihr)".
	pronouns = regexp.MustCompile(`^\(?\p{L}{2,4}/\p{L}{2,4}\)?$`)
	// How near Tim is to them, like "· 1st", "2nd" or "· 1."
	degree = regexp.MustCompile(`^[·•]?\s*(?:1st|2nd|3rd\+?|[123]\.)(?:\s.*)?$`)
	// More than one empty line in a row.
	blankless = regexp.MustCompile(`\n{3,}`)
	// A place outside NRW still says the country.
	germany = regexp.MustCompile(`, (?:Germany|Deutschland)$`)
	// A region on its own, like "Ruhr Region" or "Greater Cologne Area".
	regionLine = regexp.MustCompile(`^(?:\p{L}+[ -])+(?:Region|Area|Gebiet|Metropolregion)$|^Metropolregion `)
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

// button is a line that is one of LinkedIn's buttons, menus and counts,
// which say nothing about the person, or names someone else only, like
// "Dr. Mara is a mutual connection".
func button(l string) bool {
	lower := strings.ToLower(strings.Trim(l, "…. "))
	switch lower {
	case "message", "nachricht", "more", "mehr", "follow", "folgen", "connect", "vernetzen",
		"contact info", "kontaktinfo", "like", "comment", "kommentieren", "repost", "send", "senden",
		"see more", "mehr anzeigen", "see less", "weniger anzeigen", "show translation", "übersetzung anzeigen",
		"search", "suche", "home", "startseite", "my network", "mein netzwerk", "jobs", "messaging", "notifications",
		"mitteilungen", "me", "ich", "for business", "für unternehmen", "cover photo", "titelbild", "save", "speichern",
		"say hello", "sales insights", "key signals", "posts", "beiträge", "comments", "kommentare", "images", "bilder",
		"videos", "articles", "artikel", "documents", "dokumente", "events", "reactions", "reaktionen", "·", "•":
		return true
	}
	return strings.HasPrefix(lower, "show all ") || (strings.HasPrefix(lower, "alle ") && strings.HasSuffix(lower, " anzeigen")) ||
		strings.HasPrefix(lower, "try premium") || strings.HasPrefix(lower, "activate ") || strings.HasPrefix(lower, "1-month free trial") ||
		strings.HasSuffix(lower, " is a mutual connection") || strings.HasSuffix(lower, " are mutual connections") ||
		strings.HasSuffix(lower, " is a new connection") || chrome.MatchString(l)
}

// chrome is a line of LinkedIn's own, like "View image", "View Mara
// Beispiel's profile", "1/3", "12", "• 2nd" or "0 notifications".
var chrome = regexp.MustCompile(`^(?:View (?:image|profile|video|company: .*|.+[’']s profile.*)|\d+/\d+|[\d.,]+|•\s*(?:1st|2nd|3rd\+?)|\d+ notifications?|“Hi .*)$`)
