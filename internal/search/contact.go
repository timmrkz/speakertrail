package search

import (
	"regexp"
	"strings"
)

// WithoutContact is a profile's text with every email address and phone
// number taken out, because the app never stores either, see CLAUDE.md,
// Personal data. It runs where Exa's answer comes in, so no text with
// them reaches the file or the language model. The rubric's own contact
// pattern is wider and would take the dates of jobs too, "2019 - 2023",
// which a first message is written from.
func WithoutContact(text string) string {
	text = email.ReplaceAllString(text, "")
	text = phone.ReplaceAllStringFunc(text, func(m string) string {
		if datelike.MatchString(m) || digits(m) < 8 {
			return m
		}
		return ""
	})
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(spaces.ReplaceAllString(l, " "), " \t")
	}
	return strings.Join(lines, "\n")
}

var (
	// An address has a dot after its @, so "Founder @ Studio" is none.
	email = regexp.MustCompile(`[\w.+-]+@[\w-]+(?:\.[\w-]+)+`)
	// A number starts with + or a 0, the way German and international
	// numbers are written, and runs on through spaces, slashes, dashes,
	// dots and brackets.
	phone = regexp.MustCompile(`(?:\+|\(?\b0)\d[\d /().-]{5,}\d`)
	// A run of digits that holds a date, "01.03.2020" or "10/2019", is a
	// date range, not a number to call.
	datelike = regexp.MustCompile(`\b\d{1,2}[./]\d{1,2}[./]\d{2,4}\b|\b\d{1,2}/\d{4}\b`)
	spaces   = regexp.MustCompile(`[ \t]{2,}`)
)

func digits(s string) int {
	n := 0
	for _, r := range s {
		if r >= '0' && r <= '9' {
			n++
		}
	}
	return n
}
