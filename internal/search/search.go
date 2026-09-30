// Package search asks web search providers where people are, behind one
// interface, so providers can be switched or used together. Each provider
// has a monthly budget in Settings, "<name>_monthly_searches", and is not
// called once its budget is spent. The engine never opens LinkedIn or
// Instagram, and search results pointing there are only links.
package search

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Result is one hit of a search.
type Result struct {
	Title   string
	URL     string
	Snippet string
}

// Provider is one search engine behind an API.
type Provider interface {
	// Name is short and lower case. Its budget setting is
	// "<name>_monthly_searches".
	Name() string
	Search(ctx context.Context, query string) ([]Result, error)
}

// maxResults is how many results one search asks for. Both providers
// allow 20.
const maxResults = 20

var defaultClient = &http.Client{Timeout: 30 * time.Second}

// Tavily is the search API at tavily.com.
type Tavily struct {
	Key string
	// URL is the endpoint. Tests set it.
	URL    string
	Client *http.Client
}

func (t *Tavily) Name() string { return "tavily" }

func (t *Tavily) Search(ctx context.Context, query string) ([]Result, error) {
	body, _ := json.Marshal(map[string]any{
		"query": query, "max_results": maxResults, "search_depth": "basic", "country": "germany",
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, orDefault(t.URL, "https://api.tavily.com/search"), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+t.Key)
	req.Header.Set("Content-Type", "application/json")
	var out struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := call(t.Client, req, t.Name(), &out); err != nil {
		return nil, err
	}
	res := make([]Result, 0, len(out.Results))
	for _, r := range out.Results {
		res = append(res, Result{Title: r.Title, URL: r.URL, Snippet: r.Content})
	}
	return res, nil
}

// Brave is the Brave Search API.
type Brave struct {
	Key string
	// URL is the endpoint. Tests set it.
	URL    string
	Client *http.Client
}

func (b *Brave) Name() string { return "brave" }

func (b *Brave) Search(ctx context.Context, query string) ([]Result, error) {
	q := url.Values{"q": {query}, "count": {fmt.Sprint(maxResults)}, "country": {"DE"}, "search_lang": {"de"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, orDefault(b.URL, "https://api.search.brave.com/res/v1/web/search")+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Subscription-Token", b.Key)
	req.Header.Set("Accept", "application/json")
	var out struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := call(b.Client, req, b.Name(), &out); err != nil {
		return nil, err
	}
	res := make([]Result, 0, len(out.Web.Results))
	for _, r := range out.Web.Results {
		res = append(res, Result{Title: r.Title, URL: r.URL, Snippet: r.Description})
	}
	return res, nil
}

// call sends a request and reads a JSON answer. An answer other than 200
// is an error that quotes the start of what the provider said.
func call(c *http.Client, req *http.Request, name string, out any) error {
	if c == nil {
		c = defaultClient
	}
	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if resp.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(body))
		if len(msg) > 200 {
			msg = msg[:200]
		}
		return fmt.Errorf("%s answered %d: %s", name, resp.StatusCode, msg)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("%s: the answer is not the expected JSON: %w", name, err)
	}
	return nil
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// FromEnv returns the providers whose API keys are set: TAVILY_API_KEY and
// BRAVE_SEARCH_API_KEY.
func FromEnv() []Provider {
	var out []Provider
	if k := strings.TrimSpace(os.Getenv("TAVILY_API_KEY")); k != "" {
		out = append(out, &Tavily{Key: k})
	}
	if k := strings.TrimSpace(os.Getenv("BRAVE_SEARCH_API_KEY")); k != "" {
		out = append(out, &Brave{Key: k})
	}
	return out
}

// ErrNoBudget means no provider has searches left this month, or none is
// set up.
var ErrNoBudget = errors.New("no search provider has searches left this month")
