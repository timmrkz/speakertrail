package rubric

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
)

// The fit is a number from 0 to 100 that orders People, best first. It
// adds up parts, each with the reason it gave its points, so every point
// has a reason Tim can read. Four parts count the same for now: who they
// are, how young, how lately active on LinkedIn, and how near. See
// docs/fit-score.md, which Tim agreed on 21 November 2026.

// Part is one part of the fit, with the points it gave and why.
type Part struct {
	Key    string
	Label  string
	Points int
	Why    string
}

// PartPoints is what each of the four parts gives at most.
const PartPoints = 25

// ActiveHalf is after how long the points for being active on LinkedIn
// halve, and NearHalfKm after how many kilometres the points for being
// near halve.
const (
	ActiveHalf = 30 * 24 * time.Hour
	NearHalfKm = 40.0
)

// OldUntil is the age from which Younger gives nothing. From YoungUntil
// to it, the part falls by a tenth a year.
const OldUntil = 40

// FitPerson is what the fit reads of a person.
type FitPerson struct {
	Signals []Signal
	// BornAbout is the likely year of birth, zero when not known, and
	// AgeReason the step that shows it.
	BornAbout int
	AgeReason string
	// LastActive is the newest post, comment or reaction, zero when none
	// is known. LastWrote is the newest post or comment, and Wrote says
	// whether the kinds of their activity are known at all.
	LastActive time.Time
	LastWrote  time.Time
	Wrote      bool
	// City is where they are, and Km how far that is from Where you are,
	// when Near is true. Home is Where you are, empty until Tim sets it.
	City, Home string
	Km         float64
	Near       bool
	Now        time.Time
}

// who counts the signals the part Who they are reads. Age, activity and
// running their own business are not counted here: the first two have
// parts of their own, and everyone kept runs their own business.
var who = map[string]bool{
	"works_with_people": true, "bootstrapped": true, "runs_events": true, "author": true,
	"athlete": true, "career_switch": true, "funded": true, "enterprise": true, "backed": true,
}

// Fit is a person's fit and its parts, in the order Why shows them. The
// fit is the sum of the parts' points.
func Fit(p FitPerson) (int, []Part) {
	parts := []Part{whoPart(p, true), youngerPart(p, true), activePart(p, true), nearPart(p, true)}
	total := 0
	for _, pt := range parts {
		total += pt.Points
	}
	return total, parts
}

// FitPoints is the fit alone, the same as Fit gives, without writing why,
// so a list of thousands is sorted by it quickly.
func FitPoints(p FitPerson) int {
	return whoPart(p, false).Points + youngerPart(p, false).Points + activePart(p, false).Points + nearPart(p, false).Points
}

func points(share float64) int {
	return int(math.Round(PartPoints * math.Max(0, math.Min(1, share))))
}

// Each part says why only when say is true.
func whoPart(p FitPerson, say bool) Part {
	n := 0
	var pro, contra []string
	for _, s := range p.Signals {
		d := byKey[s.Key]
		if d == nil || !who[s.Key] {
			continue
		}
		switch {
		case s.For:
			n++
		default:
			n--
		}
		if say && s.For {
			pro = append(pro, d.Label)
		} else if say {
			contra = append(contra, d.Label)
		}
	}
	if !say {
		return Part{Points: points(float64(n) / 3)}
	}
	why := strings.Join(pro, ", ")
	if len(contra) > 0 {
		if why != "" {
			why += ", against "
		} else {
			why = "Against "
		}
		why += strings.Join(contra, ", ")
	}
	if why == "" {
		why = "Nothing beyond running their own business"
	}
	return Part{Key: "who", Label: "Who they are", Points: points(float64(n) / 3), Why: why}
}

func youngerPart(p FitPerson, say bool) Part {
	part := Part{Key: "younger", Label: "Younger"}
	if p.BornAbout == 0 {
		part.Points, part.Why = points(0.5), "Age not known"
		return part
	}
	age := p.Now.Year() - p.BornAbout
	// Points whole first, so 39 gives exactly 2.5 to round.
	part.Points = int(math.Round(math.Max(0, math.Min(PartPoints, float64(PartPoints*(OldUntil-age))/float64(OldUntil-YoungUntil)))))
	if !say {
		return part
	}
	part.Why = fmt.Sprintf("About %d", age)
	if p.AgeReason != "" {
		part.Why = agePassage(p.BornAbout, p.AgeReason, p.Now.Year())
	}
	return part
}

