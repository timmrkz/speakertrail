package extract

import (
	"net/url"
	"regexp"
	"strings"
)

// Startup is a company a portfolio links to.
type Startup struct {
	Name    string
	Website string
}

// notStartupSites are platforms, networks and tools a portfolio links to
// that are nobody's company website.
var notStartupSites = []string{
	"linkedin.com", "instagram.com", "facebook.com", "fb.com", "twitter.com", "x.com", "youtube.com", "youtu.be",
	"xing.com", "github.com", "tiktok.com", "medium.com", "google.com", "goo.gl", "apple.com", "apps.apple.com",
	"play.google.com", "meetup.com", "eventbrite.de", "eventbrite.com", "luma.com", "lu.ma", "wikipedia.org",
	"vimeo.com", "spotify.com", "t.me", "whatsapp.com", "wa.me", "calendly.com", "typeform.com", "mailchimp.com",
	"eepurl.com", "bit.ly", "linktr.ee", "crunchbase.com", "angel.co", "wellfound.com", "podcasts.apple.com",
	"soundcloud.com", "flickr.com", "pinterest.com", "threads.net", "bsky.app", "mastodon.social", "discord.gg",
	"discord.com", "slack.com", "notion.site", "gstatic.com", "googleapis.com", "cookiebot.com", "usercentrics.eu",
	"europa.eu", "bund.de", "nrw.de", "wordpress.org", "wordpress.com", "wix.com", "jimdo.com", "squarespace.com",
}

// genericText is link text that says nothing about the company.
var genericText = regexp.MustCompile(`(?i)^(?:website|webseite|zur website|zur webseite|homepage|visit|visit website|mehr|mehr erfahren|more|learn more|read more|details|link|hier|here|www\..*|https?://.*)$`)

// StartupLinks lists the company websites a portfolio page links to: links
// in the page's content, not its header, navigation or footer, to the front
// page of another website that is not a platform. Each website comes once.
func StartupLinks(body, base string) []Startup {
	own := siteOf(hostOf(base))
	seen := map[string]bool{}
	var out []Startup
	for _, l := range Links(body, base) {
		if l.Chrome {
			continue
		}
		u, err := url.Parse(l.URL)
		if err != nil {
			continue
		}
		host := hostOf(l.URL)
		site := siteOf(host)
		if host == "" || site == own || hostIs(host, notStartupSites...) || seen[site] {
			continue
		}
		// A company's front page, or its front page in one language.
		if path := strings.Trim(u.Path, "/"); strings.Contains(path, "/") || len(path) > 5 {
			continue
		}
		seen[site] = true
		out = append(out, Startup{Name: startupName(l.Text, host), Website: u.Scheme + "://" + u.Host + "/"})
	}
	return out
}

// startupName is the link's text, or the website's name when the text says
// nothing, "beispiel-robotics.de" becoming "Beispiel Robotics".
func startupName(text, host string) string {
	text = cleanText(text)
	if len([]rune(text)) >= 2 && len([]rune(text)) <= 60 && !genericText.MatchString(text) {
		return text
	}
	label := strings.Split(host, ".")[0]
	words := strings.FieldsFunc(label, func(r rune) bool { return r == '-' || r == '_' })
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

// siteOf is a host's website without subdomains: "blog.beispiel.de" is
// "beispiel.de". Two-part endings like "co.uk" keep three parts.
func siteOf(host string) string {
	parts := strings.Split(host, ".")
	if len(parts) <= 2 {
		return host
	}
	n := 2
	if l := parts[len(parts)-2]; l == "co" || l == "com" || l == "org" || l == "ac" {
		n = 3
	}
	if n > len(parts) {
		n = len(parts)
	}
	return strings.Join(parts[len(parts)-n:], ".")
}
