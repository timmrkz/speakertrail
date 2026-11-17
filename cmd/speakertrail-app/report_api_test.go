package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/afero"
)

// The report window works the draft over HTTP: what was written is kept,
// a pasted picture is attached, taken out and put back, and a report made
// is named under ~, so the Mac's user name does not reach the page.
func TestTheReportWindowWorksTheDraftOverHTTP(t *testing.T) {
	t.Setenv("HOME", "/Users/someone")
	r := newReporting(afero.NewMemMapFs(), "/config/report")
	var made string
	a := reportAPI{r: r, make: func(what string) (string, error) {
		made = what
		return filepath.Join("/Users/someone", "Desktop", "Speaker Trail report.zip"), nil
	}}
	ask := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		a.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
		return rec
	}
	draft := func(rec *httptest.ResponseRecorder) ReportDraft {
		t.Helper()
		var d ReportDraft
		if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
			t.Fatalf("%d %s is no draft: %v", rec.Code, rec.Body, err)
		}
		return d
	}

	if rec := ask("PUT", "/api/report/text", `{"text":"It froze."}`); rec.Code != http.StatusNoContent {
		t.Fatalf("keeping the text answered %d %s", rec.Code, rec.Body)
	}
	pasted, _ := json.Marshal(map[string]string{"name": "shot.png", "data": base64.StdEncoding.EncodeToString(pngOf(t, 8, 8))})
	d := draft(ask("POST", "/api/report/pasted", string(pasted)))
	if d.Text != "It froze." || len(d.Pictures) != 1 {
		t.Fatalf("after a paste the draft is %+v", d)
	}
	id := d.Pictures[0].ID
	if d := draft(ask("POST", "/api/report/remove", `{"id":"`+id+`"}`)); len(d.Pictures) != 0 {
		t.Fatalf("after remove the draft is %+v", d)
	}
	if d := draft(ask("POST", "/api/report/put-back", `{"id":"`+id+`"}`)); len(d.Pictures) != 1 {
		t.Fatalf("after put back the draft is %+v", d)
	}
	if rec := ask("GET", "/api/report/picture?id="+id, ""); rec.Code != 200 || rec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("the picture answered %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	rec := ask("POST", "/api/report/send", `{"what":"It froze."}`)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"path":"~/Desktop/Speaker Trail report.zip"`) || made != "It froze." {
		t.Fatalf("sending answered %d %s, made with %q", rec.Code, rec.Body, made)
	}
	for _, bad := range [][3]string{{"POST", "/api/report/text", "{}"}, {"DELETE", "/api/report", ""}, {"POST", "/api/report/remove", "not json"}} {
		if rec := ask(bad[0], bad[1], bad[2]); rec.Code < 400 {
			t.Errorf("%s %s answered %d", bad[0], bad[1], rec.Code)
		}
	}
}

// Dictate answers POST alone, and without a speech model it says where to
// get one.
func TestDictateOverHTTPSaysWhereTheModelIs(t *testing.T) {
	f := testListening(t, nil)
	// No model is here.
	f.models = newSpeechModels(t.TempDir(), nil)
	a := listenAPI{l: f}
	rec := httptest.NewRecorder()
	a.ServeHTTP(rec, httptest.NewRequest("GET", "/api/listen", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET answered %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	a.ServeHTTP(rec, httptest.NewRequest("POST", "/api/listen", nil))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "Download it in Settings") {
		t.Fatalf("Dictate without a model answered %d %s", rec.Code, rec.Body)
	}
	rec = httptest.NewRecorder()
	a.ServeHTTP(rec, httptest.NewRequest("POST", "/api/listen/cancel", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("cancel answered %d", rec.Code)
	}
}