// fading is what is left of something after it halves every half.
func fading(since time.Duration, half time.Duration) float64 {
	if since < 0 {
		since = 0
	}
	return math.Pow(0.5, float64(since)/float64(half))
}

func activePart(p FitPerson, say bool) Part {
	part := Part{Key: "active", Label: "Active lately"}
	day := func(t time.Time) string {
		if !say {
			return ""
		}
		return t.Format("2 January 2006")
	}
	switch {
	case p.LastActive.IsZero() && p.LastWrote.IsZero():
		part.Why = "No activity on LinkedIn known"
	case !p.Wrote || !p.LastWrote.Before(p.LastActive):
		newest := p.LastActive
		if p.LastWrote.After(newest) {
			newest = p.LastWrote
		}
		part.Points = points(fading(p.Now.Sub(newest), ActiveHalf))
		part.Why = "Last active on " + day(newest)
		if p.Wrote {
			part.Why = "Wrote on " + day(newest)
		}
	default:
		// A reaction counts half as much as a post or a comment of the
		// same day, so writing a while ago can count for more.
		reacted := fading(p.Now.Sub(p.LastActive), ActiveHalf) / 2
		wrote := 0.0
		if !p.LastWrote.IsZero() {
			wrote = fading(p.Now.Sub(p.LastWrote), ActiveHalf)
		}
		part.Points = points(math.Max(reacted, wrote))
		part.Why = "Reacted on " + day(p.LastActive)
		switch {
		case p.LastWrote.IsZero():
			part.Why += ", wrote nothing"
		case wrote >= reacted:
			part.Why = "Wrote on " + day(p.LastWrote) + ", reacted since"
		default:
			part.Why += ", wrote last on " + day(p.LastWrote)
		}
	}
	return part
}

func nearPart(p FitPerson, say bool) Part {
	part := Part{Key: "near", Label: "Near you", Points: points(0.5)}
	switch {
	case p.Home == "":
		part.Why = "Where you are is not set in Settings"
	case !p.Near && p.City == "":
		part.Why = "City not known"
	case !p.Near:
		part.Why = p.City + ", distance not known"
	case p.Km < 1:
		part.Points, part.Why = points(1), p.City+", where you are"
	default:
		part.Points = points(math.Pow(0.5, p.Km/NearHalfKm))
		if say {
			part.Why = fmt.Sprintf("%s, about %.0f km", p.City, p.Km)
		}
	}
	return part
}

// activityLine finds one entry of a profile's activity as Exa's index
// writes it, like "- [2026-10-30 · Kommentar: Starker Kurs!](…)", with its
// date and what kind of entry it is.
var activityLine = regexp.MustCompile(`\[(20\d\d-\d\d-\d\d) · ([^\]\n]*)`)

// reaction finds an entry that is only a reaction, a like and its kin.
// Anything else, a post, a comment or a share, counts as writing, so a
// word this misses makes a reaction count as much as writing, as all
// activity did before.
var reaction = regexp.MustCompile(`(?i)^(?:gefällt (?:mir|\w+)|likes?\b|liked\b|reagiert\w*|reacted\b|reaction\b|reaktion\b)`)

// Writing reads a profile's text for the newest entry of its activity that
// is a post or a comment rather than a reaction. Known is false when the
// text lists no activity, so nothing is known of what kind it is.
func Writing(text string) (newest time.Time, known bool) {
	for _, m := range activityLine.FindAllStringSubmatch(text, -1) {
		t, err := time.Parse("2006-01-02", m[1])
		if err != nil {
			continue
		}
		known = true
		if !reaction.MatchString(strings.TrimSpace(m[2])) && t.After(newest) {
			newest = t
		}
	}
	return newest, known
}
