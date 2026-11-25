// Package names finds the parts of a person's name as LinkedIn and Exa
// show it, where titles, degrees, pronouns, emoji and a tagline can stand
// beside it, like "Dr. med. Mara Beispiel", "Mara Beispiel, MBA
// (sie/ihr)" or "Beispiel, Mara". It is the one place that says what a
// first name is.
//
// It reads the name the way a person would: titles before it, degrees
// after it, and the order the profile's link gives, since LinkedIn makes
// the link from the first and the family name, like
// linkedin.com/in/mara-beispiel-4b1a2c93. No list of first names is
// needed, so a name never seen before is read as well as a common one.
// See docs/names.md for how well it reads the forms profiles use.
package names

import (
	"net/url"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// First is the first name, to greet someone by, in the case a greeting
// writes it, "Mara" of "MARA BEISPIEL". The link is their profile's, or
// empty. It is empty when the name holds none for sure, like "Dr.
// Beispiel", which is a title and a family name.
func First(name, link string) string {
	n := read(name)
	if len(n.words) == 0 || n.titled && len(n.words) == 1 {
		return ""
	}
	first := n.words[0]
	if n.given != "" {
		first = n.given
	}
	if w := byLink(n.words, link); w != "" {
		first = w
	}
	return cased(first)
}

// Family is the last word of the family name, "Beispiel" of "Mara von
// Beispiel". It is empty when the name has one word and no title before
// it, since that word is more likely a first name.
func Family(name string) string {
	n := read(name)
	switch {
	case n.family != "":
		return n.family
	case len(n.words) >= 2 || n.titled && len(n.words) == 1:
		return n.words[len(n.words)-1]
	}
	return ""
}

// parsed is a name as read: its own words in the order written, whether
// a title stood before them, and the given and family name where the
// name says which is which, as "Beispiel, Mara" does.
type parsed struct {
	words         []string
	titled        bool
	given, family string
}

// read reads a name.
func read(name string) parsed {
	// Letters styled bold or italic, and wide ones, are plain letters.
	name = norm.NFKC.String(name)
	// A nickname or pronouns in brackets are not the name, and what
	// follows a bar, a slash or a colon is a tagline.
	name = unbracket(name)
	if i := strings.IndexAny(name, "|/:"); i >= 0 {
		name = name[:i]
	}
	before, after, comma := strings.Cut(name, ",")
	words, titled := wordsOf(before)
	n := parsed{words: words, titled: titled}
	// "Beispiel, Mara" is the family name, then the given name. After a
	// comma there are degrees most often, like "Mara Beispiel, MBA", so
	// only one word before the comma and a name after it read that way.
	if comma && len(words) == 1 {
		if given, _ := wordsOf(after); len(given) > 0 && named(given, words[0]) {
			n.given, n.family = given[0], words[0]
			n.words = append(given, words[0])
		}
	}
	return n
}

// named says whether the words after a comma are a given name, not
// degrees or credentials. Those are written in capitals, like "ICF ACC",
// so a word in capitals is a name only where the family name is in
// capitals too, as in "BEISPIEL, MARA".
func named(given []string, family string) bool {
	for _, w := range given {
		if degree(w) || capitals(w) && !capitals(family) {
			return false
		}
	}
	return true
}

// capitals says whether a word of more than one letter is written in
// capitals only.
func capitals(w string) bool {
	return len([]rune(w)) > 1 && w == strings.ToUpper(w) && w != strings.ToLower(w)
}

// wordsOf finds the words of a name, and whether a title stood before
// them.
func wordsOf(name string) (out []string, titled bool) {
	lower := name == strings.ToLower(name)
	for _, w := range strings.Fields(split(name)) {
		if quoted(w) {
			// A nickname in quotes.
			continue
		}
		w = strings.TrimFunc(w, edge)
		lettered := strings.IndexFunc(w, unicode.IsLetter) >= 0
		switch {
		case !lettered && len(out) == 0:
			// An emoji before the name.
			continue
		case !lettered || strings.HasPrefix(w, "#") || strings.HasPrefix(w, "@"):
			// A dash, a dot, an emoji or a hashtag after it ends the name.
			return trim(out), titled
		case len(out) > 0 && !lower && small(w) && !particles[w]:
			// A word in small letters after it, like "für" or "geb.",
			// starts a tagline or a maiden name. "von" and "de" are part
			// of the family name.
			return trim(out), titled
		case len(out) == 0 && title(w, titled):
			titled = true
			continue
		}
		out = append(out, w)
	}
	return trim(out), titled
}

// split puts a space after the dot of a title written onto the name, so
// "Dr.Mara" reads as "Dr. Mara". "Dr.-Ing" and "Mag.a" stay one word.
func split(name string) string {
	r := []rune(name)
	var b strings.Builder
	for i, c := range r {
		b.WriteRune(c)
		if c == '.' && i+1 < len(r) && unicode.IsUpper(r[i+1]) {
			b.WriteRune(' ')
		}
	}
	return b.String()
}

// edge says whether a character at the edge of a word is no part of the
// name, like an emoji or a sparkle written onto it. A dot and a hyphen
// stay, since they mark titles and initials.
func edge(r rune) bool {
	return !unicode.IsLetter(r) && !unicode.IsMark(r) && r != '.' && r != '-' && r != '#' && r != '@'
}

// quoted says whether a word stands in quotes, as a nickname does.
func quoted(w string) bool {
	r := []rune(w)
	return len(r) > 2 && strings.ContainsRune(`"“”„«»'‘’‚`, r[0]) && strings.ContainsRune(`"“”„«»'‘’‚`, r[len(r)-1])
}

// unbracket takes out what stands in brackets, and everything after a
// bracket that is never closed.
func unbracket(name string) string {
	for {
		i := strings.IndexAny(name, "([")
		if i < 0 {
			return name
		}
		j := strings.IndexAny(name[i:], ")]")
		if j < 0 {
			return name[:i]
		}
		name = name[:i] + " " + name[i+j+1:]
	}
}

// trim takes the degrees and initials off the end of the name, but never
// its first word.
func trim(w []string) []string {
	for len(w) > 1 && degree(w[len(w)-1]) {
		w = w[:len(w)-1]
	}
	return w
}

// byLink is the first name as the profile's link orders the name: the
// word whose letters stand first in the link, joined to another word of
// the name, as "marabeispiel" joins Mara and Beispiel. Two words joined
// make sure the link is made from the name, not a business or a slogan
// that holds one of its words. It is empty when the link says nothing.
func byLink(words []string, link string) string {
	handle := fold(handleOf(link))
	if handle == "" {
		return ""
	}
	best, at := "", len(handle)
	for i, a := range words {
		for j, b := range words {
			if i == j || len([]rune(fold(a))) < 2 {
				continue
			}
			if k := strings.Index(handle, fold(a)+fold(b)); k >= 0 && k < at {
				best, at = a, k
			}
		}
	}
	return best
}

// handleOf is the part of a profile's link after /in/, like
// "mara-beispiel-4b1a2c93", with its escapes read.
func handleOf(link string) string {
	u, err := url.Parse(strings.TrimSpace(link))
	if err != nil {
		return ""
	}
	_, handle, ok := strings.Cut(strings.Trim(u.Path, "/"), "in/")
	if !ok || strings.Contains(handle, "/") {
		return ""
	}
	return handle
}

// fold writes a name the way a link may, in small letters, without
// accents, hyphens or digits, and with "ae", "oe" and "ue" as the
// umlauts they stand for, so Jörg, Joerg and jorg are the same.
func fold(s string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(strings.ToLower(s)) {
		switch {
		case r == 'ß':
			b.WriteString("ss")
		case unicode.IsLetter(r):
			b.WriteRune(r)
		}
	}
	return strings.NewReplacer("ae", "a", "oe", "o", "ue", "u").Replace(b.String())
}

// cased writes a name in capitals or small letters only, like "MARA" or
// "mara", the way a greeting does, "Mara", and "ANNA-LENA" as
// "Anna-Lena". A name in mixed case, like "McKenzie", stays as it is.
func cased(w string) string {
	if w != strings.ToUpper(w) && w != strings.ToLower(w) {
		return w
	}
	parts := strings.Split(strings.ToLower(w), "-")
	for i, p := range parts {
		r := []rune(p)
		if len(r) > 0 {
			r[0] = unicode.ToUpper(r[0])
		}
		parts[i] = string(r)
	}
	return strings.Join(parts, "-")
}

// small says whether a word starts with a small letter.
func small(w string) bool {
	r := []rune(w)
	return len(r) > 0 && unicode.IsLower(r[0])
}

// particles are the small words that are part of a family name, like
// "von" in "Mara von Beispiel" and "al" in "Fatma al Beispiel".
var particles = set("von", "vom", "van", "der", "den", "de", "del", "della",
	"di", "da", "das", "dos", "du", "des", "le", "la", "zu", "zum", "zur",
	"ten", "ter", "op", "al", "el", "bin", "ibn", "y")

// titles are the titles that stand before a name, in small letters and
// without their dots, so "Dr." and "Dr" are the same. "Mag.a" is "mag".
var titles = set("dr", "drs", "dres", "prof", "professor", "professorin",
	"doktor", "dipl", "mag", "pd", "priv", "univ", "apl", "habil", "herr",
	"frau", "mr", "mrs", "ms", "sir", "dott", "dott.ssa")

// fields are the words that say a doctor's field, like "med" in "Dr.
// med", or a degree's, like "Ing" in "Dr.-Ing". They are titles only
// after another, since "Phil", "Nat" and "Ing" are first names too.
var fields = set("med", "dent", "vet", "rer", "nat", "pol", "oec", "phil",
	"jur", "theol", "soc", "sc", "hc", "mult", "ing", "univ", "medic")

// professions are the letters of a profession written before a name, as
// they are spelt, like "RA", a lawyer, and "StB", a tax adviser.
var professions = set("RA", "RAin", "StB", "WP")

// degrees are the degrees and certificates written after a name without
// dots, as they are spelt, so "Ma", a family name, is not "MA". A word
// that ends with a dot is one too, like "M.Sc.", "LL.M." or an initial.
// Before a name, they are titles.
var degrees = set("MBA", "EMBA", "DBA", "PhD", "PHD", "MSc", "BSc", "MA",
	"BA", "MD", "LLM", "LLB", "MPH", "MEng", "BEng", "MAS", "CFA", "CPA",
	"ACC", "PCC", "MCC", "ICF", "Jr", "Sr")

// title says whether a word before a name is a title, given whether one
// stood before it. A word that ends with a dot is one, like "med." in
// "Dr. med.", and so is an initial, so "J. Mara Beispiel" is greeted as
// Mara. A word joined by hyphens is one only when each of its parts is,
// like "Dr.-Ing" and "Dipl.-Kfm.", so "Hans-J." and "Ing-Marie" are
// names.
func title(w string, titled bool) bool {
	if parts := strings.Split(w, "-"); len(parts) > 1 {
		for _, p := range parts {
			if p == "" || !title(p, true) {
				return false
			}
		}
		return true
	}
	if strings.HasSuffix(w, ".") || professions[w] || degrees[w] {
		return true
	}
	first, _, _ := strings.Cut(strings.ToLower(w), ".")
	return titles[first] || titled && fields[first]
}

// degree says whether a word after a name is a degree or an initial.
func degree(w string) bool {
	return strings.HasSuffix(w, ".") || degrees[strings.ReplaceAll(w, ".", "")]
}

func set(words ...string) map[string]bool {
	m := make(map[string]bool, len(words))
	for _, w := range words {
		m[w] = true
	}
	return m
}
