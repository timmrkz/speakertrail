// Command speakertrail finds in-person events across NRW and the people on
// stage at them.
//
//	speakertrail migrate   apply database migrations
//	speakertrail serve     run the web interface and its API
//	speakertrail worker    run the job queue
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/timmrkz/speakertrail/internal/config"
	"github.com/timmrkz/speakertrail/internal/db"
	"github.com/timmrkz/speakertrail/internal/queue"
)

const usage = `Usage: speakertrail <command> [flags]

Commands:
  migrate   apply database migrations
  serve     run the web interface and its API
  worker    run the job queue

Run "speakertrail <command> -h" for the flags of a command.
`

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
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
	switch args[0] {
	case "migrate":
		return runMigrate(ctx, cfg, args[1:])
	case "serve":
		return runServe(ctx, cfg, args[1:])
	case "worker":
		return runWorker(ctx, cfg, args[1:])
	case "help", "-h", "--help":
		fmt.Print(usage)
		return nil
	default:
		fmt.Fprint(os.Stderr, usage)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runMigrate(ctx context.Context, cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := cfg.Require("DATABASE_URL"); err != nil {
		return err
	}
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	return db.Migrate(ctx, pool)
}

func runServe(ctx context.Context, cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := cfg.Require("DATABASE_URL"); err != nil {
		return err
	}
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := pool.Ping(r.Context()); err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		fmt.Fprintln(w, "ok")
	})
	srv := &http.Server{Addr: ":" + cfg.Port, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
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

func runWorker(ctx context.Context, cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("worker", flag.ContinueOnError)
	untilIdle := fs.Bool("until-idle", false, "exit once no job is due, for scheduled runs")
	concurrency := fs.Int("concurrency", 4, "jobs running at the same time")
	dryRun := fs.Bool("dry-run", false, "never call a paid API")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := cfg.Require("DATABASE_URL"); err != nil {
		return err
	}
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	w := &queue.Worker{
		Queue: queue.New(pool, queue.Options{}),
		// Handlers for each job kind arrive with the batches that build them.
		Handlers:    map[string]queue.Handler{},
		Concurrency: *concurrency,
	}
	slog.Info("worker started", "concurrency", *concurrency, "until_idle", *untilIdle, "dry_run", *dryRun)
	if *untilIdle {
		return w.RunUntilIdle(ctx)
	}
	if err := w.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}
