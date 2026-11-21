package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Default is a setting with its value until Tim changes it.
type Default struct {
	Key         string
	Value       string
	Description string
}

// Defaults are every setting there is, in the order Settings shows them.
var Defaults = []Default{
	{"profile_searches_per_run", "10", "Due profile searches a run takes. Each is one call to Exa"},
	{"profile_search_results", "25", "Profiles one profile search asks for"},
	{"profile_search_check_days", "30", "Days between two runs of the same profile search"},
	{"profile_active_days", "183", "Days within which a person must have been active on LinkedIn to be kept"},
	{"exa_monthly_searches", "1000", "Exa searches per month. Its free $10 a month covers about 1,400"},
	{"probation_weeks", "3", "Weeks a new search may find nobody before runs leave it out"},
	{"retire_after_empty_checks", "4", "Runs in a row without anyone new before a search is left out"},
	{FirstMessagePrompt, mustJSON(DefaultFirstMessagePrompt), "What the language model is asked, to write the portrait, the reason and the reaction of a first message. The rest comes from the template. {{profile}} is where the profile goes, {{notes}} your notes on them"},
}

// FirstMessagePrompt is the setting that holds the prompt the language
// model writes the hook of a first message from, see internal/draft. Its
// fields are written {{profile}}, {{name}}, {{first_name}} and {{notes}}.
// It was first_message_prompt while the model wrote the whole message,
// and a prompt kept under that name is left alone, since it asks for
// something else.
const FirstMessagePrompt = "hook_prompt"

// ProfileField is the field of the prompt where the profile goes. A prompt
// without it would have the model write to nobody in particular.
const ProfileField = "{{profile}}"

// DefaultFirstMessagePrompt is the prompt until Tim puts his own in, the
// one he wrote on 21 November 2026, with the portrait he asked for the
// same day.
const DefaultFirstMessagePrompt = `You write one short German line for a LinkedIn message from Tim.
It finishes this sentence: "Du bist mir in meinem LinkedIn Feed aufgefallen, weil …"

Tim makes the podcast "My First Memory" and coaches kids in Brazilian Jiu-Jitsu. He likes real products, sport, people who just start things and people who bring others together.

Return JSON with three fields:
- reason: the part after "weil". Verb at the end. End with "." or "!".
- reaction: an optional short sentence. Usually "".
- portrait: what they are, in one or two German words, the way they show themselves in their profile, like "Coach", "Lehrerin", "Macher" or "Unternehmerin". It finishes "um deine Geschichte als … zu erzählen".
Reason and reaction together, under 150 characters.

Rules:
- If Tim wrote a note, use his words.
- Use their main work only. Say what it means, not what it is.
- Never repeat their headline, numbers, product description or mission.
- Only claim what the profile shows.
- Keep it plain, warm and casual. No wordplay. No jokes about the person.
- Never mention family, health, private life or their employer.
- No semicolons. No dashes. Never use the word "Erinnerung". Use du.
- Add an emoji only after a light line ending in "!".

Examples:
Neurologin, war Chefärztin, jetzt selbstständig
{"reason":"du als Neurologin den Schritt in die Selbstständigkeit gewagt hast.","reaction":"","portrait":"Neurologin"}

CEO eines Herstellers von Babytragen. Note: hardware is tough, we need more of that
{"reason":"du ein Unternehmen mit echten Produkten zum Anfassen führst.","reaction":"Das ist richtig schwer. Und das ist richtig toll!","portrait":"Unternehmerin"}

Eisbaden-Coach, vorher 24 Jahre TV-Produktion
{"reason":"du mehr kaltes Wasser unter die Menschen bringst! 😄","reaction":"","portrait":"Coach"}

Geschäftsführer einer Sportstiftung
{"reason":"du dich für Sport und Menschen einsetzt.","reaction":"Das sind auch für mich Herzens-Themen.","portrait":"Macher"}

Mitgründerin eines Klimarechners für die Landwirtschaft
{"reason":"ich digitale Landwirtschaft richtig mega finde!","reaction":"","portrait":"Gründerin"}

Profile:
{{profile}}

Tim's note:
{{notes}}`

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func defaultOf(key string) (Default, bool) {
	for _, d := range Defaults {
		if d.Key == key {
			return d, true
		}
	}
	return Default{}, false
}

// Setting is one setting as Settings shows it.
type Setting struct {
	Key         string          `json:"key"`
	Value       json.RawMessage `json:"value"`
	Description string          `json:"description"`
	UpdatedAt   time.Time       `json:"updated_at,omitzero"`
}

// AllSettings lists every setting with its value now.
func (d *Data) AllSettings() []Setting {
	out := make([]Setting, 0, len(Defaults))
	for _, def := range Defaults {
		s := Setting{Key: def.Key, Value: json.RawMessage(def.Value), Description: def.Description}
		if v, ok := d.Settings[def.Key]; ok {
			s.Value, s.UpdatedAt = v.Value, v.UpdatedAt
		}
		out = append(out, s)
	}
	return out
}

// Setting gives one setting.
func (d *Data) Setting(key string) (Setting, bool) {
	for _, s := range d.AllSettings() {
		if s.Key == key {
			return s, true
		}
	}
	return Setting{}, false
}

// SetSetting changes a setting. The value must be of the same kind as its
// default, a whole number where the default is one, and a value equal to
// the default takes the change back.
func (d *Data) SetSetting(key string, value json.RawMessage, at time.Time) error {
	def, ok := defaultOf(key)
	if !ok {
		return fmt.Errorf("there is no setting %s", key)
	}
	if kindOf(value) != kindOf(json.RawMessage(def.Value)) {
		return fmt.Errorf("%s takes a %s", key, kindOf(json.RawMessage(def.Value)))
	}
	if key == FirstMessagePrompt {
		var prompt string
		if err := json.Unmarshal(value, &prompt); err != nil || !strings.Contains(prompt, ProfileField) {
			return fmt.Errorf("the prompt needs %s, where the profile goes", ProfileField)
		}
	}
	if kindOf(value) == "number" {
		var n float64
		if err := json.Unmarshal(value, &n); err != nil || n < 0 {
			return fmt.Errorf("%s takes a number of 0 or more", key)
		}
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, value); err != nil {
		return err
	}
	if compact.String() == def.Value {
		delete(d.Settings, key)
		return nil
	}
	d.Settings[key] = SettingValue{Value: compact.Bytes(), UpdatedAt: at}
	return nil
}

func kindOf(raw json.RawMessage) string {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return "nothing"
	}
	switch v.(type) {
	case float64:
		return "number"
	case bool:
		return "yes or no"
	case string:
		return "text"
	case []any:
		return "list"
	case map[string]any:
		return "set of values"
	}
	return "nothing"
}

// Int gives a setting as a whole number.
func (d *Data) Int(key string) int {
	s, _ := d.Setting(key)
	var f float64
	if json.Unmarshal(s.Value, &f) != nil {
		return 0
	}
	return int(f)
}

// Days gives a setting in days as a duration.
func (d *Data) Days(key string) time.Duration {
	return time.Duration(d.Int(key)) * 24 * time.Hour
}

// Text gives a setting that is text.
func (d *Data) Text(key string) string {
	s, _ := d.Setting(key)
	var t string
	if json.Unmarshal(s.Value, &t) != nil {
		return ""
	}
	return t
}
