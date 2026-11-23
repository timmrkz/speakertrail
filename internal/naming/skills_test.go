package naming_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

type notSaid struct {
	what string
	re   *regexp.Regexp
}

// What no skill says, since every skill is written for any project: who
// the user is, an issue or a pull request by its number, the product's
// name, the words that only mean something here, and the tools and parts
// this kind of app has and most do not. An agent in another repository
// would look for a person, an app or a part that is not there. A link to
// a heading, `](#4-the-tests)`, is not a number, see numbers. Words any
// product may rightly use, a keychain, a search, a profile, are not
// banned, so an example told in them is still the review's to catch.
// Ported from FrameFairy's internal/naming, with Speaker Trail's words.
var notSaids = []notSaid{
	{"a person", regexp.MustCompile(`\bTim\b|timmrkz`)},
	{"an issue or pull request by number", regexp.MustCompile(`(?i)\b(issues?|pull requests?|PRs?)( number)? [0-9]+|\b(pull|issues)/[0-9]+|\bGH-[0-9]+`)},
	{"a product", regexp.MustCompile(`(?i)speaker[ -]?trail|frame[ -]?fairy|frame ?ferry|My First Memory`)},
	{"a word of this project", regexp.MustCompile(`(?i)\bExa\b|linkedin|\bNRW\b|north rhine|profile search|first message|\bwalks?\b|RunProgress|Pick\.svelte|app\.css|Morning Mist|Dusk Fog`)},
	{"a word of this kind of app", regexp.MustCompile(`(?i)\bwails\b|svelte|llama|whisper|podcast|Hugging ?Face`)},
	// FrameFairy's words, the skills' source, so a lesson brought over
	// from there names none of its app either.
	{"a word of FrameFairy's app", regexp.MustCompile(`(?i)range picker|clip timeline|video preview|\bepisodes?\b|ffmpeg|libass|\bgemma\b|parakeet|\bVP9\b|\bOpus\b|H\.264|\bAAC\b|AudioContext|AudioWorklet|sound card|playhead|frame queue|Go side|VideoToolbox|AudioToolbox|GStreamer|AppKit|\bsubtitles?\b|transcri|waveform|\bshorts\b`)},
}

// numbers finds `#12`, and with it what stands before it, so a link to a
// heading, here or in another file, can be told from an issue. Six digits
// or more are a colour, `#942192`.
var numbers = regexp.MustCompile(`(\]\([^)\s#]*)?#[0-9]{1,5}\b`)

// said returns what a line says that it should not.
func said(line string) []string {
	var found []string
	for _, r := range notSaids {
		if m := r.re.FindString(line); m != "" {
			found = append(found, r.what+", "+m)
		}
	}
	for _, m := range numbers.FindAllStringSubmatch(line, -1) {
		if m[1] == "" {
			found = append(found, "an issue or pull request by number, "+m[0])
		}
	}
	return found
}

// Every skill in .claude/skills/ is written for any project, so its folder
// can be copied into another repository as it is, see docs/SKILLS.md.
//
// On the real disk: what is read is the repository's own skills. The path
// is written out whole, so internal/workflows sees that a change to a
// skill has to start this test.
func TestSkillsAreGeneric(t *testing.T) {
	t.Parallel()
	root := filepath.FromSlash("../../.claude/skills/")
	read := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		read[strings.Split(filepath.ToSlash(rel), "/")[0]] = true
		text, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for n, line := range strings.Split(string(text), "\n") {
			for _, s := range said(line) {
				t.Errorf("%s:%d names %s: see docs/SKILLS.md", filepath.ToSlash(path), n+1, s)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !read["testing"] || !read["interface"] {
		t.Fatalf("read the skills %v without testing or interface, so this test reads the wrong folder", read)
	}
}

// The check above fails on what it should, and on nothing a skill would
// rightly say.
func TestSaidCatchesWhatItShould(t *testing.T) {
	t.Parallel()
	cases := []struct {
		line string
		bad  bool
	}{
		{"Tim asked for it.", true},
		{"assigned to timmrkz", true},
		{"see #742", true},
		{"(#97)", true},
		{"as pull request 12 did", true},
		{"Issue 7 says so.", true},
		{"what is issue number 678", true},
		{"github.com/o/r/pull/980", true},
		{"github.com/o/r/issues/5", true},
		{"fixed in GH-12", true},
		{"Speaker Trail finds guests.", true},
		{"the speakertrail app", true},
		{"ported from FrameFairy", true},
		{"the Frame Ferry app", true},
		{"a search through Exa", true},
		{"opened on LinkedIn", true},
		{"people in NRW", true},
		{"the walks run in CI", true},
		{"the fill in app.css", true},
		{"a run shows through RunProgress", true},
		{"the page's Wails runtime", true},
		{"a Svelte component", true},
		{"llama-server on loopback", true},
		{"an episode made by ffmpeg", true},
		{"the playhead and the waveform", true},
		{"It is VP9 and Opus, not H.264 and AAC.", true},
		{"the transcript grows", true},
		{"captions for people who cannot hear, WCAG", false},
		{"a JSON decoder", false},
		{"The user decides.", false},
		{"A timer in a test is a guess.", false},
		{"one frame in 60, 5 % slower", false},
		{"the first issue opened", false},
		{"see [Proof](#4-proof-at-a-fair-price)", false},
		{"the accent, `#942192`", false},
		{"it walked the tree", false},
		{"the system keychain", false},
		{"Exactly one spelling", false},
		{"a search, a profile, a run", false},
		{"Playwright records a video of every failed test", false},
		{"short and concrete", false},
	}
	for _, c := range cases {
		got := said(c.line)
		if (len(got) > 0) != c.bad {
			t.Errorf("%q: found %q, want caught %v", c.line, got, c.bad)
		}
	}
}
