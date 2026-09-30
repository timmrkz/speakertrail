package pipeline

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/timmrkz/speakertrail/internal/extract"
	"github.com/timmrkz/speakertrail/internal/fetch"
	"github.com/timmrkz/speakertrail/internal/search"
	"github.com/timmrkz/speakertrail/internal/settings"
)

// listSites are directories, review and job sites that show up in search
// results. They list businesses but are none, and their imprint names
// nobody Tim looks for.
var listSites = []string{
	"yelp.de", "yelp.com", "gelbeseiten.de", "dasoertliche.de", "dastelefonbuch.de", "11880.com", "golocal.de",
	"meinestadt.de", "cylex.de", "branchenbuch.de", "kununu.com", "tripadvisor.de", "tripadvisor.com",
	"urbansportsclub.com", "eversports.de", "eversports.com", "treatwell.de", "jameda.de", "doctolib.de",
	"stepstone.de", "indeed.com", "de.indeed.com", "google.com", "google.de", "wikipedia.org", "amazon.de",
	"eventbrite.de", "eventbrite.com", "meetup.com", "ebay-kleinanzeigen.de", "kleinanzeigen.de",
}

// checkSearch runs a search source: its query goes to a search provider,
// and each website in the results is kept like a directory's entry, to be
// looked up for its imprint and about page. LinkedIn, Instagram, platforms
// and directories among the results are skipped and never requested.
// Without a search provider, or with every budget spent, the search waits.
func (p *Pipeline) checkSearch(ctx context.Context, src Source, runID int64) error {
	if p.Search == nil || src.Query == "" || src.Status == "manual" {
		return nil
	}
	cfg, err := settings.Load(ctx, p.Pool)
	if err != nil {
		return err
	}
	start := p.now()
	rec := checkRecord{checkedAt: start, mode: "search"}
	results, _, err := p.Search.Search(ctx, src.Query)
	if errors.Is(err, search.ErrNoBudget) {
		p.log().Info("a search waits, no searches left this month", "source", src.ID)
		return nil
	}
	if err != nil {
		rec.err = err.Error()
		rec.duration = p.now().Sub(start)
		if err := p.recordCheck(ctx, src, runID, rec); err != nil {
			return err
		}
		// The provider failed, not the search. It is tried again tomorrow.
		_, err := p.Pool.Exec(ctx, `UPDATE sources SET last_checked_at = $2, next_check_at = $3 WHERE id = $1`,
			src.ID, start, start.Add(cfg.Days("active_check_interval_days", 1)))
		return err
	}
	rec.status = 200
	return p.storeEntries(ctx, src, runID, cfg, searchEntries(results), rec)
}

// searchEntries turns search results into websites, one per site, named
// by the result's title up to its first separator.
func searchEntries(results []search.Result) []extract.Startup {
	var out []extract.Startup
	seen := map[string]bool{}
	for _, r := range results {
		u, err := url.Parse(strings.TrimSpace(r.URL))
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			continue
		}
		host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
		if errors.Is(fetch.CheckAllowed(u.String()), fetch.ErrBlockedHost) || extract.IsPlatform(host) || isListSite(host) {
			continue
		}
		if seen[host] {
			continue
		}
		seen[host] = true
		out = append(out, extract.Startup{Name: titleName(r.Title, host), Website: u.Scheme + "://" + u.Host + "/"})
	}
	return out
}

func isListSite(host string) bool {
	for _, s := range listSites {
		if host == s || strings.HasSuffix(host, "."+s) {
			return true
		}
	}
	return false
}

// titleName is a result's title up to its first separator, like "Beispiel
// BJJ Köln" for "Beispiel BJJ Köln – Brazilian Jiu-Jitsu", or the
// website's name when the title says nothing.
func titleName(title, host string) string {
	name := strings.TrimSpace(title)
	for _, sep := range []string{" | ", " – ", " — ", " - ", ": "} {
		if i := strings.Index(name, sep); i > 0 {
			name = strings.TrimSpace(name[:i])
		}
	}
	if len([]rune(name)) < 2 || len([]rune(name)) > 60 {
		name = strings.Split(host, ".")[0]
	}
	return name
}
