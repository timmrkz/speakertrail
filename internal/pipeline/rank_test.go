package pipeline_test

import (
	"fmt"
	"testing"

	"github.com/timmrkz/speakertrail/internal/pipeline"
)

// A search that never ran gets its try first, Tim's own before the
// starting ones. Then the searches that led to connections, the most
// first, and last those whose people Tim never connected with.
func TestRankSearches(t *testing.T) {
	s := []pipeline.SearchYield{
		{ID: 1, Ran: true, People: 20, Connected: 0},
		{ID: 2, Ran: true, People: 10, Connected: 2},
		{ID: 3, Ran: false, Points: 0},
		{ID: 4, Ran: true, People: 5, Connected: 4},
		{ID: 5, Ran: false, Points: 2},
		{ID: 6, Ran: true, People: 0},
		{ID: 7, Ran: true, People: 30, Connected: 2},
		{ID: 8, Ran: true, People: 20, Connected: 0, Points: 1},
	}
	pipeline.RankSearches(s)
	var got []int64
	for _, y := range s {
		got = append(got, y.ID)
	}
	if want := "[5 3 4 2 7 8 1 6]"; fmt.Sprint(got) != want {
		t.Errorf("order %v, want %s", got, want)
	}
}
