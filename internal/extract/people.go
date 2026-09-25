package extract

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// NormaliseName folds a name for comparison: lower case, no accents, no
// titles, single spaces. "Dr. Jürgen  Müller" becomes "jurgen muller".
func NormaliseName(name string) string {
	s := strings.ToLower(cleanText(name))
	s = strings.NewReplacer("ß", "ss", "ä", "a", "ö", "o", "ü", "u").Replace(s)
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	if out, _, err := transform.String(t, s); err == nil {
		s = out
	}
	var words []string
	for _, w := range strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && r != '-' && r != '\'' }) {
		switch w {
		case "dr", "prof", "med", "phil", "rer", "nat", "ing", "dipl", "mba", "phd", "mr", "mrs", "ms", "frau", "herr":
			continue
		}
		words = append(words, w)
	}
	return strings.Join(words, " ")
}

// cleanName trims titles and punctuation around a name but keeps "Dr."
// where the page used it.
func cleanName(s string) string {
	s = cleanText(s)
	return strings.Trim(s, " ,;:-–—·|/\"'“”„()")
}

var nameWord = regexp.MustCompile(`^(?:[A-ZÄÖÜÀ-ÝŁŚŽČŘ][\p{Ll}'’\-]+(?:-[A-ZÄÖÜ][\p{Ll}]+)?|[A-Z]\.|[A-ZÄÖÜ][\p{Ll}]*[A-Z][\p{Ll}]+)$`)

var nameParticles = map[string]bool{
	"von": true, "van": true, "de": true, "der": true, "den": true, "zu": true, "da": true, "di": true, "del": true, "la": true, "le": true, "ten": true, "vom": true, "y": true, "bin": true, "al": true,
}

var notNameWords = map[string]bool{
	"gmbh": true, "ag": true, "ug": true, "e.v.": true, "ev": true, "kg": true, "inc": true, "ltd": true, "llc": true, "team": true,
	"uhr": true, "anmeldung": true, "tickets": true, "ticket": true, "startup": true, "startups": true, "founder": true, "founders": true,
	"köln": true, "cologne": true, "düsseldorf": true, "bonn": true, "nrw": true, "event": true, "events": true, "meetup": true,
	"pitch": true, "night": true, "summit": true, "panel": true, "keynote": true, "workshop": true, "talk": true, "networking": true,
	"und": true, "and": true, "mit": true, "with": true, "the": true, "der": false, "die": true, "das": true, "für": true, "for": true,
	"university": true, "universität": true, "hochschule": true, "institut": true, "institute": true, "foundation": true, "stiftung": true,
	"tba": true, "tbd": true, "tba.": true, "gäste": true, "guests": true, "speaker": true, "speakers": true, "more": true, "weitere": true,
	"montag": true, "dienstag": true, "mittwoch": true, "donnerstag": true, "freitag": true, "samstag": true, "sonntag": true,
	"gründer": true, "gründerin": true, "co-founder": true, "cofounder": true, "co-gründer": true, "co-gründerin": true, "ceo": true, "cto": true, "coo": true, "cfo": true,
	"head": true, "director": true, "managing": true, "geschäftsführer": true, "geschäftsführerin": true, "inhaber": true, "inhaberin": true,
	"partner": true, "partnerin": true, "leiter": true, "leiterin": true, "chief": true, "officer": true, "lead": true, "manager": true,
	"monday": true, "tuesday": true, "wednesday": true, "thursday": true, "friday": true, "saturday": true, "sunday": true,
}

// LooksLikeName reports whether s reads like a person's full name: two to
// five capitalised words, optionally with a title and name particles.
func LooksLikeName(s string) bool {
	s = cleanName(s)
	if len(s) < 4 || len(s) > 60 || strings.ContainsAny(s, "0123456789@#&:!?/+=") {
		return false
	}
	words := strings.Fields(s)
	for len(words) > 0 && (words[0] == "Dr." || words[0] == "Prof." || words[0] == "Dr" || words[0] == "Prof") {
		words = words[1:]
	}
	if len(words) < 2 || len(words) > 5 {
		return false
	}
	capitalised := 0
	for _, w := range words {
		lw := strings.ToLower(strings.Trim(w, ".,"))
		if notNameWords[lw] {
			return false
		}
		if nameParticles[lw] {
			continue
		}
		if !nameWord.MatchString(w) {
			return false
		}
		capitalised++
	}
	return capitalised >= 2
}

// roleWords maps a label in front of names to a role.
var roleWords = []struct {
	re   *regexp.Regexp
	role string
}{
	{regexp.MustCompile(`(?i)^(?:keynote(?:[- ]?speaker(?:in)?)?|keynotes? (?:von|by|from))$`), "speaker"},
	{regexp.MustCompile(`(?i)^(?:speakers?|speakerinnen|sprecher(?:in(?:nen)?)?|referent(?:in(?:nen)?|en)?|redner(?:in(?:nen)?)?|vortrag|talks?|impulse?|impulsvortrag|mit|with|on stage|auf der bühne|line-?up|lineup|gäste|guests|founder talk|fireside chat|lesung|autor(?:in(?:nen)?|en)?|authors?)$`), "speaker"},
	{regexp.MustCompile(`(?i)^(?:panel(?:ists?|isten|istinnen|diskussion)?|podium|diskutanten|jury|juror(?:in(?:nen)?|en)?)$`), "panelist"},
	{regexp.MustCompile(`(?i)^(?:pitch(?:es|ing)?|startups?|pitching (?:teams|startups)|finalist(?:en|innen|s)?|demos?|live-?demos?)$`), "pitch"},
	{regexp.MustCompile(`(?i)^(?:moderation|moderiert von|moderated by|moderator(?:in|en|innen)?|mc)$`), "moderator"},
	{regexp.MustCompile(`(?i)^(?:hosts?|hosted by|gastgeber(?:in(?:nen)?)?|host(?:in|innen)|organi[sz]ed by)$`), "host"},
}

