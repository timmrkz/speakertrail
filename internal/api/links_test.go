package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/timmrkz/speakertrail/internal/engine"
	"github.com/timmrkz/speakertrail/internal/store"
)

type linksPeople struct {
	People []personJSON
	Counts map[string]int
	Links  *linksJSON
}

// Links pasted into the search say what Add would do with them, Add adds
// the people as connected, and the run reads their profiles.
func TestAddPeopleFromLinks(t *testing.T) {
	a, _, e := newAPI(t)
	text := "Vom Frühstück: https://www.linkedin.com/in/ida-probe/ linkedin.com/in/neu-hier-3 https://www.linkedin.com/company/probe-gmbh"

	var before linksPeople
	call(t, a, "GET", "/api/people?q="+url.QueryEscape(text), "", &before)
	if before.Links == nil || before.Links.New != 2 || len(before.Links.Here) != 0 || before.Links.Other != 1 || before.Links.Act != "add" || len(before.People) != 0 {
		t.Fatalf("before = %+v", before)
	}

	var added struct {
		Run  int64
		New  []int64
		Here []int64
		Wait string
	}
	if code := call(t, a, "POST", "/api/people", `{"text":"`+text+`"}`, &added); code != http.StatusOK || added.Run == 0 || len(added.New) != 2 || added.Wait != "" {
		t.Fatalf("add: %d %+v", code, added)
	}
	e.Wait()

	var after linksPeople
	call(t, a, "GET", "/api/people?q="+url.QueryEscape(text)+"&filter=all", "", &after)
	if after.Links == nil || after.Links.New != 0 || len(after.Links.Here) != 2 || after.Links.Act != "none" || len(after.People) != 2 {
		t.Fatalf("after = %+v", after)
	}
	// One link of someone here opens them.
	var one linksPeople
	call(t, a, "GET", "/api/people?q="+url.QueryEscape("https://www.linkedin.com/in/ida-probe"), "", &one)
	if one.Links == nil || one.Links.Act != "open" || len(one.People) != 1 {
		t.Fatalf("one here = %+v", one)
	}
	var opened struct{ Open int64 }
	if code := call(t, a, "POST", "/api/people", `{"text":"https://www.linkedin.com/in/ida-probe"}`, &opened); code != http.StatusOK || opened.Open != one.People[0].ID {
		t.Fatalf("add one here: %d %+v", code, opened)
	}
	var newOnes linksPeople
	call(t, a, "GET", "/api/people?filter=new", "", &newOnes)
	if len(newOnes.People) != 2 || newOnes.Counts["new"] != 2 || newOnes.Counts["connected"] != 2 {
		t.Fatalf("new = %+v", newOnes)
	}
	for _, p := range newOnes.People {
		if !p.Connected {
			t.Errorf("%s is not connected", p.Name)
		}
		switch p.Name {
		case "Ida Probe":
			if p.Read != "" || p.City != "Essen" {
				t.Errorf("read %+v", p)
			}
		case "neu-hier-3":
			if p.Read != "not_indexed" {
				t.Errorf("not in the index %+v", p)
			}
		default:
			t.Errorf("who is %q", p.Name)
		}
	}
	// Words alone are a search, never links.
	var words linksPeople
	call(t, a, "GET", "/api/people?q=ida&filter=connected", "", &words)
	if words.Links != nil || len(words.People) != 1 {
		t.Errorf("words = %+v", words)
	}
	if code := call(t, a, "POST", "/api/people", `{"text":"Yogalehrerin Köln"}`, nil); code != http.StatusBadRequest {
		t.Errorf("words to add: %d", code)
	}
}

// Without a key the people are added, and wait says why their profiles
// wait for the next run.
func TestAddPeopleWithoutAKey(t *testing.T) {
	a, _, _ := newAPI(t)
	if err := a.Keys.Remove(); err != nil {
		t.Fatal(err)
	}
	var added struct {
		New  []int64
		Wait string
	}
	if code := call(t, a, "POST", "/api/people", `{"text":"linkedin.com/in/ida-probe"}`, &added); code != http.StatusOK || len(added.New) != 1 || added.Wait == "" {
		t.Fatalf("add: %d %+v", code, added)
	}
	var people linksPeople
	call(t, a, "GET", "/api/people?filter=connected", "", &people)
	if len(people.People) != 1 || people.People[0].Read != "next" || people.People[0].Name != "ida-probe" {
		t.Errorf("people = %+v", people)
	}
}

// People reads the list again every second or two while a run goes, and
// links are added meanwhile, by Enter or a drop. The two never wait on
// each other for good: the list asks the engine before it reads the
// store, and adding takes them the other way round.
func TestPeopleListedWhileLinksAreAddedFromSeveralGoroutines(t *testing.T) {
	a, _, e := newAPI(t)
	var wg sync.WaitGroup
	for i := range 4 {
		wg.Go(func() {
			for j := range 20 {
				link := fmt.Sprintf("https://www.linkedin.com/in/probe-%d-%d", i, j)
				if code := call(t, a, "POST", "/api/people", `{"text":"`+link+`"}`, nil); code != http.StatusOK {
					t.Errorf("add %s: %d", link, code)
				}
			}
		})
		wg.Go(func() {
			for range 40 {
				call(t, a, "GET", "/api/people?filter=new", "", nil)
				call(t, a, "GET", "/api/people/1", "", nil)
				call(t, a, "GET", "/api/runs/current", "", nil)
			}
		})
	}
	// Should listing and adding wait on each other, go test's -timeout
	// ends the test with the stack of every goroutine.
	wg.Wait()
	e.Wait()
	var list linksPeople
	call(t, a, "GET", "/api/people?filter=new", "", &list)
	if list.Counts["new"] != 80 {
		t.Errorf("%d people are new, want the 80 added", list.Counts["new"])
	}
}

