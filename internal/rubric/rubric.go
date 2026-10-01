// Package rubric says whether a person fits My First Memory, whose guests
// are people who work with people: face to face, running it themselves,
// used to talking about what matters to someone. See
// docs/search-strategy.md.
//
// It reads what is known about a person and finds signals for and against
// a fit, each with the passage that shows it. A claim without a passage
// does not count. The score is the number of signals for, less the number
// against, so every point has a reason Tim can read.
package rubric

import (
	"regexp"
	"strings"
	"unicode"
)

// Version changes whenever the rubric does, its words or its labels, so
// everyone is scored again.
const Version = 1

// Where a passage comes from.
const (
	FromTitle     = "title"
	FromEventPage = "event page"
	FromEvent     = "event"
	FromImprint   = "imprint"
	FromLookup    = "lookup"
	FromPortfolio = "portfolio"
	FromAbout     = "about page"
	FromModel     = "model"
)

// Def is one signal of the rubric.
type Def struct {
	Key   string
	For   bool
	Label string
	// Means says what the signal means, for the language model.
	Means string
	// pattern finds the signal in a text, unless finds what spoils it in
	// the same text, like "Product Owner" for an owner.
	pattern, unless *regexp.Regexp
}

func words(expr string) *regexp.Regexp {
	return regexp.MustCompile(`(?i)(?:^|[^\pL\pN])(?:` + expr + `)(?:$|[^\pL\pN])`)
}

