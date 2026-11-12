package store

import "testing"

func TestCarryOver(t *testing.T) {
	d := &Data{}
	d.fill()
	s, _ := d.AddSearch("Gründerin Köln", 0, t0)
	here, _ := d.AddFound(Found{URL: "https://www.linkedin.com/in/ida-beispiel", Name: "Ida Beispiel"}, s, 0, t0)
	people := []Carried{
		{Name: "Ida Beispiel", LinkedIn: "https://www.linkedin.com/in/ida-beispiel", Notes: "Met at the meetup"},
		{Name: "Ole Muster", LinkedIn: "https://www.linkedin.com/in/ole-muster", Headline: "Inhaber, Ole's Gym", City: "Essen", BornAbout: 1998, KeptAt: t0},
		{Name: "Pia Ohne Profil", City: "Bonn"},
		{Name: "  "},
	}
	added, there := d.CarryOver(people, t0)
	if added != 2 || there != 1 {
		t.Errorf("added %d, there %d, want 2 and 1", added, there)
	}
	if !here.Connected || here.Notes != "Met at the meetup" {
		t.Errorf("the person here = %+v", here)
	}
	ole := d.PersonByProfile("https://www.linkedin.com/in/ole-muster")
	if ole == nil || !ole.Connected || ole.LinkedIn.BornAbout != 1998 || ole.LinkedIn.Handle != "ole-muster" || ole.City != "Essen" {
		t.Errorf("ole = %+v", ole)
	}
	// Again changes nothing.
	added, there = d.CarryOver(people, t0)
	if added != 0 || there != 3 || len(d.People) != 3 {
		t.Errorf("again: added %d, there %d, people %d", added, there, len(d.People))
	}
}
