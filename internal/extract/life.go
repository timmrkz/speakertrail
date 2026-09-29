package extract

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// parked matches the page a domain shows when nobody uses it any more.
var parked = regexp.MustCompile(`(?i)(?:domain (?:is|ist|may be) (?:for sale|zu verkaufen|available)|this domain (?:is|may be) for sale|buy this domain|diese domain (?:kaufen|steht zum verkauf|ist zu verkaufen|wurde registriert)|domain (?:kaufen|erwerben)|sedo\.com|dan\.com|parkingcrew|bodis\.com|domain parking)`)

// Parked says whether a website's front page is a parked domain or a
// placeholder, which means the company no longer uses it.
func Parked(text string) bool {
	return len(strings.TrimSpace(text)) < 3000 && parked.MatchString(text)
}

var liquidation = regexp.MustCompile(`(?i)(?:\bin liquidation\b|\bi\.\s?l\.(?:\s|$|,)|\bin abwicklung\b|\binsolvenzverwalter|\binsolvenzverfahren\b)`)

// InLiquidation says whether an imprint says the company is being wound up.
func InLiquidation(text string) bool {
	return liquidation.MatchString(text)
}

var copyright = regexp.MustCompile(`(?i)(?:©|\(c\)|&copy;|copyright)\s*(?:\d{4}\s*[-–—]\s*)?((?:19|20)\d{2})`)

// CopyrightYear is the newest year in a page's copyright line, 0 without.
func CopyrightYear(text string) int {
	year := 0
	for _, m := range copyright.FindAllStringSubmatch(text, -1) {
		if y, err := strconv.Atoi(m[1]); err == nil && y > year {
			year = y
		}
	}
	return year
}

var lastmod = regexp.MustCompile(`(?i)<lastmod>\s*([0-9]{4}-[0-9]{2}-[0-9]{2})`)

// SitemapNewest is the newest change a sitemap lists, not after now. Zero
// without one.
func SitemapNewest(xml string, now time.Time) time.Time {
	var newest time.Time
	for _, m := range lastmod.FindAllStringSubmatch(xml, -1) {
		t, err := time.Parse("2006-01-02", m[1])
		if err != nil || t.After(now.Add(24*time.Hour)) {
			continue
		}
		if t.After(newest) {
			newest = t
		}
	}
	return newest
}
