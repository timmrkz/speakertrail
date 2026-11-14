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
	"encoding/json"
	"fmt"
	"github.com/timmrkz/speakertrail/internal/browser"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	apipkg "github.com/timmrkz/speakertrail/internal/api"
	"github.com/timmrkz/speakertrail/internal/engine"
	"github.com/timmrkz/speakertrail/internal/keys"
	"github.com/timmrkz/speakertrail/internal/search"
	"github.com/timmrkz/speakertrail/internal/store"
	"github.com/timmrkz/speakertrail/web"
)

func main() {
	// Started again to put a new build in place once the app has quit,
	// see install.go.
	installIfAsked()
	go tidyTemp(os.TempDir(), time.Now())
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))
	// make carry-over hands over the people connected in the Docker version,
	// see scripts/carry-over.sh. It writes the app's file and opens no
	// window.
	if len(os.Args) == 2 && os.Args[1] == "carry-over" {
		if err := carryOver(os.Stdin); err != nil {
			slog.Error("carrying over failed", "error", err)
			os.Exit(1)
		}
		return
	}
	slog.Info("speakertrail starting", "version", runningVersion(), "channel", buildChannel, "commit", buildCommit)

	answers, e, dir, err := open()
	if err != nil {
		slog.Error("speakertrail cannot start", "error", err)
		os.Exit(1)
	}
	// The updates are made once the app is, since Wails' updater is the
	// app's. The page asks for them through the same handler.
	var ups atomic.Pointer[updating]
	var app *application.App
	api := http.NewServeMux()
	api.Handle("/api/updates", updatesAPI{get: ups.Load, open: func(url string) error { return app.Browser.OpenURL(url) }})
	api.Handle("/api/updates/", updatesAPI{get: ups.Load, open: func(url string) error { return app.Browser.OpenURL(url) }})
	api.Handle("/", answers)
	app = application.New(application.Options{
		Name:        "Speaker Trail",
		Description: "Finds people in NRW worth a conversation",
		Assets: application.AssetOptions{
			Handler: handler(web.Dist(), api),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
		// A run that goes when the app closes is stopped, and what it did
		// not get to waits for the next run. A build that is ready goes in
		// place once the app is gone, see installOnQuit.
		OnShutdown: func() {
			if id, _ := e.Going(); id != 0 {
				e.Stop(id)
			}
			if c := ups.Load(); c != nil {
				c.installOnQuit()
			}
		},
		Logger: slog.Default(),
	})
	// Where updates stand is sent to the page as it changes, as an event
	// called updates, the way FrameFairy sends it. The page asks
	// /api/updates once when it starts and after a click, and hears the
	// rest. See web/src/lib/updates.svelte.ts.
	c := newUpdating(app.Updater, app.Quit, dir, e.CloseIfIdle, func(u UpdateState) {
		app.Event.Emit("updates", u)
	})
	ups.Store(c)
	c.start()
	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     "Speaker Trail",
		Width:     1100,
		Height:    820,
		MinWidth:  390,
		MinHeight: 560,
		URL:       "/app/people",
		Mac: application.MacWindow{
			Backdrop: application.MacBackdropNormal,
			// The page reaches up under the title bar, the way Mail and
			// FrameFairy look: the title bar is the page's own, --bar-h
			// tall in web/src/app.css, with the close, minimise and zoom
			// buttons on it and the screen's name beside them.
			// Hidden, not HiddenInset: a toolbar makes macOS push the
			// buttons further in than Terminal's, which FrameFairy measured
			// and found reads as wrong. A click in the top 28 points still
			// drags the window, and so does a click on any pane's bar.
			TitleBar:                application.MacTitleBarHidden,
			InvisibleTitleBarHeight: 28,
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
// ends a run the app closed under, and builds what answers the window. It
// also gives the engine and the app's folder.
func open() (http.Handler, *engine.Engine, string, error) {
	dir, err := Folder()
	if err != nil {
		return nil, nil, "", err
	}
	st, err := store.Open(dir, nil)
	if err != nil {
		return nil, nil, "", err
	}
	slog.Info("the app's file", "path", st.Path())
	if err := st.Update(func(d *store.Data) error { return d.Start(st.Now()) }); err != nil {
		return nil, nil, "", err
	}
	exa := keys.NewExa()
	e := &engine.Engine{Store: st, Finder: func() engine.Finder {
		if key := exa.Get(); key != "" {
			return &search.Exa{Key: key}
		}
		return nil
	}}
	if err := e.Recover(); err != nil {
		return nil, nil, "", err
	}
	a := apipkg.New(st, e, exa, slog.Default())
	a.Browsers = apipkg.Mac{Support: browser.Support()}
	return a, e, dir, nil
}

// carryOver adds the people connected in the Docker version, read as
// JSON.
func carryOver(r io.Reader) error {
	var people []store.Carried
	if err := json.NewDecoder(r).Decode(&people); err != nil {
		return fmt.Errorf("the people from the old database cannot be read: %w", err)
	}
	dir, err := Folder()
	if err != nil {
		return err
	}
	st, err := store.Open(dir, nil)
	if err != nil {
		return err
	}
	var added, there int
	if err := st.Update(func(d *store.Data) error {
		if err := d.Start(st.Now()); err != nil {
			return err
		}
		added, there = d.CarryOver(people, st.Now())
		return nil
	}); err != nil {
		return err
	}
	fmt.Printf("Carried over %d connected people, %d were here already. They are in %s\n", added, there, st.Path())
	return nil
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
