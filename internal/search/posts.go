package search

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Post is a LinkedIn post in a search index. The engine never opens it.
type Post struct {
	URL   string
	Title string
	// Published is when the post went up, zero when the index does not
	// say.
	Published time.Time
	// Author is the author's profile link, read from the post's link, or
	// empty when the link names nobody.
	Author string
}

// Profile is a LinkedIn profile as a search index holds it. The engine
// never opens it.
type Profile struct {
	URL      string
	Name     string
	Headline string
	// Location is the place the profile gives, like "Cologne, North
	// Rhine-Westphalia, Germany" or "Ruhr Region".
	Location string
	// Career is the jobs and the education the index lists, with their
	// years. Only what they show is kept: an age indicator and the roles
	// held now, never the list itself.
	Career []Step
	// LastActive is the date of the newest post, comment or reaction the
	// profile shows, zero when it shows none.
	LastActive time.Time
}

// Step is one job or one education in a profile.
type Step struct {
	// Education is true for a school or a degree, false for a job.
	Education bool
	// Title is the job's title or the degree, Org the company or school.
	Title, Org string
	// From and To are years, zero when the index does not say. A job
	// without an end is held now.
	From, To int
}

// PostFinder is a provider that finds LinkedIn posts and reads their
// authors' profiles from its own index, so the engine never requests
// LinkedIn. Only Exa can.
type PostFinder interface {
	Provider
	// Posts finds posts about the query published since a time.
	Posts(ctx context.Context, query string, since time.Time) ([]Post, error)
	// Profiles reads profiles by their links. One the index does not hold
	// is left out.
	Profiles(ctx context.Context, urls []string) ([]Profile, error)
	// People finds profiles of people who match a query.
	People(ctx context.Context, query string, results int) ([]Profile, error)
}

