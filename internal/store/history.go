package store

import (
	"bytes"
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
// A change Tim makes goes through Edit, which reads what it can reach,
// a person, before and after it. What differs is one step. A change that
// changed nothing is no step. A run, a check and anything else the engine
// does on its own goes through Update, and is no step.
//
// An undo puts back only the fields the step changed, never the whole
// file, because a run may have written into the same person since. When a
// field the step would put back has changed again since, it writes
// nothing, says so, and that list starts again, ErrChangedSince.
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
	// Kind is "person". Searches, exclusions, the template and the
	// settings follow, see issue 71.
	Kind string `json:"kind"`
	ID   int64  `json:"id"`
}

// PersonThing is the person with the id.
func PersonThing(id int64) Thing { return Thing{Kind: "person", ID: id} }

// fields are the fields of a thing, by name, as JSON. A field missing
// from the map is at its zero value, which is how omitempty writes it.
type fields map[string]json.RawMessage

// change is what one step did to one thing: the fields it changed, as
// they were before and after. A field it emptied is nil after.
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
		if b, a := differ(before[i], after); a != nil {
			st.changes = append(st.changes, change{thing: t, before: b, after: a})
		}
	}
	if len(st.changes) == 0 {
		return nil
	}
	s.hist.undo = append(s.hist.undo, st)
	if len(s.hist.undo) > historyDepth {
		s.hist.undo = s.hist.undo[len(s.hist.undo)-historyDepth:]
	}
	s.hist.redo = nil
	return nil
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
			if now == nil {
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
			if err := d.setFields(c.thing, put, s.now()); err != nil {
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
	}
	return nil, fmt.Errorf("no history for a %q", t.Kind)
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
	}
	return fmt.Errorf("no history for a %q", t.Kind)
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
