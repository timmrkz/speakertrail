package fetch_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/timmrkz/speakertrail/internal/fetch"
)

// recorder is a transport that notes every host it is asked for and
// answers from a local handler.
type recorder struct {
	mu    sync.Mutex
	hosts []string
	next  http.RoundTripper
}

func (r *recorder) RoundTrip(req *http.Request) (*http.Response, error) {
	r.mu.Lock()
	r.hosts = append(r.hosts, req.URL.Hostname())
	r.mu.Unlock()
	return r.next.RoundTrip(req)
}

func (r *recorder) seen() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.hosts...)
}

func newSite(t *testing.T, robots string, pages map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			if robots == "" {
				http.NotFound(w, r)
				return
			}
			fmt.Fprint(w, robots)
			return
		}
		body, ok := pages[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if strings.HasPrefix(body, "REDIRECT ") {
			http.Redirect(w, r, strings.TrimPrefix(body, "REDIRECT "), http.StatusFound)
			return
		}
		if strings.HasPrefix(body, "LATIN1 ") {
			w.Header().Set("Content-Type", "text/html; charset=iso-8859-1")
			w.Write([]byte(strings.TrimPrefix(body, "LATIN1 ")))
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-Seen-User-Agent", r.UserAgent())
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestNeverRequestsLinkedInOrInstagram(t *testing.T) {
	rec := &recorder{next: http.DefaultTransport}
	site := newSite(t, "", map[string]string{
		"/to-linkedin":  "REDIRECT https://www.linkedin.com/company/example",
		"/to-instagram": "REDIRECT https://instagram.com/example",
	})
	f := fetch.New(fetch.Options{Client: &http.Client{Transport: rec}})
	ctx := t.Context()

	for _, u := range []string{
		"https://www.linkedin.com/in/someone",
		"https://linkedin.com/company/x",
		"https://de.linkedin.com/in/someone",
		"https://lnkd.in/abc",
		"https://www.instagram.com/someone/",
		"https://instagram.com/someone",
		"http://INSTAGRAM.COM./x",
	} {
		if _, err := f.HTTP(ctx, u); !errors.Is(err, fetch.ErrBlockedHost) {
			t.Errorf("HTTP(%s) error %v, want ErrBlockedHost", u, err)
		}
		if _, err := f.Browser(ctx, u); err == nil {
			t.Errorf("Browser(%s) succeeded", u)
		}
	}
	for _, path := range []string{"/to-linkedin", "/to-instagram"} {
		if _, err := f.HTTP(ctx, site.URL+path); !errors.Is(err, fetch.ErrBlockedHost) {
			t.Errorf("redirect %s: error %v, want ErrBlockedHost", path, err)
		}
	}
	for _, h := range rec.seen() {
		if fetch.IsBlockedHost(h) {
			t.Errorf("a request reached %s", h)
		}
	}
	if fetch.IsBlockedHost("notlinkedin.com") || fetch.IsBlockedHost("linkedin.com.example.org") {
		t.Error("lookalike hosts are blocked too")
	}
}

func TestHTTPFetch(t *testing.T) {
	site := newSite(t, "User-agent: *\nDisallow: /private\n\nUser-agent: SpeakerTrailBot\nDisallow: /nobots\nAllow: /\n", map[string]string{
		"/events":  `<html><head><title>x</title><script>var a=1</script></head><body><h1>Events</h1><p>Pitch Night <b>Köln</b></p><div style="display:none">hidden</div></body></html>`,
		"/nobots":  "no",
		"/private": "private",
		"/umlaut":  "LATIN1 <p>Gr\xfcnder Abend in D\xfcsseldorf</p>",
	})
	f := fetch.New(fetch.Options{Contact: "https://example.org/bot"})
	ctx := t.Context()

	p, err := f.HTTP(ctx, site.URL+"/events")
	if err != nil {
		t.Fatal(err)
	}
	if p.Mode != fetch.ModeHTTP || p.Status != 200 {
		t.Errorf("mode %s status %d", p.Mode, p.Status)
	}
	if p.Text != "Events\nPitch Night Köln" {
		t.Errorf("visible text %q", p.Text)
	}
	if ua := f.UserAgent(); !strings.HasPrefix(ua, "SpeakerTrailBot/") || !strings.Contains(ua, "https://example.org/bot") {
		t.Errorf("user agent %q does not name the bot and a contact", ua)
	}

	// The bot's own group applies, not the * group.
	if _, err := f.HTTP(ctx, site.URL+"/nobots"); !errors.Is(err, fetch.ErrRobots) {
		t.Errorf("/nobots: error %v, want ErrRobots", err)
	}
	if _, err := f.HTTP(ctx, site.URL+"/private"); err != nil {
		t.Errorf("/private is only closed for other bots: %v", err)
	}

	p, err = f.HTTP(ctx, site.URL+"/umlaut")
	if err != nil {
		t.Fatal(err)
	}
	if p.Text != "Gründer Abend in Düsseldorf" {
		t.Errorf("latin-1 page decoded as %q", p.Text)
	}
}

func TestUnreachableRobotsMeansNoCrawl(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		fmt.Fprint(w, "<p>page</p>")
	}))
	defer srv.Close()
	f := fetch.New(fetch.Options{})
	if _, err := f.HTTP(t.Context(), srv.URL+"/events"); !errors.Is(err, fetch.ErrRobots) {
		t.Errorf("error %v, want ErrRobots when robots.txt answers 503", err)
	}
}

