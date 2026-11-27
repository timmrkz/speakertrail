package store

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"github.com/timmrkz/speakertrail/internal/place"
	"github.com/timmrkz/speakertrail/internal/rubric"
)

// Spread makes up to n new profile searches for a run that has room, so
// Run always searches, even when no search is due (Tim, 27 November 2026:
// "I need to search for more people. Doesn't matter whether a search is
// due"). Each asks a kind of person that led to connections, like
// "Gründer unter 30" of "Gründer unter 30 Köln", in a city of NRW it was
// never asked in. The kinds go by the connections they led to, the most
// first, and take turns, each its nearest city to Where you are first.
// Only the big cities are asked, see place.BigCities. A new search keeps
// the search it was made from, so Searches can say so, and is paused
// like any other. Nothing is made from a kind with no connection yet,
// from one with more searches paused than searches Tim added still
// going, nor with words an exclusion leaves out.
func (d *Data) Spread(n int, at time.Time) []*Search {
	if n <= 0 {
		return nil
	}
	type kind struct {
		name              string
		connected, people int
		paused, going     int
		from              *Search
		fromConnected     int
		asked             map[string]bool
	}
	yields := d.Yields()
	kinds := map[string]*kind{}
	var order []*kind
	for _, s := range d.Searches {
		name, city := place.Split(s.Query)
		if city == "" || name == "" {
			continue
		}
		key := strings.ToLower(name)
		k := kinds[key]
		if k == nil {
			k = &kind{name: name, asked: map[string]bool{}}
			kinds[key] = k
			order = append(order, k)
		}
		k.asked[city] = true
		// A search a run made never speaks for its kind, only against it
		// when paused, so pausing the searches Tim added stops a kind
		// however far it spread.
		switch {
		case s.Status == "retired":
			k.paused++
		case s.From == "":
			k.going++
		}
		y := yields[s.ID]
		k.connected += y.Connected
		k.people += y.People
		if k.from == nil || y.Connected > k.fromConnected {
			k.from, k.fromConnected = s, y.Connected
		}
	}
	phrases := d.Phrases()
	order = slices.DeleteFunc(order, func(k *kind) bool {
		_, excluded := rubric.Excluded(k.name, phrases)
		return k.connected == 0 || k.paused > k.going || excluded
	})
	slices.SortStableFunc(order, func(a, b *kind) int {
		// The most connections first, then the larger share of the people
		// found, a.connected/a.people against b's without dividing.
		return cmp.Or(cmp.Compare(b.connected, a.connected), cmp.Compare(b.connected*a.people, a.connected*b.people))
	})
	if len(order) == 0 {
		return nil
	}
	// The nearest cities first. Until Where you are is set, the biggest.
	cities := slices.Clone(place.NRWCities[:place.BigCities])
	if home := d.Text(HomeCity); home != "" {
		km := map[string]float64{}
		for _, c := range cities {
			if v, ok := place.Km(home, c); ok {
				km[c] = v
			}
		}
		if len(km) == len(cities) {
			slices.SortStableFunc(cities, func(a, b string) int { return cmp.Compare(km[a], km[b]) })
		}
	}
	var out []*Search
	for len(out) < n {
		made := false
		for _, k := range order {
			if len(out) == n {
				break
			}
			for _, c := range cities {
				if k.asked[c] {
					continue
				}
				k.asked[c] = true
				s, err := d.AddSearch(k.name+" "+c, 0, at)
				if err != nil {
					// There already under other spaces or capitals, or too long.
					continue
				}
				s.From = k.from.Query
				out = append(out, s)
				made = true
				break
			}
		}
		if !made {
			break
		}
	}
	return out
}