// Defs is the rubric, signals for first. The words cover German and
// English, the languages of pages in NRW.
var Defs = []Def{
	{Key: "works_with_people", For: true, Label: "Works with people",
		Means:   "coaches, teaches, trains, treats or counsels people face to face, or runs a gym, studio or academy where people train",
		pattern: words(`\w*coach(?:in|es|ing)?|mentor(?:in)?|\w*trainer(?:in)?|lehrer(?:in)?|teacher|dozent(?:in)?|instructor|übungsleiter(?:in)?|yoga\w*|pilates|physiotherapeut(?:in)?|ergotherapeut(?:in)?|ernährungsberater(?:in)?|heilpraktiker(?:in)?|hebamme|counsell?or|begleitet menschen|bjj|jiu[- ]?jitsu|kampfsport\w*|boxing|boxen|dojo|crossfit|gym|fitness\w*|tanzschule`),
		unless:  words(`agile coach|scrum|sales[- ]?trainer|vertriebstrainer`)},
	{Key: "owner_operator", For: true, Label: "Runs it themselves",
		Means:   "owns and runs a small business, studio, gym or practice themselves, with a small team",
		pattern: words(`inhaber(?:in)?|owner|besitzer(?:in)?|einzelunternehmer(?:in)?|e\.\s?k\.|selbstständig\w*|selbständig\w*|freiberuflich\w*|in eigener praxis|eigene[nrs]? (?:praxis|studio|schule|gym|laden|betrieb|café|cafe)`),
		unless:  words(`product owner|process owner|business owner at`)},
	{Key: "bootstrapped", For: true, Label: "Built it from own means",
		Means:   "built the business from their own means, without investors",
		pattern: words(`bootstrapp\w*|ohne investor(?:en)?|ohne fremdkapital|eigenfinanziert|aus eigene[rn] (?:kraft|mitteln|tasche)|self[- ]funded|selbst finanziert`)},
	{Key: "runs_events", For: true, Label: "Runs their own events",
		Means:   "hosts or organises their own workshops, seminars, open mats, meetups, readings or retreats, not only speaks at them",
		pattern: words(`veranstalter(?:in)?|organisator(?:in)?|gastgeber(?:in)?|initiator(?:in)?|host of|organi[sz]er of|hosts (?:a|the|her|his|their)`)},
	{Key: "author", For: true, Label: "Author",
		Means:   "has written a book",
		pattern: words(`autor(?:in)?|author|buchautor(?:in)?|bestseller\w*|schriftsteller(?:in)?|(?:sein|ihr|his|her|new|neues|erstes|first) (?:\w+ )?(?:buch|book)`)},
	{Key: "athlete", For: true, Label: "Athlete",
		Means: "competes or competed in a sport, holds a belt or a title",
		// A master craftsman is a Meister too, so only a champion's title counts.
		pattern: words(`athlet(?:in)?|athlete|(?:leistungs|profi|spitzen|kampf)sportler(?:in)?|(?:welt|europa|landes|bundes)meister(?:in)?|deutsche[rn]? meister(?:in)?|meistertitel|olympi\w+|world champion|champion|black ?belt|schwarzgurt|schwarzer gurt|profiboxer(?:in)?|triathlet(?:in)?|ultraläufer(?:in)?|marathonläufer(?:in)?|nationalspieler(?:in)?|nationalmannschaft|bundesliga\w*`)},
	{Key: "career_switch", For: true, Label: "Switched careers towards people",
		Means:   "left another career to work with people",
		pattern: words(`quereinst\w*|career (?:change|switch)|changed careers|switched careers|neuorientierung|beruflich neu \w+|umgesattelt|sattelte um`)},
	{Key: "still_active", For: true, Label: "Still active",
		Means: "what they run shows recent signs of life"},

	{Key: "funded", For: false, Label: "Raised money",
		Means:   "raised money from investors: funding rounds, Series A, backed by investors",
		pattern: words(`series [a-d]|seed[- ]?(?:runde|round|finanzierung|investment|investition)|finanzierungsrunde|funding round|raised|eingesammelt|venture capital|risikokapital|backed by|finanziert von|funded by|investment von`)},
	{Key: "corporate", For: false, Label: "Corporate title",
		Means:   "a corporate title like Head of, VP or Chief … Officer at a large company",
		pattern: words(`head of|vp|vice president|chief \w+ officer|director of|senior director|managing partner|bereichsleiter(?:in)?|abteilungsleiter(?:in)?|vorstand\w*|country manager|global \w+ lead`),
		// The managing director an imprint names runs the company.
		unless: words(`managing director`)},
	{Key: "large_company", For: false, Label: "Large company",
		Means:   "a large team, or a company without a person behind it",
		pattern: words(`konzern\w*|corporation|\d{3,}\s*(?:mitarbeiter\w*|mitarbeitende\w*|employees)|weltweit führend\w*`)},
	{Key: "enterprise", For: false, Label: "Sells to companies",
		Means:   "sells software or services to other companies",
		pattern: words(`b2b|saas|enterprise|software (?:für|for) (?:unternehmen|companies|enterprises|businesses)|lösungen für unternehmen|plattform für unternehmen|für den mittelstand`)},
	{Key: "therapist", For: false, Label: "Licensed therapist",
		Means:   "a licensed psychotherapist, who has strict rules about what they say in public. Coaches are not therapists",
		pattern: words(`psychotherapeut(?:in)?|psychotherapist|psychotherapie|psychologische[rn]? psychotherapeut(?:in)?|approbiert\w*`)},
	{Key: "backed", For: false, Label: "Backed by a startup programme",
		Means: "listed in the portfolio of an accelerator, incubator or startup programme"},
	{Key: "inactive", For: false, Label: "No longer active",
		Means: "what they run is being wound up or gone"},
}

var byKey = func() map[string]*Def {
	m := map[string]*Def{}
	for i := range Defs {
		m[Defs[i].Key] = &Defs[i]
	}
	return m
}()

// Lookup returns the signal's definition, or nil for an unknown key.
func Lookup(key string) *Def {
	return byKey[key]
}

// Signal is one reason for or against a fit.
type Signal struct {
	Key     string
	For     bool
	Passage string
	Where   string
}

// Appearance is one event a person is on stage at.
type Appearance struct {
	Role     string
	Event    string
	Evidence string
}

