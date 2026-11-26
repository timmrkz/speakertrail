package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// withPerson opens a store holding one invented person, found by a
// search, and gives their id.
func withPerson(t *testing.T, dir string) (*Store, int64) {
	t.Helper()
	s := open(t, dir)
	var id int64
	if err := s.Update(func(d *Data) error {
		search, err := d.AddSearch("Yogalehrerin Köln", 0, t0)
		if err != nil {
			return err
		}
		p, _ := d.AddFound(Found{URL: "https://www.linkedin.com/in/ada-beispiel", Name: "Ada Beispiel", Headline: "Inhaberin", City: "Köln"}, search, 0, t0)
		id = p.ID
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return s, id
}

func (s *Store) person(t *testing.T, id int64) Person {
	t.Helper()
	var out Person
	s.Read(func(d *Data) {
		p := d.Person(id)
		if p == nil {
			t.Fatalf("person %d is gone", id)
		}
		out = *p
	})
	return out
}

func mustStep(t *testing.T, how string, u Undone, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", how, err)
	}
	if !u.Done {
		t.Fatalf("%s did nothing", how)
	}
}

func TestUndoAndRedoAChangeToAPerson(t *testing.T) {
	s, id := withPerson(t, t.TempDir())
	if err := s.Edit(func(d *Data) error {
		d.Person(id).Connect(true, t0)
		return nil
	}, PersonThing(id)); err != nil {
		t.Fatal(err)
	}
	u, err := s.Undo()
	mustStep(t, "undo", u, err)
	if u.Shows == nil || *u.Shows != PersonThing(id) {
		t.Errorf("the undo shows %+v, want the person", u.Shows)
	}
	if p := s.person(t, id); p.Connected || !p.ConnectedAt.IsZero() {
		t.Errorf("after undo connected is %v since %v, want open", p.Connected, p.ConnectedAt)
	}
	u, err = s.Redo()
	mustStep(t, "redo", u, err)
	if p := s.person(t, id); !p.Connected || !p.ConnectedAt.Equal(t0) {
		t.Errorf("after redo connected is %v since %v, want connected since %v", p.Connected, p.ConnectedAt, t0)
	}
	if u, err := s.Redo(); err != nil || u.Done {
		t.Errorf("a second redo did %+v, %v, want nothing", u, err)
	}
}

func TestAChangeThatChangedNothingIsNoStep(t *testing.T) {
	s, id := withPerson(t, t.TempDir())
	_ = s.Edit(func(d *Data) error {
		d.Person(id).UpdatedAt = t0.Add(time.Hour)
		return nil
	}, PersonThing(id))
	_ = s.Edit(func(*Data) error { return errors.New("refused") }, PersonThing(id))
	if u, err := s.Undo(); err != nil || u.Done {
		t.Errorf("undo did %+v, %v, want nothing to undo", u, err)
	}
}

func TestAnEmptiedFieldComesBackAndGoesAgain(t *testing.T) {
	s, id := withPerson(t, t.TempDir())
	_ = s.Edit(func(d *Data) error {
		p := d.Person(id)
		p.Draft, p.DraftedAt = "Hallo Ada", t0
		return nil
	}, PersonThing(id))
	_ = s.Edit(func(d *Data) error {
		p := d.Person(id)
		p.Draft, p.DraftedAt = "", time.Time{}
		return nil
	}, PersonThing(id))
	u, err := s.Undo()
	mustStep(t, "undo", u, err)
	if p := s.person(t, id); p.Draft != "Hallo Ada" || !p.DraftedAt.Equal(t0) {
		t.Errorf("after undo the draft is %q at %v", p.Draft, p.DraftedAt)
	}
	u, err = s.Undo()
	mustStep(t, "second undo", u, err)
	if p := s.person(t, id); p.Draft != "" || !p.DraftedAt.IsZero() {
		t.Errorf("after the second undo the draft is %q at %v, want none", p.Draft, p.DraftedAt)
	}
	u, err = s.Redo()
	mustStep(t, "redo", u, err)
	if p := s.person(t, id); p.Draft != "Hallo Ada" {
		t.Errorf("after redo the draft is %q", p.Draft)
	}
}

func TestAnUndoLeavesWhatARunChangedSince(t *testing.T) {
	s, id := withPerson(t, t.TempDir())
	_ = s.Edit(func(d *Data) error {
		d.Person(id).Connect(true, t0)
		return nil
	}, PersonThing(id))
	// A run reads the person again, which is no step.
	_ = s.Update(func(d *Data) error {
		p := d.Person(id)
		p.Headline, p.UpdatedAt = "Inhaberin, Studio Beispiel", t0.Add(time.Hour)
		return nil
	})
	u, err := s.Undo()
	mustStep(t, "undo", u, err)
	if p := s.person(t, id); p.Connected || p.Headline != "Inhaberin, Studio Beispiel" {
		t.Errorf("after undo connected is %v and the headline %q, want open and the run's headline", p.Connected, p.Headline)
	}
}

func TestAnUndoOfAFieldChangedSinceWritesNothing(t *testing.T) {
	s, id := withPerson(t, t.TempDir())
	for _, notes := range []string{"first", "second"} {
		_ = s.Edit(func(d *Data) error {
			d.Person(id).Notes = notes
			return nil
		}, PersonThing(id))
	}
	// Something that is no step changes the notes again.
	_ = s.Update(func(d *Data) error {
		d.Person(id).Notes = "third"
		return nil
	})
	if _, err := s.Undo(); !errors.Is(err, ErrChangedSince) {
		t.Fatalf("undo said %v, want ErrChangedSince", err)
	}
	if p := s.person(t, id); p.Notes != "third" {
		t.Errorf("the refused undo wrote %q", p.Notes)
	}
	// The step behind it rests on it, so the list started again.
	if u, err := s.Undo(); err != nil || u.Done {
		t.Errorf("a second undo did %+v, %v, want nothing left", u, err)
	}
}

func TestANewStepClearsRedo(t *testing.T) {
	s, id := withPerson(t, t.TempDir())
	edit := func(notes string) {
		_ = s.Edit(func(d *Data) error {
			d.Person(id).Notes = notes
			return nil
		}, PersonThing(id))
	}
	edit("first")
	u, err := s.Undo()
	mustStep(t, "undo", u, err)
	edit("second")
	if u, err := s.Redo(); err != nil || u.Done {
		t.Errorf("redo did %+v, %v after a new step, want nothing", u, err)
	}
}

func TestAnUndoPutsTheProfileTextBackInItsFile(t *testing.T) {
	dir := t.TempDir()
	s, id := withPerson(t, dir)
	_ = s.Edit(func(d *Data) error {
		d.Paste(d.Person(id), Pasted{Text: "Ada Beispiel\nYogalehrerin", Name: "Ada Beispiel", Headline: "Yogalehrerin", City: "Bonn"}, t0)
		return nil
	}, PersonThing(id))
	u, err := s.Undo()
	mustStep(t, "undo", u, err)
	again := open(t, dir)
	if p := again.person(t, id); p.LinkedIn.Text != "" || p.City != "Köln" || p.Headline != "Inhaberin" {
		t.Errorf("read back after undo: text %q, city %q, headline %q", p.LinkedIn.Text, p.City, p.Headline)
	}
	u, err = s.Redo()
	mustStep(t, "redo", u, err)
	again = open(t, dir)
	if p := again.person(t, id); p.LinkedIn.Text != "Ada Beispiel\nYogalehrerin" || p.City != "Bonn" || !p.LinkedIn.TextAt.Equal(t0) {
		t.Errorf("read back after redo: text %q at %v, city %q", p.LinkedIn.Text, p.LinkedIn.TextAt, p.City)
	}
}

func TestUndoGoesBackTwoHundredSteps(t *testing.T) {
	s, id := withPerson(t, t.TempDir())
	for i := range historyDepth + 5 {
		_ = s.Edit(func(d *Data) error {
			d.Person(id).Notes = fmt.Sprint(i)
			return nil
		}, PersonThing(id))
	}
	n := 0
	for {
		u, err := s.Undo()
		if err != nil {
			t.Fatal(err)
		}
		if !u.Done {
			break
		}
		n++
	}
	if n != historyDepth {
		t.Errorf("undid %d steps, want %d", n, historyDepth)
	}
	if p := s.person(t, id); p.Notes != "4" {
		t.Errorf("the oldest step kept left the notes at %q, want 4", p.Notes)
	}
}

// Changes, undos, redos and runs come from several goroutines at once:
// the requests of the interface and the engine's run.
func TestHistoryFromSeveralGoroutines(t *testing.T) {
	s, id := withPerson(t, t.TempDir())
	var wg sync.WaitGroup
	for g := range 4 {
		wg.Go(func() {
			for i := range 20 {
				_ = s.Edit(func(d *Data) error {
					d.Person(id).Notes = fmt.Sprint(g, i)
					return nil
				}, PersonThing(id))
				_, _ = s.Undo()
				_, _ = s.Redo()
				_ = s.Update(func(d *Data) error {
					d.Person(id).Headline = fmt.Sprint(g, i)
					return nil
				})
			}
		})
	}
	wg.Wait()
	// Whatever was left, every step still undoes or says it cannot.
	for {
		u, err := s.Undo()
		if err != nil && !errors.Is(err, ErrChangedSince) {
			t.Fatal(err)
		}
		if !u.Done {
			break
		}
	}
}

// Every run scores everyone again, so the fit a paste worked out has
// changed by the time Tim takes the paste back. That is no reason to
// refuse it.
func TestAnUndoIsNotRefusedForAFitWorkedOutAgain(t *testing.T) {
	s, id := withPerson(t, t.TempDir())
	_ = s.Edit(func(d *Data) error {
		p := d.Person(id)
		d.Paste(p, Pasted{Text: "Ada Beispiel\nYogalehrerin", Name: "Ada Beispiel", Headline: "Yogalehrerin", City: "Bonn"}, t0)
		p.Score, p.Signals = 40, []Signal{{Key: "founder", For: true}}
		return nil
	}, PersonThing(id))
	_ = s.Update(func(d *Data) error {
		d.Person(id).Score = 35
		return nil
	})
	u, err := s.Undo()
	mustStep(t, "undo", u, err)
	if p := s.person(t, id); p.LinkedIn.Text != "" || p.Score != 0 || len(p.Signals) != 0 {
		t.Errorf("after undo: text %q, score %d, signals %v", p.LinkedIn.Text, p.Score, p.Signals)
	}
}

// edit makes a change Tim makes to what, and fails the test when it
// cannot.
func (s *Store) edit(t *testing.T, fn func(d *Data) error, what ...Thing) {
	t.Helper()
	if err := s.Edit(fn, what...); err != nil {
		t.Fatal(err)
	}
}

// add adds a search or an exclusion as Tim does, and gives its id.
func (s *Store) add(t *testing.T, fn func(d *Data) (Thing, error)) int64 {
	t.Helper()
	var id int64
	if err := s.Add(func(d *Data) (Thing, error) {
		th, err := fn(d)
		id = th.ID
		return th, err
	}); err != nil {
		t.Fatal(err)
	}
	return id
}

func (s *Store) search(id int64) *Search {
	var out *Search
	s.Read(func(d *Data) {
		if x := d.Search(id); x != nil {
			c := *x
			out = &c
		}
	})
	return out
}

func (s *Store) exclusionOf(id int64) *Exclusion {
	var out *Exclusion
	s.Read(func(d *Data) {
		if x := d.exclusion(id); x != nil {
			c := *x
			out = &c
		}
	})
	return out
}

func addSearch(query string) func(d *Data) (Thing, error) {
	return func(d *Data) (Thing, error) {
		x, err := d.AddSearch(query, 0, t0)
		if err != nil {
			return Thing{}, err
		}
		return SearchThing(x.ID), nil
	}
}

func addExclusion(phrase string) func(d *Data) (Thing, error) {
	return func(d *Data) (Thing, error) {
		x, err := d.AddExclusion(phrase, t0)
		if err != nil {
			return Thing{}, err
		}
		return ExclusionThing(x.ID), nil
	}
}

func TestUndoAndRedoPausingASearch(t *testing.T) {
	s := open(t, t.TempDir())
	id := s.add(t, addSearch("Yogalehrerin Köln"))
	s.edit(t, func(d *Data) error {
		d.Search(id).Pause(t0.Add(time.Hour))
		return nil
	}, SearchThing(id))
	u, err := s.Undo()
	mustStep(t, "undo", u, err)
	if u.Shows == nil || *u.Shows != SearchThing(id) {
		t.Errorf("the undo shows %+v, want the search", u.Shows)
	}
	if x := s.search(id); x.Status != "candidate" || !x.StatusChanged.Equal(t0) {
		t.Errorf("after undo the search is %s since %v, want candidate since %v", x.Status, x.StatusChanged, t0)
	}
	u, err = s.Redo()
	mustStep(t, "redo", u, err)
	if x := s.search(id); x.Status != "retired" {
		t.Errorf("after redo the search is %s, want retired", x.Status)
	}
}

func TestUndoTakesOutASearchTimAddedAndRedoPutsItBack(t *testing.T) {
	s := open(t, t.TempDir())
	first := s.add(t, addSearch("Yogalehrerin Köln"))
	id := s.add(t, addSearch("Coach Bonn"))
	last := s.add(t, addSearch("Bäckerin Essen"))
	// Taken out and put back in the middle, where it was.
	_ = s.Update(func(d *Data) error {
		d.Searches[0], d.Searches[2] = d.Searches[2], d.Searches[0]
		return nil
	})
	if _, err := s.Undo(); err != nil {
		t.Fatal(err)
	}
	if s.search(last) != nil {
		t.Fatal("the undo left the last search Tim added")
	}
	u, err := s.Undo()
	mustStep(t, "second undo", u, err)
	if s.search(id) != nil {
		t.Fatal("the second undo left the search Tim added")
	}
	u, err = s.Redo()
	mustStep(t, "redo", u, err)
	if x := s.search(id); x == nil || x.Query != "Coach Bonn" {
		t.Fatalf("after redo the search is %+v, want Coach Bonn with its id", x)
	}
	var order []int64
	s.Read(func(d *Data) {
		for _, x := range d.Searches {
			order = append(order, x.ID)
		}
	})
	if len(order) != 2 || order[0] != first || order[1] != id {
		t.Errorf("the searches are %v, want %d then %d", order, first, id)
	}
}

func TestASearchThatRanSinceItWasAddedStays(t *testing.T) {
	s := open(t, t.TempDir())
	id := s.add(t, addSearch("Yogalehrerin Köln"))
	// A run takes it and finds someone.
	_ = s.Update(func(d *Data) error {
		x := d.Search(id)
		d.AddFound(Found{URL: "https://www.linkedin.com/in/ada-beispiel", Name: "Ada Beispiel", Headline: "Inhaberin", City: "Köln"}, x, 0, t0)
		d.Ran(x, Check{SearchID: id, At: t0, PeopleFound: 1, PeopleNew: 1})
		return nil
	})
	if _, err := s.Undo(); !errors.Is(err, ErrChangedSince) {
		t.Fatalf("undo said %v, want ErrChangedSince", err)
	}
	if s.search(id) == nil {
		t.Error("the refused undo took the search out")
	}
}

func TestASearchThatFoundSomeoneIsNeverTakenOut(t *testing.T) {
	s := open(t, t.TempDir())
	id := s.add(t, addSearch("Yogalehrerin Köln"))
	// Someone found by it, without the search itself changing.
	_ = s.Update(func(d *Data) error {
		d.AddFound(Found{URL: "https://www.linkedin.com/in/ada-beispiel", Name: "Ada Beispiel", Headline: "Inhaberin", City: "Köln"}, d.Search(id), 0, t0)
		return nil
	})
	if _, err := s.Undo(); !errors.Is(err, ErrChangedSince) {
		t.Fatalf("undo said %v, want ErrChangedSince", err)
	}
	if s.search(id) == nil {
		t.Error("the refused undo took the search out")
	}
}

func TestUndoPutsBackARemovedExclusionWithItsID(t *testing.T) {
	s := open(t, t.TempDir())
	id := s.add(t, addExclusion("Hochzeitsplanung"))
	s.edit(t, func(d *Data) error {
		d.DeleteExclusion(id)
		return nil
	}, ExclusionThing(id))
	u, err := s.Undo()
	mustStep(t, "undo", u, err)
	if x := s.exclusionOf(id); x == nil || x.Phrase != "Hochzeitsplanung" || !x.CreatedAt.Equal(t0) {
		t.Fatalf("after undo the exclusion is %+v, want Hochzeitsplanung with its id", x)
	}
	u, err = s.Redo()
	mustStep(t, "redo", u, err)
	if s.exclusionOf(id) != nil {
		t.Error("after redo the exclusion is still there")
	}
	// Undo, and once more, takes out the exclusion Tim added.
	for range 2 {
		u, err = s.Undo()
		mustStep(t, "undo", u, err)
	}
	if s.exclusionOf(id) != nil {
		t.Error("undoing the add left the exclusion")
	}
}

func TestARemovedExclusionAddedAgainSinceStaysOne(t *testing.T) {
	s := open(t, t.TempDir())
	id := s.add(t, addExclusion("Hochzeitsplanung"))
	s.edit(t, func(d *Data) error {
		d.DeleteExclusion(id)
		return nil
	}, ExclusionThing(id))
	// The same words, in another case, come again by another way.
	_ = s.Update(func(d *Data) error {
		_, err := d.AddExclusion("hochzeitsplanung", t0)
		return err
	})
	if _, err := s.Undo(); !errors.Is(err, ErrChangedSince) {
		t.Fatalf("undo said %v, want ErrChangedSince", err)
	}
	var n int
	s.Read(func(d *Data) {
		for _, x := range d.Exclusions {
			if strings.EqualFold(x.Phrase, "Hochzeitsplanung") {
				n++
			}
		}
	})
	if n != 1 {
		t.Errorf("Hochzeitsplanung is there %d times, want once", n)
	}
}

func TestUndoAndRedoTheTemplate(t *testing.T) {
	s := open(t, t.TempDir())
	first := DefaultTemplate()
	greeting := first.Parts[1].Name
	s.edit(t, func(d *Data) error {
		return d.SetTemplatePart(greeting, []string{"Moin"}, -1)
	}, TemplateThing())
	changed := func() Template {
		var out Template
		s.Read(func(d *Data) { out = d.FirstMessageTemplate() })
		return out
	}
	mine := changed()
	s.edit(t, func(d *Data) error {
		d.ResetTemplate()
		return nil
	}, TemplateThing())
	u, err := s.Undo()
	mustStep(t, "undo of the reset", u, err)
	if u.Shows == nil || *u.Shows != TemplateThing() {
		t.Errorf("the undo shows %+v, want the template", u.Shows)
	}
	if got, _ := changed().Part(greeting); !slices.Equal(got.Variations, []string{"Moin"}) {
		t.Errorf("after undoing the reset %s is %q, want Moin", greeting, got.Variations)
	}
	if !reflect.DeepEqual(changed(), mine) {
		t.Error("after undoing the reset the template is not Tim's again")
	}
	u, err = s.Undo()
	mustStep(t, "undo of the change", u, err)
	var tpl *Template
	s.Read(func(d *Data) { tpl = d.Template })
	if tpl != nil {
		t.Errorf("after undoing both the template is %+v, want the first one", tpl)
	}
}

func TestUndoAndRedoASetting(t *testing.T) {
	s := open(t, t.TempDir())
	set := func(v string) {
		s.edit(t, func(d *Data) error {
			return d.SetSetting("profile_searches_per_run", json.RawMessage(v), t0)
		}, SettingThing("profile_searches_per_run"))
	}
	set("12")
	set("14")
	u, err := s.Undo()
	mustStep(t, "undo", u, err)
	if u.Shows == nil || *u.Shows != SettingThing("profile_searches_per_run") {
		t.Errorf("the undo shows %+v, want the setting", u.Shows)
	}
	read := func() (n int, kept bool) {
		s.Read(func(d *Data) {
			n = d.Int("profile_searches_per_run")
			_, kept = d.Settings["profile_searches_per_run"]
		})
		return n, kept
	}
	if n, _ := read(); n != 12 {
		t.Errorf("after one undo it is %d, want 12", n)
	}
	u, err = s.Undo()
	mustStep(t, "second undo", u, err)
	if n, kept := read(); n != 10 || kept {
		t.Errorf("after two undos it is %d, kept %v, want the default 10 and nothing kept", n, kept)
	}
	u, err = s.Redo()
	mustStep(t, "redo", u, err)
	if n, _ := read(); n != 12 {
		t.Errorf("after redo it is %d, want 12", n)
	}
}

func TestUndoTheBrowserProfileLinkedInOpensIn(t *testing.T) {
	s := open(t, t.TempDir())
	for _, w := range []Where{{App: "Google Chrome", Folder: "Default"}, {App: "Google Chrome", Folder: "Profile 1"}} {
		s.edit(t, func(d *Data) error {
			d.LinkedInOpens = w
			return nil
		}, SettingThing(LinkedInOpensKey))
	}
	opens := func() (w Where) {
		s.Read(func(d *Data) { w = d.LinkedInOpens })
		return w
	}
	u, err := s.Undo()
	mustStep(t, "undo", u, err)
	if w := opens(); w.Folder != "Default" {
		t.Errorf("after one undo LinkedIn opens in %+v, want Default", w)
	}
	u, err = s.Undo()
	mustStep(t, "second undo", u, err)
	if w := opens(); w != (Where{}) {
		t.Errorf("after two undos LinkedIn opens in %+v, want none chosen", w)
	}
}

// Searches and exclusions added, changed and taken back from several
// goroutines, while runs take the searches.
func TestSearchesAndExclusionsFromSeveralGoroutines(t *testing.T) {
	s := open(t, t.TempDir())
	var wg sync.WaitGroup
	for g := range 4 {
		wg.Go(func() {
			for i := range 15 {
				var id int64
				_ = s.Add(func(d *Data) (Thing, error) {
					x, err := d.AddSearch(fmt.Sprint("Coach Köln ", g, " ", i), 0, t0)
					if err != nil {
						return Thing{}, err
					}
					id = x.ID
					return SearchThing(x.ID), nil
				})
				_ = s.Edit(func(d *Data) error {
					if x := d.Search(id); x != nil {
						x.Pause(t0)
					}
					return nil
				}, SearchThing(id))
				_ = s.Add(addExclusion(fmt.Sprint("Phrase ", g, " ", i)))
				_, _ = s.Undo()
				_, _ = s.Redo()
				_, _ = s.Undo()
				_ = s.Update(func(d *Data) error {
					if x := d.Search(id); x != nil {
						d.Ran(x, Check{SearchID: id, At: t0})
					}
					return nil
				})
			}
		})
	}
	wg.Wait()
	for {
		u, err := s.Undo()
		if err != nil && !errors.Is(err, ErrChangedSince) {
			t.Fatal(err)
		}
		if !u.Done && err == nil {
			break
		}
	}
	// Ids stay one per thing, whatever came back.
	seen := map[int64]bool{}
	s.Read(func(d *Data) {
		for _, x := range d.Searches {
			if seen[x.ID] {
				t.Errorf("search %d is there twice", x.ID)
			}
			seen[x.ID] = true
		}
		for _, x := range d.Exclusions {
			if seen[x.ID] {
				t.Errorf("exclusion %d is there twice", x.ID)
			}
			seen[x.ID] = true
		}
	})
}

// A run lists the searches it takes as it starts and changes nothing on
// them until each check ends, so the search must stay while it runs.
func TestASearchARunTakesNowStays(t *testing.T) {
	s := open(t, t.TempDir())
	id := s.add(t, addSearch("Yogalehrerin Köln"))
	_ = s.Update(func(d *Data) error {
		d.Runs = append(d.Runs, &Run{ID: d.NewID(), Kind: "manual", StartedAt: t0, Searches: []int64{id}})
		return nil
	})
	if _, err := s.Undo(); !errors.Is(err, ErrChangedSince) {
		t.Fatalf("undo said %v, want ErrChangedSince", err)
	}
	if s.search(id) == nil {
		t.Error("the refused undo took out the search the run takes")
	}
}

// A search put back by Redo whose words were added again since stays
// one, as an exclusion does.
func TestASearchAddedAgainSinceStaysOne(t *testing.T) {
	s := open(t, t.TempDir())
	id := s.add(t, addSearch("Floristin Bonn"))
	u, err := s.Undo()
	mustStep(t, "undo", u, err)
	_ = s.Update(func(d *Data) error {
		_, err := d.AddSearch("floristin bonn", 0, t0)
		return err
	})
	if _, err := s.Redo(); !errors.Is(err, ErrChangedSince) {
		t.Fatalf("redo said %v, want ErrChangedSince", err)
	}
	if s.search(id) != nil {
		t.Error("the refused redo put the first search back beside the second")
	}
}