func roleFor(label string) (string, bool) {
	label = strings.TrimSpace(strings.Trim(label, "*_#-–•·"))
	for _, rw := range roleWords {
		if rw.re.MatchString(label) {
			return rw.role, true
		}
	}
	return "", false
}

var (
	labelLine   = regexp.MustCompile(`^\s*[*_•·\-–]*\s*([\p{L} \-]{2,40}?)\s*[*_]*\s*:\s*(.*)$`)
	bulletLine  = regexp.MustCompile(`^\s*(?:[-–•·*▪️►>]|\d+[.)])\s*(.+)$`)
	listSplit   = regexp.MustCompile(`\s*(?:,\s*(?:und|and|&)\s+|\s+(?:und|and|&)\s+|;\s*|\s+\|\s+|\s*/\s+)`)
	parenAff    = regexp.MustCompile(`^(.+?)\s*\(([^()]{2,80})\)\s*$`)
	sepAff      = regexp.MustCompile(`^(.+?)\s*(?:,|\s[–—-]\s|\s@\s|\s\|\s)\s*(.{2,80})$`)
	vonPrefix   = regexp.MustCompile(`(?i)^(?:von|by|mit|with)\s+`)
	speakerOfRe = regexp.MustCompile(`(?i)(?:keynote|vortrag|talk|impuls|lesung)\s+(?:von|by|mit|with)\s+((?:(?:Dr|Prof)\.\s*)*[^.,;:\n(]+?(?:\s*\([^)]+\))?)(?:[.,;:\n]|$)`)
)

// PeopleFromText finds people in free text, but only on clearly marked
// lines: a role label followed by a colon ("Speaker: Lea Beispiel
// (Beispiel GmbH)"), bullet lists under such a label, and "Keynote von ...".
// Anything that does not read like a full name is ignored.
func PeopleFromText(text string) []Person {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	var people []Person
	add := func(item, role string) {
		for _, p := range splitPeople(item, role) {
			people = mergePeople(people, []Person{p})
		}
	}
	lines := strings.Split(text, "\n")
	currentRole := ""
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			currentRole = ""
			continue
		}
		if m := labelLine.FindStringSubmatch(line); m != nil {
			if role, ok := roleFor(m[1]); ok {
				if rest := strings.TrimSpace(m[2]); rest != "" {
					add(rest, role)
					currentRole = ""
				} else {
					currentRole = role
				}
				continue
			}
		}
		if currentRole != "" {
			if m := bulletLine.FindStringSubmatch(line); m != nil {
				add(m[1], currentRole)
				continue
			}
			if p := splitPeople(line, currentRole); len(p) > 0 && len(line) < 120 {
				add(line, currentRole)
				continue
			}
			currentRole = ""
		}
		for _, m := range speakerOfRe.FindAllStringSubmatch(line, -1) {
			add(m[1], "speaker")
		}
	}
	return people
}

// splitPeople splits "A (X), B und C" into people with the given role.
func splitPeople(s, role string) []Person {
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "."))
	var out []Person
	for _, part := range splitOutsideParens(s) {
		part = vonPrefix.ReplaceAllString(strings.TrimSpace(part), "")
		name, aff := part, ""
		if m := parenAff.FindStringSubmatch(part); m != nil {
			name, aff = m[1], m[2]
		} else if !LooksLikeName(part) {
			if m := sepAff.FindStringSubmatch(part); m != nil {
				name, aff = m[1], m[2]
			}
		}
		name = cleanName(name)
		if !LooksLikeName(name) {
			continue
		}
		out = append(out, Person{Name: name, Role: role, Affiliation: cleanText(aff)})
	}
	return out
}

// splitOutsideParens splits a list of names on commas and "und"/"and"
// that are not inside parentheses.
func splitOutsideParens(s string) []string {
	var parts []string
	depth, last := 0, 0
	masked := []rune(s)
	for i, r := range masked {
		switch r {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		default:
			if depth > 0 {
				masked[i] = '_'
			}
		}
	}
	m := string(masked)
	for _, loc := range listSplit.FindAllStringIndex(m, -1) {
		parts = append(parts, s[last:loc[0]])
		last = loc[1]
	}
	parts = append(parts, s[last:])
	// Plain commas separate names too, unless the part after the comma is
	// an affiliation of the name before it.
	var out []string
	for _, p := range parts {
		sub := strings.Split(maskedSplit(p), "\x00")
		if len(sub) > 1 && allNames(sub) {
			out = append(out, sub...)
		} else {
			out = append(out, p)
		}
	}
	return out
}

func maskedSplit(s string) string {
	depth := 0
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		}
		if r == ',' && depth == 0 {
			b.WriteRune('\x00')
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func allNames(parts []string) bool {
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if m := parenAff.FindStringSubmatch(p); m != nil {
			p = m[1]
		}
		if !LooksLikeName(p) {
			return false
		}
	}
	return true
}

// NormaliseRole maps free role words to the roles the data model knows.
func NormaliseRole(role string) string {
	switch r := strings.ToLower(strings.TrimSpace(role)); r {
	case "speaker", "panelist", "pitch", "host", "moderator":
		return r
	case "":
		return "speaker"
	default:
		if mapped, ok := roleFor(r); ok {
			return mapped
		}
		return "speaker"
	}
}