// Company is what a person founded or runs, as lookups saw it.
type Company struct {
	Name         string
	Activity     string
	ActivityNote string
	// Founders is how many people its imprint names.
	Founders int
	// Portfolio names the startup programme that lists it, if any.
	Portfolio string
	// Note is what a lookup noted, like "not a young company (AG)".
	Note string
}

// Person is everything known about a person that the rubric reads.
type Person struct {
	Headline    string
	FitEvidence string
	Appearances []Appearance
	Companies   []Company
	// Found holds signals found on pages that are not kept, like an about
	// page, by the rules or by the language model, with where they come
	// from. Unknown keys and empty passages are left out.
	Found []Signal
}

type text struct{ where, s string }

// Signals finds the rubric's signals in what is known about a person, each
// once, with the first passage that shows it. Signals for come first, in
// the rubric's order.
func Signals(p Person) []Signal {
	var texts []text
	if p.Headline != "" {
		texts = append(texts, text{FromTitle, p.Headline})
	}
	imprint := strings.Contains(p.FitEvidence, ", by its imprint")
	if p.FitEvidence != "" {
		where := FromEventPage
		if imprint {
			where = FromImprint
		}
		texts = append(texts, text{where, p.FitEvidence})
	}
	for _, a := range p.Appearances {
		if a.Evidence != "" {
			texts = append(texts, text{FromEventPage, a.Evidence})
		}
	}

	found := map[string]Signal{}
	add := func(key, passage, where string) {
		d := byKey[key]
		if d == nil || passage == "" || contact.MatchString(passage) {
			return
		}
		if _, ok := found[key]; !ok {
			found[key] = Signal{Key: key, For: d.For, Passage: passage, Where: where}
		}
	}
	for _, d := range Defs {
		if d.pattern == nil {
			continue
		}
		for _, t := range texts {
			if d.unless != nil && d.unless.MatchString(t.s) {
				continue
			}
			if loc := d.pattern.FindStringIndex(t.s); loc != nil {
				add(d.Key, passage(t.s, loc[0], loc[1]), t.where)
				break
			}
		}
	}

	// Hosting an event, as the page puts it, or as its structure says.
	for _, a := range p.Appearances {
		if (a.Role == "host" || a.Role == "moderator") && a.Evidence != "" {
			add("runs_events", passage(a.Evidence, 0, 0), FromEventPage)
		}
	}
	for _, a := range p.Appearances {
		if (a.Role == "host" || a.Role == "moderator") && a.Event != "" {
			add("runs_events", `Hosts "`+a.Event+`"`, FromEvent)
		}
	}

	for _, c := range p.Companies {
		// The imprint names one or two people who run the company.
		if imprint && c.Founders >= 1 && c.Founders <= 2 {
			add("owner_operator", p.FitEvidence, FromImprint)
		}
		// A startup programme's portfolio is built to grow with investors.
		if c.Portfolio != "" {
			add("backed", "In the portfolio of "+c.Portfolio, FromPortfolio)
		}
		switch c.Activity {
		case "active":
			add("still_active", c.Name+": "+c.ActivityNote, FromLookup)
		case "dissolved", "gone":
			add("inactive", c.Name+": "+c.ActivityNote, FromLookup)
		}
		if strings.Contains(c.Note, "(AG)") || strings.Contains(c.Note, "(SE)") {
			add("large_company", c.Name+": "+c.Note, FromLookup)
		}
	}

	for _, s := range p.Found {
		where := s.Where
		if where == "" {
			where = FromModel
		}
		add(s.Key, strings.TrimSpace(s.Passage), where)
	}

	var out []Signal
	for _, d := range Defs {
		if s, ok := found[d.Key]; ok {
			out = append(out, s)
		}
	}
	return out
}

// Score is the number of signals for, less the number against.
func Score(signals []Signal) int {
	n := 0
	for _, s := range signals {
		if s.For {
			n++
		} else {
			n--
		}
	}
	return n
}

