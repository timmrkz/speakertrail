package store

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The template a first message is put together from, in the way of
// Tracery (https://github.com/galaxykate/tracery): each part has its
// variations, a draft picks one of them at random, and every #part#
// inside it is put together the same way, starting from origin. The app
// fills first_name, and the language model the portrait, the reason and
// the reaction, see internal/draft. Tim edits every part in Settings.

// Template is the parts of a first message, in the order Tim keeps them.
type Template struct {
	Parts []Part
}

// Part is one part of the template and the variations a draft picks from.
// An empty variation leaves the part out, and a variation written twice
// is picked twice as often.
type Part struct {
	Name       string   `json:"name"`
	Variations []string `json:"variations"`
}

// Field is a part of the message that is filled in rather than picked.
type Field struct {
	Name string `json:"name"`
	// By is who fills it, app or model.
	By    string `json:"by"`
	About string `json:"about"`
}

// The names of the parts and fields, as #name# writes them.
const (
	TemplateOrigin = "origin"
	FirstNameField = "first_name"
	PortraitField  = "portrait"
	ReasonField    = "reason"
	ReactionField  = "reaction_part"
)

// TemplateFields are the parts the app and the language model fill in.
// No part of the template takes their names.
var TemplateFields = []Field{
	{FirstNameField, "app", "Their first name"},
	{PortraitField, "model", "What they are, the way they show themselves, like Coach or Gründerin"},
	{ReasonField, "model", "Why they caught Tim's eye, the part after \"weil\""},
	{ReactionField, "model", "Nothing, or a space and a short reaction"},
}

// Limits of a template, so a slip never makes a file or a draft without
// end.
const (
	maxParts      = 100
	maxVariations = 200
	maxVariation  = 2000
	// maxDepth is how deep parts may lie inside each other.
	maxDepth = 20
)

//go:embed first_message_template.json
var defaultTemplate []byte

// DefaultTemplate is the template until Tim changes it, the one he wrote
// on 21 November 2026.
func DefaultTemplate() Template {
	var t Template
	if err := json.Unmarshal(defaultTemplate, &t); err != nil {
		panic(err)
	}
	if err := t.Check(); err != nil {
		panic(err)
	}
	return t
}

// partName is how a part may be named: small letters, digits and _,
// starting with a letter.
var partName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// ref is a part or a field inside a variation.
var ref = regexp.MustCompile(`#([a-z][a-z0-9_]*)#`)

// Part gives the part of that name.
func (t Template) Part(name string) (Part, bool) {
	i := t.index(name)
	if i < 0 {
		return Part{}, false
	}
	return t.Parts[i], true
}

func (t Template) index(name string) int {
	return slices.IndexFunc(t.Parts, func(p Part) bool { return p.Name == name })
}

func isField(name string) bool {
	return slices.ContainsFunc(TemplateFields, func(f Field) bool { return f.Name == name })
}

// Check says what is wrong with a template, in words Tim reads in
// Settings: a part named wrong or without a variation, a #name# that is
// no part, a part inside itself, or no #reason# to be found from origin.
func (t Template) Check() error {
	if len(t.Parts) > maxParts {
		return fmt.Errorf("a template takes at most %d parts", maxParts)
	}
	for i, p := range t.Parts {
		switch {
		case !partName.MatchString(p.Name):
			return fmt.Errorf("%q cannot name a part. Use small letters, digits and _, starting with a letter", p.Name)
		case isField(p.Name):
			return fmt.Errorf("%s is filled in by the %s, so no part takes its name", p.Name, fieldBy(p.Name))
		case slices.ContainsFunc(t.Parts[:i], func(o Part) bool { return o.Name == p.Name }):
			return fmt.Errorf("there are two parts named %s", p.Name)
		case len(p.Variations) == 0:
			return fmt.Errorf("%s needs at least one variation. An empty one leaves it out", p.Name)
		case len(p.Variations) > maxVariations:
			return fmt.Errorf("%s takes at most %d variations", p.Name, maxVariations)
		}
		for _, v := range p.Variations {
			if err := checkVariation(t, p.Name, v); err != nil {
				return err
			}
		}
	}
	if t.index(TemplateOrigin) < 0 {
		return errors.New("the template needs a part named origin, where the message starts")
	}
	reached, err := t.walk()
	if err != nil {
		return err
	}
	if !reached[ReasonField] {
		return errors.New("the template needs #reason#, where the language model's reason goes")
	}
	return nil
}

func fieldBy(name string) string {
	for _, f := range TemplateFields {
		if f.Name == name && f.By == "model" {
			return "language model"
		}
	}
	return "app"
}

func checkVariation(t Template, part, v string) error {
	if !utf8.ValidString(v) {
		return fmt.Errorf("a variation of %s is not text", part)
	}
	if n := utf8.RuneCountInString(v); n > maxVariation {
		return fmt.Errorf("a variation of %s has %d letters, at most %d fit", part, n, maxVariation)
	}
	for _, r := range v {
		if r != '\n' && unicode.IsControl(r) {
			return fmt.Errorf("a variation of %s holds a character that cannot be written", part)
		}
	}
	for _, m := range ref.FindAllStringSubmatch(v, -1) {
		if name := m[1]; t.index(name) < 0 && !isField(name) {
			return fmt.Errorf("%s uses #%s#, and there is no part of that name", part, name)
		}
	}
	return nil
}

