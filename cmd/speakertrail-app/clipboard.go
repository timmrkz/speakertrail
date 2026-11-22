package main

import "net/http"

// clipboardAPI puts text on the Mac's clipboard for the window. The
// window's own navigator.clipboard.writeText is refused in the app's web
// view with a NotAllowedError, so Copy failed with the web view's words,
// Tim saw it testing #60 on 22 November 2026. WebKit does not say which
// of its rules for a page writing the clipboard it applied. The app writes the
// clipboard itself, through Wails, and the window asks over HTTP, the
// way it asks for everything.
//
//	POST /api/clipboard  {"text": "…"} puts the text on the clipboard, answers 204
type clipboardAPI struct {
	set func(text string) bool
}

// clipboardMax is the most a copy may carry, far more than a first
// message or the details of a build.
const clipboardMax = 1 << 20

func (a clipboardAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		answer(w, http.StatusMethodNotAllowed, map[string]string{"error": "Only POST"})
		return
	}
	var body struct {
		Text string `json:"text"`
	}
	if !readBody(w, r, clipboardMax, &body) {
		return
	}
	if !a.set(body.Text) {
		answer(w, http.StatusInternalServerError, map[string]string{"error": "The Mac did not take it on the clipboard. Select the text and press Cmd+C."})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
