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
	"errors"
	"fmt"
	"image"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"github.com/spf13/afero"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/updater"

	apipkg "github.com/timmrkz/speakertrail/internal/api"
	"github.com/timmrkz/speakertrail/internal/browser"
	"github.com/timmrkz/speakertrail/internal/draft"
	"github.com/timmrkz/speakertrail/internal/engine"
	"github.com/timmrkz/speakertrail/internal/keys"
	"github.com/timmrkz/speakertrail/internal/language"
	"github.com/timmrkz/speakertrail/internal/search"
	"github.com/timmrkz/speakertrail/internal/speech"
	"github.com/timmrkz/speakertrail/internal/speech/asr"
	"github.com/timmrkz/speakertrail/internal/store"
	"github.com/timmrkz/speakertrail/web"
)

func main() {
	// Started again to put a new build in place once the app has quit,
	// see install.go.
	installIfAsked()
	// Started again by Relaunch, as Wails' updater: this program waits for
	// the app to quit, puts the new build in place, opens it and ends.
	// application.New does it too, but only after everything above it
	// here, and a helper that got that far had begun to load the language
	// model, ended inside application.New and left llama-server running
	// with 16 GB, once for every Relaunch. So it is done first, before the
	// app starts anything. FrameFairy loads its model only after
	// application.New and has no need of it.
	updater.HandleHelperMode()
	go tidyTemp(os.TempDir(), time.Now())
	// make carry-over hands over the people connected in the Docker version,
	// see scripts/carry-over.sh. It writes the app's file, opens no window
	// and says what went wrong in the terminal it was started in.
	if len(os.Args) == 2 && os.Args[1] == "carry-over" {
		if err := carryOver(os.Stdin); err != nil {
			fmt.Fprintln(os.Stderr, "carrying over failed:", err)
			os.Exit(1)
		}
		return
	}
	theLog.started()

	answers, e, dir, err := open()
	if err != nil {
		theLog.line(zerolog.ErrorLevel, "app").Err(err).Msg("the app cannot start")
		os.Exit(1)
	}
	// The updates are made once the app is, since Wails' updater is the
	// app's. The page asks for them through the same handler.
	var ups atomic.Pointer[updating]
	var app *application.App
	api := http.NewServeMux()
	api.Handle("/api/updates", updatesAPI{get: ups.Load, open: func(url string) error { return app.Browser.OpenURL(url) }})
	api.Handle("/api/updates/", updatesAPI{get: ups.Load, open: func(url string) error { return app.Browser.OpenURL(url) }})
	// Where macOS put the title bar and its buttons, once the window is
	// made, see chrome.go.
	var chrome, reportChrome atomic.Pointer[chromeWatch]
	api.Handle("/api/chrome", chromeAPI{get: chrome.Load, report: reportChrome.Load})
	// The window's lines and acts, and the log as Settings shows it, see
	// applog_api.go.
	api.Handle("/api/log", logAPI{log: theLog, show: showInFolder})
	api.Handle("/api/log/", logAPI{log: theLog, show: showInFolder})
	// The speech models in Settings, fetched and removed there, see
	// speechmodels.go. Where a download stands is the event speech.
	voices := newSpeechModels(speech.Dir(dir), func(st SpeechState) {
		if app != nil {
			app.Event.Emit("speech", st)
		}
	})
	api.Handle("/api/speech", speechAPI{s: voices})
	api.Handle("/api/speech/", speechAPI{s: voices})
	// The language model that drafts first messages, fetched and removed
	// in Settings, see languagemodels.go, and kept loaded between drafts.
	// Where a download stands is the event language, where a draft
	// stands the event draft.
	// The model is loaded as the app starts and kept until it quits, so
	// every draft writes at once, Tim's choice on 21 November 2026.
	writer := &language.Host{Log: theLog.slog("language", zerolog.TraceLevel), Notes: language.NotesDir(dir), Always: true}
	writers := newLanguageModels(language.Dir(dir), writer, func(st LanguageState) {
		if app != nil {
			app.Event.Emit("language", st)
		}
	})
	// Gemma is the file FrameFairy uses too, so a Mac that has it in
	// FrameFairy's folder takes it from there rather than downloading it.
	writers.in.Elsewhere = []string{language.FrameFairyModels()}
	// The model picked in Settings is kept in the app's file.
	writers.chosen = func() (name string) {
		e.Store.Read(func(d *store.Data) { name = d.LanguageModel })
		return name
	}
	writers.choose = func(name string) error {
		return e.Store.Update(func(d *store.Data) error {
			d.LanguageModel = name
			return nil
		})
	}
	// A llama-server this app left running when it crashed holds the
	// model's memory until it is stopped, see leftover.go. Then the model
	// is loaded, once that memory is free.
	go func() {
		for _, pid := range language.StopLeftoverServers(writer.Notes) {
			theLog.line(zerolog.WarnLevel, "language").Int("pid", pid).Msg("a llama-server left running was stopped")
		}
		writers.warm()
	}()
	api.Handle("/api/language", languageAPI{s: writers})
	api.Handle("/api/language/", languageAPI{s: writers})
	answers.Drafts = &draft.Drafter{Store: e.Store, Host: writer, Model: writers.inUse,
		Check: func(m language.Model) error { return language.Check(m, writers.dir) },
		Finder: func() (draft.Finder, error) {
			f, err := e.Finder()
			if err != nil {
				return nil, err
			}
			reader, ok := f.(draft.Finder)
			if !ok {
				return nil, errors.New("the search provider cannot read a profile")
			}
			return reader, nil
		},
		Told: func(j draft.Job) {
			if app != nil {
				app.Event.Emit("draft", j)
			}
		},
		Log: theLog.slog("draft", zerolog.TraceLevel),
	}
	// Report a Problem in a window of its own, with Dictate, see bugs.go,
	// report.go and listen.go. The speech model is loaded on the first
	// press of Dictate and kept until it is removed or the app quits.
	host := &speech.Host{Open: asr.OpenOne}
	voices.hearing, voices.letGo = host.Held, host.KeepNone
	emit := func(name string, data any) {
		if app != nil {
			app.Event.Emit(name, data)
		}
	}
	dictate := &listening{host: host, models: voices,
		say:  func(d Dictation) { emit("dictation", d) },
		loud: func(n Loudness) { emit("loudness", n) },
	}
	report := newReporting(afero.NewOsFs(), filepath.Join(dir, "report"))
	report.stop = dictate.CancelListening
	report.told = func(open bool) { emit("report-window", open) }
	report.said = func(d ReportDraft) { emit("report-draft", d) }
	report.pick = func() ([]string, error) {
		return app.Dialog.OpenFile().
			SetTitle("Attach screenshots or recordings").
			CanChooseFiles(true).
			AddFilter("Screenshots and recordings", "*.png;*.jpg;*.jpeg;"+recordingPattern()).
			PromptForMultipleSelection()
	}
	reports := reportMaker{report: report, st: e.Store}
	api.Handle("/api/report", reportAPI{r: report, make: reports.ReportProblem})
	api.Handle("/api/report/", reportAPI{r: report, make: reports.ReportProblem})
	api.Handle("/api/listen", listenAPI{l: dictate})
	api.Handle("/api/listen/", listenAPI{l: dictate})
	// Nothing the app started outlives it: a run that goes is stopped, the
	// microphone is closed and the speech model let go, and a build that
	// is ready goes in place, once, see closeDown. On macOS app.Run never
	// returns, Cmd+Q ends the process from inside Cocoa once the shutdown
	// hooks have run, so this has to be one of them.
	quit := sync.OnceFunc(func() { closeDown(e, ups.Load(), dictate, host, writer) })
	// What Cmd+Q does, see quit.go. The hook above stays for whatever ends
	// the app without asking, a signal from the terminal among them.
	leave := &leaving{
		asked:  func() bool { return ups.Load().relaunchingNow() },
		system: quitBySystem,
		say:    func(what string) { app.Event.Emit("quit", quitSaid(what)) },
		stop:   quit,
		quit:   func() { app.Quit() },
	}
	api.Handle("/api/quit/", quitAPI{leave: leave})
	api.Handle("/api/clipboard", clipboardAPI{set: func(text string) bool { return app.Clipboard.SetText(text) }})
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
		OnShutdown: quit,
		ShouldQuit: leave.shouldQuit,
		// What Wails says goes into the app's log. Its debug lines are one
		// for every request the window makes.
		Logger: theLog.slog("wails", zerolog.InfoLevel),
		// A panic Wails catches, in a window's event, an event of the app
		// or a call from the window, goes into the app's log and the app
		// goes on. Without a handler of ours Wails ends the app. The
		// window's requests Wails does not recover, see handler.
		PanicHandler: wailsPanicked,
	})
	// Where updates stand is sent to the page as it changes, as an event
	// called updates, the way FrameFairy sends it. The page asks
	// /api/updates once when it starts and after a click, and hears the
	// rest. See web/src/lib/updates.svelte.ts.
	c := newUpdating(app.Updater, app.Quit, dir, e.CloseIfIdle, func(u UpdateState) {
		app.Event.Emit("updates", u)
	})
	ups.Store(c)
	// The log as Settings shows it, while Settings is open, as lines are
	// written, about once a second at most, as the event applog. See
	// watch in applog.go.
	go theLog.watch(func(st LogStatus) { app.Event.Emit("applog", st) },
		func() <-chan time.Time { return time.After(time.Second) })
	c.start()
	// Undo and Redo in Edit go to the page, see menu.go.
	app.Menu.SetApplicationMenu(appMenu(app))
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     "Speaker Trail",
		Width:     1100,
		Height:    820,
		MinWidth:  390,
		MinHeight: 560,
		URL:       "/app/people",
		Mac: application.MacWindow{
			Backdrop: application.MacBackdropNormal,
			// The page reaches up under the title bar, the way Mail and
			// FrameFairy look: the title bar is the page's own, exactly as
			// tall as the one macOS lays out, see chrome.go, with the close,
			// minimise and zoom buttons on it and the screen's name beside
			// them.
			// Hidden, not HiddenInset: a toolbar makes macOS push the
			// buttons further in than Terminal's, which FrameFairy measured
			// and found reads as wrong. A click in the top 28 points still
			// drags the window, and so does a click on any pane's bar.
			TitleBar:                application.MacTitleBarHidden,
			InvisibleTitleBarHeight: 28,
		},
	})
	// The title bar is as tall as the one macOS lays out, measured rather
	// than chosen, the way FrameFairy does it. The page hears it on the
	// event called chrome whenever it changes, see lib/chrome.svelte.ts.
	chrome.Store(watchChrome(func(c Chrome) { app.Event.Emit("chrome", c) }, window))
	// The report window goes when the app's window does, see bugs.go.
	window.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
		leave.closing()
		report.closeWindow("app")
	})
	report.make = func(closed func()) (front, shut func()) {
		return openReportWindow(app, window, report, closed, func(w *application.WebviewWindow) {
			reportChrome.Store(watchChrome(func(c Chrome) { app.Event.Emit("chrome-report", c) }, w))
		})
	}
	if err := app.Run(); err != nil {
		theLog.line(zerolog.ErrorLevel, "app").Err(err).Msg("the app stopped")
		os.Exit(1)
	}
}

