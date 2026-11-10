package store

import (
	_ "embed"
	"encoding/csv"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// startingSearches are the profile searches a new app begins with: a kind
// of person who fits and a place in NRW. Those aimed at younger people
// carry a point, so runs take them first.
//
//go:embed starting_searches.csv
var startingSearches string

// Start adds the starting searches, once. A search Tim removes later does
// not come back.
func (d *Data) Start(at time.Time) error {
	if d.Started {
		return nil
	}
	rows, err := csv.NewReader(strings.NewReader(startingSearches)).ReadAll()
	if err != nil {
		return fmt.Errorf("starting searches: %w", err)
	}
	for _, row := range rows[1:] {
		points, _ := strconv.ParseFloat(row[1], 64)
		if _, err := d.AddSearch(row[0], points, at); err != nil && err != ErrSearchExists {
			return err
		}
	}
	d.Started = true
	return nil
}

// ErrSearchExists says a search with the same words is there already.
var ErrSearchExists = fmt.Errorf("that search is there already")

// AddSearch adds a profile search. A new search is due at once.
func (d *Data) AddSearch(query string, points float64, at time.Time) (*Search, error) {
	query = strings.Join(strings.Fields(query), " ")
	if query == "" {
		return nil, fmt.Errorf("enter a kind of person and a place, like Yogalehrerin Köln")
	}
	if len([]rune(query)) > 200 {
		return nil, fmt.Errorf("a search has at most 200 characters")
	}
	for _, s := range d.Searches {
		if strings.EqualFold(s.Query, query) {
			return s, ErrSearchExists
		}
	}
	s := &Search{ID: d.NewID(), Query: query, Status: "candidate", StatusChanged: at, Points: points, CreatedAt: at}
	d.Searches = append(d.Searches, s)
	return s, nil
}

// Pause leaves a search out of runs until Resume.
func (s *Search) Pause(at time.Time) {
	if s.Status != "retired" {
		s.Status, s.StatusChanged = "retired", at
	}
}

// Resume lets runs take a search again, in the next run.
func (s *Search) Resume(at time.Time) {
	if s.Status == "retired" {
		s.Status, s.StatusChanged = "candidate", at
		s.NextRun = time.Time{}
		s.EmptyInRow = 0
	}
}

// Due lists the searches a run takes now: those not paused whose time has
// come, the longest due first, then those with more points, then the
// oldest. At most limit of them.
func (d *Data) Due(now time.Time, limit int) []*Search {
	var due []*Search
	for _, s := range d.Searches {
		if s.Status == "retired" || s.Query == "" {
			continue
		}
		if !s.NextRun.IsZero() && s.NextRun.After(now) {
			continue
		}
		due = append(due, s)
	}
	slices.SortStableFunc(due, func(a, b *Search) int {
		switch {
		case a.NextRun.IsZero() != b.NextRun.IsZero():
			if a.NextRun.IsZero() {
				return -1
			}
			return 1
		case !a.NextRun.Equal(b.NextRun):
			return a.NextRun.Compare(b.NextRun)
		case a.Points != b.Points:
			if a.Points > b.Points {
				return -1
			}
			return 1
		}
		return int(a.ID - b.ID)
	})
	if limit >= 0 && len(due) > limit {
		due = due[:limit]
	}
	return due
}

// Ran moves a search on after a run took it: active while it finds new
// people, on probation when a new one found nobody, and paused when it
// found nobody new for too long, until Tim resumes it. A search that
// failed is due again in the next run.
func (d *Data) Ran(s *Search, c Check) {
	s.Last = &c
	if c.Error != "" {
		return
	}
	s.Runs++
	s.LastRun = c.At
	status := s.Status
	switch {
	case c.PeopleNew > 0:
		s.EmptyInRow, status = 0, "active"
	default:
		s.EmptyInRow++
		switch {
		case s.Status == "candidate":
			status = "probation"
		case s.Status == "probation" && c.At.Sub(s.StatusChanged) > time.Duration(d.Int("probation_weeks"))*7*24*time.Hour,
			s.Status == "active" && s.EmptyInRow >= d.Int("retire_after_empty_checks"):
			status = "retired"
		}
	}
	if status != s.Status {
		s.Status, s.StatusChanged = status, c.At
	}
	// A little early, so a search due on the same day next month is taken
	// by a run at any time of that day.
	s.NextRun = c.At.Add(d.Days("profile_search_check_days") - 2*time.Hour)
}
