package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
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
		p.Connect(true, t0)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	again := open(t, dir)
	again.Read(func(d *Data) {
		if len(d.Searches) != 82 {
			t.Errorf("%d searches, want the 82 starting ones", len(d.Searches))
		}
		if len(d.Exclusions) != 6 {
			t.Errorf("%d exclusions, want the 6 starting ones", len(d.Exclusions))
		}
		if len(d.People) != 1 || !d.People[0].Connected || !d.People[0].ConnectedAt.Equal(t0) || d.People[0].LinkedIn.BornAbout != 1999 {
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
	var lastID int64
	s.Read(func(d *Data) { lastID = d.LastID })
	err := s.Update(func(d *Data) error {
		_, _ = d.AddSearch("Yogalehrerin Köln", 0, t0)
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	s.Read(func(d *Data) {
		if len(d.Searches) != 0 || d.LastID != lastID {
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
	d.Exclusions = []*Exclusion{}
	s, _ := d.AddSearch("Gründer Köln", 0, t0)
	add := func(name string, born, score int, connected bool, seen time.Time) *Person {
		p, _ := d.AddFound(Found{URL: "https://www.linkedin.com/in/" + name, Name: name, Headline: "Gründer", City: "Köln", BornAbout: born}, s, 0, seen)
		p.FirstSeen, p.Score = seen, score
		p.Connect(connected, seen)
		return p
	}
	old := add("Bea", 1980, 3, false, t0.Add(-10*24*time.Hour))
	young := add("Cem", 2001, 1, true, t0.Add(time.Hour))
	dana := add("Dana", 2003, 2, false, t0.Add(2*time.Hour))
	unknown := add("Ann", 0, 2, false, t0.Add(3*time.Hour))

	now := t0.Add(4 * time.Hour)
	list := func(q PeopleQuery) []*Person {
		q.Now = now
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
	// Someone Tim connected with is only under Connected, so All holds
	// whom he can still reach out to. New holds whoever came in during
	// the last week, connected too, the newest first, whatever the order.
	// The best fit first: with nothing else known, the youngest, then
	// whose age is not known.
	same("best fit first", list(PeopleQuery{}), dana, unknown, old)
	same("young", list(PeopleQuery{Sort: "young"}), dana, old, unknown)
	same("new", list(PeopleQuery{Sort: "new"}), unknown, dana, old)
	same("name", list(PeopleQuery{Sort: "name"}), unknown, old, dana)
	same("fit", list(PeopleQuery{Sort: "fit"}), dana, unknown, old)
	same("new", list(PeopleQuery{Filter: "new"}), unknown, dana, young)
	same("new, whatever the order", list(PeopleQuery{Filter: "new", Sort: "name"}), unknown, dana, young)
	same("connected", list(PeopleQuery{Filter: "connected"}), young)
	same("words", list(PeopleQuery{Q: "dana"}), dana)
	same("words of someone connected", list(PeopleQuery{Q: "cem"}))
	same("words of someone connected, under Connected", list(PeopleQuery{Q: "cem", Filter: "connected"}), young)

	_, counts, _ := d.ListPeople(PeopleQuery{Filter: "connected", Now: now})
	if counts["all"] != 3 || counts["new"] != 3 || counts["connected"] != 1 {
		t.Errorf("counts = %v", counts)
	}
	for _, f := range []string{"founder", "kept", "skipped", "young"} {
		if _, _, err := d.ListPeople(PeopleQuery{Filter: f}); err == nil {
			t.Errorf("the filter %s was taken", f)
		}
	}

	// An exclusion hides whom it matches, never someone connected, and
	// removing it brings them back.
	dana.Headline = "Online-Marketing-Expertin"
	young.Headline = "Onlinemarketing und Yoga"
	x, err := d.AddExclusion("online  marketing", t0)
	if err != nil || x.Phrase != "online marketing" {
		t.Fatalf("added %+v, %v", x, err)
	}
	same("with an exclusion", list(PeopleQuery{}), unknown, old)
	same("with an exclusion, connected", list(PeopleQuery{Filter: "connected"}), young)
	if h := d.Hidden(); len(h) != 1 || h[dana.ID] != "online marketing" {
		t.Errorf("hidden = %v", h)
	}
	_, counts, _ = d.ListPeople(PeopleQuery{Now: now})
	if counts["all"] != 2 || counts["new"] != 2 || counts["connected"] != 1 {
		t.Errorf("counts with an exclusion = %v", counts)
	}
	if !d.DeleteExclusion(x.ID) || d.DeleteExclusion(x.ID) {
		t.Error("an exclusion should be removed once")
	}
	same("without it", list(PeopleQuery{}), dana, unknown, old)

	// Someone whose profile Tim rejected is only under Rejected, and
	// comes back on Undo. Someone connected stays under Connected.
	dana.LinkedIn.Review = "rejected"
	same("rejected leaves all", list(PeopleQuery{}), unknown, old)
	same("rejected leaves new", list(PeopleQuery{Filter: "new"}), unknown, young)
	same("rejected", list(PeopleQuery{Filter: "rejected"}), dana)
	same("words of someone rejected", list(PeopleQuery{Q: "dana"}))
	same("words of someone rejected, under Rejected", list(PeopleQuery{Q: "dana", Filter: "rejected"}), dana)
	young.LinkedIn.Review = "rejected"
	same("connected and rejected", list(PeopleQuery{Filter: "connected"}), young)
	same("connected and rejected, not under Rejected", list(PeopleQuery{Filter: "rejected"}), dana)
	_, counts, _ = d.ListPeople(PeopleQuery{Now: now})
	if counts["all"] != 2 || counts["new"] != 2 || counts["connected"] != 1 || counts["rejected"] != 1 {
		t.Errorf("counts with someone rejected = %v", counts)
	}
	dana.LinkedIn.Review = "open"
	same("undone", list(PeopleQuery{}), dana, unknown, old)
	same("undone, not under Rejected", list(PeopleQuery{Filter: "rejected"}))
}

// Someone Tim added from a link is connected and named by the handle
// until their profile is read. A link to someone here already only marks
// them connected.
func TestAddLink(t *testing.T) {
	d := &Data{}
	d.fill()
	p, isNew := d.AddLink("https://www.linkedin.com/in/ida-probe-7f", "ida-probe-7f", 5, t0)
	if !isNew || p.Name != "ida-probe-7f" || !p.Connected || !p.ToRead || p.Evidence != AddedByHand || p.FirstRunID != 5 {
		t.Fatalf("added %+v", p)
	}
	again, isNew := d.AddLink("https://www.linkedin.com/in/ida-probe-7f", "ida-probe-7f", 6, t0.Add(time.Hour))
	if isNew || again != p || len(d.People) != 1 {
		t.Fatalf("the same link added %+v again, new %v", again, isNew)
	}
	d.ReadLink(p, Found{Name: "Ida Probe", Headline: "Inhaberin, Probe Studio", City: "Essen", Text: "Ida Probe\nInhaberin"}, t0.Add(time.Minute))
	if p.Name != "Ida Probe" || p.Headline != "Inhaberin, Probe Studio" || p.City != "Essen" || p.ToRead || p.LinkedIn.Text == "" {
		t.Errorf("read %+v", p)
	}
	s, _ := d.AddSearch("Gründer Köln", 0, t0)
	found, _ := d.AddFound(Found{URL: "https://www.linkedin.com/in/jo-muster", Name: "Jo Muster"}, s, 1, t0)
	if p, isNew := d.AddLink(found.LinkedIn.URL, "jo-muster", 7, t0.Add(time.Hour)); isNew || p != found || !p.Connected || p.ToRead || p.Name != "Jo Muster" {
		t.Errorf("someone a search found, added from a link: %+v, new %v", p, isNew)
	}
}

func TestExclusions(t *testing.T) {
	d := &Data{}
	d.fill()
	if got := d.Phrases(); len(got) != 6 || got[0] != "Online Marketing" {
		t.Fatalf("starting exclusions %v", got)
	}
	if _, err := d.AddExclusion("ONLINE marketing", t0); err != ErrExclusionExists {
		t.Errorf("the same phrase in another case: %v", err)
	}
	if _, err := d.AddExclusion(" - ", t0); err != ErrNoPhrase {
		t.Errorf("a phrase without a word: %v", err)
	}
	if _, err := d.AddExclusion(strings.Repeat("a", MaxPhrase+1), t0); err != ErrLongPhrase {
		t.Errorf("a long phrase: %v", err)
	}
	if _, err := d.AddExclusion("Coach", t0); err != nil {
		t.Fatal(err)
	}
	sorted := d.SortedExclusions()
	if sorted[0].Phrase != "Coach" || sorted[len(sorted)-1].Phrase != "Social Media Marketing" {
		t.Errorf("sorted %v", sorted)
	}
	// Removing every exclusion keeps them removed, the starting ones do
	// not come back.
	for _, x := range slices.Clone(d.Exclusions) {
		d.DeleteExclusion(x.ID)
	}
	raw, _ := json.Marshal(d)
	again := &Data{}
	if err := json.Unmarshal(raw, again); err != nil {
		t.Fatal(err)
	}
	again.fill()
	if len(again.Exclusions) != 0 {
		t.Errorf("the starting exclusions came back: %v", again.Phrases())
	}
}

// A file from before connections: kept becomes connected, skipped open
// again, and the old words are dropped.
func TestOldDecisionsBecomeConnections(t *testing.T) {
	dir := t.TempDir()
	old := `{"version": 1, "last_id": 2, "people": [
		{"id": 1, "name": "Ada Beispiel", "decision": "kept", "decided_at": "2026-11-01T10:00:00Z"},
		{"id": 2, "name": "Ben Muster", "decision": "skipped", "decided_at": "2026-11-02T10:00:00Z"}]}`
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	s := open(t, dir)
	if err := s.Update(func(*Data) error { return nil }); err != nil {
		t.Fatal(err)
	}
	s.Read(func(d *Data) {
		ada, ben := d.Person(1), d.Person(2)
		if !ada.Connected || ada.ConnectedAt.IsZero() || ben.Connected || ada.Decision != "" || ben.Decision != "" {
			t.Errorf("read as %+v and %+v", ada, ben)
		}
		if len(d.Exclusions) != 6 || d.Exclusions[0].ID != 3 {
			t.Errorf("a file without exclusions gets the starting ones, with new ids: %+v", d.Exclusions[0])
		}
	})
	raw, _ := os.ReadFile(filepath.Join(dir, FileName))
	if strings.Contains(string(raw), `"decision"`) {
		t.Error("the file still says decision")
	}
}

// Runs take the searches that led to connections before those that did
// not, and never-run searches before both.
func TestDueTakesTheBestSearchesFirst(t *testing.T) {
	d := &Data{}
	d.fill()
	none, _ := d.AddSearch("Gründer Köln", 0, t0)
	good, _ := d.AddSearch("Yogalehrerin Bonn", 0, t0)
	fresh, _ := d.AddSearch("Tanzschule Essen", 0, t0)
	for i, s := range []*Search{none, good} {
		for j := range 3 {
			p, _ := d.AddFound(Found{URL: fmt.Sprintf("https://www.linkedin.com/in/p-%d-%d", i, j), Name: "Someone"}, s, 0, t0)
			p.Connect(s == good && j == 0, t0)
		}
		s.Runs, s.LastRun = 1, t0
	}
	due := d.Due(t0, 10)
	if len(due) != 3 || due[0] != fresh || due[1] != good || due[2] != none {
		t.Errorf("due %v", due)
	}
	y := d.Yields()[good.ID]
	if y.People != 3 || y.Connected != 1 || !y.Ran {
		t.Errorf("yield %+v", y)
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

// The whole profile text is kept, the newest Exa gave, and a search whose
// answer had none leaves the one there.
func TestAddFoundKeepsTheProfileText(t *testing.T) {
	d := &Data{}
	d.fill()
	s, _ := d.AddSearch("Gründer Köln", 0, t0)
	url := "https://www.linkedin.com/in/eva-beispiel"
	d.AddFound(Found{URL: url, Name: "Eva Beispiel", Text: "# Eva Beispiel\n\n## About\n\nAlt"}, s, 1, t0)
	p, _ := d.AddFound(Found{URL: url, Name: "Eva Beispiel", Text: "# Eva Beispiel\n\n## About\n\nNeu"}, s, 2, t0.Add(time.Hour))
	if p.LinkedIn.Text != "# Eva Beispiel\n\n## About\n\nNeu" || !p.LinkedIn.TextAt.Equal(t0.Add(time.Hour)) {
		t.Errorf("text %q from %v", p.LinkedIn.Text, p.LinkedIn.TextAt)
	}
	p, _ = d.AddFound(Found{URL: url, Name: "Eva Beispiel"}, s, 3, t0.Add(2*time.Hour))
	if p.LinkedIn.Text == "" || !p.LinkedIn.TextAt.Equal(t0.Add(time.Hour)) {
		t.Errorf("an answer without text took the text away: %q from %v", p.LinkedIn.Text, p.LinkedIn.TextAt)
	}
}

// Builds before where LinkedIn opens had a field of its own kept it as two
// settings. Opening such a file keeps the choice and drops the settings.
func TestWhereLinkedInOpensMovesOutOfTheSettings(t *testing.T) {
	dir := t.TempDir()
	old := `{"version":1,"settings":{
		"linkedin_browser":{"value":"Brave Browser","updated_at":"2026-11-14T10:00:00Z"},
		"linkedin_profile":{"value":"Profile 1","updated_at":"2026-11-14T10:00:00Z"},
		"profile_search_results":{"value":"30","updated_at":"2026-11-14T10:00:00Z"}}}`
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	st.Read(func(d *Data) {
		if d.LinkedInOpens != (Where{App: "Brave Browser", Folder: "Profile 1"}) {
			t.Errorf("where = %+v", d.LinkedInOpens)
		}
		if _, ok := d.Settings["linkedin_profile"]; ok || len(d.Settings) != 1 {
			t.Errorf("settings = %v", d.Settings)
		}
	})
}