// openReportWindow makes the report window, beside the app's window, with
// the app's own page showing Report a Problem alone, at /report. It gives
// what brings it forward and what closes it, and calls closed once it is
// gone. Pictures dropped on it are attached to the report, see bugs.go.
// Ported from FrameFairy's main.go.
func openReportWindow(app *application.App, main *application.WebviewWindow, report *reporting, closed func(), watch func(*application.WebviewWindow)) (front, shut func()) {
	x, y := 0, 0
	start := application.WindowCentered
	if main != nil {
		b := main.Bounds()
		at := image.Rect(b.X, b.Y, b.X+b.Width, b.Y+b.Height)
		work := at
		if screen, err := main.GetScreen(); err == nil && screen != nil {
			wa := screen.WorkArea
			work = image.Rect(wa.X, wa.Y, wa.X+wa.Width, wa.Y+wa.Height)
		}
		p := beside(at, work, reportWidth, reportHeight)
		x, y, start = p.X, p.Y, application.WindowXY
	}
	w := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:            "report",
		Title:           "Report a Problem",
		Width:           reportWidth,
		Height:          reportHeight,
		MinWidth:        reportMinWidth,
		MinHeight:       reportMinHeight,
		InitialPosition: start,
		X:               x,
		Y:               y,
		URL:             "/report",
		Mac: application.MacWindow{
			Backdrop:                application.MacBackdropNormal,
			InvisibleTitleBarHeight: 28,
			TitleBar:                application.MacTitleBarHidden,
		},
		// Pictures dropped anywhere on it are attached, see bugs.go.
		EnableFileDrop: true,
	})
	w.OnWindowEvent(events.Common.WindowFilesDropped, func(e *application.WindowEvent) {
		files := e.Context().DroppedFiles()
		go report.dropped(files)
	})
	w.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) { closed() })
	// Its own title bar, measured on its own, before the page can ask.
	watch(w)
	w.Show()
	w.Focus()
	return func() { w.Show(); w.Focus() }, func() { w.Close() }
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
func open() (*apipkg.API, *engine.Engine, string, error) {
	dir, err := Folder()
	if err != nil {
		return nil, nil, "", err
	}
	st, err := store.Open(dir, nil)
	if err != nil {
		return nil, nil, "", err
	}
	theLog.line(zerolog.InfoLevel, "app").Str("file", st.Path()).Msg("the app's file is read")
	if err := st.Update(func(d *store.Data) error { return d.Start(st.Now()) }); err != nil {
		return nil, nil, "", err
	}
	exa := keys.NewExa(dir, theLog.slog("keys", zerolog.TraceLevel))
	e := &engine.Engine{Store: st, Log: theLog.slog("engine", zerolog.TraceLevel), Act: theLog.acting, Finder: func() (engine.Finder, error) {
		key, err := exa.Get()
		switch {
		case errors.Is(err, keys.ErrNoKey):
			return nil, engine.ErrNoKey
		case err != nil:
			return nil, err
		}
		return &search.Exa{Key: key}, nil
	}}
	if err := e.Recover(); err != nil {
		return nil, nil, "", err
	}
	a := apipkg.New(st, e, exa, theLog.slog("api", zerolog.TraceLevel))
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
		// Wails serves each request on a goroutine of its own with no
		// recover, so a panic in one ended the whole app. Now it fails
		// that request and says so in the app's log, with its stack.
		// http.ErrAbortHandler is how a handler ends a response on
		// purpose, which net/http expects to see again, so it goes on.
		defer func() {
			caught := recover()
			if caught == nil {
				return
			}
			if caught == http.ErrAbortHandler {
				panic(caught)
			}
			theLog.line(zerolog.ErrorLevel, "api").Str("path", r.URL.Path).Str("stack", string(debug.Stack())).
				Msgf("the window's request stopped unexpectedly: %v", caught)
			said, _ := json.Marshal(map[string]string{"error": fmt.Sprintf("the request stopped unexpectedly: %v", caught)})
			http.Error(w, string(said), http.StatusInternalServerError)
		}()
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
