package main

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// GitHub's REST API as far as an upload, a file of the repository and
// the open pull requests go: a release that is not there is made by the
// first upload, a file of the same name is replaced, a file comes back as
// it was sent, and a fork's pull request is never one of ours.
func TestTheRESTClientAsksGitHubAsItAnswers(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var asked []string
	made := false
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		asked = append(asked, r.Method+" "+r.URL.Path)
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Errorf("%s %s without the token", r.Method, r.URL.Path)
		}
		switch r.Method + " " + r.URL.Path {
		case "GET /repos/o/r/releases/tags/dev":
			if !made {
				http.NotFound(w, r)
				return
			}
			_, _ = io.WriteString(w, `{"upload_url": "`+srv.URL+`/upload/assets{?name,label}",
				"assets": [{"id": 9, "name": "channels.json"}, {"id": 10, "name": "other.json"}]}`)
		case "POST /repos/o/r/releases":
			made = true
			_, _ = io.WriteString(w, `{}`)
		case "DELETE /repos/o/r/releases/assets/9":
			w.WriteHeader(http.StatusNoContent)
		case "POST /upload/assets":
			if r.URL.Query().Get("name") != "channels.json" || r.Header.Get("Content-Type") != "application/json" {
				t.Errorf("uploaded %s as %s", r.URL.Query().Get("name"), r.Header.Get("Content-Type"))
			}
			_, _ = io.WriteString(w, `{}`)
		case "GET /repos/o/r/contents/cmd/speakertrail-app/appfiles.go":
			text := base64.StdEncoding.EncodeToString([]byte("const appVersion = \"0.3.0\"\n"))
			body, _ := json.Marshal(map[string]string{"encoding": "base64", "content": text[:10] + "\n" + text[10:]})
			_, _ = w.Write(body)
		case "GET /repos/o/r/pulls":
			_, _ = io.WriteString(w, `[{"number": 1, "title": "ours", "head": {"ref": "a", "sha": "s1", "repo": {"full_name": "o/r"}}},
				{"number": 2, "title": "a fork's", "head": {"ref": "a", "sha": "s2", "repo": {"full_name": "f/r"}}},
				{"number": 3, "title": "a fork that is gone", "head": {"ref": "a", "sha": "s3", "repo": null}}]`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	g := &restGitHub{api: srv.URL, repo: "o/r", token: "token", client: srv.Client(), release: devBuilds}

	if err := g.upload("channels.json", []byte("{}")); err != nil {
		t.Fatal(err)
	}
	want := "GET /repos/o/r/releases/tags/dev POST /repos/o/r/releases GET /repos/o/r/releases/tags/dev " +
		"DELETE /repos/o/r/releases/assets/9 POST /upload/assets"
	if got := strings.Join(asked, " "); got != want {
		t.Errorf("asked %s", got)
	}
	file, err := g.fileAt("cmd/speakertrail-app/appfiles.go", "abc")
	if err != nil || string(file) != "const appVersion = \"0.3.0\"\n" {
		t.Errorf("the file came as %q, %v", file, err)
	}
	pulls, err := g.openPulls()
	if err != nil || len(pulls) != 1 || pulls[0].number != 1 || pulls[0].head != "s1" || pulls[0].branch != "a" {
		t.Errorf("the open pull requests are %+v, %v", pulls, err)
	}
	if _, err := g.mainCommit(); err == nil {
		t.Error("a 404 was no error")
	}
}
