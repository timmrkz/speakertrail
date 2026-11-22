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
	// Text is the whole profile as Exa's index has it, see LinkedIn.Text.
	Text string
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
	p.take(d, f, at)
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

// AddedByHand is how someone Tim added from a link was found.
const AddedByHand = "Added by hand"

// AddLink finds the person behind a LinkedIn profile link Tim added, or
// adds them, named by the handle in the link until a run reads their
// profile. Either way they are marked connected: Tim adds people he
// connected with. It reports whether they are new.
func (d *Data) AddLink(url, handle string, runID int64, at time.Time) (*Person, bool) {
	p := d.PersonByProfile(url)
	isNew := p == nil
	if isNew {
		p = &Person{
			ID: d.NewID(), Name: handle, FirstSeen: at, FirstRunID: runID, Evidence: AddedByHand,
			LinkedIn: LinkedIn{URL: url, Handle: handle, Review: "confirmed"}, ToRead: true,
		}
		d.People = append(d.People, p)
	}
	p.Connect(true, at)
	p.UpdatedAt = at
	return p, isNew
}

// ReadLink fills in someone Tim added from a link with what their profile
// says, the name too.
func (d *Data) ReadLink(p *Person, f Found, at time.Time) {
	if f.Name != "" {
		p.Name = f.Name
	}
	p.take(d, f, at)
	p.ToRead, p.ReadError = false, ""
}

// take keeps what a profile says now.
func (p *Person) take(d *Data, f Found, at time.Time) {
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
	if f.Text != "" {
		d.SetProfileText(p, f.Text, at)
	}
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

// NewFor is how long someone counts as new under New after they came
// in, found by a search or added by hand.
const NewFor = 7 * 24 * time.Hour

// Rejected says whether Tim said the LinkedIn profile a search found is
// not this person's.
func (p *Person) Rejected() bool {
	return p.LinkedIn.Review == "rejected"
}

// Filters are the ways People can be narrowed, and Sorts the orders it
// can be in.
var (
	Filters = []string{"all", "new", "connected", "rejected"}
	Sorts   = []string{"young", "new", "fit", "name"}
)

// PeopleQuery is what People asks for.
type PeopleQuery struct {
	Q, Sort, Filter string
	Now             time.Time
}

// ListPeople lists the people a query asks for, and how many each filter
// would show for the same words, so a filter that shows nobody never hides
// that others were found. Someone Tim connected with is only under
// Connected, and someone whose profile he rejected only under Rejected,
// so All shrinks to whom he can still reach out to.
// Connected comes first, since a person he connected with is real
// whatever the profile. New is whoever came in during the last week,
// connected or not, the newest first whatever the order, so those just
// added stand on top. Whom an exclusion leaves out is in no list,
// unless Tim connected with them. They stay in the file, so removing the exclusion brings them
// back.
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
		open := !p.Connected && !p.Rejected()
		in := map[string]bool{
			"all":       open,
			"new":       !(p.Rejected() && !p.Connected) && q.Now.Sub(p.FirstSeen) < NewFor,
			"connected": p.Connected,
			"rejected":  !p.Connected && p.Rejected(),
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
	sort := q.Sort
	if q.Filter == "new" {
		sort = "new"
	}
	slices.SortStableFunc(out, func(a, b *Person) int {
		switch sort {
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
