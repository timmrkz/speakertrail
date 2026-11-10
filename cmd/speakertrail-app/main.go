// Command speakertrail-app is Speaker Trail as a desktop app: one window
// with the workspace in it, and everything it knows in plain files in the
// user's own folder. No server, no database, no Docker.
//
// The window is a webview run by Wails. It shows the interface built from
// web/ and asks for its data at /api/, which never leaves the process: the
// requests are answered by handler below, in Go, the same way the old web
// server answered them.
package main

import (
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"

	apipkg "github.com/timmrkz/speakertrail/internal/api"
	"github.com/timmrkz/speakertrail/internal/engine"
	"github.com/timmrkz/speakertrail/internal/keys"
	"github.com/timmrkz/speakertrail/internal/search"
	"github.com/timmrkz/speakertrail/internal/store"
	"github.com/timmrkz/speakertrail/web"
)

// version is set at build time to the commit.
var version = "dev"

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))
	slog.Info("speakertrail starting", "version", version)

	answers, closeDown, err := open()
	if err != nil {
		slog.Error("speakertrail cannot start", "error", err)
		os.Exit(1)
	}
	app := application.New(application.Options{
		Name:        "Speaker Trail",
		Description: "Finds people in NRW worth a conversation",
		Assets: application.AssetOptions{
			Handler: handler(web.Dist(), answers),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
		// A run that goes when the app closes is stopped, and what it did
		// not get to waits for the next run.
		OnShutdown: closeDown,
		Logger:     slog.Default(),
	})
	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     "Speaker Trail",
		Width:     1100,
		Height:    820,
		MinWidth:  390,
		MinHeight: 560,
		URL:       "/app/people",
		Mac: application.MacWindow{
			Backdrop: application.MacBackdropNormal,
		},
	})
	if err := app.Run(); err != nil {
		slog.Error("speakertrail failed", "error", err)
		os.Exit(1)
	}
}

// Folder is where the app keeps its file: ~/Library/Application
// Support/Speaker Trail on a Mac. SPEAKERTRAIL_DIR names another, for a
// try that should not touch the real one.
func Folder() (string, error) {
	if dir := os.Getenv("SPEAKERTRAIL_DIR"); dir != "" {
		return dir, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "Speaker Trail"), nil
}

// open reads the app's file, adds the starting searches the first time,
// ends a run the app closed under, and builds what answers the window.
func open() (http.Handler, func(), error) {
	dir, err := Folder()
	if err != nil {
		return nil, nil, err
	}
	st, err := store.Open(dir, nil)
	if err != nil {
		return nil, nil, err
	}
	slog.Info("the app's file", "path", st.Path())
	if err := st.Update(func(d *store.Data) error { return d.Start(st.Now()) }); err != nil {
		return nil, nil, err
	}
	exa := keys.NewExa()
	e := &engine.Engine{Store: st, Finder: func() engine.Finder {
		if key := exa.Get(); key != "" {
			return &search.Exa{Key: key}
		}
		return nil
	}}
	if err := e.Recover(); err != nil {
		return nil, nil, err
	}
	closeDown := func() {
		if id, _ := e.Going(); id != 0 {
			e.Stop(id)
		}
	}
	return apipkg.New(st, e, exa, slog.Default()), closeDown, nil
}

// handler answers everything the window asks for: /api/ goes to api, the
// rest is the built interface. A path that names no file is a screen of
// the interface, like /app/people, so it gets index.html and the
// interface's own router takes over.
func handler(ui fs.FS, api http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
			if api == nil {
				http.Error(w, `{"error":"Not ready yet"}`, http.StatusServiceUnavailable)
				return
			}
			api.ServeHTTP(w, r)
			return
		}
		if ui == nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.WriteString(w, notBuiltPage)
			return
		}
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name != "" {
			if st, err := fs.Stat(ui, name); err == nil && !st.IsDir() {
				http.ServeFileFS(w, r, ui, name)
				return
			}
			if path.Ext(name) != "" {
				http.NotFound(w, r)
				return
			}
		}
		http.ServeFileFS(w, r, ui, "index.html")
	})
}

const notBuiltPage = `<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8"><title>Speaker Trail</title>
<style>body{font-family:-apple-system,system-ui,sans-serif;background:#16171a;color:#ebe8e3;
margin:0;display:grid;place-items:center;height:100vh}main{max-width:32rem;padding:2rem}
code{background:#262930;padding:2px 6px;border-radius:4px}</style></head>
<body><main><h1>The interface is not built</h1>
<p>This program was built without its interface. Build it with <code>make</code> from the
top of the repository, which builds the interface first.</p>
</main></body></html>`
