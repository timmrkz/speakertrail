// Package names finds the parts of a person's name as LinkedIn and Exa
// show it, where titles, degrees, pronouns and a tagline can stand
// beside it, like "Dr. med. Mara Beispiel" or "Mara Beispiel, MBA
// (sie/ihr)". It is the one place that says what a first name is.
package names

import (
	"strings"
	"unicode"
)

// Words are the words of the name itself, without titles before it,
// degrees after it, pronouns or a tagline.
func Words(name string) []string {
	w, _ := words(name)
	return w
}

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
	// What follows a comma, a bar or a bracket is a degree, pronouns or a
	// tagline, never the name.
	if i := strings.IndexAny(name, ",|([/"); i >= 0 {
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
		case len(out) == 0 && title(w):
			titled = true
			continue
		}
		out = append(out, w)
	}
	return trim(out), titled
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
// without their dots, so "Dr." and "Dr" are the same. A word whose first
// part, before a dot or a hyphen, is one of them is a title too, like
// "Dr.-Ing", "Dipl.-Kfm." and "Mag.a".
var titles = set("dr", "drs", "prof", "professor", "professorin", "doktor",
	"dipl", "mag", "ing", "pd", "priv", "univ", "apl", "habil", "herr", "frau",
	"mr", "mrs", "ms", "sir")

// degrees are the degrees and certificates written after a name without
// dots, as they are spelt, so "Ma", a family name, is not "MA". A word
// that ends with a dot is one too, like "M.Sc.", "LL.M." or an initial.
var degrees = set("MBA", "EMBA", "DBA", "PhD", "PHD", "MSc", "BSc", "MA",
	"BA", "MD", "LLM", "LLB", "MPH", "MEng", "BEng", "MAS", "CFA", "CPA",
	"ACC", "PCC", "MCC")

// title says whether a word before a name is a title. Any word that ends
// with a dot there is one, like "med." in "Dr. med.", and so is an
// initial, so "J. Mara Beispiel" is greeted as Mara.
func title(w string) bool {
	if strings.HasSuffix(w, ".") {
		return true
	}
	first, _, _ := strings.Cut(strings.ReplaceAll(w, "-", "."), ".")
	return titles[strings.ToLower(first)]
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
