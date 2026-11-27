package store

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/timmrkz/speakertrail/internal/place"
	"github.com/timmrkz/speakertrail/internal/rubric"
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
func (d *Data) AddLink(url, handle string, runID int64, at time.Time) (p *Person, isNew, connected bool) {
	p = d.PersonByProfile(url)
	isNew = p == nil
	if isNew {
		p = &Person{
			ID: d.NewID(), Name: handle, FirstSeen: at, FirstRunID: runID, Evidence: AddedByHand,
			LinkedIn: LinkedIn{URL: url, Handle: handle, Review: "confirmed"}, ToRead: true,
		}
		d.People = append(d.People, p)
	}
	// Someone added again whose profile Exa did not have, or could not
	// give, is asked for once more.
	if p.ReadError != "" {
		p.ToRead = true
	}
	connected = !p.Connected
	p.Connect(true, at)
	p.UpdatedAt = at
	return p, isNew, connected && !isNew
}

// readCutAgain marks once, for the next run, everyone whose profile text
// Exa cut. Builds before 27 November 2026 asked Exa for 8000 characters,
// and Exa writes the activity last, so a long career took all of it along
// and the person showed No activity on LinkedIn known (Tim's report of
// that day), or took its end, where Exa lists the likes, and the person
// showed an older Active lately. A read now keeps it, see
// search.AskedChars. It is marked once, so a profile that is long at
// Exa is not asked for at every start. A build before this one drops
// CutRead from the file, so after a switch to one and back they are
// read once more.
func (d *Data) readCutAgain() {
	if d.CutRead {
		return
	}
	d.CutRead = true
	for _, p := range d.People {
		if p.LinkedIn.URL != "" && cutByExa(p.LinkedIn.Text) {
			p.ToRead = true
		}
	}
}

// cutLength is how long a profile text from Exa is that was cut at the
// 8000 characters Exa was asked for: a little less, since the email
// addresses and phone numbers in it are taken out.
const cutLength = 7000

// cutByExa says a profile text is one Exa wrote, which starts with the
// name as a heading, "# Erika Beispiel", as a pasted page never does, and
// at least cutLength long.
func cutByExa(text string) bool {
	return strings.HasPrefix(text, "# ") && utf8.RuneCountInString(text) >= cutLength
}

// AddPage adds someone from a LinkedIn page Tim pasted, under the link
// of their profile, named as the page names them, and connected when
// the page says so. Like someone added from a link, Undo takes them out
// again, see Store.Bring. The page is their profile, see Paste, so no
// run reads it. Someone here under that link already is given back.
func (d *Data) AddPage(url, name string, connected bool, at time.Time) (p *Person, isNew bool) {
	if p = d.PersonByProfile(url); p != nil {
		return p, false
	}
	p = &Person{
		ID: d.NewID(), Name: name, FirstSeen: at, Evidence: AddedByHand, UpdatedAt: at,
		LinkedIn: LinkedIn{URL: url, Handle: url[strings.LastIndex(url, "/")+1:], Review: "confirmed"},
	}
	p.Connect(connected, at)
	d.People = append(d.People, p)
	return p, true
}

// ErrNoPerson is an id nobody has.
var ErrNoPerson = errors.New("no such person")

// takeOut takes a person out of the app, for Undo of the step that
// brought them in, and leaves them out of the runs that had not read
// their profile yet. A run that read it keeps the read, which was paid
// for.
func (d *Data) takeOut(id int64) {
	i := slices.IndexFunc(d.People, func(p *Person) bool { return p.ID == id })
	if i < 0 {
		return
	}
	if d.People[i].LinkedIn.Text != "" {
		d.textsChanged = true
	}
	d.People = slices.Delete(d.People, i, i+1)
	for _, r := range d.Runs {
		if !slices.ContainsFunc(r.Reads, func(x Read) bool { return x.PersonID == id }) {
			r.Links = slices.DeleteFunc(r.Links, func(x int64) bool { return x == id })
		}
	}
}

// Pasted is a profile Tim pasted from his browser, or what a person was
// before, which Undo puts back.
type Pasted struct {
	Text     string `json:"text"`
	Name     string `json:"name"`
	Headline string `json:"headline"`
	City     string `json:"city"`
}

