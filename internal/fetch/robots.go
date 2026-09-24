package fetch

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Robots decides whether a URL may be fetched, following RFC 9309. It keeps
// each site's rules for a day.
type Robots struct {
	get   func(ctx context.Context, url string) (status int, body []byte, err error)
	agent string
	ttl   time.Duration
	now   func() time.Time

	mu    sync.Mutex
	cache map[string]robotsEntry
}

type robotsEntry struct {
	rules   *robotsRules
	fetched time.Time
}

// NewRobots returns a robots.txt checker. get fetches a robots.txt file and
// agent is the bot's product token, matched against User-agent lines.
func NewRobots(get func(ctx context.Context, url string) (int, []byte, error), agent string) *Robots {
	return &Robots{get: get, agent: strings.ToLower(agent), ttl: 24 * time.Hour, now: time.Now, cache: map[string]robotsEntry{}}
}

// Allowed reports whether the bot may fetch rawURL.
func (r *Robots) Allowed(ctx context.Context, rawURL string) (bool, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false, err
	}
	origin := u.Scheme + "://" + u.Host
	rules, err := r.rulesFor(ctx, origin)
	if err != nil {
		return false, err
	}
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	if u.RawQuery != "" {
		path += "?" + u.RawQuery
	}
	return rules.allowed(path), nil
}

func (r *Robots) rulesFor(ctx context.Context, origin string) (*robotsRules, error) {
	r.mu.Lock()
	e, ok := r.cache[origin]
	r.mu.Unlock()
	if ok && r.now().Sub(e.fetched) < r.ttl {
		return e.rules, nil
	}
	status, body, err := r.get(ctx, origin+"/robots.txt")
	var rules *robotsRules
	switch {
	case err != nil:
		// An unreachable robots.txt means the site may not be crawled.
		return nil, fmt.Errorf("robots.txt of %s: %w", origin, err)
	case status >= 200 && status < 300:
		rules = parseRobots(string(body), r.agent)
	case status >= 400 && status < 500:
		rules = &robotsRules{}
	default:
		rules = &robotsRules{disallowAll: true}
	}
	r.mu.Lock()
	r.cache[origin] = robotsEntry{rules: rules, fetched: r.now()}
	r.mu.Unlock()
	return rules, nil
}

type robotsRule struct {
	allow   bool
	pattern string
}

type robotsRules struct {
	disallowAll bool
	rules       []robotsRule
}

// allowed applies the longest matching rule. On a tie, allow wins.
func (rr *robotsRules) allowed(path string) bool {
	if rr.disallowAll {
		return false
	}
	best, bestLen, found := true, -1, false
	for _, rule := range rr.rules {
		if rule.pattern == "" {
			continue
		}
		if !robotsMatch(rule.pattern, path) {
			continue
		}
		l := len(rule.pattern)
		if l > bestLen || (l == bestLen && rule.allow && !best) {
			best, bestLen, found = rule.allow, l, true
		}
	}
	if !found {
		return true
	}
	return best
}

// robotsMatch matches a robots.txt path pattern with * and a trailing $.
func robotsMatch(pattern, path string) bool {
	anchored := strings.HasSuffix(pattern, "$")
	pattern = strings.TrimSuffix(pattern, "$")
	parts := strings.Split(pattern, "*")
	for i, p := range parts {
		parts[i] = regexp.QuoteMeta(p)
	}
	expr := "^" + strings.Join(parts, ".*")
	if anchored {
		expr += "$"
	}
	re, err := regexp.Compile(expr)
	return err == nil && re.MatchString(path)
}

// parseRobots reads the groups for agent, or the * group when no group
// names the agent.
func parseRobots(body, agent string) *robotsRules {
	type group struct {
		agents []string
		rules  []robotsRule
	}
	var groups []*group
	var cur *group
	lastWasAgent := false
	sc := bufio.NewScanner(io.LimitReader(strings.NewReader(body), 500<<10))
	sc.Buffer(make([]byte, 64<<10), 64<<10)
	for sc.Scan() {
		line := sc.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		switch key {
		case "user-agent":
			if cur == nil || !lastWasAgent {
				cur = &group{}
				groups = append(groups, cur)
			}
			cur.agents = append(cur.agents, strings.ToLower(value))
			lastWasAgent = true
		case "allow", "disallow":
			lastWasAgent = false
			if cur == nil {
				continue
			}
			cur.rules = append(cur.rules, robotsRule{allow: key == "allow", pattern: value})
		default:
			lastWasAgent = false
		}
	}

	var mine, star []robotsRule
	matched := false
	for _, g := range groups {
		for _, a := range g.agents {
			switch {
			case a == "*":
				star = append(star, g.rules...)
			case a != "" && strings.Contains(agent, a):
				mine = append(mine, g.rules...)
				matched = true
			}
		}
	}
	if matched {
		return &robotsRules{rules: mine}
	}
	return &robotsRules{rules: star}
}

// httpGetter adapts an http.Client for robots.txt requests.
func httpGetter(client *http.Client, userAgent string) func(ctx context.Context, url string) (int, []byte, error) {
	return func(ctx context.Context, rawURL string) (int, []byte, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return 0, nil, err
		}
		req.Header.Set("User-Agent", userAgent)
		resp, err := client.Do(req)
		if err != nil {
			return 0, nil, err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(io.LimitReader(resp.Body, 500<<10))
		if err != nil {
			return 0, nil, err
		}
		return resp.StatusCode, body, nil
	}
}
