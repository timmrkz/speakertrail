package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/timmrkz/speakertrail/internal/extract"
	"github.com/timmrkz/speakertrail/internal/fetch"
	"github.com/timmrkz/speakertrail/internal/llm"
)

// runPeople is the experiment: for each page it prints who is on stage,
// once by the engine's rules and once by the local model. It stores
// nothing.
func runPeople(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("people", flag.ContinueOnError)
	file := fs.Bool("file", false, "read saved pages from files instead of loading addresses")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return errors.New("usage: speakertrail people [-file] <url or file>...")
	}
	opts := fetch.Options{Contact: "https://github.com/timmrkz/speakertrail"}
	if !*file && fetch.FindChromium() != "" {
		b := fetch.NewBrowser(fetch.BrowserOptions{})
		defer b.Close()
		opts.Browser = b
	}
	f := fetch.New(opts)
	model := llm.FromEnv()
	fmt.Printf("Model %s at %s\n", model.Model, model.URL)
	for _, arg := range fs.Args() {
		if err := peopleOn(ctx, f, model, arg, *file); err != nil {
			if errors.Is(err, llm.ErrUnreachable) || ctx.Err() != nil {
				return err
			}
			fmt.Printf("\n%s\n  %v\n", arg, err)
		}
	}
	return nil
}

func peopleOn(ctx context.Context, f *fetch.Fetcher, model *llm.Client, arg string, fromFile bool) error {
	page, err := loadPage(ctx, f, arg, fromFile)
	if err != nil {
		return err
	}
	res := extract.Extract(extract.Page{URL: page.FinalURL, ContentType: page.ContentType, Body: page.Body})
	title := pageTitle(page.Body)
	if len(res.Events) > 0 {
		title = res.Events[0].Title
	}
	if title == "" {
		title = arg
	}
	where := arg
	if fromFile {
		where = filepath.Base(arg)
	}
	fmt.Printf("\n%s\n%s, %s, %d characters of text\n", title, where, page.Mode, len(page.Text))

	fmt.Println("\n  By the engine's rules:")
	seen := map[string]bool{}
	for _, e := range res.Events {
		for _, p := range e.People {
			if n := extract.NormaliseName(p.Name); !seen[n] {
				seen[n] = true
				fmt.Printf("    %s\n", describe(p.Name, p.Role, p.Affiliation))
			}
		}
	}
	if len(seen) == 0 {
		fmt.Println("    nobody")
	}

	found, err := model.People(ctx, title, page.Text)
	if err != nil {
		return err
	}
	parts := ""
	if found.Parts > 1 {
		parts = fmt.Sprintf(", in %d parts", found.Parts)
	}
	fmt.Printf("\n  By the model, %s%s:\n", found.Took.Round(100*time.Millisecond), parts)
	for _, p := range found.People {
		fmt.Printf("    %s\n", describe(p.Name, p.Role, p.Affiliation))
		if ev := strings.Join(strings.Fields(p.Evidence), " "); ev != "" {
			fmt.Printf("      \"%s\"\n", ev)
		}
	}
	if len(found.People) == 0 {
		fmt.Println("    nobody")
	}
	if len(found.Dropped) > 0 {
		fmt.Println("\n  Left out, the page does not name them like this:")
		for _, p := range found.Dropped {
			fmt.Printf("    %s\n", describe(p.Name, p.Role, p.Affiliation))
		}
	}
	if found.Unread > 0 {
		fmt.Printf("\n  The last %d characters of this long page were not read.\n", found.Unread)
	}
	return nil
}

var titleTag = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)

// pageTitle is the page's own title, for pages without event data.
func pageTitle(body string) string {
	if m := titleTag.FindStringSubmatch(body); m != nil {
		return strings.Join(strings.Fields(html.UnescapeString(m[1])), " ")
	}
	return ""
}

func describe(name, role, affiliation string) string {
	s := name + ", " + role
	if affiliation != "" {
		s += ", " + affiliation
	}
	return s
}

// loadPage loads an address politely, in the browser when the page is an
// empty JavaScript shell, or reads a saved page.
func loadPage(ctx context.Context, f *fetch.Fetcher, arg string, fromFile bool) (*fetch.Page, error) {
	if fromFile {
		body, err := os.ReadFile(arg)
		if err != nil {
			return nil, err
		}
		p := &fetch.Page{URL: arg, FinalURL: "file://" + filepath.Base(arg), Mode: "file", ContentType: "text/html", Body: string(body)}
		p.Text = fetch.VisibleText(p.Body)
		return p, nil
	}
	if !strings.HasPrefix(arg, "http://") && !strings.HasPrefix(arg, "https://") {
		return nil, errors.New("give a full address starting with https://")
	}
	page, err := f.HTTP(ctx, arg)
	if err != nil {
		return nil, err
	}
	if page.IsHTML() && fetch.LooksLikeJSShell(page) && f.HasBrowser() {
		return f.Browser(ctx, arg)
	}
	return page, nil
}