type countingLimiter struct {
	mu    sync.Mutex
	calls []string
}

func (l *countingLimiter) Wait(ctx context.Context, u string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, u)
	return nil
}

func TestEveryRequestWaitsForItsSlot(t *testing.T) {
	site := newSite(t, "User-agent: *\nAllow: /\n", map[string]string{"/a": "<p>a</p>", "/b": "<p>b</p>"})
	lim := &countingLimiter{}
	f := fetch.New(fetch.Options{Limiter: lim})
	for _, p := range []string{"/a", "/b"} {
		if _, err := f.HTTP(t.Context(), site.URL+p); err != nil {
			t.Fatal(err)
		}
	}
	// robots.txt once, then each page.
	want := []string{site.URL + "/robots.txt", site.URL + "/a", site.URL + "/b"}
	if strings.Join(lim.calls, " ") != strings.Join(want, " ") {
		t.Errorf("limiter calls %v, want %v", lim.calls, want)
	}
}

const plainPage = `<html><body><h1>Events at Startplatz</h1>
<ul><li>Mon 28 Sep 18:00, Rheinland Pitch #129, four founders pitch to the audience and a jury</li>
<li>Thu 1 Oct 09:30, START-UP Breakfast Düsseldorf, open networking breakfast for founders</li>
<li>Thu 15 Oct 08:30, Startup Breakfast Köln, three founders tell their story at Stadtgarten</li></ul>
<p>Startplatz is the startup hub in Cologne and Düsseldorf with coworking, an accelerator and events for founders all year.</p>
<p>All events are free unless stated otherwise. Register on the event page to get a reminder the day before.</p>
</body></html>`

const jsShell = `<html><head><script src="/app.js"></script></head><body><div id="root"></div>
<noscript>You need to enable JavaScript to run this app.</noscript>
<script>
document.getElementById('root').innerHTML = '<h1>Offline Club Cologne</h1><p>Thu 1 Oct 19:00 Phone-free evening at Ehrenfeld</p>';
</script></body></html>`

func TestJSShellDetection(t *testing.T) {
	site := newSite(t, "", map[string]string{"/plain": plainPage, "/shell": jsShell, "/app.js": "// app"})
	f := fetch.New(fetch.Options{})
	plain, err := f.HTTP(t.Context(), site.URL+"/plain")
	if err != nil {
		t.Fatal(err)
	}
	if fetch.LooksLikeJSShell(plain) {
		t.Error("a plain page was taken for a JavaScript shell")
	}
	shell, err := f.HTTP(t.Context(), site.URL+"/shell")
	if err != nil {
		t.Fatal(err)
	}
	if !fetch.LooksLikeJSShell(shell) {
		t.Errorf("a JavaScript shell was not detected, text %q", shell.Text)
	}
}

