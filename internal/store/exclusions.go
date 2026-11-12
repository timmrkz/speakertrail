package store

import (
	"cmp"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/timmrkz/speakertrail/internal/rubric"
)

// Exclusion is a word or phrase, like "Online Marketing", that leaves a
// person out of profile searches and People. Tim asked for them on 10
// November 2026, because online and LinkedIn marketing experts made up
// half of what some searches brought. How a phrase matches is
// rubric.Excluded.
type Exclusion struct {
	ID        int64     `json:"id"`
	Phrase    string    `json:"phrase"`
	CreatedAt time.Time `json:"created_at,omitzero"`
}

// startingExclusions are those a new file begins with.
var startingExclusions = []string{
	"Online Marketing", "Social Media Marketing", "Performance Marketing",
	"Personal Branding", "LinkedIn", "Sichtbarkeit",
}

// MaxPhrase is the longest an exclusion can be, in characters.
const MaxPhrase = 60

// ErrExclusionExists says the same phrase, in any case, is there already.
var ErrExclusionExists = errors.New("this exclusion is already there")

// ErrNoPhrase says an exclusion has no word in it.
var ErrNoPhrase = errors.New("enter a word or phrase, like Online Marketing")

// ErrLongPhrase says an exclusion is longer than MaxPhrase.
var ErrLongPhrase = errors.New("an exclusion is at most 60 characters")

// AddExclusion adds a word or phrase, with its words in their own case and
// single spaces between them.
func (d *Data) AddExclusion(phrase string, at time.Time) (*Exclusion, error) {
	phrase = rubric.NormalPhrase(phrase)
	switch {
	case phrase == "":
		return nil, ErrNoPhrase
	case len([]rune(phrase)) > MaxPhrase:
		return nil, ErrLongPhrase
	}
	for _, x := range d.Exclusions {
		if strings.EqualFold(x.Phrase, phrase) {
			return x, ErrExclusionExists
		}
	}
	x := &Exclusion{ID: d.NewID(), Phrase: phrase, CreatedAt: at}
	d.Exclusions = append(d.Exclusions, x)
	return x, nil
}

// DeleteExclusion removes one. The people it hid come back. It reports
// whether it was there.
func (d *Data) DeleteExclusion(id int64) bool {
	n := len(d.Exclusions)
	d.Exclusions = slices.DeleteFunc(d.Exclusions, func(x *Exclusion) bool { return x.ID == id })
	return len(d.Exclusions) < n
}

// Phrases are the exclusions' phrases, in the order they were added.
func (d *Data) Phrases() []string {
	out := make([]string, 0, len(d.Exclusions))
	for _, x := range d.Exclusions {
		out = append(out, x.Phrase)
	}
	return out
}

// SortedExclusions are the exclusions in the order of their phrases, as
// Tim sees them.
func (d *Data) SortedExclusions() []*Exclusion {
	out := slices.Clone(d.Exclusions)
	slices.SortStableFunc(out, func(a, b *Exclusion) int {
		return cmp.Compare(strings.ToLower(a.Phrase), strings.ToLower(b.Phrase))
	})
	return out
}

// Hidden finds the people an exclusion leaves out, by their headline and
// the roles their LinkedIn profile says they hold now, each with the
// phrase that does, the first that matches in the order of the phrases.
// Whoever Tim connected with is never hidden.
func (d *Data) Hidden() map[int64]string {
	phrases := make([]string, 0, len(d.Exclusions))
	for _, x := range d.SortedExclusions() {
		phrases = append(phrases, x.Phrase)
	}
	out := map[int64]string{}
	if len(phrases) == 0 {
		return out
	}
	for _, p := range d.People {
		if p.Connected {
			continue
		}
		roles := ""
		if p.LinkedIn.Review != "rejected" {
			roles = p.LinkedIn.Roles
		}
		if phrase, ok := rubric.Excluded(p.Headline+" "+roles, phrases); ok {
			out[p.ID] = phrase
		}
	}
	return out
}
