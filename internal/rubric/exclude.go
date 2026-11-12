package rubric

import (
	"strings"
	"unicode"
)

// Excluded tells whether a person's headline and roles contain one of
// Tim's exclusions, like "Online Marketing", and which. Tim asked for them
// on 10 November 2026: half of what some searches bring are online and
// LinkedIn marketing experts, and he wants more kinds of people.
//
// A phrase matches at the start of a word, in any case, and whatever
// stands between its words: "Online Marketing" matches "Online-Marketing",
// "online marketing" and "Online-Marketing-Expertin", and also the word
// written as one, "Onlinemarketing" or "#onlinemarketing". It does not
// match inside a word, so "Coach" leaves "Lifecoach" alone.
func Excluded(text string, phrases []string) (string, bool) {
	words := " " + strings.Join(splitWords(text), " ")
	for _, p := range phrases {
		pw := splitWords(p)
		if len(pw) == 0 {
			continue
		}
		if strings.Contains(words, " "+strings.Join(pw, " ")) || strings.Contains(words, " "+strings.Join(pw, "")) {
			return p, true
		}
	}
	return "", false
}

// NormalPhrase is an exclusion as it is stored: its words, in their own
// case, with single spaces between them. Empty when it has no word.
func NormalPhrase(p string) string {
	return strings.Join(strings.FieldsFunc(p, notWord), " ")
}

func splitWords(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), notWord)
}

func notWord(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }
