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
	w.Header().Set("Cache-Control", "no-store")
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/speech":
		answer(w, http.StatusOK, a.s.state())
	case r.Method == http.MethodPost && r.URL.Path == "/api/speech/install":
		var body struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
			answer(w, http.StatusBadRequest, map[string]string{"error": "The request is not valid JSON"})
			return
		}
		answer(w, http.StatusOK, a.s.install(body.Name))
	case r.Method == http.MethodPost && r.URL.Path == "/api/speech/cancel":
		var body struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
			answer(w, http.StatusBadRequest, map[string]string{"error": "The request is not valid JSON"})
			return
		}
		if err := a.s.stop(body.ID); err != nil {
			answer(w, http.StatusConflict, map[string]string{"error": sentence(err.Error())})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/speech/models/"):
		if err := a.s.remove(strings.TrimPrefix(r.URL.Path, "/api/speech/models/")); err != nil {
			answer(w, http.StatusConflict, map[string]string{"error": sentence(err.Error())})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		answer(w, http.StatusNotFound, map[string]string{"error": "Not found"})
	}
}
