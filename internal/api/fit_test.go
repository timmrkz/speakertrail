package api

import (
	"testing"

	"github.com/timmrkz/speakertrail/internal/rubric"
	"github.com/timmrkz/speakertrail/internal/store"
)

// Tim, 22 November 2026: Fit and Why said the same thing twice. One list
// now shows each part with its points, and under it its ticks and
// crosses: the signals under Who they are, the why under the others.
func TestFitPartsAreOneList(t *testing.T) {
	parts := []rubric.Part{
		{Key: "who", Label: "Who they are", Points: -13, Why: "Studies or in training"},
		{Key: "active", Label: "Active lately", Points: 0, Why: "Last active on 22 June 2026, not known whether they wrote", Mark: -1},
		{Key: "near", Label: "Near you", Points: 11, Why: "Düsseldorf, about 36 km", Mark: 1},
		{Key: "younger", Label: "Younger", Points: 5, Why: "Age not known"},
	}
	signals := []store.Signal{
		{Key: "owner_operator", Label: "Runs their own business", For: true, Passage: "Inhaber, Probe Shop", FoundIn: "LinkedIn profile"},
		{Key: "active_on_linkedin", Label: "Active on LinkedIn", For: true, Passage: "Last active on 22 June 2026"},
		{Key: "studies", Label: "Studies or in training", Passage: "Student Business Law @Probe"},
	}
	got := fitParts(parts, signals)
	want := [][]fitLineJSON{
		{{Mark: "for", Text: "Runs their own business", Passage: "Inhaber, Probe Shop", FoundIn: "LinkedIn profile"},
			{Mark: "against", Text: "Studies or in training", Passage: "Student Business Law @Probe"}},
		{{Mark: "against", Text: "Last active on 22 June 2026, not known whether they wrote"}},
		{{Mark: "for", Text: "Düsseldorf, about 36 km"}},
		{{Mark: "neither", Text: "Age not known"}},
	}
	for i, part := range got {
		if part.Points != parts[i].Points || len(part.Lines) != len(want[i]) {
			t.Fatalf("%s: %+v", part.Key, part)
		}
		for j, l := range part.Lines {
			if l != want[i][j] {
				t.Errorf("%s line %d: %+v, want %+v", part.Key, j, l, want[i][j])
			}
		}
	}
	// Without a signal, Who they are says why it has none.
	if l := fitParts(parts[:1], nil)[0].Lines; len(l) != 1 || l[0] != (fitLineJSON{Mark: "neither", Text: "Studies or in training"}) {
		t.Errorf("no signals: %+v", l)
	}
}