func chromium(t *testing.T) string {
	t.Helper()
	p := fetch.FindChromium()
	if p == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("no Chromium found in CI")
		}
		t.Skip("no Chromium found, set CHROME_PATH")
	}
	return p
}

// browserBudget covers what the browser allows itself: up to 60 seconds for
// Chromium to start cold, then 30 seconds for the page. A busy build runner
// can take most of the first.
const browserBudget = 2 * time.Minute

func TestBrowserRendersJavaScriptPage(t *testing.T) {
	exec := chromium(t)
	site := newSite(t, "", map[string]string{"/shell": jsShell, "/app.js": "// app"})
	b := fetch.NewBrowser(fetch.BrowserOptions{ExecPath: exec})
	defer b.Close()
	f := fetch.New(fetch.Options{Browser: b})

	ctx, cancel := context.WithTimeout(t.Context(), browserBudget)
	defer cancel()
	p, err := f.Browser(ctx, site.URL+"/shell")
	if err != nil {
		t.Fatal(err)
	}
	if p.Mode != fetch.ModeBrowser || p.Status != 200 {
		t.Errorf("mode %s status %d", p.Mode, p.Status)
	}
	if !strings.Contains(p.Text, "Phone-free evening at Ehrenfeld") {
		t.Errorf("rendered text %q misses the script's content", p.Text)
	}
	if !strings.Contains(p.Body, "Offline Club Cologne") {
		t.Error("rendered HTML misses the script's content")
	}
	if len(p.Screenshot) == 0 {
		t.Error("no screenshot")
	}
}

// TestBrowserNeverReachesBlockedHosts loads a page that embeds LinkedIn and
// Instagram through a proxy that records every host Chromium asks for.
func TestBrowserNeverReachesBlockedHosts(t *testing.T) {
	exec := chromium(t)
	page := `<html><body><h1>Meetup</h1>
<img src="https://www.linkedin.com/px.png">
<script src="https://platform.linkedin.com/in.js"></script>
<iframe src="https://www.instagram.com/p/abc/embed"></iframe>
<img src="https://allowed.example.org/logo.png">
<script>fetch('https://www.instagram.com/api/x').catch(()=>{}); fetch('https://media.licdn.com/y').catch(()=>{})</script>
</body></html>`
	site := newSite(t, "", map[string]string{"/page": page})
	siteURL, _ := url.Parse(site.URL)

	var mu sync.Mutex
	var hosts []string
	note := func(h string) {
		mu.Lock()
		hosts = append(hosts, h)
		mu.Unlock()
	}
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodConnect {
			host, _, _ := net.SplitHostPort(r.Host)
			note(host)
			http.Error(w, "no outside access in tests", http.StatusForbidden)
			return
		}
		note(r.URL.Hostname())
		if r.URL.Host != siteURL.Host {
			http.Error(w, "no outside access in tests", http.StatusForbidden)
			return
		}
		resp, err := http.DefaultTransport.RoundTrip(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		for k, v := range resp.Header {
			w.Header()[k] = v
		}
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	}))
	defer proxy.Close()

	upstream, _ := url.Parse(proxy.URL)
	b := fetch.NewBrowser(fetch.BrowserOptions{ExecPath: exec, UpstreamProxy: upstream})
	defer b.Close()
	f := fetch.New(fetch.Options{Browser: b})
	ctx, cancel := context.WithTimeout(t.Context(), browserBudget)
	defer cancel()
	if _, err := f.Browser(ctx, site.URL+"/page"); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	sawAllowed := false
	for _, h := range hosts {
		if fetch.IsBlockedHost(h) {
			t.Errorf("Chromium asked for %s", h)
		}
		if h == "allowed.example.org" {
			sawAllowed = true
		}
	}
	if !sawAllowed {
		t.Errorf("the proxy saw no outside request, so the test proves nothing: %v", hosts)
	}
}
