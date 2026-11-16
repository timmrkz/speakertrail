package main

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/timmrkz/speakertrail/internal/speech"
)

// A model archive of the test's own, the way the real one comes, served
// by a server of the test's own. Tests need no network.
func testSpeechModel(t *testing.T, body func(w http.ResponseWriter, archive []byte)) speech.Model {
	t.Helper()
	if _, err := exec.LookPath("bzip2"); err != nil {
		t.Skip("bzip2 is not installed")
	}
	var plain bytes.Buffer
	w := tar.NewWriter(&plain)
	_ = w.WriteHeader(&tar.Header{Name: "test-model/tokens.txt", Mode: 0o644, Size: 4, Typeflag: tar.TypeReg})
	_, _ = w.Write([]byte("a 0\n"))
	_ = w.Close()
	cmd := exec.Command("bzip2", "-c")
	cmd.Stdin = &plain
	archive, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if body == nil {
		body = func(w http.ResponseWriter, archive []byte) {
			w.Header().Set("content-length", strconv.Itoa(len(archive)))
			_, _ = w.Write(archive)
		}
	}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { body(w, archive) }))
	t.Cleanup(s.Close)
	return speech.Model{Name: "test-model", Title: "Test model", URL: s.URL, Download: int64(len(archive))}
}

// speechFor is the speech models of a folder of the test's own, with only
// m in the list, and what it told the window.
func speechFor(t *testing.T, m speech.Model) (*speechModels, func() []SpeechState) {
	t.Helper()
	var mu sync.Mutex
	var told []SpeechState
	s := newSpeechModels(t.TempDir(), func(st SpeechState) {
		mu.Lock()
		told = append(told, st)
		mu.Unlock()
	})
	s.models = func() []speech.Model { return []speech.Model{m} }
	return s, func() []SpeechState {
		mu.Lock()
		defer mu.Unlock()
		return append([]SpeechState(nil), told...)
	}
}

func (s *speechModels) waitForDownload() {
	s.mu.Lock()
	done := s.done
	s.mu.Unlock()
	if done != nil {
		<-done
	}
}

// Download in Settings fetches the model, answers at once with a download
// that runs, so the click shows, and tells the window when it is done.
//
// On the real disk: the model is unpacked into a folder the speech library loads.
func TestDownloadFetchesTheSpeechModel(t *testing.T) {
	m := testSpeechModel(t, nil)
	s, told := speechFor(t, m)
	if st := s.state(); len(st.Models) != 1 || st.Models[0].Installed || st.Job != nil {
		t.Fatalf("before anything: %+v", st)
	}
	j := s.install(m.Name)
	if j.State != "running" || j.Kind != "model" || j.Title != m.Title {
		t.Fatalf("Download answered %+v, want a download running", j)
	}
	s.waitForDownload()
	st := s.state()
	if !st.Models[0].Installed || st.Job == nil || st.Job.State != "done" {
		t.Fatalf("after the download: %+v", st)
	}
	if _, dir := s.inUse(); dir == "" {
		t.Error("the model fetched is not the one in use")
	}
	all := told()
	if len(all) < 2 || all[len(all)-1].Job.State != "done" {
		t.Errorf("the window was told %+v, want the download ending last", all)
	}
}

// A model that is not offered is refused with why, in a download that
// failed, the way FrameFairy's job is refused.
func TestDownloadOfAModelNotOfferedFails(t *testing.T) {
	s, _ := speechFor(t, speech.Model{Name: "test-model"})
	j := s.install("no-such-model")
	if j.State != "failed" || !strings.Contains(j.Error, "no-such-model") {
		t.Errorf("answered %+v", j)
	}
}

// Cancel stops the download, which ends as cancelled with no model.
//
// On the real disk: the download is written into the models folder.
func TestCancelStopsTheDownload(t *testing.T) {
	sent := make(chan struct{})
	gone := make(chan struct{})
	m := testSpeechModel(t, func(w http.ResponseWriter, archive []byte) {
		w.Header().Set("content-length", strconv.Itoa(len(archive)*10))
		_, _ = w.Write(archive[:1])
		w.(http.Flusher).Flush()
		close(sent)
		<-gone
	})
	s, _ := speechFor(t, m)
	j := s.install(m.Name)
	<-sent
	if err := s.stop(j.ID); err != nil {
		t.Fatal(err)
	}
	s.waitForDownload()
	close(gone)
	st := s.state()
	if st.Job.State != "cancelled" || st.Models[0].Installed {
		t.Errorf("after Cancel: %+v", st)
	}
	if err := s.stop(j.ID); err == nil {
		t.Error("a download that ended was stopped again")
	}
}

// A model Dictate is hearing with cannot be removed, and says why.
//
// On the real disk: the model is a folder on the disk.
func TestAModelInUseIsNotRemoved(t *testing.T) {
	m := testSpeechModel(t, nil)
	s, _ := speechFor(t, m)
	s.install(m.Name)
	s.waitForDownload()
	s.hearing = func() bool { return true }
	if err := s.remove(m.Name); err != errSpeechLocked {
		t.Errorf("removing while hearing said %v", err)
	}
	s.hearing = func() bool { return false }
	if err := s.remove(m.Name); err != nil {
		t.Fatal(err)
	}
	if s.state().Models[0].Installed {
		t.Error("the model is still there")
	}
}

// Settings can press everything at once: Download, Cancel, Remove and
// reading the state, from several goroutines, while a download runs.
// One download at a time comes of it, and nothing races.
//
// On the real disk: the model is unpacked into the models folder.
func TestSpeechModelsFromSeveralAtOnce(t *testing.T) {
	m := testSpeechModel(t, nil)
	s, _ := speechFor(t, m)
	var wg sync.WaitGroup
	ids := make(chan string, 64)
	for range 8 {
		wg.Go(func() {
			for range 4 {
				ids <- s.install(m.Name).ID
				_ = s.state()
				_ = s.stop("speech-1")
				_ = s.remove("nothing")
			}
		})
	}
	wg.Wait()
	close(ids)
	s.waitForDownload()
	for id := range ids {
		if id == "" {
			t.Error("a download without an id")
		}
	}
	if j := s.state().Job; j == nil || (j.State != "done" && j.State != "cancelled") {
		t.Errorf("the last download is %+v", j)
	}
}

// The routes answer the way docs/api.md says.
//
// On the real disk: the model is unpacked into the models folder.
func TestSpeechRoutes(t *testing.T) {
	m := testSpeechModel(t, nil)
	s, _ := speechFor(t, m)
	h := speechAPI{s: s}
	ask := func(method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		return w
	}
	var st SpeechState
	if w := ask("GET", "/api/speech", ""); w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &st) != nil || len(st.Models) != 1 {
		t.Fatalf("GET: %d %s", w.Code, w.Body)
	}
	var j SpeechJob
	if w := ask("POST", "/api/speech/install", `{"name":"test-model"}`); w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &j) != nil || j.ID == "" {
		t.Fatalf("install: %d %s", w.Code, w.Body)
	}
	s.waitForDownload()
	if w := ask("POST", "/api/speech/cancel", `{"id":"`+j.ID+`"}`); w.Code != http.StatusConflict {
		t.Errorf("cancelling one that ended: %d", w.Code)
	}
	if w := ask("DELETE", "/api/speech/models/test-model", ""); w.Code != http.StatusNoContent {
		t.Errorf("remove: %d %s", w.Code, w.Body)
	}
	if w := ask("POST", "/api/speech/install", `not json`); w.Code != http.StatusBadRequest {
		t.Errorf("not JSON: %d", w.Code)
	}
}
