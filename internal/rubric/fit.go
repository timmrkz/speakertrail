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
// has a reason Tim can read. Four parts, in the order of how much they
// count: who they are, how lately active on LinkedIn, how near, and how
// young. See docs/fit-score.md, which Tim agreed on 21 November 2026.

// Part is one part of the fit, with the points it gave and why.
type Part struct {
	Key    string
	Label  string
	Points int
	Why    string
	// Mark says whether Why speaks for the person, 1, against, -1, or
	// neither, 0, for the tick or the cross beside it. Who they are
	// leaves it 0, its signals carry their own.
	Mark int
}

// PartOf is the key of the part of the fit a signal shows under: Likely
// under 30 under Younger, the activity on LinkedIn under Active lately,
// and every other signal under Who they are, whether it counts there or
// not, like Runs their own business.
func PartOf(key string) string {
	switch key {
	case "young":
		return "younger"
	case "active_on_linkedin", "quiet_on_linkedin":
		return "active"
	}
	return "who"
}

// mark is a tick for half of a part or more, a cross for less than a
// quarter, and neither between them or when it is not known.
func mark(pts, most int, known bool) int {
	switch {
	case !known:
		return 0
	case 2*pts >= most:
		return 1
	case 4*pts < most:
		return -1
	}
	return 0
}

// What each part gives at most. Tim, 22 November 2026: "the most
// important thing is that they are doing something. And then the next
// most important thing is that they're actually active on doing that
// thing and that they are talking about it. And then I think the next
// important thing is that they are close to me. And then the next
// important thing at the end is that they are younger."
const (
	WhoPoints     = 40
	ActivePoints  = 30
	NearPoints    = 20
	YoungerPoints = 10
)

// ActiveHalf is after how long the points for being active on LinkedIn
// halve, and NearHalfKm after how many kilometres the points for being
// near halve.
const (
	ActiveHalf = 30 * 24 * time.Hour
	NearHalfKm = 40.0
)

// OldUntil is the age from which Younger gives nothing. From YoungUntil
// on, the part falls by a tenth a year, and past OldUntil it goes on
// falling below none, down to minus twice the part at 60. Tim, 27
// November 2026: people around 50 should not make it to the top. At no
// more than none from 40, a 53 year old stood ninth of his open people.
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
	"studies": true, "employed": true, "social_media": true, "ai": true,
}

// Fit is a person's fit and its parts, in the order Why shows them. The
// fit is the sum of the parts' points. Who they are and Younger can take
// points away, but never more than the other parts give, so the fit is
// never below 0.
func Fit(p FitPerson) (int, []Part) {
	parts := []Part{whoPart(p, true), activePart(p, true), nearPart(p, true), youngerPart(p, true)}
	pts := [4]int{parts[0].Points, parts[1].Points, parts[2].Points, parts[3].Points}
	fit := sum(&pts)
	for i := range parts {
		parts[i].Points = pts[i]
	}
	return fit, parts
}

// FitPoints is the fit alone, the same as Fit gives, without writing why,
// so a list of thousands is sorted by it quickly.
func FitPoints(p FitPerson) int {
	pts := [4]int{whoPart(p, false).Points, activePart(p, false).Points, nearPart(p, false).Points, youngerPart(p, false).Points}
	return sum(&pts)
}

// sum adds up the points of the parts, after a part below none, in the
// order of the parts, takes away no more than the others have left.
func sum(pts *[4]int) int {
	left := 0
	for _, n := range pts {
		left += max(n, 0)
	}
	for i, n := range pts {
		if n < 0 {
			pts[i] = max(n, -left)
			left += pts[i]
		}
	}
	return left
}

// points is a share of most, from none to all of it.
func points(most int, share float64) int {
	return int(math.Round(float64(most) * math.Max(0, math.Min(1, share))))
}