// Adding links is one step: Undo in the Edit menu takes out again whom
// it brought in, opens again whom it marked connected, and leaves
// everyone else. Redo does it again.
func TestUndoAddingPeople(t *testing.T) {
	a, st, e := newAPI(t)
	var found int64
	_ = st.Update(func(d *store.Data) error {
		s, _ := d.AddSearch("Gründerin Köln", 0, t0)
		p, _ := d.AddFound(store.Found{URL: "https://www.linkedin.com/in/jo-muster", Name: "Jo Muster"}, s, 1, t0)
		found = p.ID
		return nil
	})
	var added struct{ New, Connected []int64 }
	call(t, a, "POST", "/api/people", `{"text":"https://www.linkedin.com/in/neu-hier-3 https://www.linkedin.com/in/jo-muster"}`, &added)
	e.Wait()
	if len(added.New) != 1 || !slices.Equal(added.Connected, []int64{found}) {
		t.Fatalf("added %+v", added)
	}
	var u store.Undone
	if code := call(t, a, "POST", "/api/undo", "{}", &u); code != http.StatusOK || !u.Done || u.Said != "Took out 1 person you added" || u.Shows.Kind != "people" {
		t.Fatalf("undo: %d %+v", code, u)
	}
	if code := call(t, a, "GET", fmt.Sprintf("/api/people/%d", added.New[0]), "", nil); code != http.StatusNotFound {
		t.Errorf("the person taken out is still there: %d", code)
	}
	var jo personDetail
	if call(t, a, "GET", fmt.Sprintf("/api/people/%d", found), "", &jo); jo.Connected {
		t.Error("Jo is still connected after the undo")
	}
	if code := call(t, a, "POST", "/api/redo", "{}", &u); code != http.StatusOK || !u.Done || u.Said != "Put back 1 person you added" {
		t.Fatalf("redo: %d %+v", code, u)
	}
	var back personDetail
	if code := call(t, a, "GET", fmt.Sprintf("/api/people/%d", added.New[0]), "", &back); code != http.StatusOK || back.Name != "neu-hier-3" || !back.Connected {
		t.Errorf("after the redo: %d %+v", code, back)
	}
	if call(t, a, "GET", fmt.Sprintf("/api/people/%d", found), "", &jo); !jo.Connected {
		t.Error("Jo is open after the redo")
	}
}

// A LinkedIn page Tim pasted becomes the profile of someone Exa has no
// copy of, with their name, headline and city, and Undo puts back what
// they were.
func TestPastingAProfile(t *testing.T) {
	a, st, e := newAPI(t)
	var added struct{ New []int64 }
	call(t, a, "POST", "/api/people", `{"text":"https://www.linkedin.com/in/neu-hier-3"}`, &added)
	e.Wait()
	path := fmt.Sprintf("/api/people/%d/profile", added.New[0])
	page := "Startseite\nNeu Hier\nGründerin | Hier Studio\nDortmund, Nordrhein-Westfalen, Deutschland\nInfo\nIch gründe in Dortmund. neu@hier.example"
	body, _ := json.Marshal(map[string]string{"text": page})
	var out struct {
		Person struct {
			Name, Headline, City string
			Read                 string     `json:"read"`
			ProfileReadAt        *time.Time `json:"profile_read_at"`
		}
		Named bool
	}
	if code := call(t, a, "PUT", path, string(body), &out); code != http.StatusOK {
		t.Fatalf("paste: %d", code)
	}
	if out.Person.Name != "Neu Hier" || out.Person.Headline != "Gründerin | Hier Studio" || out.Person.City != "Dortmund" || out.Person.ProfileReadAt == nil || !out.Named {
		t.Errorf("pasted %+v", out)
	}
	if out.Person.Read != "" {
		t.Errorf("still says %q", out.Person.Read)
	}
	st.Read(func(d *store.Data) {
		p := d.Person(added.New[0])
		if p.ToRead || p.ReadError != "" || strings.Contains(p.LinkedIn.Text, "Startseite") || strings.Contains(p.LinkedIn.Text, "neu@") {
			t.Errorf("kept %+v, text %q", p, p.LinkedIn.Text)
		}
	})
	// Undo in the Edit menu puts back what they were.
	if code := call(t, a, "POST", "/api/undo", "{}", nil); code != http.StatusOK {
		t.Fatalf("undo: %d", code)
	}
	st.Read(func(d *store.Data) {
		p := d.Person(added.New[0])
		if p.Name != "neu-hier-3" || p.City != "" || p.LinkedIn.Text != "" || p.ReadError != engine.NotIndexed {
			t.Errorf("after Undo %+v", p)
		}
	})
	if code := call(t, a, "PUT", path, `{"text":"  \n "}`, nil); code != http.StatusBadRequest {
		t.Errorf("an empty paste: %d", code)
	}
	if code := call(t, a, "PUT", "/api/people/999999/profile", string(body), nil); code != http.StatusNotFound {
		t.Errorf("nobody: %d", code)
	}
}
