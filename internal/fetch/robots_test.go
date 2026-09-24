package fetch

import "testing"

func TestRobotsRules(t *testing.T) {
	body := `
# comment
User-agent: Googlebot
Disallow: /

User-agent: *
User-agent: OtherBot
Disallow: /admin
Disallow: /*.pdf$
Disallow: /events/*/private
Allow: /admin/public
Disallow: /search?
`
	rules := parseRobots(body, "speakertrailbot")
	for path, want := range map[string]bool{
		"/":                       true,
		"/events":                 true,
		"/admin":                  false,
		"/admin/settings":         false,
		"/admin/public/page":      true,
		"/flyer.pdf":              false,
		"/flyer.pdf?x=1":          true,
		"/events/42/private":      false,
		"/events/42/private/more": false,
		"/events/42/public":       true,
		"/search?q=pitch":         false,
		"/search":                 true,
	} {
		if got := rules.allowed(path); got != want {
			t.Errorf("allowed(%q) = %v, want %v", path, got, want)
		}
	}

	own := parseRobots("User-agent: *\nDisallow: /\n\nUser-agent: SpeakerTrailBot\nAllow: /\n", "speakertrailbot")
	if !own.allowed("/events") {
		t.Error("the bot's own group should win over *")
	}
	if !parseRobots("", "speakertrailbot").allowed("/anything") {
		t.Error("an empty robots.txt allows everything")
	}
	if (&robotsRules{disallowAll: true}).allowed("/") {
		t.Error("disallow all allowed a path")
	}
}
