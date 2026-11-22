package store

import (
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
)

// Shared is one person as Report a Problem takes them along when Tim
// switches People on, so Claude can read the data behind what People
// shows without screenshots, and score a profile again from its text.
// Only what is listed here goes, never the name, the profile link, the
// handle, the notes or the first message. The person's name and handle
// are taken out of every text that goes, the profile text too, see
// unnamed. Tim chose this on 22 November 2026, over the same without
// the profile texts and over numbers alone, issue #73.
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
	// Fit and FitParts are the fit People orders by, as the person shows
	// it on the day of the report, see Data.Fit. LastWrote is the newest
	// post or comment the profile text shows, and Wrote whether its
	// activity says what kind it is.
	Fit       int          `json:"fit"`
	FitParts  []SharedPart `json:"fit_parts"`
	LastWrote *time.Time   `json:"last_wrote,omitempty"`
	Wrote     bool         `json:"wrote"`
	// Connected says Tim marked that he connected with them, Review is
	// confirmed or rejected, and HiddenBy the exclusion that leaves them
	// out of People, if one does.
	Connected   bool       `json:"connected"`
	ConnectedAt *time.Time `json:"connected_at,omitempty"`
	Review      string     `json:"review"`
	HiddenBy    string     `json:"hidden_by,omitempty"`
	// Text is the whole profile as Exa's index had it at TextAt, empty
	// while the app has none.
	Text   string     `json:"text,omitempty"`
	TextAt *time.Time `json:"text_at,omitempty"`
	// HasDraft says a first message was drafted for them.
	HasDraft  bool          `json:"has_draft"`
	FirstSeen time.Time     `json:"first_seen"`
	FoundBy   []SharedFound `json:"found_by"`
}

// SharedPart is one part of the fit, with its points and why.
type SharedPart struct {
	Key    string `json:"key"`
	Points int    `json:"points"`
	Why    string `json:"why"`
}

// SharedFound is a search that found a person, by its words, and when it
// last did.
type SharedFound struct {
	Search string    `json:"search"`
	At     time.Time `json:"at"`
}

// Share gives every person the way Report a Problem takes them along,
// in the order they were found. What it gives is copied under the lock,
// and the names are taken out after, since that reads every profile text
// and the window and a run would wait on it: 1.5 s for 1000 people of
// 8000 characters on a 4-core build runner.
func (s *Store) Share() []Shared {
	var out []Shared
	var names [][2]string
	now := s.now()
	s.Read(func(d *Data) { out, names = d.share(now) })
	for i := range out {
		clean := unnamed(names[i][0], names[i][1])
		sh := &out[i]
		sh.Headline, sh.Roles, sh.AgeReason, sh.Text = clean(sh.Headline), clean(sh.Roles), clean(sh.AgeReason), clean(sh.Text)
		for j := range sh.Signals {
			sh.Signals[j].Label, sh.Signals[j].Passage = clean(sh.Signals[j].Label), clean(sh.Signals[j].Passage)
		}
		for j := range sh.FitParts {
			sh.FitParts[j].Why = clean(sh.FitParts[j].Why)
		}
	}
	return out
}

// share copies every person as Shared, names and all, with each one's
// name and handle beside it for Share to take out.
func (d *Data) share(now time.Time) ([]Shared, [][2]string) {
	hidden := d.Hidden()
	out := make([]Shared, 0, len(d.People))
	names := make([][2]string, 0, len(d.People))
	for _, p := range d.People {
		l := p.LinkedIn
		s := Shared{
			ID: p.ID, Headline: p.Headline, City: p.City, Roles: l.Roles,
			BornAbout: l.BornAbout, AgeReason: l.AgeReason, LastActive: when(l.LastActive),
			Score: p.Score, Rubric: p.Rubric, Signals: append(make([]Signal, 0, len(p.Signals)), p.Signals...),
			Connected: p.Connected, ConnectedAt: when(p.ConnectedAt), Review: l.Review, HiddenBy: hidden[p.ID],
			Text: l.Text, TextAt: when(l.TextAt), HasDraft: p.Draft != "",
			FirstSeen: p.FirstSeen, FoundBy: make([]SharedFound, 0, len(p.FoundBy)),
		}
		fit, parts := d.Fit(p, now)
		s.Fit, s.FitParts, s.LastWrote, s.Wrote = fit, make([]SharedPart, 0, len(parts)), when(l.LastWrote), l.Wrote
		for _, pt := range parts {
			s.FitParts = append(s.FitParts, SharedPart{Key: pt.Key, Points: pt.Points, Why: pt.Why})
		}
		for _, f := range p.FoundBy {
			if search := d.Search(f.SearchID); search != nil {
				s.FoundBy = append(s.FoundBy, SharedFound{Search: search.Query, At: f.At})
			}
		}
		out = append(out, s)
		names = append(names, [2]string{p.Name, l.Handle})
	}
	return out, names
}

func when(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// links are the addresses in a text, like the link to each post in the
// activity of a profile text, which lead to the person. Each is written
// [link].
var links = regexp.MustCompile(`(?i)\b(?:https?://|www\.)[^\s)\]>"]+`)

// unnamed gives what takes a person's own name out of a text, as the
// app has it: each word of the name of three letters or more that starts
// with a capital, and the profile's handle whole, are written [name]
// wherever they stand as a word of their own, in any case, also with an
// s after them, "Annas Studio", and in the letters of a keyboard without
// umlauts, "Mueller" for "Müller". A business named after its owner,
// "Studio Lea Probe", so leaves no name behind in the roles or a
// passage. Every link is taken out too, see links. Other people's names
// in a text go along, as Tim accepted.
//
// "von" and "der" in "Anna von der Heide" stay, since they are words of
// every German text. A surname that is a word too, like Koch, is taken
// out everywhere, since a name left in is worse than a word lost. The
// words of a handle are not taken one by one, since one like yoga-mit-lea
// would take out "yoga" and "mit" everywhere.
func unnamed(name, handle string) func(string) string {
	var words []string
	add := func(w string) {
		if w = strings.ToLower(w); !slices.Contains(words, w) {
			words = append(words, w)
		}
	}
	if len([]rune(handle)) >= 3 {
		add(handle)
	}
	for _, w := range strings.FieldsFunc(name, func(r rune) bool { return !unicode.IsLetter(r) }) {
		if len([]rune(w)) >= 3 && unicode.IsUpper([]rune(w)[0]) {
			add(w)
			add(plain.Replace(w))
		}
	}
	if len(words) == 0 {
		return func(s string) string { return links.ReplaceAllString(s, "[link]") }
	}
	// Longest first, so a longer word is never cut by a shorter one.
	slices.SortFunc(words, func(a, b string) int { return len(b) - len(a) })
	for i, w := range words {
		words[i] = regexp.QuoteMeta(w)
	}
	// A word of letters alone, with an s after it or not: no letter or
	// digit on either side.
	re := regexp.MustCompile(`(?i)(^|[^\p{L}\p{N}])(?:` + strings.Join(words, "|") + `)(s?)($|[^\p{L}\p{N}])`)
	return func(s string) string {
		s = links.ReplaceAllString(s, "[link]")
		// Twice, since two names next to each other share the space
		// between them and the first match takes it.
		for range 2 {
			s = re.ReplaceAllString(s, "${1}[name]${2}${3}")
		}
		return s
	}
}

// plain writes a name in the letters of a keyboard without umlauts.
var plain = strings.NewReplacer("ä", "ae", "ö", "oe", "ü", "ue", "Ä", "Ae", "Ö", "Oe", "Ü", "Ue", "ß", "ss", "é", "e", "è", "e")
