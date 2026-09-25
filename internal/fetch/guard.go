package fetch

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// blockedDomains are never requested, not even for robots.txt. LinkedIn and
// Instagram forbid automated access, and Tim's LinkedIn account is the
// channel that brings in podcast guests. Profile links only ever come from
// search results.
var blockedDomains = []string{
	"linkedin.com",
	"lnkd.in",
	"licdn.com",
	"instagram.com",
	"instagr.am",
	"cdninstagram.com",
}

// ErrBlockedHost means the URL points to a site the engine never requests.
var ErrBlockedHost = errors.New("host is never requested")

// ErrRobots means robots.txt disallows the URL for this bot.
var ErrRobots = errors.New("disallowed by robots.txt")

// CheckAllowed returns ErrBlockedHost for URLs on a blocked domain or one of
// its subdomains, and an error for anything that is not plain http or https.
func CheckAllowed(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse %q: %w", rawURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%q: only http and https are fetched", rawURL)
	}
	if IsBlockedHost(u.Hostname()) {
		return fmt.Errorf("%s: %w", u.Hostname(), ErrBlockedHost)
	}
	return nil
}

// IsBlockedHost reports whether host is a blocked domain or a subdomain of
// one.
func IsBlockedHost(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	for _, d := range blockedDomains {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}
