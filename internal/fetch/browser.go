package fetch

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// BrowserOptions configure the headless browser.
type BrowserOptions struct {
	// ExecPath is the Chromium binary. Empty looks for it on the PATH.
	ExecPath string
	// Pages is how many pages load at the same time. Default 2.
	Pages int
	// Timeout caps one page load. Default 30 seconds.
	Timeout time.Duration
	// UpstreamProxy sends the browser's traffic on through another proxy.
	// Tests use it to record every host the browser asks for.
	UpstreamProxy *url.URL
}

// Browser loads pages in one shared headless Chromium.
type Browser struct {
	opts   BrowserOptions
	slots  chan struct{}
	once   sync.Once
	ctx    context.Context
	cancel context.CancelFunc
	proxy  *filterProxy
	err    error
}

// FindChromium returns the path of a Chromium or Chrome binary, or "".
func FindChromium() string {
	if p := os.Getenv("CHROME_PATH"); p != "" {
		return p
	}
	for _, name := range []string{"headless-shell", "chromium", "chromium-browser", "google-chrome", "google-chrome-stable"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	for _, p := range []string{"/headless-shell/headless-shell", "/opt/pw-browsers/chromium"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// NewBrowser returns a browser. Chromium starts on the first page load.
func NewBrowser(opts BrowserOptions) *Browser {
	if opts.Pages <= 0 {
		opts.Pages = 2
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 30 * time.Second
	}
	if opts.ExecPath == "" {
		opts.ExecPath = FindChromium()
	}
	return &Browser{opts: opts, slots: make(chan struct{}, opts.Pages)}
}

func (b *Browser) start(userAgent string) error {
	b.once.Do(func() {
		if b.opts.ExecPath == "" {
			b.err = errors.New("no Chromium found, set CHROME_PATH")
			return
		}
		proxy, err := startFilterProxy(b.opts.UpstreamProxy)
		if err != nil {
			b.err = err
			return
		}
		b.proxy = proxy
		flags := append(chromedp.DefaultExecAllocatorOptions[:],
			// All traffic, loopback included, goes through the filter.
			chromedp.ProxyServer(proxy.URL()),
			chromedp.Flag("proxy-bypass-list", "<-loopback>"),
			chromedp.ExecPath(b.opts.ExecPath),
			chromedp.UserAgent(userAgent),
			chromedp.WindowSize(1280, 1600),
			chromedp.Flag("hide-scrollbars", true),
			chromedp.Flag("mute-audio", true),
			chromedp.Flag("disable-dev-shm-usage", true),
		)
		if os.Geteuid() == 0 {
			flags = append(flags, chromedp.NoSandbox)
		}
		allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), flags...)
		ctx, cancelBrowser := chromedp.NewContext(allocCtx)
		if err := chromedp.Run(ctx); err != nil {
			cancelBrowser()
			cancelAlloc()
			proxy.Close()
			b.err = fmt.Errorf("start Chromium: %w", err)
			return
		}
		b.ctx = ctx
		b.cancel = func() { cancelBrowser(); cancelAlloc() }
	})
	return b.err
}

// Close stops Chromium.
func (b *Browser) Close() {
	if b.cancel != nil {
		b.cancel()
	}
	if b.proxy != nil {
		b.proxy.Close()
	}
}

// Load opens rawURL in a new tab, waits until the network is quiet, scrolls
// to load lazy content and returns the rendered page with a screenshot.
func (b *Browser) Load(ctx context.Context, rawURL, userAgent string) (*Page, error) {
	if err := b.start(userAgent); err != nil {
		return nil, err
	}
	select {
	case b.slots <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-b.slots }()

	start := time.Now()
	tab, closeTab := chromedp.NewContext(b.ctx)
	defer closeTab()
	tab, cancel := context.WithTimeout(tab, b.opts.Timeout)
	defer cancel()
	stop := context.AfterFunc(ctx, cancel)
	defer stop()

	// Track requests in flight to know when the network is idle, and the
	// status of the main document.
	var inflight atomic.Int64
	var lastActivity atomic.Int64
	var status atomic.Int64
	lastActivity.Store(time.Now().UnixNano())
	chromedp.ListenTarget(tab, func(ev any) {
		switch e := ev.(type) {
		case *network.EventRequestWillBeSent:
			inflight.Add(1)
			lastActivity.Store(time.Now().UnixNano())
		case *network.EventLoadingFinished, *network.EventLoadingFailed:
			if inflight.Add(-1) < 0 {
				inflight.Store(0)
			}
			lastActivity.Store(time.Now().UnixNano())
		case *network.EventResponseReceived:
			if e.Type == network.ResourceTypeDocument && status.Load() == 0 {
				status.Store(e.Response.Status)
			}
		}
	})
	waitIdle := chromedp.ActionFunc(func(ctx context.Context) error {
		const quiet = 700 * time.Millisecond
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if inflight.Load() <= 0 && time.Since(time.Unix(0, lastActivity.Load())) > quiet {
				return nil
			}
			select {
			case <-time.After(100 * time.Millisecond):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	})

	page := &Page{URL: rawURL, Mode: ModeBrowser, ContentType: "text/html"}
	var finalURL string
	err := chromedp.Run(tab,
		network.Enable(),
		chromedp.Navigate(rawURL),
		waitIdle,
	)
	if err == nil {
		for range 3 {
			if err = chromedp.Run(tab, chromedp.Evaluate(`window.scrollTo(0, document.body ? document.body.scrollHeight : 0)`, nil), waitIdle); err != nil {
				break
			}
		}
	}
	if err == nil {
		err = chromedp.Run(tab,
			chromedp.Evaluate(`window.scrollTo(0, 0)`, nil),
			chromedp.Location(&finalURL),
			chromedp.OuterHTML("html", &page.Body, chromedp.ByQuery),
			chromedp.Evaluate(`document.body ? document.body.innerText : ""`, &page.Text),
		)
	}
	if err == nil {
		// A screenshot failure should not lose the page.
		_ = chromedp.Run(tab, chromedp.FullScreenshot(&page.Screenshot, 60))
	}
	page.Duration = time.Since(start)
	page.FinalURL = finalURL
	page.Status = int(status.Load())
	if err != nil {
		return page, fmt.Errorf("browser %s: %w", rawURL, err)
	}
	if page.Status >= 400 {
		return page, fmt.Errorf("browser %s: status %d", rawURL, page.Status)
	}
	return page, nil
}
