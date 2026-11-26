package store

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Undo and redo
//
// Every change Tim makes can be taken back with Undo in the Edit menu, and
// done again with Redo. Ported from FrameFairy's history.go and undo.go,
// with one history for the app instead of one per episode, since Speaker
// Trail has no episodes.
//
// A change Tim makes goes through Edit, which reads what it can reach, a
// person, a search, an exclusion, the template or a setting, before and
// after it. What differs is one step. A change that changed nothing is no
// step. A search or an exclusion Tim adds goes through Add, since it has
// no id before it is added. A run, a check and anything else the engine
// does on its own goes through Update, and is no step.
//
// An undo puts back only the fields the step changed, never the whole
// file, because a run may have written into the same person or search
// since. When a field the step would put back has changed again since, it
// writes nothing, says so, and that list starts again, ErrChangedSince.
// A search or an exclusion that was added is taken out by the undo, and
// one that was removed comes back with its id.
//
// Nothing is kept when the app closes. Everything is saved the moment it
// is done, so there is nothing to lose by closing, and a history that
// outlived the app would have to survive runs that replace what it knew.
// ---------------------------------------------------------------------------

// historyDepth is how many steps back Undo goes. A step holds only the
// fields it changed, mostly a few bytes, so this is generous.
const historyDepth = 200

// ErrChangedSince refuses an undo or a redo whose fields were changed
// since by something else, a run above all.
var ErrChangedSince = errors.New("this has changed since, so it cannot be taken back")

// Thing is what one change reaches, and what an undo of it shows.
type Thing struct {
	// Kind is person, search, exclusion, template or setting.
	Kind string `json:"kind"`
	// ID is the person's, the search's or the exclusion's.
	ID int64 `json:"id,omitempty"`
	// Key is the setting's.
	Key string `json:"key,omitempty"`
}

// PersonThing is the person with the id.
func PersonThing(id int64) Thing { return Thing{Kind: "person", ID: id} }

// SearchThing is the profile search with the id.
func SearchThing(id int64) Thing { return Thing{Kind: "search", ID: id} }

// ExclusionThing is the exclusion with the id.
func ExclusionThing(id int64) Thing { return Thing{Kind: "exclusion", ID: id} }

// TemplateThing is the template of a first message, all its parts.
func TemplateThing() Thing { return Thing{Kind: "template"} }

// SettingThing is the setting with the key, one of Defaults or
// LinkedInOpensKey.
func SettingThing(key string) Thing { return Thing{Kind: "setting", Key: key} }

// LinkedInOpensKey is the setting of the browser profile LinkedIn opens
// in, which is kept apart from the others, see Data.LinkedInOpens.
const LinkedInOpensKey = "linkedin_opens"

// fields are the fields of a thing, by name, as JSON. A field missing
// from the map is at its zero value, which is how omitempty writes it.
// Nil fields are a thing that is not there.
type fields map[string]json.RawMessage

// change is what one step did to one thing: the fields it changed, as
// they were before and after. A field it emptied is nil after. A thing
// it added is nil before, and one it removed nil after.
type change struct {
	thing         Thing
	before, after fields
}

// step is one change Tim made.
type step struct {
	changes []change
}

// history is the undo and redo of the app, under the store's lock.
type history struct {
	undo, redo []step
}

// Undone is what an undo or a redo did: whether there was anything to do,
// and what it changed, so the interface can show it.
type Undone struct {
	Done  bool   `json:"done"`
	Shows *Thing `json:"shows,omitempty"`
}

// Edit is Update for a change Tim makes, which Undo can take back. what
// is everything fn may change that Undo should put back. Whatever else fn
// changes is saved and stays when the step is undone, like how long a
// draft took.
func (s *Store) Edit(fn func(d *Data) error, what ...Thing) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	before := make([]fields, len(what))
	for i, t := range what {
		f, err := s.data.fieldsOf(t)
		if err != nil {
			return err
		}
		before[i] = f
	}
	if err := s.update(fn); err != nil {
		return err
	}
	var st step
	for i, t := range what {
		after, err := s.data.fieldsOf(t)
		if err != nil {
			return err
		}
		if c, ok := changed(t, before[i], after); ok {
			st.changes = append(st.changes, c)
		}
	}
	s.record(st)
	return nil
}

