package store

import (
	"errors"
	"fmt"
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
