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
	{FirstMessagePrompt, mustJSON(DefaultFirstMessagePrompt), "What the language model is asked to write a first message from. {{profile}} is where the profile goes"},
}

// FirstMessagePrompt is the setting that holds the prompt a first message
// is drafted from, see internal/draft. Its fields are written
// {{profile}}, {{name}}, {{first_name}} and {{notes}}.
const FirstMessagePrompt = "first_message_prompt"

// ProfileField is the field of the prompt where the profile goes. A prompt
// without it would have the model write to nobody in particular.
const ProfileField = "{{profile}}"

// DefaultFirstMessagePrompt is the prompt until Tim puts his own in.
const DefaultFirstMessagePrompt = `You write the first LinkedIn message from Tim to {{name}}, a new connection he just made.

Tim hosts the podcast "My First Memory". Each guest tells the story of their earliest memory and what it says about who they became. Tim looks for people in North Rhine-Westphalia who run their own business.

Write the message in the language of the profile, German when in doubt, and use "du" in German. Three to five short sentences. Thank them for connecting, name one specific thing from their profile that caught Tim's interest, and ask whether they would like to hear more about being a guest. No subject line, no hashtags, no emojis, no flattery. Address them by first name, {{first_name}}. Sign with "Tim". Write only the message.

Tim's notes about them:
{{notes}}

Their LinkedIn profile:
{{profile}}`

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
