package api

import (
	"net/http"
	"net/url"
	"testing"
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
	if before.Links == nil || before.Links.New != 2 || len(before.Links.Here) != 0 || before.Links.Other != 1 || len(before.People) != 0 {
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
	if after.Links == nil || after.Links.New != 0 || len(after.Links.Here) != 2 || len(after.People) != 2 {
		t.Fatalf("after = %+v", after)
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
	if err := a.Keys.Set(""); err != nil {
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
