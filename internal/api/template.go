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
	Parts  []store.Part  `json:"parts"`
	Fields []store.Field `json:"fields"`
	// Changed says Tim changed it, so Reset has something to put back.
	Changed bool `json:"changed"`
}

func templateOf(d *store.Data) templateJSON {
	t := d.FirstMessageTemplate()
	for i := range t.Parts {
		if t.Parts[i].Variations == nil {
			t.Parts[i].Variations = []string{}
		}
	}
	return templateJSON{Parts: t.Parts, Fields: store.TemplateFields, Changed: d.Template != nil}
}

func (a *API) template(w http.ResponseWriter, r *http.Request) {
	var out templateJSON
	a.Store.Read(func(d *store.Data) { out = templateOf(d) })
	send(w, http.StatusOK, out)
}

// changeTemplate changes the template with fn and answers with it, a 400
// with what is wrong when the template that comes of it cannot work.
func (a *API) changeTemplate(w http.ResponseWriter, r *http.Request, fn func(d *store.Data) error) {
	var out templateJSON
	err := a.Store.Update(func(d *store.Data) error {
		if err := fn(d); err != nil {
			return err
		}
		out = templateOf(d)
		return nil
	})
	var bad errBad
	switch {
	case err == nil:
		send(w, http.StatusOK, out)
	case errors.Is(err, store.ErrNoPart):
		fail(w, http.StatusNotFound, draft.Sentence(err.Error()))
	case errors.As(err, &bad):
		fail(w, http.StatusBadRequest, draft.Sentence(bad.Error()))
	default:
		a.internal(w, r, err)
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
		if err := d.SetTemplatePart(r.PathValue("name"), body.Variations, at); err != nil {
			return errBad{err}
		}
		return nil
	})
}

func (a *API) deleteTemplatePart(w http.ResponseWriter, r *http.Request) {
	a.changeTemplate(w, r, func(d *store.Data) error {
		err := d.RemoveTemplatePart(r.PathValue("name"))
		if err != nil && !errors.Is(err, store.ErrNoPart) {
			return errBad{err}
		}
		return err
	})
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
	a.changeTemplate(w, r, func(d *store.Data) error {
		if err := d.SetTemplate(store.Template{Parts: body.Parts}); err != nil {
			return errBad{err}
		}
		return nil
	})
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
