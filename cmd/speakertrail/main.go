// Command speakertrail finds in-person events across NRW and the people on
// stage at them.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"github.com/timmrkz/speakertrail/internal/config"
	"github.com/timmrkz/speakertrail/internal/db"
	"github.com/timmrkz/speakertrail/internal/extract"
	"github.com/timmrkz/speakertrail/internal/fetch"
	"github.com/timmrkz/speakertrail/internal/importer"
	"github.com/timmrkz/speakertrail/internal/llm"
	"github.com/timmrkz/speakertrail/internal/pipeline"
	"github.com/timmrkz/speakertrail/internal/queue"
	"github.com/timmrkz/speakertrail/internal/server"
	"github.com/timmrkz/speakertrail/internal/settings"
	"github.com/timmrkz/speakertrail/web"
)

// version is set at build time to the commit.
var version = "dev"

const usage = `Usage: speakertrail <command> [flags]

Commands:
  serve           run the web interface, its API and a small worker
  nightly         check every due source once, then exit (the scheduled job)
  worker          work on queued jobs until stopped
  migrate         apply database migrations
  import          load the sources and seeds from the brief
  fetch <url>     show what the engine finds on one page
  people [url]... who is on stage, by the rules and by the local model.
                  Without addresses it takes event pages the runs found
  report          what the engine did lately, as Markdown without names, to share
  hash-password   print the hash for UI_PASSWORD_HASH, reading the password from stdin

Run "speakertrail <command> -h" for the flags of a command.
`

