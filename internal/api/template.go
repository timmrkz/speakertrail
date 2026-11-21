package api

import (
	"errors"
	"math/rand/v2"
	"net/http"

	"github.com/timmrkz/speakertrail/internal/draft"
	"github.com/timmrkz/speakertrail/internal/store"
)

// The template a first message is put together from, as Settings edits
// it, see store.Template and docs/api.md, First message template.

// templateJSON is the template with what the app and the model fill in.
type templateJSON struct {
	Parts  []partJSON    `json:"parts"`
	Fields []store.Field `json:"fields"`
	// Changed says Tim changed it, so Reset has something to put back.
	Changed bool `json:"changed"`
}

// partJSON is a part, and why it cannot be taken out, empty when it can.
// The app says it, so the window only shows it.
type partJSON struct {
	Name       string   `json:"name"`
	Variations []string `json:"variations"`
	Kept       string   `json:"kept,omitempty"`
}

func templateOf(d *store.Data) templateJSON {
	t := d.FirstMessageTemplate()
	parts := make([]partJSON, 0, len(t.Parts))
	for _, p := range t.Parts {
		out := partJSON{Name: p.Name, Variations: p.Variations}
		if out.Variations == nil {
			out.Variations = []string{}
		}
		if err := t.Removable(p.Name); err != nil {
			out.Kept = draft.Sentence(err.Error())
		}
		parts = append(parts, out)
	}
	return templateJSON{Parts: parts, Fields: store.TemplateFields, Changed: d.Template != nil}
}

func (a *API) template(w http.ResponseWriter, r *http.Request) {
	var out templateJSON
	a.Store.Read(func(d *store.Data) { out = templateOf(d) })
	send(w, http.StatusOK, out)
}

// changeTemplate changes the template with fn and answers with it: 404
// for a part that is not there, 400 with what is wrong when the template
// that comes of it cannot work.
func (a *API) changeTemplate(w http.ResponseWriter, r *http.Request, fn func(d *store.Data) error) {
	var out templateJSON
	if a.update(w, r, func(d *store.Data) error {
		err := fn(d)
		switch {
		case errors.Is(err, store.ErrNoPart):
			return errNotFound
		case err != nil:
			return errBad{errors.New(draft.Sentence(err.Error()))}
		}
		out = templateOf(d)
		return nil
	}) {
		send(w, http.StatusOK, out)
	}
}

// putTemplatePart gives a part its variations, or adds it at at.
func (a *API) putTemplatePart(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Variations []string `json:"variations"`
		At         *int     `json:"at"`
	}
	if !decode(w, r, &body) {
		return
	}
	at := -1
	if body.At != nil {
		at = *body.At
	}
	a.changeTemplate(w, r, func(d *store.Data) error {
		return d.SetTemplatePart(r.PathValue("name"), body.Variations, at)
	})
}

func (a *API) deleteTemplatePart(w http.ResponseWriter, r *http.Request) {
	a.changeTemplate(w, r, func(d *store.Data) error { return d.RemoveTemplatePart(r.PathValue("name")) })
}

// putTemplate puts a whole template in place, the one before Reset when
// it is undone.
func (a *API) putTemplate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Parts []store.Part `json:"parts"`
	}
	if !decode(w, r, &body) {
		return
	}
	a.changeTemplate(w, r, func(d *store.Data) error { return d.SetTemplate(store.Template{Parts: body.Parts}) })
}

// resetTemplate puts Tim's first template back.
func (a *API) resetTemplate(w http.ResponseWriter, r *http.Request) {
	a.changeTemplate(w, r, func(d *store.Data) error {
		d.ResetTemplate()
		return nil
	})
}

// templateExample puts one message together at random, with what the app
// and the model fill in shown by name, so Tim sees how the parts fit.
func (a *API) templateExample(w http.ResponseWriter, r *http.Request) {
	var t store.Template
	a.Store.Read(func(d *store.Data) { t = d.FirstMessageTemplate() })
	if err := t.Check(); err != nil {
		fail(w, http.StatusConflict, draft.Sentence(err.Error()))
		return
	}
	fields := map[string]string{}
	for _, f := range store.TemplateFields {
		fields[f.Name] = "[" + f.Name + "]"
	}
	fields[store.ReactionField] = " [reaction]"
	send(w, http.StatusOK, map[string]string{"text": t.Expand(fields, rand.IntN)})
}
