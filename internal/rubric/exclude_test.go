package rubric_test

import (
	"testing"

	"github.com/timmrkz/speakertrail/internal/rubric"
)

func TestExcluded(t *testing.T) {
	phrases := []string{"Online Marketing", "LinkedIn", "Coach"}
	for text, want := range map[string]string{
		"Online-Marketing-Expertin | Gründerin":             "Online Marketing",
		"Gründerin · online marketing für Coaches":          "Online Marketing",
		"#onlinemarketing #selbstständig":                   "Online Marketing",
		"Onlinemarketingberaterin, Inhaberin":               "Online Marketing",
		"Ich mache dich sichtbar auf LinkedIn":              "LinkedIn",
		"LinkedIn-Trainerin":                                "LinkedIn",
		"Business Coach":                                    "Coach",
		"Lifecoach und Yogalehrerin":                        "",
		"Gründer von Probe Studio, Marketing nebenbei":      "",
		"Personal Trainerin | Selbstständig":                "",
		"Inhaberin Beispiel Online-Shop, Marketing im Team": "",
	} {
		got, ok := rubric.Excluded(text, phrases)
		if got != want || ok != (want != "") {
			t.Errorf("%q: %q %t, want %q", text, got, ok, want)
		}
	}
	if _, ok := rubric.Excluded("anything", []string{" - ", ""}); ok {
		t.Error("a phrase without a word matches")
	}
	if got := rubric.NormalPhrase("  Online -  Marketing "); got != "Online Marketing" {
		t.Errorf("normal phrase %q", got)
	}
}
