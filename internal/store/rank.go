package store

import (
	"cmp"
	"slices"
)

// SearchYield is what a profile search has brought so far: whether it ran,
// everyone it found, and how many of them Tim connected with.
type SearchYield struct {
	ID        int64
	Points    float64
	Ran       bool
	People    int
	Connected int
}

// Yields counts, for every search, the people it found and how many of
// them Tim connected with.
func (d *Data) Yields() map[int64]SearchYield {
	out := make(map[int64]SearchYield, len(d.Searches))
	for _, s := range d.Searches {
		out[s.ID] = SearchYield{ID: s.ID, Points: s.Points, Ran: s.Runs > 0 || !s.LastRun.IsZero()}
	}
	for _, p := range d.People {
		for _, f := range p.FoundBy {
			y, ok := out[f.SearchID]
			if !ok {
				continue
			}
			y.People++
			if p.Connected {
				y.Connected++
			}
			out[f.SearchID] = y
		}
	}
	return out
}

// RankSearches orders due profile searches the way a run takes them, best
// first. A search that never ran comes first, so each gets one try, those
// Tim added before the starting ones by their points. Then the searches
// that led to connections, the most first, and among the same, those with
// the larger share of connections. Last come searches whose people Tim
// never connected with, those that found more people first, so a search
// that found nobody runs only when there is room. Tim asked for this on 10
// November 2026: searches that are good for him run, those that never
// yield much wait.
func RankSearches(s []SearchYield) {
	group := func(y SearchYield) int {
		switch {
		case !y.Ran:
			return 0
		case y.Connected > 0:
			return 1
		}
		return 2
	}
	slices.SortStableFunc(s, func(a, b SearchYield) int {
		if c := cmp.Compare(group(a), group(b)); c != 0 {
			return c
		}
		switch group(a) {
		case 0:
			if c := cmp.Compare(b.Points, a.Points); c != 0 {
				return c
			}
		case 1:
			if c := cmp.Compare(b.Connected, a.Connected); c != 0 {
				return c
			}
			// a.Connected/a.People against b's, without dividing.
			if c := cmp.Compare(b.Connected*a.People, a.Connected*b.People); c != 0 {
				return c
			}
		case 2:
			if c := cmp.Compare(b.People, a.People); c != 0 {
				return c
			}
			if c := cmp.Compare(b.Points, a.Points); c != 0 {
				return c
			}
		}
		return cmp.Compare(a.ID, b.ID)
	})
}