// Paste keeps a profile page Tim copied in his browser as a person's
// profile, for when Exa's index has none, so a first message can be
// written from it. Their name, headline and city become the page's, a
// name only when it gives one. Nobody reads their profile from Exa then.
// An empty text takes the profile away again, and someone Tim added from
// a link waits for the next run to read it from Exa.
func (d *Data) Paste(p *Person, f Pasted, at time.Time) {
	d.SetProfileText(p, f.Text, at)
	if f.Name != "" {
		p.Name = f.Name
	}
	p.Headline, p.LinkedIn.Headline, p.City = f.Headline, f.Headline, f.City
	p.UpdatedAt = at
	p.ToRead, p.ReadError = f.Text == "" && p.Evidence == AddedByHand, ""
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

// Rejected says whether Tim said the LinkedIn profile a search found is
// not this person's.
func (p *Person) Rejected() bool {
	return p.LinkedIn.Review == "rejected"
}

// Filters are the ways People can be narrowed, and Sorts the orders it
// can be in, the best fit first by default, which Tim asked for on 21
// November 2026, so he picks the best from the top.
var (
	Filters = []string{"all", "new", "connected", "rejected"}
	Sorts   = []string{"fit", "young", "new", "name"}
)

// Fit is a person's fit from 0 to 100 as of now, with its parts, see
// rubric.Fit. It is worked out whenever it is asked for and never kept,
// because it changes with the date and with Where you are.
func (d *Data) Fit(p *Person, now time.Time) (int, []rubric.Part) {
	home := d.Text(HomeCity)
	in := fitOf(p, home, now, nil)
	in.Km, in.Near = place.Km(home, p.City)
	return rubric.Fit(in)
}

// Fitter gives the fit alone of one person after another as of now,
// without its parts, reading the settings it needs once, for a list. It
// keeps what it worked out, so it serves one goroutine.
func (d *Data) Fitter(now time.Time) func(p *Person) int {
	home := d.Text(HomeCity)
	// Few cities, many people: each distance is worked out once, and the
	// signals of one person after another share one slice.
	type away struct {
		km   float64
		near bool
	}
	cities := map[string]away{}
	var sigs []rubric.Signal
	return func(p *Person) int {
		in := fitOf(p, "", now, sigs[:0])
		sigs = in.Signals
		a, ok := cities[p.City]
		if !ok {
			a.km, a.near = place.Km(home, p.City)
			cities[p.City] = a
		}
		in.Home, in.Km, in.Near = home, a.km, a.near
		return rubric.FitPoints(in)
	}
}

// fitOf is what the fit reads of a person, but for the distance, with
// their signals appended to sigs.
func fitOf(p *Person, home string, now time.Time, sigs []rubric.Signal) rubric.FitPerson {
	in := rubric.FitPerson{
		BornAbout: p.LinkedIn.BornAbout, AgeReason: p.LinkedIn.AgeReason,
		LastActive: p.LinkedIn.LastActive, LastWrote: p.LinkedIn.LastWrote, Wrote: p.LinkedIn.Wrote,
		City: p.City, Home: home, Now: now,
	}
	for _, s := range p.Signals {
		sigs = append(sigs, rubric.Signal{Key: s.Key, For: s.For})
	}
	in.Signals = sigs
	return in
}

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
// whatever the profile. New is whoever came in during the days the
// setting new_days gives, a week until Tim changes it, connected or not,
// in the order Tim chose like every list, the best fit first unless he
// picks another (Tim, 27 November 2026: Best fit did nothing under New).
// Whom an exclusion leaves out is in no list, unless Tim connected with
// them. They stay in the file, so removing the exclusion brings them
// back.
func (d *Data) ListPeople(q PeopleQuery) ([]*Person, map[string]int, error) {
	if q.Sort == "" {
		q.Sort = "fit"
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
	newFor := d.Days(NewDays)
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
			"new":       !(p.Rejected() && !p.Connected) && q.Now.Sub(p.FirstSeen) < newFor,
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
	if q.Sort == "fit" {
		// Each fit once, then sorted by it, rather than worked out again on
		// every comparison.
		fit := d.Fitter(q.Now)
		type fitted struct {
			p   *Person
			fit int
		}
		all := make([]fitted, len(out))
		for i, p := range out {
			all[i] = fitted{p, fit(p)}
		}
		slices.SortStableFunc(all, func(a, b fitted) int {
			return cmp.Or(cmp.Compare(b.fit, a.fit), b.p.FirstSeen.Compare(a.p.FirstSeen), cmp.Compare(b.p.ID, a.p.ID))
		})
		for i := range all {
			out[i] = all[i].p
		}
		return out, counts, nil
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
