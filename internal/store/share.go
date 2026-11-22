package store

import (
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
)

// Shared is one person as Report a Problem takes them along when Tim
// ticks People, so Claude can read the data behind what People shows
// without screenshots. Only what is listed here goes, never the name,
// the profile link, the handle, the notes or the first message. The
// person's name and handle are taken out of every text that goes, see
// unnamed. Tim agreed to what goes on 22 November 2026, issue #73.
type Shared struct {
	ID       int64  `json:"id"`
	Headline string `json:"headline"`
	City     string `json:"city"`
	Roles    string `json:"roles,omitempty"`
	// BornAbout is the likely year of birth, zero when unknown, and
	// AgeReason the step on the profile it was read from.
	BornAbout  int        `json:"born_about,omitempty"`
	AgeReason  string     `json:"age_reason,omitempty"`
	LastActive *time.Time `json:"last_active,omitempty"`
	// Score and Signals are the fit rubric's, as the person screen shows
	// them, and Rubric the version that scored them.
	Score   int      `json:"score"`
	Rubric  int      `json:"rubric"`
	Signals []Signal `json:"signals"`
	// Connected says Tim marked that he connected with them, Review is
	// confirmed or rejected, and HiddenBy the exclusion that leaves them
	// out of People, if one does.
	Connected   bool       `json:"connected"`
	ConnectedAt *time.Time `json:"connected_at,omitempty"`
	Review      string     `json:"review"`
	HiddenBy    string     `json:"hidden_by,omitempty"`
	// HasText says the app has their profile text, and TextAt when Exa's
	// index gave it. The text itself stays on the Mac.
	HasText bool       `json:"has_text"`
	TextAt  *time.Time `json:"text_at,omitempty"`
	// HasDraft says a first message was drafted for them.
	HasDraft  bool          `json:"has_draft"`
	FirstSeen time.Time     `json:"first_seen"`
	FoundBy   []SharedFound `json:"found_by"`
}

// SharedFound is a search that found a person, by its words, and when it
// last did.
type SharedFound struct {
	Search string    `json:"search"`
	At     time.Time `json:"at"`
}

// Share gives every person the way Report a Problem takes them along,
// in the order they were found.
func (d *Data) Share() []Shared {
	hidden := d.Hidden()
	out := make([]Shared, 0, len(d.People))
	for _, p := range d.People {
		clean := unnamed(p.Name, p.LinkedIn.Handle)
		l := p.LinkedIn
		s := Shared{
			ID: p.ID, Headline: clean(p.Headline), City: p.City, Roles: clean(l.Roles),
			BornAbout: l.BornAbout, AgeReason: clean(l.AgeReason), LastActive: when(l.LastActive),
			Score: p.Score, Rubric: p.Rubric, Signals: make([]Signal, 0, len(p.Signals)),
			Connected: p.Connected, ConnectedAt: when(p.ConnectedAt), Review: l.Review, HiddenBy: hidden[p.ID],
			HasText: l.Text != "", TextAt: when(l.TextAt), HasDraft: p.Draft != "",
			FirstSeen: p.FirstSeen, FoundBy: make([]SharedFound, 0, len(p.FoundBy)),
		}
		for _, sig := range p.Signals {
			sig.Label, sig.Passage = clean(sig.Label), clean(sig.Passage)
			s.Signals = append(s.Signals, sig)
		}
		for _, f := range p.FoundBy {
			if search := d.Search(f.SearchID); search != nil {
				s.FoundBy = append(s.FoundBy, SharedFound{Search: search.Query, At: f.At})
			}
		}
		out = append(out, s)
	}
	return out
}

func when(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// unnamed gives what takes a person's name out of a text: each word of
// the name, of three letters or more, and the profile's handle whole, are
// written [name] wherever they stand as a word of their own, in any case.
// A business named after its owner, "Studio Lea Probe", so leaves no
// name behind in the roles or a passage. The words of a handle are not
// taken one by one, since one like yoga-mit-lea would take out "yoga"
// and "mit" everywhere.
func unnamed(name, handle string) func(string) string {
	var words []string
	if len([]rune(handle)) >= 3 {
		words = append(words, strings.ToLower(handle))
	}
	for _, w := range strings.FieldsFunc(name, func(r rune) bool { return !unicode.IsLetter(r) }) {
		if len([]rune(w)) >= 3 && !slices.Contains(words, strings.ToLower(w)) {
			words = append(words, strings.ToLower(w))
		}
	}
	if len(words) == 0 {
		return func(s string) string { return s }
	}
	// Longest first, so a longer word is never cut by a shorter one.
	slices.SortFunc(words, func(a, b string) int { return len(b) - len(a) })
	for i, w := range words {
		words[i] = regexp.QuoteMeta(w)
	}
	// A word of letters alone: no letter or digit on either side.
	re := regexp.MustCompile(`(?i)(^|[^\p{L}\p{N}])(` + strings.Join(words, "|") + `)($|[^\p{L}\p{N}])`)
	return func(s string) string {
		// Twice, since two names next to each other share the space
		// between them and the first match takes it.
		for range 2 {
			s = re.ReplaceAllString(s, "${1}[name]${3}")
		}
		return s
	}
}