func main() {
	level := slog.LevelInfo
	if os.Getenv("DEBUG") != "" {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		slog.Error("speakertrail failed", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return errors.New("no command given")
	}
	cfg := config.FromEnv()
	if args[0] == "serve" || args[0] == "nightly" || args[0] == "worker" {
		slog.Info("speakertrail starting", "command", args[0], "version", version)
	}
	switch args[0] {
	case "serve":
		return runServe(ctx, cfg, args[1:])
	case "nightly":
		return runNightly(ctx, cfg, args[1:])
	case "worker":
		return runWorker(ctx, cfg, args[1:])
	case "migrate":
		return withDB(ctx, cfg, func(pool *pgxpool.Pool) error {
			if err := db.Migrate(ctx, pool); err != nil {
				return err
			}
			slog.Info("database is up to date")
			return nil
		})
	case "import":
		return withDB(ctx, cfg, func(pool *pgxpool.Pool) error {
			if err := db.Migrate(ctx, pool); err != nil {
				return err
			}
			res, err := importer.Import(ctx, pool)
			if err != nil {
				return err
			}
			slog.Info("import done", "result", res.String())
			return nil
		})
	case "fetch":
		return runFetch(ctx, args[1:])
	case "people":
		return runPeople(ctx, cfg, args[1:])
	case "report":
		return runReport(ctx, cfg)
	case "hash-password":
		return runHashPassword()
	case "help", "-h", "--help":
		fmt.Print(usage)
		return nil
	default:
		fmt.Fprint(os.Stderr, usage)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func withDB(ctx context.Context, cfg config.Config, fn func(*pgxpool.Pool) error) error {
	if err := cfg.Require("DATABASE_URL"); err != nil {
		return err
	}
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	return fn(pool)
}

// engine builds the pipeline with a polite fetcher and, when Chromium is
// there, a headless browser.
func engine(ctx context.Context, pool *pgxpool.Pool) (*pipeline.Pipeline, func(), error) {
	st, err := settings.Load(ctx, pool)
	if err != nil {
		return nil, nil, err
	}
	var browser *fetch.Browser
	if exec := fetch.FindChromium(); exec != "" {
		browser = fetch.NewBrowser(fetch.BrowserOptions{ExecPath: exec, Pages: st.Int("browser_pages", 2)})
		slog.Info("headless browser available", "path", exec)
	} else {
		slog.Warn("no Chromium found, pages that need JavaScript are loaded without it")
	}
	opts := fetch.Options{
		Contact: st.String("user_agent_contact", "https://github.com/timmrkz/speakertrail"),
		Limiter: queue.NewSiteLimiter(pool, time.Duration(st.Int("site_request_interval_seconds", 5))*time.Second, nil),
	}
	if browser != nil {
		opts.Browser = browser
	}
	p := &pipeline.Pipeline{Pool: pool, Fetcher: fetch.New(opts), Queue: queue.New(pool, queue.Options{})}
	// The local model reads event pages for people, where one is set up.
	if model := llm.FromEnvIfSet(); model != nil {
		p.Reader = model
		slog.Info("language model reads event pages", "model", model.Model, "url", model.URL)
	}
	cleanup := func() {
		if browser != nil {
			browser.Close()
		}
	}
	return p, cleanup, nil
}

func newWorker(p *pipeline.Pipeline, concurrency int) *queue.Worker {
	return &queue.Worker{Queue: p.Queue, Handlers: p.Handlers(), Concurrency: concurrency}
}

func runServe(ctx context.Context, cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	work := fs.Bool("work", true, "also work on queued jobs, so Check now and seeds run right away")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := cfg.Require("DATABASE_URL"); err != nil {
		return err
	}
	if cfg.UIPasswordHash == "" || len(cfg.SessionSecret) < 16 {
		slog.Warn("the login is off until UI_PASSWORD_HASH and SESSION_SECRET (16 characters or more) are set")
	}
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		return err
	}
	p, cleanup, err := engine(ctx, pool)
	if err != nil {
		return err
	}
	defer cleanup()
	if *work {
		go func() {
			if err := newWorker(p, 4).Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				slog.Error("background worker stopped", "error", err)
			}
		}()
	}

	ui := web.Dist()
	if ui == nil {
		slog.Warn("the interface is not built into this binary, run npm run build in web/ first")
	}
	h := server.New(server.Options{Pool: pool, Pipeline: p, UI: ui, PasswordHash: cfg.UIPasswordHash, SessionSecret: cfg.SessionSecret})
	srv := &http.Server{Addr: ":" + cfg.Port, Handler: h, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()
	slog.Info("serving", "port", cfg.Port)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func runNightly(ctx context.Context, cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("nightly", flag.ContinueOnError)
	concurrency := fs.Int("concurrency", 4, "jobs running at the same time")
	importFirst := fs.Bool("import", true, "load the starting data first if it is missing")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return withDB(ctx, cfg, func(pool *pgxpool.Pool) error {
		if err := db.Migrate(ctx, pool); err != nil {
			return err
		}
		if *importFirst {
			res, err := importer.Import(ctx, pool)
			if err != nil {
				return err
			}
			if res.SourcesAdded+res.SeedsAdded > 0 {
				slog.Info("starting data imported", "result", res.String())
			}
		}
		p, cleanup, err := engine(ctx, pool)
		if err != nil {
			return err
		}
		defer cleanup()
		return p.Nightly(ctx, newWorker(p, *concurrency))
	})
}

func runWorker(ctx context.Context, cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("worker", flag.ContinueOnError)
	concurrency := fs.Int("concurrency", 4, "jobs running at the same time")
	untilIdle := fs.Bool("until-idle", false, "exit once no job is due")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return withDB(ctx, cfg, func(pool *pgxpool.Pool) error {
		if err := db.Migrate(ctx, pool); err != nil {
			return err
		}
		p, cleanup, err := engine(ctx, pool)
		if err != nil {
			return err
		}
		defer cleanup()
		w := newWorker(p, *concurrency)
		if *untilIdle {
			return w.RunUntilIdle(ctx)
		}
		if err := w.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
		return nil
	})
}

// runFetch loads one page like a source check would and prints what it
// finds. With -save it writes the page to a file for a test fixture.
func runFetch(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("fetch", flag.ContinueOnError)
	browser := fs.Bool("browser", false, "load the page in the headless browser")
	save := fs.String("save", "", "write the page to this file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: speakertrail fetch [-browser] [-save file] <url>")
	}
	opts := fetch.Options{Contact: "https://github.com/timmrkz/speakertrail"}
	if *browser {
		b := fetch.NewBrowser(fetch.BrowserOptions{})
		defer b.Close()
		opts.Browser = b
	}
	f := fetch.New(opts)
	get := f.HTTP
	if *browser {
		get = f.Browser
	}
	page, err := get(ctx, fs.Arg(0))
	if err != nil {
		return err
	}
	if *save != "" {
		if err := os.WriteFile(*save, []byte(page.Body), 0o644); err != nil {
			return err
		}
	}
	res := extract.Extract(extract.Page{URL: page.FinalURL, ContentType: page.ContentType, Body: page.Body})
	fmt.Printf("%s %d, %s, %d bytes, %d events, looks like a JavaScript shell: %v\n",
		page.Mode, page.Status, page.Duration.Round(time.Millisecond), len(page.Body), len(res.Events), fetch.LooksLikeJSShell(page))
	for _, e := range res.Events {
		fmt.Printf("\n%s  %s\n  %s, %s, %s, %s (%s)\n  %s\n", e.Start.In(extract.Berlin).Format("Mon 02 Jan 15:04"), e.Title, e.Venue, e.City, e.Format, e.Type, e.Method, e.URL)
		for _, p := range e.People {
			fmt.Printf("  - %s (%s) %s\n", p.Name, p.Role, p.Affiliation)
		}
	}
	for _, l := range res.Follow {
		fmt.Println("follow:", l)
	}
	for _, l := range res.Links {
		fmt.Println("link:", l)
	}
	return nil
}

func runHashPassword() error {
	fmt.Fprint(os.Stderr, "Password: ")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return err
	}
	pw := strings.TrimRight(line, "\r\n")
	if len(pw) < 12 {
		return errors.New("use at least 12 characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pw), 12)
	if err != nil {
		return err
	}
	fmt.Println(string(hash))
	return nil
}
