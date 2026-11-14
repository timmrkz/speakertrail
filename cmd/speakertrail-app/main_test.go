package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestHandlerServesTheInterfaceAndTheAPI(t *testing.T) {
	ui := fstest.MapFS{
		"index.html":       {Data: []byte("index")},
		"assets/app.js":    {Data: []byte("script")},
		"favicon.svg":      {Data: []byte("icon")},
		"assets/dir/.keep": {Data: nil},
	}
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "api "+r.URL.Path) })
	h := handler(ui, api)
	for _, c := range []struct {
		path, want string
		code       int
	}{
		{"/", "index", 200},
		{"/app/people", "index", 200},
		{"/assets/app.js", "script", 200},
		{"/favicon.svg", "icon", 200},
		{"/assets/missing.js", "", 404},
		{"/api/people", "api /api/people", 200},
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", c.path, nil))
		if rec.Code != c.code {
			t.Errorf("%s: code %d, want %d", c.path, rec.Code, c.code)
		}
		if c.want != "" && !strings.Contains(rec.Body.String(), c.want) {
			t.Errorf("%s: body %q, want %q", c.path, rec.Body.String(), c.want)
		}
	}
}

func TestHandlerWithoutTheInterfaceSaysHowToBuildIt(t *testing.T) {
	rec := httptest.NewRecorder()
	handler(nil, nil).ServeHTTP(rec, httptest.NewRequest("GET", "/app/people", nil))
	if !strings.Contains(rec.Body.String(), "make") {
		t.Errorf("body %q does not say to run make", rec.Body.String())
	}
}
