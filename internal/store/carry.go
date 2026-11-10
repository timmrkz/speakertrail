package store

import (
	"strings"
	"time"
)

// Carried is a person Tim kept in the Docker version of Speaker Trail, as
// scripts/carry-over.sh reads them out of its database.
type Carried struct {
	Name       string    `json:"name"`
	Headline   string    `json:"headline"`
	City       string    `json:"city"`
	Notes      string    `json:"notes"`
	KeptAt     time.Time `json:"kept_at"`
	FirstSeen  time.Time `json:"first_seen"`
	LinkedIn   string    `json:"linkedin"`
	BornAbout  int       `json:"born_about"`
	AgeReason  string    `json:"age_reason"`
	Roles      string    `json:"roles"`
	LastActive time.Time `json:"last_active"`
}

// CarryOver adds the people Tim kept in the Docker version, kept here
// too. Someone already here, by their LinkedIn profile or else by their
// name, is kept and keeps what is here. It is safe to repeat. It returns
// how many it added and how many were here already.
func (d *Data) CarryOver(people []Carried, at time.Time) (added, there int) {
	for _, c := range people {
		c.Name = strings.TrimSpace(c.Name)
		if c.Name == "" {
			continue
		}
		var p *Person
		if c.LinkedIn != "" {
			p = d.PersonByProfile(c.LinkedIn)
		}
		if p == nil {
			for _, x := range d.People {
				if strings.EqualFold(x.Name, c.Name) && (c.LinkedIn == "" || x.LinkedIn.URL == "") {
					p = x
					break
				}
			}
		}
		kept := c.KeptAt
		if kept.IsZero() {
			kept = at
		}
		if p != nil {
			there++
			if p.Decision != "kept" {
				p.Decision, p.DecidedAt, p.UpdatedAt = "kept", kept, at
			}
			if p.Notes == "" {
				p.Notes = c.Notes
			}
			continue
		}
		added++
		first := c.FirstSeen
		if first.IsZero() {
			first = at
		}
		p = &Person{
			ID: d.NewID(), Name: c.Name, Headline: c.Headline, City: c.City, Notes: c.Notes,
			Decision: "kept", DecidedAt: kept, FirstSeen: first, UpdatedAt: at,
			Evidence: "Kept in the Docker version of Speaker Trail",
			Signals:  []Signal{},
		}
		if c.LinkedIn != "" {
			p.LinkedIn = LinkedIn{URL: c.LinkedIn, Handle: c.LinkedIn[strings.LastIndex(c.LinkedIn, "/")+1:], Headline: c.Headline,
				Review: "confirmed", BornAbout: c.BornAbout, AgeReason: c.AgeReason, Roles: c.Roles, LastActive: c.LastActive}
		}
		d.People = append(d.People, p)
	}
	return added, there
}
