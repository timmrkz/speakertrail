package store

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"
)

// Found is a LinkedIn profile a profile search kept.
type Found struct {
	URL, Handle, Name, Headline, City string
	BornAbout                         int
	AgeReason, Roles                  string
	LastActive                        time.Time
}

// AddFound finds the person behind a profile, or adds them, with what the
// profile says now. It reports whether they are new.
func (d *Data) AddFound(f Found, search *Search, runID int64, at time.Time) (*Person, bool) {
	p := d.PersonByProfile(f.URL)
	isNew := p == nil
	if isNew {
		p = &Person{
			ID: d.NewID(), Name: f.Name, FirstSeen: at, FirstRunID: runID,
			Evidence: "Found by the profile search " + search.Query,
			LinkedIn: LinkedIn{URL: f.URL, Handle: f.Handle, Review: "confirmed"},
		}
		d.People = append(d.People, p)
	}
	p.UpdatedAt = at
	if p.City == "" {
		p.City = f.City
	}
	if f.Headline != "" {
		p.Headline, p.LinkedIn.Headline = f.Headline, f.Headline
	}
	l := &p.LinkedIn
	if f.BornAbout != 0 {
		l.BornAbout, l.AgeReason = f.BornAbout, f.AgeReason
	}
	if f.Roles != "" {
		l.Roles = f.Roles
	}
	if f.LastActive.After(l.LastActive) {
		l.LastActive = f.LastActive
	}
	seen := false
	for i := range p.FoundBy {
		if p.FoundBy[i].SearchID == search.ID {
			p.FoundBy[i].At, seen = at, true
		}
	}
	if !seen {
		p.FoundBy = append(p.FoundBy, Sighting{SearchID: search.ID, At: at})
	}
	return p, isNew
}

// Connect marks a person as connected on LinkedIn, or open again. Tim
// connects with someone on LinkedIn and marks it here. Whoever he does
// not connect with stays open, there is no skipping.
func (p *Person) Connect(connected bool, at time.Time) {
	if p.Connected == connected {
		return
	}
	p.Connected, p.UpdatedAt = connected, at
	p.ConnectedAt = time.Time{}
	if connected {
		p.ConnectedAt = at
	}
}

// YoungUntil is the age up to which a person counts as young, by the
// likely year of birth their profile shows.
const YoungUntil = 30

// Young says whether a person is likely under 30 in the year of now.
func (p *Person) Young(now time.Time) bool {
	return p.LinkedIn.BornAbout > now.Year()-YoungUntil
}

// Filters are the ways People can be narrowed, and Sorts the orders it
// can be in.
var (
	Filters = []string{"all", "young", "connected"}
	Sorts   = []string{"young", "new", "fit", "name"}
)

// PeopleQuery is what People asks for.
type PeopleQuery struct {
	Q, Sort, Filter string
	Now             time.Time
}

// ListPeople lists the people a query asks for, and how many each filter
// would show for the same words, so a filter that shows nobody never hides
// that others were found. Whom an exclusion leaves out is in neither,
// unless Tim connected with them. They stay in the file, so removing the
// exclusion brings them back.
func (d *Data) ListPeople(q PeopleQuery) ([]*Person, map[string]int, error) {
	if q.Sort == "" {
		q.Sort = "young"
	}
	if q.Filter == "" {
		q.Filter = "all"
	}
	if !slices.Contains(Sorts, q.Sort) {
		return nil, nil, fmt.Errorf("sort must be %s", strings.Join(Sorts, ", "))
	}
	if !slices.Contains(Filters, q.Filter) {
		return nil, nil, fmt.Errorf("filter must be %s", strings.Join(Filters, ", "))
	}
	words := strings.ToLower(strings.TrimSpace(q.Q))
	counts := map[string]int{}
	hidden := d.Hidden()
	var out []*Person
	for _, p := range d.People {
		if _, out := hidden[p.ID]; out {
			continue
		}
		if words != "" && !strings.Contains(strings.ToLower(p.Name+"\n"+p.Headline+"\n"+p.City), words) {
			continue
		}
		in := map[string]bool{
			"all":       true,
			"young":     p.Young(q.Now),
			"connected": p.Connected,
		}
		for f, ok := range in {
			if ok {
				counts[f]++
			}
		}
		if in[q.Filter] {
			out = append(out, p)
		}
	}
	newest := func(a, b *Person) int {
		return cmp.Or(b.FirstSeen.Compare(a.FirstSeen), cmp.Compare(b.ID, a.ID))
	}
	slices.SortStableFunc(out, func(a, b *Person) int {
		switch q.Sort {
		case "new":
			return newest(a, b)
		case "name":
			return cmp.Or(strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)), cmp.Compare(a.ID, b.ID))
		case "fit":
			return cmp.Or(cmp.Compare(b.Score, a.Score), newest(a, b))
		}
		// The likely youngest first, those without a guess after them.
		return cmp.Or(cmpBool(a.LinkedIn.BornAbout == 0, b.LinkedIn.BornAbout == 0),
			cmp.Compare(b.LinkedIn.BornAbout, a.LinkedIn.BornAbout), cmp.Compare(b.Score, a.Score), newest(a, b))
	})
	return out, counts, nil
}

// cmpBool puts false before true.
func cmpBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return 1
	}
	return -1
}