// walk goes through every part origin reaches, and says which parts and
// fields it found. A part that holds itself, or parts too deep inside
// each other, would never end, so they are refused.
func (t Template) walk() (map[string]bool, error) {
	reached := map[string]bool{}
	var visit func(name string, path []string) error
	visit = func(name string, path []string) error {
		if slices.Contains(path, name) {
			return fmt.Errorf("%s holds itself, through %s", name, strings.Join(append(path, name), " → "))
		}
		if len(path) >= maxDepth {
			return fmt.Errorf("the parts lie more than %d deep inside each other", maxDepth)
		}
		reached[name] = true
		p, ok := t.Part(name)
		if !ok {
			return nil
		}
		for _, v := range p.Variations {
			for _, m := range ref.FindAllStringSubmatch(v, -1) {
				if err := visit(m[1], append(slices.Clip(path), name)); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return reached, visit(TemplateOrigin, nil)
}

// Expand puts a message together from origin: pick tells which of n
// variations to take, from 0 to n-1, and fields what the app and the
// model filled in. What fields hold is put in as it is, never read for
// #name# again, so whatever the model wrote stays text. Expand trusts a
// template that passed Check.
func (t Template) Expand(fields map[string]string, pick func(n int) int) string {
	var b strings.Builder
	var expand func(name string, depth int)
	expand = func(name string, depth int) {
		if v, ok := fields[name]; ok && isField(name) {
			b.WriteString(v)
			return
		}
		p, ok := t.Part(name)
		if !ok || depth > maxDepth || len(p.Variations) == 0 {
			return
		}
		i := pick(len(p.Variations))
		if i < 0 || i >= len(p.Variations) {
			i = 0
		}
		v := p.Variations[i]
		last := 0
		for _, m := range ref.FindAllStringSubmatchIndex(v, -1) {
			b.WriteString(v[last:m[0]])
			expand(v[m[2]:m[3]], depth+1)
			last = m[1]
		}
		b.WriteString(v[last:])
	}
	expand(TemplateOrigin, 0)
	return b.String()
}

// MarshalJSON writes the template as Tracery does, an object of parts,
// in Tim's order.
func (t Template) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, p := range t.Parts {
		if i > 0 {
			b.WriteByte(',')
		}
		name, err := json.Marshal(p.Name)
		if err != nil {
			return nil, err
		}
		vs, err := json.Marshal(nonNil(p.Variations))
		if err != nil {
			return nil, err
		}
		b.Write(name)
		b.WriteByte(':')
		b.Write(vs)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// UnmarshalJSON reads a template written the way of Tracery, keeping the
// order of its parts. Keys starting with _ are notes and left out.
func (t *Template) UnmarshalJSON(raw []byte) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if tok != json.Delim('{') {
		return errors.New("a template is an object of parts")
	}
	var parts []Part
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		name, _ := tok.(string)
		var vs []string
		if strings.HasPrefix(name, "_") {
			var skip json.RawMessage
			if err := dec.Decode(&skip); err != nil {
				return err
			}
			continue
		}
		if err := dec.Decode(&vs); err != nil {
			return fmt.Errorf("the part %s is not a list of variations: %w", name, err)
		}
		parts = append(parts, Part{Name: name, Variations: vs})
	}
	if _, err := dec.Token(); err != nil {
		return err
	}
	t.Parts = parts
	return nil
}

// FirstMessageTemplate is the template a draft is put together from now.
func (d *Data) FirstMessageTemplate() Template {
	if d.Template == nil {
		return DefaultTemplate()
	}
	return d.Template.clone()
}

func (t Template) clone() Template {
	out := Template{Parts: make([]Part, len(t.Parts))}
	for i, p := range t.Parts {
		out.Parts[i] = Part{Name: p.Name, Variations: slices.Clone(p.Variations)}
	}
	return out
}

// SetTemplatePart gives a part of the template its variations, a new part
// at position at, or at the end when at is out of range. A part that
// exists keeps its place. The template that comes of it must pass Check.
func (d *Data) SetTemplatePart(name string, variations []string, at int) error {
	t := d.FirstMessageTemplate()
	p := Part{Name: name, Variations: slices.Clone(nonNil(variations))}
	if i := t.index(name); i >= 0 {
		t.Parts[i] = p
	} else {
		if at < 0 || at > len(t.Parts) {
			at = len(t.Parts)
		}
		t.Parts = slices.Insert(t.Parts, at, p)
	}
	return d.setTemplate(t)
}

// ErrNoPart is a part asked for that the template does not have.
var ErrNoPart = errors.New("the template has no part")

// RemoveTemplatePart takes a part out of the template. A part another
// still uses, or origin, stays.
func (d *Data) RemoveTemplatePart(name string) error {
	t := d.FirstMessageTemplate()
	i := t.index(name)
	if i < 0 {
		return fmt.Errorf("%w %s", ErrNoPart, name)
	}
	if name == TemplateOrigin {
		return errors.New("origin is where the message starts, so it stays")
	}
	t.Parts = slices.Delete(t.Parts, i, i+1)
	for _, p := range t.Parts {
		for _, v := range p.Variations {
			for _, m := range ref.FindAllStringSubmatch(v, -1) {
				if m[1] == name {
					return fmt.Errorf("%s still uses #%s#. Take it out there first", p.Name, name)
				}
			}
		}
	}
	return d.setTemplate(t)
}

// ResetTemplate puts Tim's template of 21 November 2026 back.
func (d *Data) ResetTemplate() {
	d.Template = nil
}

// SetTemplate puts a whole template in place, the one before a reset when
// it is undone. It must pass Check.
func (d *Data) SetTemplate(t Template) error {
	return d.setTemplate(t.clone())
}

func (d *Data) setTemplate(t Template) error {
	if err := t.Check(); err != nil {
		return err
	}
	if bytes.Equal(must(json.Marshal(t)), must(json.Marshal(DefaultTemplate()))) {
		d.Template = nil
		return nil
	}
	d.Template = &t
	return nil
}

func must(b []byte, err error) []byte {
	if err != nil {
		panic(err)
	}
	return b
}
