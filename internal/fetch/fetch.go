// Package fetch loads pages politely: plain HTTP first, a headless browser
// where a page needs JavaScript. It identifies the bot, respects robots.txt,
// keeps each website at one request per interval and never requests
// LinkedIn or Instagram.
package fetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/html/charset"
)

// Mode is how a page was loaded.
type Mode string

const (
	ModeHTTP    Mode = "http"
	ModeBrowser Mode = "browser"
)

// Page is one loaded page.
type Page struct {
	URL         string
	FinalURL    string
	Mode        Mode
	Status      int
	ContentType string
	// Body is the page as UTF-8: HTML, iCal or JSON.
	Body       string
	Text       string
	Screenshot []byte
	Duration   time.Duration
}

// IsHTML reports whether the page is an HTML document.
func (p *Page) IsHTML() bool {
	return p.ContentType == "" || strings.Contains(p.ContentType, "html")
}

// Limiter spaces requests to the same website.
type Limiter interface {
	Wait(ctx context.Context, rawURL string) error
}

// Options configure a Fetcher.
type Options struct {
	// Contact is a URL where site owners learn about the bot.
	Contact string
	Limiter Limiter
	// Client defaults to a client with a 30 second timeout.
	Client *http.Client
	// Browser loads pages that need JavaScript. Nil disables it.
	Browser *Browser
	// MaxBytes caps the size of a page. Default 5 MB.
	MaxBytes int64
}

// Fetcher loads pages.
type Fetcher struct {
	opts      Options
	userAgent string
	robots    *Robots
}

// BotName is the product token in the User-Agent and in robots.txt.
const BotName = "SpeakerTrailBot"

// New returns a Fetcher.
func New(opts Options) *Fetcher {
	if opts.Client == nil {
		opts.Client = &http.Client{Timeout: 30 * time.Second}
	}
	if opts.MaxBytes <= 0 {
		opts.MaxBytes = 5 << 20
	}
	ua := BotName + "/0.1"
	if opts.Contact != "" {
		ua += " (+" + opts.Contact + ")"
	}
	f := &Fetcher{opts: opts, userAgent: ua}
	client := *opts.Client
	client.CheckRedirect = checkRedirect
	f.opts.Client = &client
	f.robots = NewRobots(f.limitedGet, BotName)
	return f
}

// UserAgent is the header the bot sends.
func (f *Fetcher) UserAgent() string { return f.userAgent }

// HasBrowser reports whether pages can be loaded with JavaScript.
func (f *Fetcher) HasBrowser() bool { return f.opts.Browser != nil }

func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("too many redirects")
	}
	return CheckAllowed(req.URL.String())
}

// limitedGet is a GET for robots.txt that honours the site limit.
func (f *Fetcher) limitedGet(ctx context.Context, rawURL string) (int, []byte, error) {
	if err := f.wait(ctx, rawURL); err != nil {
		return 0, nil, err
	}
	return httpGetter(f.opts.Client, f.userAgent)(ctx, rawURL)
}

func (f *Fetcher) wait(ctx context.Context, rawURL string) error {
	if f.opts.Limiter == nil {
		return nil
	}
	return f.opts.Limiter.Wait(ctx, rawURL)
}

// allowed runs every check a URL must pass before it is requested.
func (f *Fetcher) allowed(ctx context.Context, rawURL string) error {
	if err := CheckAllowed(rawURL); err != nil {
		return err
	}
	ok, err := f.robots.Allowed(ctx, rawURL)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%s: %w", rawURL, ErrRobots)
	}
	return nil
}

// HTTP loads a page with a plain GET request.
func (f *Fetcher) HTTP(ctx context.Context, rawURL string) (*Page, error) {
	if err := f.allowed(ctx, rawURL); err != nil {
		return nil, err
	}
	if err := f.wait(ctx, rawURL); err != nil {
		return nil, err
	}
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", f.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/calendar,application/json;q=0.9,*/*;q=0.5")
	req.Header.Set("Accept-Language", "de-DE,de;q=0.9,en;q=0.8")
	resp, err := f.opts.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", rawURL, err)
	}
	defer resp.Body.Close()

	page := &Page{URL: rawURL, FinalURL: resp.Request.URL.String(), Mode: ModeHTTP, Status: resp.StatusCode}
	page.ContentType, _, _ = mime.ParseMediaType(resp.Header.Get("Content-Type"))
	reader, err := charset.NewReader(io.LimitReader(resp.Body, f.opts.MaxBytes), resp.Header.Get("Content-Type"))
	if err != nil {
		reader = io.LimitReader(resp.Body, f.opts.MaxBytes)
	}
	body, err := io.ReadAll(reader)
	page.Duration = time.Since(start)
	if err != nil {
		return page, fmt.Errorf("read %s: %w", rawURL, err)
	}
	page.Body = string(body)
	if page.IsHTML() {
		page.Text = VisibleText(page.Body)
	} else {
		page.Text = page.Body
	}
	if resp.StatusCode >= 400 {
		return page, fmt.Errorf("get %s: status %d", rawURL, resp.StatusCode)
	}
	return page, nil
}

// Browser loads a page in the headless browser.
func (f *Fetcher) Browser(ctx context.Context, rawURL string) (*Page, error) {
	if f.opts.Browser == nil {
		return nil, errors.New("no headless browser configured")
	}
	if err := f.allowed(ctx, rawURL); err != nil {
		return nil, err
	}
	if err := f.wait(ctx, rawURL); err != nil {
		return nil, err
	}
	return f.opts.Browser.Load(ctx, rawURL, f.userAgent)
}

// LooksLikeJSShell reports whether an HTML page seems to be an empty shell
// that only fills itself with JavaScript.
func LooksLikeJSShell(p *Page) bool {
	if p == nil || !p.IsHTML() {
		return false
	}
	text := strings.TrimSpace(p.Text)
	lower := strings.ToLower(p.Body)
	scripts := strings.Count(lower, "<script")
	switch {
	case len(text) < 300:
		return true
	case len(text) < 1500 && strings.Contains(lower, "<noscript") &&
		(strings.Contains(lower, "enable javascript") || strings.Contains(lower, "javascript aktivieren") ||
			strings.Contains(lower, "javascript to run") || strings.Contains(lower, "requires javascript")):
		return true
	case len(text) < 800 && scripts >= 5:
		return true
	}
	return false
}
