package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 10, 18, 0, 0, 0, time.UTC)

func open(t *testing.T, dir string) *Store {
	t.Helper()
	s, err := Open(dir, func() time.Time { return t0 })
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestWhatIsWrittenIsReadBack(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	if err := s.Update(func(d *Data) error {
		if err := d.Start(t0); err != nil {
			return err
		}
		search := d.Searches[0]
		p, isNew := d.AddFound(Found{URL: "https://www.linkedin.com/in/ada-beispiel", Name: "Ada Beispiel", Headline: "Inhaberin, Studio Beispiel", City: "Köln", BornAbout: 1999}, search, 0, t0)
		if !isNew {
			t.Error("a new profile should be a new person")
		}
		return p.Decide("kept", t0)
	}); err != nil {
		t.Fatal(err)
	}
	again := open(t, dir)
	again.Read(func(d *Data) {
		if len(d.Searches) != 64 {
			t.Errorf("%d searches, want the 64 starting ones", len(d.Searches))
		}
		if len(d.People) != 1 || d.People[0].Decision != "kept" || d.People[0].LinkedIn.BornAbout != 1999 {
			t.Errorf("people read back as %+v", d.People)
		}
	})
}

func TestStartAddsTheStartingSearchesOnce(t *testing.T) {
	s := open(t, t.TempDir())
	_ = s.Update(func(d *Data) error {
		if err := d.Start(t0); err != nil {
			return err
		}
		d.Searches = d.Searches[:1]
		return d.Start(t0)
	})
	s.Read(func(d *Data) {
		if len(d.Searches) != 1 {
			t.Errorf("Start added the searches again: %d", len(d.Searches))
		}
	})
}

func TestAFailedUpdateChangesNothing(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	boom := errors.New("boom")
	err := s.Update(func(d *Data) error {
		_, _ = d.AddSearch("Yogalehrerin Köln", 0, t0)
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	s.Read(func(d *Data) {
		if len(d.Searches) != 0 || d.LastID != 0 {
			t.Errorf("memory kept the failed change: %+v", d.Searches)
		}
	})
	if _, err := os.Stat(filepath.Join(dir, FileName)); !os.IsNotExist(err) {
		t.Errorf("a failed update wrote the file: %v", err)
	}
}

func TestAFileFromANewerAppIsNotOpened(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(`{"version": 99}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir, nil); err == nil {
		t.Error("a file of version 99 was opened")
	}
}

// Runs and the window change the data from several goroutines at once.
func TestUpdatesFromSeveralGoroutines(t *testing.T) {
	s := open(t, t.TempDir())
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			for j := range 5 {
				_ = s.Update(func(d *Data) error {
					_, err := d.AddSearch(string(rune('A'+i))+string(rune('a'+j))+" Köln", 0, t0)
					return err
				})
				s.Read(func(d *Data) { _ = d.Due(t0, 10) })
			}
		})
	}
	wg.Wait()
	s.Read(func(d *Data) {
		if len(d.Searches) != 40 {
			t.Errorf("%d searches, want 40", len(d.Searches))
		}
	})
}

func TestDueAndRan(t *testing.T) {
	d := &Data{}
	d.fill()
	a, _ := d.AddSearch("Gründer Köln", 0, t0)
	b, _ := d.AddSearch("Gen Z Unternehmer Köln", 1, t0)
	c, _ := d.AddSearch("Yogalehrerin Bonn", 0, t0)
	c.Pause(t0)
	if due := d.Due(t0, 10); len(due) != 2 || due[0] != b || due[1] != a {
		t.Fatalf("due = %v, want the search with a point first and the paused one left out", due)
	}
	if due := d.Due(t0, 1); len(due) != 1 {
		t.Errorf("the limit was not kept: %d", len(due))
	}

	d.Ran(a, Check{At: t0, PeopleFound: 3, PeopleNew: 2})
	if a.Status != "active" || !a.NextRun.After(t0.Add(29*24*time.Hour)) {
		t.Errorf("after finding people: %s, next %v", a.Status, a.NextRun)
	}
	if due := d.Due(t0.Add(time.Hour), 10); len(due) != 1 || due[0] != b {
		t.Errorf("a search that ran is due again: %v", due)
	}
	d.Ran(b, Check{At: t0, Error: "Exa failed"})
	if b.Status != "candidate" || !b.NextRun.IsZero() {
		t.Errorf("a failed search should be due in the next run: %s %v", b.Status, b.NextRun)
	}
	d.Ran(b, Check{At: t0})
	if b.Status != "probation" {
		t.Errorf("a new search that found nobody: %s", b.Status)
	}
	at := t0
	for range 4 {
		at = at.Add(31 * 24 * time.Hour)
		d.Ran(a, Check{At: at, PeopleFound: 2})
	}
	if a.Status != "retired" {
		t.Errorf("a search that found nobody new four times: %s", a.Status)
	}
	a.Resume(at)
	if a.Status != "candidate" || len(d.Due(at, 10)) != 2 {
		t.Errorf("a resumed search is not due: %s", a.Status)
	}
}

func TestAddSearchRefusesTheSameWordsTwice(t *testing.T) {
	d := &Data{}
	d.fill()
	if _, err := d.AddSearch("  Yogalehrerin   Köln ", 0, t0); err != nil {
		t.Fatal(err)
	}
	if _, err := d.AddSearch("yogalehrerin köln", 0, t0); !errors.Is(err, ErrSearchExists) {
		t.Errorf("err = %v, want ErrSearchExists", err)
	}
	if _, err := d.AddSearch(" ", 0, t0); err == nil {
		t.Error("an empty search was added")
	}
}

func TestReserveKeepsTheMonthlyBudget(t *testing.T) {
	d := &Data{}
	d.fill()
	if err := d.SetSetting("exa_monthly_searches", json.RawMessage("2"), t0); err != nil {
		t.Fatal(err)
	}
	if !d.Reserve("exa", t0) || !d.Reserve("exa", t0) {
		t.Fatal("the budget refused a call it has room for")
	}
	if d.Reserve("exa", t0) {
		t.Error("a third call went past a budget of 2")
	}
	if !d.Reserve("exa", t0.AddDate(0, 1, 0)) {
		t.Error("the next month has no budget")
	}
	if d.Used("exa", t0) != 2 {
		t.Errorf("used = %d", d.Used("exa", t0))
	}
}

func TestSettings(t *testing.T) {
	d := &Data{}
	d.fill()
	if d.Int("profile_searches_per_run") != 10 {
		t.Errorf("default = %d", d.Int("profile_searches_per_run"))
	}
	if err := d.SetSetting("profile_searches_per_run", json.RawMessage(`"many"`), t0); err == nil {
		t.Error("text was taken for a number")
	}
	if err := d.SetSetting("profile_searches_per_run", json.RawMessage(`-1`), t0); err == nil {
		t.Error("a negative number was taken")
	}
	if err := d.SetSetting("nothing_like_it", json.RawMessage(`1`), t0); err == nil {
		t.Error("an unknown setting was taken")
	}
	if err := d.SetSetting("profile_searches_per_run", json.RawMessage(`5`), t0); err != nil || d.Int("profile_searches_per_run") != 5 {
		t.Errorf("err = %v, value %d", err, d.Int("profile_searches_per_run"))
	}
	if err := d.SetSetting("profile_searches_per_run", json.RawMessage(`10`), t0); err != nil || len(d.Settings) != 0 {
		t.Errorf("setting the default back should forget the change: %v %v", err, d.Settings)
	}
}

func TestListPeople(t *testing.T) {
	d := &Data{}
	d.fill()
	s, _ := d.AddSearch("Gründer Köln", 0, t0)
	add := func(name string, born, score int, decision string, seen time.Time) *Person {
		p, _ := d.AddFound(Found{URL: "https://www.linkedin.com/in/" + name, Name: name, Headline: "Gründer", City: "Köln", BornAbout: born}, s, 0, seen)
		p.FirstSeen, p.Score = seen, score
		_ = p.Decide(decision, seen)
		return p
	}
	old := add("Bea", 1980, 3, "", t0)
	young := add("Cem", 2001, 1, "kept", t0.Add(time.Hour))
	skipped := add("Dana", 2003, 2, "skipped", t0.Add(2*time.Hour))
	unknown := add("Ann", 0, 2, "", t0.Add(3*time.Hour))

	list := func(q PeopleQuery) []*Person {
		q.Now = t0
		out, _, err := d.ListPeople(q)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	same := func(what string, got []*Person, want ...*Person) {
		t.Helper()
		if len(got) != len(want) {
			t.Errorf("%s: %d people, want %d", what, len(got), len(want))
			return
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("%s: %d is %s, want %s", what, i, got[i].Name, want[i].Name)
			}
		}
	}
	same("young first", list(PeopleQuery{}), young, old, unknown, skipped)
	same("new", list(PeopleQuery{Sort: "new"}), unknown, skipped, young, old)
	same("name", list(PeopleQuery{Sort: "name"}), unknown, old, young, skipped)
	same("fit", list(PeopleQuery{Sort: "fit"}), old, unknown, young, skipped)
	same("under 30", list(PeopleQuery{Filter: "young"}), young, skipped)
	same("kept", list(PeopleQuery{Filter: "kept"}), young)
	same("words", list(PeopleQuery{Q: "dana"}), skipped)

	_, counts, _ := d.ListPeople(PeopleQuery{Filter: "kept", Now: t0})
	if counts["all"] != 4 || counts["young"] != 2 || counts["kept"] != 1 || counts["skipped"] != 1 {
		t.Errorf("counts = %v", counts)
	}
	if _, _, err := d.ListPeople(PeopleQuery{Filter: "founder"}); err == nil {
		t.Error("an unknown filter was taken")
	}
}

func TestAddFoundMergesTheSameProfile(t *testing.T) {
	d := &Data{}
	d.fill()
	a, _ := d.AddSearch("Gründer Köln", 0, t0)
	b, _ := d.AddSearch("Gründerin Bonn", 0, t0)
	url := "https://www.linkedin.com/in/eva-beispiel"
	p1, new1 := d.AddFound(Found{URL: url, Name: "Eva Beispiel", City: "Köln", Headline: "Gründerin"}, a, 1, t0)
	p2, new2 := d.AddFound(Found{URL: url, Name: "Eva Beispiel", City: "Bonn", BornAbout: 2000, LastActive: t0}, b, 2, t0.Add(time.Hour))
	if !new1 || new2 || p1 != p2 || len(d.People) != 1 {
		t.Fatalf("the same profile made two people")
	}
	if p2.City != "Köln" || p2.Headline != "Gründerin" || p2.LinkedIn.BornAbout != 2000 || len(p2.FoundBy) != 2 {
		t.Errorf("merged into %+v", p2)
	}
}
