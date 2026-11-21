package draft

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The hook is what the language model writes of a first message: what
// the person is, why they caught Tim's eye and a short reaction. The rest
// of the message comes from the template in Settings, see
// store.Template.

// HookSchema is the JSON the model has to answer with, Tim's schema of 21
// November 2026 with the portrait he asked for the same day. llama-server
// holds the model to it, and Hook checks it again. It is not in Settings,
// because the app reads exactly these fields.
const HookSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "Hook",
  "type": "object",
  "additionalProperties": false,
  "required": ["reason", "reaction", "portrait"],
  "properties": {
    "reason": { "type": "string", "minLength": 5, "maxLength": 120 },
    "reaction": { "type": "string", "maxLength": 60 },
    "portrait": { "type": "string", "minLength": 2, "maxLength": 40 }
  }
}`

// Hook is the model's answer once it passed the schema.
type Hook struct {
	Reason, Reaction, Portrait string
}

// Missing is what a draft puts where the model's part should be when its
// answer did not fit, for Tim to write himself.
const Missing = "…"

// ParseHook reads the model's answer and checks it against HookSchema.
// The texts come back on one line each, without characters that cannot be
// written, because they go into the middle of a sentence. They are text,
// and stay text: nothing in them is read as a part of the template.
func ParseHook(answer string) (Hook, error) {
	dec := json.NewDecoder(strings.NewReader(strings.TrimSpace(answer)))
	dec.DisallowUnknownFields()
	var raw struct {
		Reason   *string `json:"reason"`
		Reaction *string `json:"reaction"`
		Portrait *string `json:"portrait"`
	}
	if err := dec.Decode(&raw); err != nil {
		return Hook{}, fmt.Errorf("it is not the JSON the schema asks for: %s", why(err))
	}
	if dec.More() {
		return Hook{}, errors.New("it wrote more after the JSON")
	}
	for _, f := range []struct {
		name     string
		v        *string
		min, max int
	}{{"reason", raw.Reason, 5, 120}, {"reaction", raw.Reaction, 0, 60}, {"portrait", raw.Portrait, 2, 40}} {
		if f.v == nil {
			return Hook{}, fmt.Errorf("it left out %s", f.name)
		}
		*f.v = oneLine(*f.v)
		switch n := utf8.RuneCountInString(*f.v); {
		case n < f.min:
			return Hook{}, fmt.Errorf("its %s has %d letters, at least %d are asked for", f.name, n, f.min)
		case n > f.max:
			return Hook{}, fmt.Errorf("its %s has %d letters, at most %d fit", f.name, n, f.max)
		}
	}
	return Hook{Reason: *raw.Reason, Reaction: *raw.Reaction, Portrait: *raw.Portrait}, nil
}

// why says in a few words what was wrong with the JSON, never the model's
// words themselves, which may name the person.
func why(err error) string {
	var syntax *json.SyntaxError
	var kind *json.UnmarshalTypeError
	switch {
	case errors.Is(err, io.ErrUnexpectedEOF):
		return "it stops before the JSON ends"
	case errors.As(err, &syntax):
		return fmt.Sprintf("it is not JSON from character %d on", syntax.Offset)
	case errors.As(err, &kind):
		return fmt.Sprintf("%s is not text", kind.Field)
	case strings.Contains(err.Error(), "unknown field"):
		return "it has a field the schema does not know"
	case errors.Is(err, io.EOF):
		return "it is empty"
	}
	return "it cannot be read"
}

// oneLine trims the text, turns every run of white space, line breaks
// too, into one space and leaves out characters that cannot be written.
func oneLine(s string) string {
	var b bytes.Buffer
	space := false
	for _, r := range strings.TrimSpace(s) {
		switch {
		case unicode.IsSpace(r):
			space = true
		case unicode.IsControl(r), r == utf8.RuneError, unicode.Is(unicode.Cf, r) && r != '‍':
			// Not written, like a zero width space. The joiner stays,
			// because emojis are made with it.
		default:
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = false
			b.WriteRune(r)
		}
	}
	return b.String()
}