// Posts asks Exa's index for LinkedIn posts. It reads nothing live.
func (e *Exa) Posts(ctx context.Context, query string, since time.Time) ([]Post, error) {
	body, _ := json.Marshal(map[string]any{
		"query": query, "type": "auto", "numResults": 10,
		"includeDomains":     []string{"linkedin.com/posts"},
		"startPublishedDate": since.UTC().Format(time.RFC3339),
		"contents":           map[string]any{"text": map[string]any{"maxCharacters": 300}, "livecrawl": "never"},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, orDefault(e.URL, "https://api.exa.ai/search"), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", e.Key)
	req.Header.Set("Content-Type", "application/json")
	var out struct {
		Results []struct {
			Title         string `json:"title"`
			URL           string `json:"url"`
			PublishedDate string `json:"publishedDate"`
			Text          string `json:"text"`
		} `json:"results"`
	}
	if err := call(e.Client, req, e.Name(), &out); err != nil {
		return nil, err
	}
	res := make([]Post, 0, len(out.Results))
	for _, r := range out.Results {
		p := Post{URL: r.URL, Title: postTitle(r.Title, r.Text), Author: AuthorOf(r.URL)}
		if t, err := time.Parse(time.RFC3339, r.PublishedDate); err == nil {
			p.Published = t
		}
		res = append(res, p)
	}
	return res, nil
}

// Profiles reads profiles from Exa's index, never live, so LinkedIn is
// not asked even when the index does not hold one. A profile it does not
// hold costs nothing and is left out.
func (e *Exa) Profiles(ctx context.Context, urls []string) ([]Profile, error) {
	if len(urls) == 0 {
		return nil, nil
	}
	body, _ := json.Marshal(map[string]any{
		"urls": urls, "text": map[string]any{"maxCharacters": 600}, "livecrawl": "never",
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, contentsURL(e.URL), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", e.Key)
	req.Header.Set("Content-Type", "application/json")
	var out struct {
		Results []exaProfile `json:"results"`
	}
	if err := call(e.Client, req, e.Name(), &out); err != nil {
		return nil, err
	}
	return profilesOf(out.Results), nil
}

// People asks Exa's index for LinkedIn profiles of people who match a
// query, like "Personal Trainer Düsseldorf", with what each profile says:
// the place, the jobs and education, and its latest activity. It reads
// nothing live.
func (e *Exa) People(ctx context.Context, query string, results int) ([]Profile, error) {
	body, _ := json.Marshal(map[string]any{
		"query": query, "category": "people", "numResults": results,
		// Enough of the text to reach the activity, which comes last.
		"contents": map[string]any{"text": map[string]any{"maxCharacters": 8000}, "livecrawl": "never"},
	})
	var out struct {
		Results []exaProfile `json:"results"`
	}
	// Exa's people search fails now and then with an internal error that
	// is gone a moment later, so it is asked once more after one.
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, orDefault(e.URL, "https://api.exa.ai/search"), bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("x-api-key", e.Key)
		req.Header.Set("Content-Type", "application/json")
		err = call(e.Client, req, e.Name(), &out)
		if err == nil {
			break
		}
		if attempt > 0 || !strings.Contains(err.Error(), " answered 5") {
			return nil, err
		}
		select {
		case <-time.After(e.retryWait()):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	var res []Profile
	for _, p := range profilesOf(out.Results) {
		// The people category holds other sites' pages of people too.
		if u, err := url.Parse(p.URL); err == nil && strings.HasSuffix(strings.ToLower(u.Hostname()), "linkedin.com") && strings.HasPrefix(u.Path, "/in/") {
			res = append(res, p)
		}
	}
	return res, nil
}

// retryWait is how long People waits before it asks again.
func (e *Exa) retryWait() time.Duration {
	if e.RetryWait > 0 {
		return e.RetryWait
	}
	return 3 * time.Second
}

// exaProfile is a LinkedIn profile as Exa's index answers it.
type exaProfile struct {
	Title    string `json:"title"`
	URL      string `json:"url"`
	Text     string `json:"text"`
	Entities []struct {
		Type       string `json:"type"`
		Properties struct {
			Name        string `json:"name"`
			Location    string `json:"location"`
			WorkHistory []struct {
				Title   string    `json:"title"`
				Dates   *exaDates `json:"dates"`
				Company exaNamed  `json:"company"`
			} `json:"workHistory"`
			EducationHistory []struct {
				Degree      string    `json:"degree"`
				Dates       *exaDates `json:"dates"`
				Institution exaNamed  `json:"institution"`
			} `json:"educationHistory"`
		} `json:"properties"`
	} `json:"entities"`
}

// activityDate finds the date of one entry in a profile's activity, like
// "[2026-07-22 · Kommentar zu …]".
var activityDate = regexp.MustCompile(`\[(20\d\d-\d\d-\d\d) ·`)

// profilesOf reads the profiles of Exa's answer. One without a name is
// left out.
func profilesOf(results []exaProfile) []Profile {
	res := make([]Profile, 0, len(results))
	for _, r := range results {
		p := Profile{URL: r.URL, Name: strings.TrimSpace(r.Title)}
		for _, en := range r.Entities {
			if en.Type == "person" {
				if en.Properties.Name != "" {
					p.Name = strings.TrimSpace(en.Properties.Name)
				}
				p.Location = strings.TrimSpace(en.Properties.Location)
				for _, w := range en.Properties.WorkHistory {
					from, to := w.Dates.years()
					// A job without a start says neither when nor whether
					// it is held now.
					if from == 0 {
						continue
					}
					p.Career = append(p.Career, Step{Title: strings.TrimSpace(w.Title), Org: strings.TrimSpace(w.Company.Name), From: from, To: to})
				}
				for _, ed := range en.Properties.EducationHistory {
					from, to := ed.Dates.years()
					if from == 0 && to == 0 {
						continue
					}
					p.Career = append(p.Career, Step{Education: true, Title: strings.TrimSpace(ed.Degree), Org: strings.TrimSpace(ed.Institution.Name), From: from, To: to})
				}
				break
			}
		}
		p.Headline, p.Location = profileLines(r.Text, p.Location)
		for _, m := range activityDate.FindAllStringSubmatch(r.Text, -1) {
			if t, err := time.Parse("2006-01-02", m[1]); err == nil && t.After(p.LastActive) {
				p.LastActive = t
			}
		}
		if p.Name == "" {
			continue
		}
		res = append(res, p)
	}
	return res
}

type exaNamed struct {
	Name string `json:"name"`
}

// exaDates are a step's dates in Exa's index, like "2019-10-01" or
// "2019", with no end for a job held now.
type exaDates struct {
	From *string `json:"from"`
	To   *string `json:"to"`
}

// years reads the years of a step, zero where they are not known.
func (d *exaDates) years() (from, to int) {
	if d == nil {
		return 0, 0
	}
	if d.From != nil {
		from = yearOf(*d.From)
	}
	if d.To != nil {
		to = yearOf(*d.To)
	}
	return from, to
}

// yearOf is the year a date starts with, or zero.
func yearOf(date string) int {
	if len(date) < 4 {
		return 0
	}
	y := 0
	for _, c := range date[:4] {
		if c < '0' || c > '9' {
			return 0
		}
		y = y*10 + int(c-'0')
	}
	if y < 1940 || y > 2100 {
		return 0
	}
	return y
}

// contentsURL is Exa's contents endpoint, beside the search endpoint a
// test sets.
func contentsURL(searchURL string) string {
	if searchURL == "" {
		return "https://api.exa.ai/contents"
	}
	return strings.TrimSuffix(searchURL, "/search") + "/contents"
}

// placeLine is a profile line that gives its place, like "Cologne, North
// Rhine-Westphalia, Germany (DE)".
var placeLine = regexp.MustCompile(`^(.+?)\s*\([A-Z]{2}\)$`)

// profileLines reads the headline, and the place when the index gave none,
// from the start of a profile's text: the name as a heading, then the
// headline, then the place, then the counts of connections. A profile
// without a headline goes straight to the place.
func profileLines(text, location string) (headline, place string) {
	var lines []string
	for _, l := range strings.Split(text, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "# ") {
			continue
		}
		if strings.HasPrefix(l, "## ") || strings.Contains(l, " connections") || strings.Contains(l, " followers") {
			break
		}
		lines = append(lines, l)
		if len(lines) == 2 {
			break
		}
	}
	isPlace := func(l string) (string, bool) {
		if m := placeLine.FindStringSubmatch(l); m != nil {
			return m[1], true
		}
		return l, location != "" && strings.HasPrefix(l, location)
	}
	place = location
	for i, l := range lines {
		if p, ok := isPlace(l); ok {
			if place == "" {
				place = p
			}
			if i == 1 {
				headline = lines[0]
			}
			return headline, place
		}
	}
	if len(lines) > 0 {
		headline = lines[0]
	}
	return headline, place
}

// postTitle is a post's first line, without the author's name and the
// date the index adds, like "Ein Jahr selbstständig. | Erika Beispiel ·
// LinkedIn · 2026-08-13".
func postTitle(title, text string) string {
	t := strings.TrimSpace(title)
	if t == "" {
		t = strings.TrimSpace(strings.TrimPrefix(strings.SplitN(strings.TrimSpace(text), "\n", 2)[0], "# "))
	}
	if i := strings.LastIndex(t, " | "); i > 0 {
		t = t[:i]
	}
	if i := strings.Index(t, " · "); i > 0 {
		t = t[:i]
	}
	t = strings.TrimSpace(t)
	if r := []rune(t); len(r) > 200 {
		t = string(r[:199]) + "…"
	}
	return t
}

// AuthorOf is the profile link of a post's author, read from the post's
// link: linkedin.com/posts/erika-beispiel_ein-jahr-… was written by
// linkedin.com/in/erika-beispiel. A link of another shape names nobody.
func AuthorOf(postURL string) string {
	u, err := url.Parse(strings.TrimSpace(postURL))
	if err != nil {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	if host != "linkedin.com" && !strings.HasSuffix(host, ".linkedin.com") {
		return ""
	}
	rest, ok := strings.CutPrefix(u.EscapedPath(), "/posts/")
	if !ok {
		return ""
	}
	slug, _, ok := strings.Cut(rest, "_")
	if !ok || slug == "" || strings.Contains(slug, "/") {
		return ""
	}
	name, err := url.PathUnescape(slug)
	if err != nil || name == "" {
		return ""
	}
	return "https://www.linkedin.com/in/" + url.PathEscape(name)
}
