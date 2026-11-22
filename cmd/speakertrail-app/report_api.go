package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// reportAPI answers the report window and the Report button of a toast.
// FrameFairy's window calls a binding for each, and this one asks over
// HTTP, the way Tim chose for every call. Whether the report window is
// open is sent as the event report-window, and what became of pictures
// dropped on it as report-draft. See bugs.go and report.go.
//
//	GET  /api/report                the draft: what was written and the pictures
//	GET  /api/report/window         {"open": true} while the report window is open
//	GET  /api/report/picture?id=p1  a picture of the draft, as PNG
//	POST /api/report/open           {"how": "bugs"} opens the report window, or brings it forward
//	POST /api/report/about          {"text": "…"} opens it as a report of a toast's words
//	PUT  /api/report/text           {"text": "…"} keeps what was written
//	POST /api/report/pick           asks for pictures in the open box, gives the draft
//	POST /api/report/pasted         {"name": "…", "data": "base64"} attaches a pasted picture
//	POST /api/report/remove         {"id": "p1"} takes a picture out, gives the draft
//	POST /api/report/put-back       {"id": "p1"} puts it back, gives the draft
//	POST /api/report/clear          lets go of the draft, gives the empty one
//	PUT  /api/report/people         {"people": true} takes People along, gives the draft
//	POST /api/report/send           {"what": "…", "people": true} makes the report, gives {"path": "~/Desktop/…"}
type reportAPI struct {
	r    *reporting
	make func(what string, people bool) (string, error)
}

// reportBodyMax is the most a request may carry: a pasted picture of
// 10 MB in base64, and room for its name.
const reportBodyMax = pictureBytesMax*4/3 + 4096

func (a reportAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var body struct {
		How    string `json:"how"`
		Text   string `json:"text"`
		Name   string `json:"name"`
		Data   string `json:"data"`
		ID     string `json:"id"`
		What   string `json:"what"`
		People bool   `json:"people"`
	}
	if (r.Method == http.MethodPost || r.Method == http.MethodPut) && !readBody(w, r, reportBodyMax, &body) {
		return
	}
	route := r.Method + " " + r.URL.Path
	switch route {
	case "GET /api/report":
		answer(w, http.StatusOK, a.r.ReportDraft())
	case "GET /api/report/window":
		answer(w, http.StatusOK, map[string]bool{"open": a.r.ReportWindowOpen()})
	case "GET /api/report/picture":
		a.r.servePicture(w, r)
	case "POST /api/report/open":
		a.r.OpenReport(body.How)
		w.WriteHeader(http.StatusNoContent)
	case "POST /api/report/about":
		a.r.ReportAbout(body.Text)
		w.WriteHeader(http.StatusNoContent)
	case "PUT /api/report/text":
		if err := a.r.SaveReportText(body.Text); err != nil {
			answer(w, http.StatusInternalServerError, map[string]string{"error": sentence(err.Error())})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case "PUT /api/report/people":
		answer(w, http.StatusOK, a.r.TakePeople(body.People))
	case "POST /api/report/pick":
		answer(w, http.StatusOK, a.r.AttachPictures())
	case "POST /api/report/pasted":
		answer(w, http.StatusOK, a.r.AttachPasted(body.Name, body.Data))
	case "POST /api/report/remove":
		answer(w, http.StatusOK, a.r.RemovePicture(body.ID))
	case "POST /api/report/put-back":
		answer(w, http.StatusOK, a.r.PutBackPicture(body.ID))
	case "POST /api/report/clear":
		answer(w, http.StatusOK, a.r.ClearReport())
	case "POST /api/report/send":
		path, err := a.make(body.What, body.People)
		if err != nil {
			answer(w, http.StatusInternalServerError, map[string]string{"error": sentence(err.Error())})
			return
		}
		answer(w, http.StatusOK, map[string]string{"path": tildePath(path)})
	default:
		answer(w, http.StatusNotFound, map[string]string{"error": "Not found"})
	}
}

// listenAPI answers Dictate in the report window. What is being written
// down is sent as the event dictation, and how loud the microphone is as
// loudness. See listen.go.
//
//	POST /api/listen          starts a recording, gives {"recording": 1}
//	POST /api/listen/stop     ends it and gives every piece written down
//	POST /api/listen/settle   {"recording": 1, "pieces": 2, "text": "…"} the window took those pieces in
//	POST /api/listen/break    {"recording": 1, "pieces": 2} the window took them in and moved on
//	POST /api/listen/cancel   drops the recording and what it heard
type listenAPI struct {
	l *listening
}

func (a listenAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		answer(w, http.StatusMethodNotAllowed, map[string]string{"error": "Only POST"})
		return
	}
	var body struct {
		Recording int    `json:"recording"`
		Pieces    int    `json:"pieces"`
		Text      string `json:"text"`
	}
	if !readBody(w, r, 1<<20, &body) {
		return
	}
	switch r.URL.Path {
	case "/api/listen":
		id, err := a.l.Listen()
		if err != nil {
			answer(w, http.StatusConflict, map[string]string{"error": sentence(err.Error())})
			return
		}
		answer(w, http.StatusOK, map[string]int{"recording": id})
	case "/api/listen/stop":
		d, err := a.l.StopListening()
		if err != nil {
			answer(w, http.StatusConflict, map[string]string{"error": sentence(err.Error())})
			return
		}
		answer(w, http.StatusOK, d)
	case "/api/listen/settle":
		a.l.SettleListening(body.Recording, body.Pieces, body.Text)
		w.WriteHeader(http.StatusNoContent)
	case "/api/listen/break":
		a.l.BreakListening(body.Recording, body.Pieces)
		w.WriteHeader(http.StatusNoContent)
	case "/api/listen/cancel":
		a.l.CancelListening()
		w.WriteHeader(http.StatusNoContent)
	default:
		answer(w, http.StatusNotFound, map[string]string{"error": "Not found"})
	}
}

// readBody reads a request's JSON into v, at most limit bytes. An empty
// body leaves v as it is. It answers a body that is not JSON itself, and
// says so by false.
func readBody(w http.ResponseWriter, r *http.Request, limit int64, v any) bool {
	err := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit)).Decode(v)
	if err == nil || errors.Is(err, io.EOF) {
		return true
	}
	answer(w, http.StatusBadRequest, map[string]string{"error": "The request is not valid JSON"})
	return false
}
