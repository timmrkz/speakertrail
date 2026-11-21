package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

// speechAPI answers Settings about the speech models. FrameFairy's
// window calls the bindings Setup, InstallSpeechModel, CancelJob and
// RemoveSpeechModel, and this one asks over HTTP, the way Tim chose for
// every call. Where a download stands is sent as the event speech.
//
//	GET    /api/speech                 every model, whether it is here, the folder, the latest download
//	POST   /api/speech/install         {"name": "…"} fetches that model, answers the download at once
//	POST   /api/speech/cancel          {"id": "speech-1"} stops that download, what arrived stays
//	DELETE /api/speech/models/{name}   takes that model off the Mac, after Settings asked
type speechAPI struct {
	s *speechModels
}

func (a speechAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	modelsAPI{
		base:    "/api/speech",
		state:   func() any { return a.s.state() },
		install: func(name string) any { return a.s.install(name) },
		stop:    a.s.stop,
		remove:  a.s.remove,
	}.ServeHTTP(w, r)
}

// modelsAPI is the four calls Settings makes about a kind of model, the
// speech models and the language models alike, under base.
type modelsAPI struct {
	base    string
	state   func() any
	install func(name string) any
	stop    func(id string) error
	remove  func(name string) error
	// use picks the model in use, where a kind has more than one to pick
	// from. Nil answers 404.
	use func(name string) error
}

func (a modelsAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	switch {
	case r.Method == http.MethodGet && r.URL.Path == a.base:
		answer(w, http.StatusOK, a.state())
	case r.Method == http.MethodPost && r.URL.Path == a.base+"/install":
		var body struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
			answer(w, http.StatusBadRequest, map[string]string{"error": "The request is not valid JSON"})
			return
		}
		answer(w, http.StatusOK, a.install(body.Name))
	case r.Method == http.MethodPost && r.URL.Path == a.base+"/cancel":
		var body struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
			answer(w, http.StatusBadRequest, map[string]string{"error": "The request is not valid JSON"})
			return
		}
		if err := a.stop(body.ID); err != nil {
			answer(w, http.StatusConflict, map[string]string{"error": sentence(err.Error())})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPost && r.URL.Path == a.base+"/use" && a.use != nil:
		var body struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
			answer(w, http.StatusBadRequest, map[string]string{"error": "The request is not valid JSON"})
			return
		}
		if err := a.use(body.Name); err != nil {
			answer(w, http.StatusConflict, map[string]string{"error": sentence(err.Error())})
			return
		}
		answer(w, http.StatusOK, a.state())
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, a.base+"/models/"):
		if err := a.remove(strings.TrimPrefix(r.URL.Path, a.base+"/models/")); err != nil {
			answer(w, http.StatusConflict, map[string]string{"error": sentence(err.Error())})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		answer(w, http.StatusNotFound, map[string]string{"error": "Not found"})
	}
}
