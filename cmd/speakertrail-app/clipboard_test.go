package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// Copy hands the text to the app, which puts it on the Mac's clipboard.
// The window's own clipboard was refused in the app's web view, Tim saw
// it testing #60 on 22 November 2026.
func TestCopyPutsTheTextOnTheClipboard(t *testing.T) {
	var mu sync.Mutex
	var got []string
	a := clipboardAPI{set: func(text string) bool {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, text)
		return true
	}}
	ask := func(method, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		a.ServeHTTP(rec, httptest.NewRequest(method, "/api/clipboard", strings.NewReader(body)))
		return rec
	}

	if rec := ask("POST", `{"text":"Hallo Mara,\n\nLiebe Grüße, Tim"}`); rec.Code != http.StatusNoContent {
		t.Fatalf("copy answered %d %s", rec.Code, rec.Body)
	}
	if len(got) != 1 || got[0] != "Hallo Mara,\n\nLiebe Grüße, Tim" {
		t.Fatalf("the clipboard got %q", got)
	}
	if rec := ask("GET", ""); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET answered %d", rec.Code)
	}

	// Several copies at once, as two clicks in a row can make.
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if rec := ask("POST", `{"text":"PS"}`); rec.Code != http.StatusNoContent {
				t.Errorf("copy answered %d", rec.Code)
			}
		}()
	}
	wg.Wait()
	if len(got) != 9 {
		t.Fatalf("the clipboard was set %d times, not 9", len(got))
	}
}

// A clipboard the Mac does not take says so in words a person reads.
func TestACopyTheMacRefusesSaysWhatToDo(t *testing.T) {
	a := clipboardAPI{set: func(string) bool { return false }}
	rec := httptest.NewRecorder()
	a.ServeHTTP(rec, httptest.NewRequest("POST", "/api/clipboard", strings.NewReader(`{"text":"Hallo"}`)))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("a refused copy answered %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Cmd+C") {
		t.Fatalf("a refused copy says %s", rec.Body)
	}
}
