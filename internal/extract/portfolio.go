package extract

import (
	"net/url"
	"regexp"
	"strings"
)

// Startup is a company a portfolio links to: its website, or the page
// about it on the portfolio's own site, which links to the website.
type Startup struct {
	Name    string
	Website string
	Page    string
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

// PortfolioPage is what one page of a portfolio shows.
type PortfolioPage struct {
	Startups []Startup
	// More are the portfolio's further pages, like "/p2" or "?page=3".
	More []string
}

var (
	pageNumber   = regexp.MustCompile(`(?i)(?:/p|/page/|/seite/)(\d+)/?$`)
	pageQuery    = regexp.MustCompile(`(?i)(?:^|&)(?:page|p|seite|paged)=(\d+)(?:&|$)`)
	startupPaths = regexp.MustCompile(`(?i)(?:start-?ups?|portfolio|companies|company|unternehmen|gruender|founders?|ventures?|teams|alumni|batch|kohorte|cohort)`)
)

// Portfolio reads one page of a portfolio. A startup is a link in the
// page's content, not its header, navigation or footer, either to the front
// page of another website that is not a platform, or to one of many pages
// about single startups on the portfolio's own site, like
// "/startups/beispiel-robotics". Each startup comes once.
func Portfolio(body, base string) PortfolioPage {
	var out PortfolioPage
	own := siteOf(hostOf(base))
	baseURL, _ := url.Parse(base)
	links := Links(body, base)
	seen := map[string]bool{}
	var local []Link
	for _, l := range links {
		if l.Chrome {
			continue
		}
		u, err := url.Parse(l.URL)
		if err != nil {
			continue
		}
		host := hostOf(l.URL)
		site := siteOf(host)
		if host == "" || hostIs(host, notStartupSites...) {
			continue
		}
		if site == own {
			if isMorePage(u, baseURL) {
				if !seen["page "+l.URL] {
					seen["page "+l.URL] = true
					out.More = append(out.More, l.URL)
				}
				continue
			}
			local = append(local, l)
			continue
		}
		// A company's front page, or its front page in one language.
		if path := strings.Trim(u.Path, "/"); strings.Contains(path, "/") || len(path) > 5 || seen[site] {
			continue
		}
		seen[site] = true
		out.Startups = append(out.Startups, Startup{Name: startupName(l.Text, host), Website: u.Scheme + "://" + u.Host + "/"})
	}
	// Pages about single startups share one parent path, like "/startups/".
	// The parent with the most of them wins, when it says so or has many.
	parents := map[string]int{}
	for _, l := range local {
		if parent := parentPath(l.URL); parent != "" {
			parents[parent]++
		}
	}
	best, most := "", 0
	for p, n := range parents {
		if (n >= 3 && startupPaths.MatchString(p) || n >= 8) && (n > most || n == most && p < best) {
			best, most = p, n
		}
	}
	if best == "" {
		return out
	}
	for _, l := range local {
		if parentPath(l.URL) != best {
			continue
		}
		u, _ := url.Parse(l.URL)
		u.RawQuery, u.Fragment = "", ""
		page := u.String()
		if seen[page] || (baseURL != nil && strings.TrimSuffix(u.Path, "/") == strings.TrimSuffix(baseURL.Path, "/")) {
			continue
		}
		seen[page] = true
		slug := strings.Trim(u.Path[strings.LastIndex(strings.TrimSuffix(u.Path, "/"), "/")+1:], "/")
		out.Startups = append(out.Startups, Startup{Name: startupName(l.Text, slug), Page: page})
	}
	return out
}

// StartupLinks lists the startups a portfolio page links to.
func StartupLinks(body, base string) []Startup {
	return Portfolio(body, base).Startups
}

// parentPath is a link's path without its last part, "/en/startups/" for
// "/en/startups/beispiel". The front page has none.
func parentPath(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	p := strings.TrimSuffix(u.Path, "/")
	i := strings.LastIndex(p, "/")
	if i <= 0 {
		return ""
	}
	return p[:i+1]
}

// isMorePage says whether u is another page of the list at base: the same
// path with a page number.
func isMorePage(u, base *url.URL) bool {
	if base == nil {
		return false
	}
	bp := strings.TrimSuffix(pageNumber.ReplaceAllString(base.Path, ""), "/")
	if m := pageNumber.FindStringSubmatch(u.Path); m != nil && m[1] != "1" {
		return strings.TrimSuffix(pageNumber.ReplaceAllString(u.Path, ""), "/") == bp
	}
	if m := pageQuery.FindStringSubmatch(u.RawQuery); m != nil && m[1] != "1" {
		return strings.TrimSuffix(u.Path, "/") == strings.TrimSuffix(base.Path, "/") || strings.TrimSuffix(u.Path, "/") == bp
	}
	return false
}

// websiteText is how a page about a startup names the link to its website.
var websiteText = regexp.MustCompile(`(?i)^(?:website|webseite|homepage|zur website|zur webseite|visit|visit website|go to .+|zu .+|www\..+|https?://.+)$`)

// StartupWebsite finds a startup's own website on the page a portfolio has
// about it: a link in the page's content to the front page of another
// website that is not a platform. A link that says "Website" wins.
func StartupWebsite(body, base string) string {
	own := siteOf(hostOf(base))
	first := ""
	for _, l := range Links(body, base) {
		host := hostOf(l.URL)
		if l.Chrome || host == "" || siteOf(host) == own || hostIs(host, notStartupSites...) {
			continue
		}
		u, err := url.Parse(l.URL)
		if err != nil {
			continue
		}
		site := u.Scheme + "://" + u.Host + "/"
		if websiteText.MatchString(strings.TrimSpace(l.Text)) {
			return site
		}
		if first == "" {
			first = site
		}
	}
	return first
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