// Add is Edit for a change that adds a search or an exclusion, which has
// no id before fn gives it one. fn says what it added. Undo takes it out
// again, and Redo puts it back with the same id.
func (s *Store) Add(fn func(d *Data) (Thing, error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var t Thing
	if err := s.update(func(d *Data) (err error) {
		t, err = fn(d)
		return err
	}); err != nil {
		return err
	}
	after, err := s.data.fieldsOf(t)
	if err != nil {
		return err
	}
	if c, ok := changed(t, nil, after); ok {
		s.record(step{changes: []change{c}})
	}
	return nil
}

// record keeps a step, unless it changed nothing. A new step clears Redo.
func (s *Store) record(st step) {
	if len(st.changes) == 0 {
		return
	}
	s.hist.undo = append(s.hist.undo, st)
	if len(s.hist.undo) > historyDepth {
		s.hist.undo = s.hist.undo[len(s.hist.undo)-historyDepth:]
	}
	s.hist.redo = nil
}

// changed is what a change did to t, when it did anything: the fields
// that differ, or all of them for a thing it added or removed.
func changed(t Thing, before, after fields) (change, bool) {
	switch {
	case before == nil && after == nil:
		return change{}, false
	case before == nil || after == nil:
		return change{thing: t, before: before, after: after}, true
	}
	b, a := differ(before, after)
	if a == nil {
		return change{}, false
	}
	return change{thing: t, before: b, after: a}, true
}

// Undo takes back the last change Tim made.
func (s *Store) Undo() (Undone, error) { return s.step(true) }

// Redo does again what the last undo took back.
func (s *Store) Redo() (Undone, error) { return s.step(false) }

func (s *Store) step(back bool) (Undone, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	from, to := &s.hist.undo, &s.hist.redo
	if !back {
		from, to = to, from
	}
	if len(*from) == 0 {
		return Undone{}, nil
	}
	st := (*from)[len(*from)-1]
	err := s.update(func(d *Data) error {
		for _, c := range st.changes {
			want, put := c.after, c.before
			if !back {
				want, put = put, want
			}
			now, err := d.fieldsOf(c.thing)
			if err != nil {
				return err
			}
			if (now == nil) != (want == nil) {
				return ErrChangedSince
			}
			for k, v := range want {
				if derived[k] {
					continue
				}
				if !bytes.Equal(now[k], v) {
					return ErrChangedSince
				}
			}
			switch {
			case put == nil:
				err = d.remove(c.thing)
			case now == nil:
				err = d.create(c.thing, put)
			default:
				err = d.setFields(c.thing, put, s.now())
			}
			if err != nil {
				return err
			}
		}
		return nil
	})
	if errors.Is(err, ErrChangedSince) {
		// Every step behind it in the same list may rest on it, so that
		// list starts again from here. The other list stays: each of its
		// steps checks its own fields when it is taken.
		*from = nil
		return Undone{}, err
	}
	if err != nil {
		return Undone{}, err
	}
	*from = (*from)[:len(*from)-1]
	*to = append(*to, st)
	shows := st.changes[0].thing
	return Undone{Done: true, Shows: &shows}, nil
}

// differ gives the fields that differ between before and after, as they
// were in each, or nil, nil when none do.
func differ(before, after fields) (fields, fields) {
	var b, a fields
	for _, k := range keys(before, after) {
		if bytes.Equal(before[k], after[k]) {
			continue
		}
		if b == nil {
			b, a = fields{}, fields{}
		}
		b[k], a[k] = before[k], after[k]
	}
	return b, a
}

func keys(of ...fields) []string {
	seen := map[string]bool{}
	for _, f := range of {
		for k := range f {
			seen[k] = true
		}
	}
	return slices.Collect(maps.Keys(seen))
}

// fieldsOf reads a thing's fields, nil when there is no such thing.
func (d *Data) fieldsOf(t Thing) (fields, error) {
	switch t.Kind {
	case "person":
		p := d.Person(t.ID)
		if p == nil {
			return nil, nil
		}
		return personFields(p)
	case "search":
		if x := d.Search(t.ID); x != nil {
			return fieldsOfValue(x)
		}
		return nil, nil
	case "exclusion":
		if x := d.exclusion(t.ID); x != nil {
			return fieldsOfValue(x)
		}
		return nil, nil
	case "template":
		// The template is one field, since its parts are kept in an
		// order and nothing but Tim changes them.
		f := fields{}
		if d.Template != nil {
			raw, err := json.Marshal(d.Template)
			if err != nil {
				return nil, err
			}
			f["parts"] = raw
		}
		return f, nil
	case "setting":
		return d.settingFields(t.Key)
	}
	return nil, fmt.Errorf("no history for a %q", t.Kind)
}

// settingFields reads a setting as one field, its value, missing while it
// is at its default. When it changed is no field, as for a person.
func (d *Data) settingFields(key string) (fields, error) {
	if key == LinkedInOpensKey {
		f := fields{}
		if d.LinkedInOpens != (Where{}) {
			raw, err := json.Marshal(d.LinkedInOpens)
			if err != nil {
				return nil, err
			}
			f["value"] = raw
		}
		return f, nil
	}
	if _, ok := defaultOf(key); !ok {
		return nil, nil
	}
	f := fields{}
	if v, ok := d.Settings[key]; ok {
		f["value"] = v.Value
	}
	return f, nil
}

// setFields puts a thing's fields back as put has them, and leaves the
// rest as they are.
func (d *Data) setFields(t Thing, put fields, now time.Time) error {
	switch t.Kind {
	case "person":
		p := d.Person(t.ID)
		if p == nil {
			return ErrChangedSince
		}
		return d.setPersonFields(p, put, now)
	case "search":
		// The search stays the one in the list, since a run may hold it.
		x := d.Search(t.ID)
		if x == nil {
			return ErrChangedSince
		}
		return setValueFields(x, put)
	case "exclusion":
		x := d.exclusion(t.ID)
		if x == nil {
			return ErrChangedSince
		}
		return setValueFields(x, put)
	case "template":
		raw := put["parts"]
		if raw == nil {
			d.Template = nil
			return nil
		}
		var tpl Template
		if err := json.Unmarshal(raw, &tpl); err != nil {
			return err
		}
		d.Template = &tpl
		return nil
	case "setting":
		raw := put["value"]
		if t.Key == LinkedInOpensKey {
			d.LinkedInOpens = Where{}
			if raw == nil {
				return nil
			}
			return json.Unmarshal(raw, &d.LinkedInOpens)
		}
		if raw == nil {
			delete(d.Settings, t.Key)
		} else {
			d.Settings[t.Key] = SettingValue{Value: raw, UpdatedAt: now}
		}
		return nil
	}
	return fmt.Errorf("no history for a %q", t.Kind)
}

// create puts back a search or an exclusion a step removed, or added and
// an undo took out, with its id and in its place among the others. One
// with the same words added since stays, and the step cannot be taken.
func (d *Data) create(t Thing, put fields) error {
	switch t.Kind {
	case "search":
		var x Search
		if err := from(put, &x); err != nil {
			return err
		}
		if slices.ContainsFunc(d.Searches, func(o *Search) bool { return strings.EqualFold(o.Query, x.Query) }) {
			return ErrChangedSince
		}
		d.Searches = insertByID(d.Searches, &x, func(o *Search) int64 { return o.ID })
		return nil
	case "exclusion":
		var x Exclusion
		if err := from(put, &x); err != nil {
			return err
		}
		if slices.ContainsFunc(d.Exclusions, func(o *Exclusion) bool { return strings.EqualFold(o.Phrase, x.Phrase) }) {
			return ErrChangedSince
		}
		d.Exclusions = insertByID(d.Exclusions, &x, func(o *Exclusion) int64 { return o.ID })
		return nil
	}
	return fmt.Errorf("no history adds a %q", t.Kind)
}

// remove takes out a search or an exclusion a step added, or removed and
// an undo put back. A search that found someone since stays, since they
// say it found them.
func (d *Data) remove(t Thing) error {
	switch t.Kind {
	case "search":
		for _, p := range d.People {
			if slices.ContainsFunc(p.FoundBy, func(f Sighting) bool { return f.SearchID == t.ID }) {
				return ErrChangedSince
			}
		}
		d.Searches = slices.DeleteFunc(d.Searches, func(x *Search) bool { return x.ID == t.ID })
		return nil
	case "exclusion":
		d.DeleteExclusion(t.ID)
		return nil
	}
	return fmt.Errorf("no history removes a %q", t.Kind)
}

// insertByID puts x into list in the order of the ids, which is the order
// things were added in.
func insertByID[T any](list []T, x T, id func(T) int64) []T {
	i, _ := slices.BinarySearchFunc(list, id(x), func(o T, want int64) int { return cmp.Compare(id(o), want) })
	return slices.Insert(list, i, x)
}

// fieldsOfValue reads the JSON fields of a search or an exclusion.
func fieldsOfValue(v any) (fields, error) {
	out := fields{}
	if err := into(out, "", v); err != nil {
		return nil, err
	}
	return out, nil
}

// setValueFields puts back the fields of a search or an exclusion, in
// place, and leaves the rest as they are.
func setValueFields[T any](x *T, put fields) error {
	f, err := fieldsOfValue(x)
	if err != nil {
		return err
	}
	for k, v := range put {
		if v == nil {
			delete(f, k)
		} else {
			f[k] = v
		}
	}
	var n T
	if err := from(f, &n); err != nil {
		return err
	}
	*x = n
	return nil
}

// derived are the fields worked out from the others, the fit's signals
// and score, which every run works out again for everyone. They go back
// with the step, but a run that worked them out since does not make an
// undo refuse, as FrameFairy leaves what follows from a clip out of its
// changes. The next run works them out again.
var derived = map[string]bool{"signals": true, "score": true, "rubric": true}

// linkedInPrefix marks the fields of a person's LinkedIn profile, so a
// change to one of them is a change to that field alone.
const linkedInPrefix = "linkedin."

// The profile text is kept in the second file, so JSON leaves it out of
// a person. It is a field all the same.
const (
	textField   = linkedInPrefix + "text"
	textAtField = linkedInPrefix + "text_at"
)

// personFields reads a person's fields. When they changed is no field:
// every change sets it, a run's too, so it would make every undo refuse.
func personFields(p *Person) (fields, error) {
	out := fields{}
	if err := into(out, "", p); err != nil {
		return nil, err
	}
	delete(out, "linkedin")
	delete(out, "updated_at")
	if err := into(out, linkedInPrefix, p.LinkedIn); err != nil {
		return nil, err
	}
	for k, v := range map[string]any{textField: p.LinkedIn.Text, textAtField: p.LinkedIn.TextAt} {
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		out[k] = raw
	}
	return out, nil
}

// setPersonFields puts back a person's fields. The person stays the one
// in the list, since a run may hold it.
func (d *Data) setPersonFields(p *Person, put fields, now time.Time) error {
	f, err := personFields(p)
	if err != nil {
		return err
	}
	for k, v := range put {
		if v == nil {
			delete(f, k)
		} else {
			f[k] = v
		}
	}
	top, linkedin := fields{}, fields{}
	for k, v := range f {
		if rest, ok := strings.CutPrefix(k, linkedInPrefix); ok {
			linkedin[rest] = v
		} else {
			top[k] = v
		}
	}
	var text string
	var at time.Time
	if err := json.Unmarshal(linkedin["text"], &text); err != nil {
		return err
	}
	if err := json.Unmarshal(linkedin["text_at"], &at); err != nil {
		return err
	}
	var np Person
	if err := from(top, &np); err != nil {
		return err
	}
	if err := from(linkedin, &np.LinkedIn); err != nil {
		return err
	}
	// The text goes through SetProfileText, which writes it to its own
	// file, so it starts as it is now.
	np.LinkedIn.Text, np.LinkedIn.TextAt = p.LinkedIn.Text, p.LinkedIn.TextAt
	np.LinkedIn.LastWrote, np.LinkedIn.Wrote = p.LinkedIn.LastWrote, p.LinkedIn.Wrote
	*p = np
	d.SetProfileText(p, text, at)
	p.UpdatedAt = now
	return nil
}

// into adds v's JSON fields to out, each name after prefix.
func into(out fields, prefix string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	var m fields
	if err := json.Unmarshal(raw, &m); err != nil {
		return err
	}
	for k, x := range m {
		out[prefix+k] = x
	}
	return nil
}

// from reads fields into v, which starts at its zero value.
func from(f fields, v any) error {
	raw, err := json.Marshal(f)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, v)
}
