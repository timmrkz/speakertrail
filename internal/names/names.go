// Package names finds the parts of a person's name as LinkedIn and Exa
// show it, where titles, degrees, pronouns and a tagline can stand
// beside it, like "Dr. med. Mara Beispiel" or "Mara Beispiel, MBA
// (sie/ihr)". It is the one place that says what a first name is.
package names

import (
	"strings"
	"unicode"
)

// First is the first name, to greet someone by. It is empty when the
// name holds none for sure, like "Dr. Beispiel", which is a title and a
// family name.
func First(name string) string {
	w, titled := words(name)
	if len(w) == 0 || titled && len(w) == 1 {
		return ""
	}
	return w[0]
}

// Family is the last word of the family name, "Beispiel" of "Mara von
// Beispiel". It is empty when the name has one word and no title before
// it, since that word is more likely a first name.
func Family(name string) string {
	w, titled := words(name)
	if len(w) >= 2 || titled && len(w) == 1 {
		return w[len(w)-1]
	}
	return ""
}

// words finds the name's own words, and whether a title stood before
// them.
func words(name string) (out []string, titled bool) {
	// A nickname or pronouns in brackets are not the name, and what
	// follows a comma, a bar, a slash or a colon is a degree or a tagline.
	name = unbracket(name)
	if i := strings.IndexAny(name, ",|/:"); i >= 0 {
		name = name[:i]
	}
	for _, w := range strings.Fields(name) {
		lettered := strings.IndexFunc(w, unicode.IsLetter) >= 0
		switch {
		case !lettered && len(out) == 0:
			// An emoji before the name.
			continue
		case !lettered:
			// A dash, a dot or an emoji after it ends the name.
			return trim(out), titled
		case len(out) == 0 && title(w, titled):
			titled = true
			continue
		}
		out = append(out, w)
	}
	return trim(out), titled
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

// titles are the titles that stand before a name, in small letters and
// without their dots, so "Dr." and "Dr" are the same. "Mag.a" is "mag".
var titles = set("dr", "drs", "prof", "professor", "professorin", "doktor",
	"dipl", "mag", "ing", "pd", "priv", "univ", "apl", "habil", "herr", "frau",
	"mr", "mrs", "ms", "sir")

// fields are the words that say a doctor's field, like "med" in "Dr.
// med". They are titles only after another, since "Phil" and "Nat" are
// first names too.
var fields = set("med", "dent", "vet", "rer", "nat", "pol", "oec", "phil",
	"jur", "theol", "soc", "sc", "hc", "mult")

// professions are the letters of a profession written before a name, as
// they are spelt, like "RA", a lawyer, and "StB", a tax adviser.
var professions = set("RA", "RAin", "StB", "WP")

// degrees are the degrees and certificates written after a name without
// dots, as they are spelt, so "Ma", a family name, is not "MA". A word
// that ends with a dot is one too, like "M.Sc.", "LL.M." or an initial.
var degrees = set("MBA", "EMBA", "DBA", "PhD", "PHD", "MSc", "BSc", "MA",
	"BA", "MD", "LLM", "LLB", "MPH", "MEng", "BEng", "MAS", "CFA", "CPA",
	"ACC", "PCC", "MCC")

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
	if strings.HasSuffix(w, ".") || professions[w] {
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
