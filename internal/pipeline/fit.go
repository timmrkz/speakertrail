package pipeline

import (
	"regexp"
	"strings"
	"sync"

	"github.com/timmrkz/speakertrail/internal/extract"
	"github.com/timmrkz/speakertrail/internal/settings"
)

// FitRules decide whether an event goes into the calendar.
type FitRules struct {
	Region    map[string]bool
	KeepWords []string
	DropWords []string
}

// RulesFrom reads the fit rules from the settings.
func RulesFrom(s settings.Settings) FitRules {
	return FitRules{
		Region:    s.Region(),
		KeepWords: s.Strings("keep_words", []string{"founder", "gründer", "pitch", "meetup", "sport", "maker"}),
		DropWords: s.Strings("drop_words", []string{"webinar", "training", "sales"}),
	}
}

// Fit returns whether an event is kept, and why. Online events, events
// outside the region and titles with a drop word are dropped. Everything
// else in the region is kept, and the reason names a keep word when one
// matches.
func (r FitRules) Fit(e extract.Event) (kept bool, reason string) {
	if e.Format == "online" {
		return false, "online"
	}
	if e.City == "" {
		return false, "city unknown"
	}
	if !r.Region[strings.ToLower(e.City)] {
		return false, "outside the region: " + e.City
	}
	if w := firstWord(e.Title, r.DropWords); w != "" {
		return false, "drop word: " + w
	}
	for _, text := range []string{e.Title, e.Description} {
		if w := firstWord(text, r.KeepWords); w != "" {
			return true, "keep word: " + w
		}
	}
	return true, "in person in " + e.City
}

// wordCache holds compiled word patterns. Workers share it.
var wordCache sync.Map

// firstWord returns the first word of words that appears in text, matched
// at a word start so "sales" does not match "wholesalers".
func firstWord(text string, words []string) string {
	for _, w := range words {
		w = strings.TrimSpace(w)
		if w == "" {
			continue
		}
		v, ok := wordCache.Load(w)
		if !ok {
			v, _ = wordCache.LoadOrStore(w, regexp.MustCompile(`(?i)(?:^|[^\p{L}])`+regexp.QuoteMeta(w)))
		}
		if v.(*regexp.Regexp).MatchString(text) {
			return w
		}
	}
	return ""
}
