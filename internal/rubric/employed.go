package rubric

import (
	"regexp"
	"strings"
	"unicode"
)

// ownerTitle finds a title that runs a company, beside the words of
// Runs their own business, like "CEO" or "Partnerin".
var ownerTitle = words(`ceo|cto|coo|cfo|managing director|partner(?:in)?|teilhaber(?:in)?|unternehmer(?:in)?|entrepreneur`)

// sideRole finds a role held beside the work, not a job: a membership, an
// honour, a talk or a seat on a board.
var sideRole = words(`mitglied|member|ehrenamt\w*|volunteer|freiwillige[rn]?|botschafter(?:in)?|ambassador|speaker(?:in)?|redner(?:in)?|mentor(?:in)?|beirat\w*|advisory|board|jury|juror(?:in)?|host|moderator(?:in)?|podcast\w*|alumn\w*|stipendiat(?:in)?|fellow|gast\w*|vorsitz\w*|kassenwart(?:in)?|schriftführer(?:in)?|autor(?:in)?|author`)

// speaks finds a headline speaking to or as someone, like "Ich helfe dir
// bei …", which is no job at an organisation.
var speaks = words(`ich|mich|dir|dich|du|wir|uns|i|you|we|helfe\w*|help\w*|unterstütze\w*|begleite\w*`)

// headlineParts splits a headline into what it lists, like "Founder of
// Beispiel AI und Projekte beim Beispielverein" into its two halves.
var headlineParts = regexp.MustCompile(`\s*(?:\||·|•|,|/|\s\+\s|\s&\s|\sund\s|\sand\s)\s*`)

// atOrg finds where a title meets the organisation it is held at, like
// " bei " or " @ ".
var atOrg = regexp.MustCompile(`(?i)\s(?:bei|beim|at)\s|\s?@\s?`)

// ofOrg finds where an owner's title meets what they own, like " of " in
// "Founder of Beispiel".
var ofOrg = regexp.MustCompile(`(?i)\s(?:of|von|der|des)\s`)

type held struct{ title, org, passage, where string }

// employedElsewhere finds a job at another organisation than the one the
// person runs: a title that neither runs a company, nor works with
// people, nor is a role beside the work, at an organisation that is not
// theirs. A freelance trainer at a gym works with people, and stays. It
// gives the role that shows it, and where it was found.
func employedElsewhere(headline string, roles []string, name string) (passage, where string) {
	var all []held
	for _, r := range roles {
		title, org, _ := strings.Cut(r, ", ")
		all = append(all, held{title, org, r, FromProfile})
	}
	for _, seg := range headlineParts.Split(headline, -1) {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		h := held{title: seg, passage: seg, where: FromTitle}
		if loc := atOrg.FindStringIndex(seg); loc != nil {
			h.title, h.org = strings.TrimSpace(seg[:loc[0]]), strings.TrimSpace(seg[loc[1]:])
		} else if loc := ofOrg.FindStringIndex(seg); loc != nil && runsIt(seg) {
			h.title, h.org = strings.TrimSpace(seg[:loc[0]]), strings.TrimSpace(seg[loc[1]:])
		}
		all = append(all, h)
	}

	// Only beside a company known by name, one they run or one named
	// after them: "Inhaberin" over a role as "Fotografin, Lichtblick" is
	// most likely the same business.
	var owned []string
	for _, h := range all {
		if h.org != "" && (runsIt(h.title) || namedAfter(h.org, name)) {
			owned = append(owned, h.org)
		}
	}
	if len(owned) == 0 {
		return "", ""
	}
	for _, h := range all {
		if h.org == "" || runsIt(h.title) || runsIt(h.org) || worksWithPeople(h.title) || sideRole.MatchString(h.title) || namedAfter(h.org, name) {
			continue
		}
		// A headline says more than jobs: only a short title at
		// something named, like "Projekte beim Beispielverein", counts.
		if h.where == FromTitle && (len(strings.Fields(h.title)) > 4 || speaks.MatchString(h.title) || !named(h.org)) {
			continue
		}
		if !sameOrg(h.org, owned) {
			return h.passage, h.where
		}
	}
	return "", ""
}

// runsIt tells a title that runs a company, like "Inhaberin" or "CEO".
func runsIt(title string) bool {
	d := byKey["owner_operator"]
	return d.pattern.MatchString(title) && !d.unless.MatchString(title) || ownerTitle.MatchString(title)
}

func worksWithPeople(title string) bool {
	d := byKey["works_with_people"]
	return d.pattern.MatchString(title) && !d.unless.MatchString(title)
}

// named tells whether what a headline names reads like an organisation's
// name: every word starts with a capital or a figure, and it is more than
// one word, carries a figure or is in capitals, like "1. FC Köln" or
// "WDR". A German noun alone, like "Hochzeiten", is not enough.
func named(org string) bool {
	ws := strings.Fields(org)
	if len(ws) == 0 {
		return false
	}
	figure := false
	for _, w := range ws {
		r := []rune(w)[0]
		if !unicode.IsUpper(r) && !unicode.IsDigit(r) {
			return false
		}
		figure = figure || strings.ContainsAny(w, "0123456789")
	}
	return len(ws) > 1 || figure || len(org) > 1 && strings.ToUpper(org) == org
}

// sameOrg tells whether org is one of those owned, in any spelling, like
// "Beispiel AI" and "Beispiel AI GmbH".
func sameOrg(org string, owned []string) bool {
	a := letters(org)
	if a == "" {
		return true
	}
	for _, o := range owned {
		b := letters(o)
		if len(a) >= 3 && len(b) >= 3 && (strings.Contains(a, b) || strings.Contains(b, a)) {
			return true
		}
	}
	return false
}

// letters is s in lower case with only its letters and figures.
func letters(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
