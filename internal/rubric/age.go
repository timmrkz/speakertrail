package rubric

import (
	"fmt"
	"regexp"
)

// Step is one job or one education on a person's profile, with its years,
// zero where the profile does not say.
type Step struct {
	Education  bool
	Title, Org string
	From, To   int
}

// YoungUntil is the age below which a person counts as younger.
const YoungUntil = 30

// A step's kind, and the age most people are when it starts and ends.
type stage struct {
	name     string
	pattern  *regexp.Regexp
	from, to int
}

// Degrees and schools by how old people usually are, in Germany. An MBA,
// a course or a certificate can come at any age and says nothing.
var schooling = []stage{
	{"Doctorate", words(`ph\.? ?d\.?|dr\.|doktor\w*|promotion|doctor\w*`), 26, 29},
	{"Master", words(`master\w*|m\.? ?sc\.?|msc|m\.? ?a\.?|m\.? ?eng\.?|diplom\w*|dipl\.|magister|staatsexamen`), 23, 25},
	{"Bachelor", words(`bachelor\w*|b\.? ?sc\.?|bsc|b\.? ?a\.?|b\.? ?eng\.?|studium|undergraduate`), 19, 22},
	// Not "Ausbildung" alone, which is also a yoga teacher's training at 40.
	{"Apprenticeship", words(`apprenticeship|berufsausbildung|duale? ausbildung|lehre|\w*kauf(?:mann|frau)|geselle|gesellin|ihk`), 17, 20},
	{"School", words(`abitur|fachabitur|abi|gymnasium|realschule|gesamtschule|hauptschule|mittlere reife|high school|secondary school|a-levels?|schule`), 0, 19},
}

// Jobs people usually start younger than a first proper job.
var (
	studentJob    = words(`praktikant\w*|praktikum|intern|internship|werkstudent\w*|working student|studentische hilfskraft|student assistant|aushilfe|minijob|ferienjob`)
	apprenticeJob = words(`azubi|auszubildende[rn]?|apprentice|lehrling`)
)

// BornAbout guesses the year a person was born from their profile: from
// each step that says something about age, the year they would have been
// born in if they were as old as most people are at that step. The
// earliest of these counts, because a late degree makes a person look
// younger than they are, and an early start never does. Jobs count only
// when there are two or more, since one job held now is no first job. The
// reason names the step that decided it. Without any such step, the year
// is zero.
func BornAbout(steps []Step) (year int, reason string) {
	jobs := 0
	for _, s := range steps {
		if !s.Education && s.From > 0 {
			jobs++
		}
	}
	try := func(born int, why string) {
		if year == 0 || born < year {
			year, reason = born, why
		}
	}
	firstJob := 0
	for _, s := range steps {
		if s.Education {
			for _, st := range schooling {
				if !st.pattern.MatchString(s.Title) {
					continue
				}
				switch {
				case s.From > 0 && st.from > 0:
					try(s.From-st.from, fmt.Sprintf("%s from %d", st.name, s.From))
				case s.To > 0:
					try(s.To-st.to, fmt.Sprintf("%s until %d", st.name, s.To))
				}
				break
			}
			continue
		}
		if jobs < 2 || s.From == 0 {
			continue
		}
		switch {
		case apprenticeJob.MatchString(s.Title):
			try(s.From-17, fmt.Sprintf("Apprenticeship from %d", s.From))
		case studentJob.MatchString(s.Title):
			try(s.From-19, fmt.Sprintf("Student job from %d", s.From))
		default:
			if firstJob == 0 || s.From < firstJob {
				firstJob = s.From
			}
		}
	}
	if firstJob > 0 {
		try(firstJob-22, fmt.Sprintf("First job from %d", firstJob))
	}
	return year, reason
}

// agePassage says how old a person likely is and why.
func agePassage(born int, reason string, year int) string {
	return fmt.Sprintf("%s, so about %d", reason, year-born)
}