// signed is a share of most from minus all of it to all of it.
func signed(most int, share float64) int {
	return int(math.Round(float64(most) * math.Max(-1, math.Min(1, share))))
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
	// Each signal against takes a third away, below none too, so a
	// student or someone employed elsewhere falls behind an owner who
	// shows nothing more (Tim, 22 November 2026).
	if !say {
		return Part{Points: signed(WhoPoints, float64(n)/3)}
	}
	// What counts against says so by its name, like Employed elsewhere,
	// and by the points, so it needs no "Against" (Tim, 22 November 2026).
	why := strings.Join(append(pro, contra...), ", ")
	if why == "" {
		why = "Nothing beyond running their own business"
	}
	return Part{Key: "who", Label: "Who they are", Points: signed(WhoPoints, float64(n)/3), Why: why}
}

func youngerPart(p FitPerson, say bool) Part {
	part := Part{Key: "younger", Label: "Younger"}
	if p.BornAbout == 0 {
		part.Points, part.Why = points(YoungerPoints, 0.5), "Age not known"
		return part
	}
	age := p.Now.Year() - p.BornAbout
	// Points whole first, so no fraction near a half rounds the wrong way.
	part.Points = int(math.Round(math.Max(-2*YoungerPoints, math.Min(YoungerPoints, float64(YoungerPoints*(OldUntil-age))/float64(OldUntil-YoungUntil)))))
	part.Mark = mark(part.Points, YoungerPoints, true)
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
	case !p.Wrote:
		// Most people on LinkedIn do not post, only 11 of 60 had in six
		// months in a test on 30 September 2026, so activity of a kind not
		// known counts as a reaction. Tim saw people high up who did not post, 22 November.
		part.Points = points(ActivePoints, fading(p.Now.Sub(p.LastActive), ActiveHalf)/2)
		part.Why = "Last active on " + day(p.LastActive) + ", not known whether they wrote"
	case !p.LastWrote.Before(p.LastActive):
		part.Points = points(ActivePoints, fading(p.Now.Sub(p.LastWrote), ActiveHalf))
		part.Why = "Wrote on " + day(p.LastWrote)
	default:
		// A reaction counts half as much as a post or a comment of the
		// same day, so writing a while ago can count for more.
		reacted := fading(p.Now.Sub(p.LastActive), ActiveHalf) / 2
		wrote := 0.0
		if !p.LastWrote.IsZero() {
			wrote = fading(p.Now.Sub(p.LastWrote), ActiveHalf)
		}
		part.Points = points(ActivePoints, math.Max(reacted, wrote))
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
	part.Mark = mark(part.Points, ActivePoints, true)
	return part
}

func nearPart(p FitPerson, say bool) Part {
	part := Part{Key: "near", Label: "Near you", Points: points(NearPoints, 0.5)}
	switch {
	case p.Home == "":
		part.Why = "Where you are is not set in Settings"
	case !p.Near && p.City == "":
		part.Why = "City not known"
	case !p.Near:
		part.Why = p.City + ", distance not known"
	case p.Km < 1:
		part.Points, part.Why = points(NearPoints, 1), p.City+", where you are"
	default:
		part.Points = points(NearPoints, math.Pow(0.5, p.Km/NearHalfKm))
		if say {
			part.Why = fmt.Sprintf("%s, about %.0f km", p.City, p.Km)
		}
	}
	part.Mark = mark(part.Points, NearPoints, p.Home != "" && p.Near)
	return part
}

// activityLine finds one entry of a profile's activity as Exa's index
// writes it, like "- [2026-10-30 · Kommentar: Starker Kurs!](…)", with its
// date and what kind of entry it is.
var activityLine = regexp.MustCompile(`\[(20\d\d-\d\d-\d\d) · ([^\]\n]*)`)

// reaction finds an entry that is only a reaction, a like and its kin,
// with the kind first, like "Gefällt mir: …", or after the person's name,
// as Exa's index writes it, like "Erika Beispiel liked this" or "…
// reacted on this" (Tim's report of 27 November 2026). Anything else, a
// post, a comment or a share, counts as writing, so a word this misses
// makes a reaction count as much as writing, as all activity did before.
var reaction = regexp.MustCompile(`(?i)^(?:gefällt (?:mir|\w+)|likes?\b|liked\b|reagiert\w*|reacted\b|reaction\b|reaktion\b)|\b(?:liked|reacted (?:on|to)) this$`)

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