// contact finds an email address or a phone number. A passage with one is
// never kept, because contact details are never stored.
var contact = regexp.MustCompile(`@|\+?\d[\d /().-]{6,}\d`)

// firstPerson finds a page speaking as "ich", like a sole trader's.
var firstPerson = words(`ich|mich|mir|mein\w*|i|my|me`)

// About finds the rubric's signals on a website's about page for one of
// the people its imprint names: in the sentences that name them, and when
// they run it alone, in those that speak as "ich" too, which also says
// they run it themselves. It reads what the page says, not what the rubric
// already knows, and each signal comes once.
func About(text, name string, alone bool) []Signal {
	var parts []string
	for _, w := range strings.Fields(name) {
		if len([]rune(w)) > 2 {
			parts = append(parts, regexp.QuoteMeta(w))
		}
	}
	if len(parts) == 0 {
		return nil
	}
	named := words(strings.Join(parts, "|"))
	var texts []string
	for _, s := range sentences(text) {
		if contact.MatchString(s) {
			continue
		}
		if named.MatchString(s) || alone && firstPerson.MatchString(s) {
			texts = append(texts, s)
		}
	}
	var out []Signal
	have := map[string]bool{}
	for _, d := range Defs {
		if d.pattern == nil {
			continue
		}
		for _, t := range texts {
			if d.unless != nil && d.unless.MatchString(t) {
				continue
			}
			if loc := d.pattern.FindStringIndex(t); loc != nil {
				out = append(out, Signal{Key: d.Key, For: d.For, Passage: passage(t, loc[0], loc[1]), Where: FromAbout})
				have[d.Key] = true
				break
			}
		}
	}
	if alone && !have["owner_operator"] {
		for _, t := range texts {
			// A heading like "Über mich" is not the page speaking.
			if len(strings.Fields(t)) < 4 {
				continue
			}
			if loc := firstPerson.FindStringIndex(t); loc != nil {
				out = append(out, Signal{Key: "owner_operator", For: true, Passage: passage(t, loc[0], loc[1]), Where: FromAbout})
				break
			}
		}
	}
	return out
}

// sentences splits a page's text into sentences and lines.
func sentences(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		start := 0
		for i := 0; i < len(line); i++ {
			if (line[i] == '.' || line[i] == '!' || line[i] == '?') && (i+1 == len(line) || line[i+1] == ' ') {
				if s := strings.TrimSpace(line[start : i+1]); s != "" {
					out = append(out, s)
				}
				start = i + 1
			}
		}
		if s := strings.TrimSpace(line[start:]); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// maxPassage is how long a passage may be, in characters.
const maxPassage = 160

// passage quotes the sentence of s around the match from i to j, and cuts
// a long one around the match. It stays a quote from s, with an ellipsis
// where it was cut.
func passage(s string, i, j int) string {
	start := 0
	for k := i - 1; k > 0; k-- {
		if (s[k-1] == '.' || s[k-1] == '!' || s[k-1] == '?' || s[k-1] == '\n') && unicode.IsSpace(rune(s[k])) {
			start = k
			break
		}
	}
	end := len(s)
	for k := j; k < len(s)-1; k++ {
		if (s[k] == '.' || s[k] == '!' || s[k] == '?' || s[k] == '\n') && unicode.IsSpace(rune(s[k+1])) {
			end = k + 1
			break
		}
	}
	out := strings.TrimSpace(s[start:end])
	if len([]rune(out)) <= maxPassage {
		return out
	}
	// Cut to about 70 characters on either side of the match, at spaces.
	from, to := max(i-70, start), min(j+70, end)
	for from > start && s[from-1] != ' ' {
		from--
	}
	for to < end && s[to] != ' ' {
		to++
	}
	cut := strings.TrimSpace(s[from:to])
	if from > start {
		cut = "…" + cut
	}
	if to < end {
		cut += "…"
	}
	return cut
}
