package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/timmrkz/speakertrail/internal/place"
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
	{HomeCity, `""`, "Where you are, a city in NRW like Köln. People nearer to it come first"},
	{FirstMessagePrompt, mustJSON(DefaultFirstMessagePrompt), "What the language model is asked, to write the portrait, the reason and the reaction of a first message. The rest comes from the template. {{profile}} is where the profile goes, {{notes}} your notes on them"},
}

// HomeCity is the setting that says where Tim is, the city distances
// for the fit are measured from, see docs/fit-score.md.
const HomeCity = "home_city"

// FirstMessagePrompt is the setting that holds the prompt the language
// model writes the hook of a first message from, see internal/draft. Its
// fields are written {{profile}}, {{name}}, {{first_name}} and {{notes}}.
// It was first_message_prompt while the model wrote the whole message,
// and a prompt kept under that name is dropped when the file is read,
// since it asks for something else.
const FirstMessagePrompt = "hook_prompt"

// ProfileField is the field of the prompt where the profile goes. A prompt
// without it would have the model write to nobody in particular.
const ProfileField = "{{profile}}"

// DefaultFirstMessagePrompt is the prompt until Tim puts his own in, the
// one he wrote on 21 November 2026 with the examples he added, and the
// portrait he asked for the same day, as he then changed it in Settings.
const DefaultFirstMessagePrompt = `You write one short German line for a LinkedIn message, that  Tim sends to new LinkedIn contacts. It finishes this sentence: "Du bist mir in meinem LinkedIn Feed aufgefallen, weil …"

Tim makes the podcast "My First Memory". He likes real products, sport, people who just start things and people who bring others together.

Return JSON with three fields:
- reason: the part after "weil". End with "." or "!".
- reaction: an optional short sentence. Usually empty.
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

Profile: Neurologin, war Chefärztin, jetzt selbstständig
Tim's note: none
Output: {"reason":"du als Neurologin den Schritt in die Selbstständigkeit gewagt hast.","reaction":"","portrait":"Neurologin"}

Profile: CEO eines Herstellers von Babytragen
Tim's note: hardware is tough, we need more of that
Output: {"reason":"du ein Unternehmen mit echten Produkten zum Anfassen führst.","reaction":"Das ist richtig schwer. Und das ist richtig toll!","portrait":"Unternehmerin"}

Profile: Eisbaden-Coach, vorher 24 Jahre TV-Produktion
Tim's note: none
Output: {"reason":"du mehr kaltes Wasser unter die Menschen bringst! 😄","reaction":"","portrait":"Coach"}

Profile: Geschäftsführer einer Sportstiftung
Tim's note: none
Output: {"reason":"du dich für Sport und Menschen einsetzt.","reaction":"Das sind auch für mich Herzens-Themen.","portrait":"Macher"}

Profile: Mitgründerin eines Klimarechners für die Landwirtschaft
Tim's note: none
Output: {"reason":"ich digitale Landwirtschaft richtig mega finde!","reaction":"","portrait":"Gründerin"}

Profile: Gründerin eines jungen Startups, postet über den Aufbau
Tim's note: none
Output: {"reason":"du etwas in der Welt bewegen möchtest, und einfach anpackst.","reaction":"Davon gibt's viel zu wenige!","portrait":"Gründerin"}

Profile: Gründer einer Plattform, die Software automatisch in Produktion bringt
Tim's note: I like the first principles thinking. He builds something and automates things
Output: {"reason":"du Dinge von Grund auf durchdenkst und sie dann einfach automatisierst.","reaction":"","portrait":"Gründer"}

Profile: Mitgründer eines Unternehmens, das für Firmen Wälder in Deutschland pflanzt
Tim's note: none
Output: {"reason":"Bäume ja doch schon voll gut sind! 😄","reaction":"","portrait":"Gründer"}

Profile: Gründer einer Agentur, die Marken in Google und ChatGPT sichtbar macht
Tim's note: none
Output: {"reason":"du dich damit beschäftigst, wie wir Dinge finden.","reaction":"Das Thema find ich mega spannend!","portrait":"Unternehmer"}

Profile: Führt mit ihren Geschwistern ein Süßwaren-Familienunternehmen in vierter Generation
Tim's note: none
Output: {"reason":"ihr ein Familienunternehmen pflegt, und weil damit ganz viel Verantwortung verbunden ist.","reaction":"","portrait":"Unternehmerin"}

Profile: Mitgründerin eines Labels, das Kleidung aus Stoffresten macht
Tim's note: none
Output: {"reason":"ihr versucht ein total schwieriges Recycling Problem zu lösen.","reaction":"","portrait":"Gründerin"}

Profile: Geschäftsführerin eines Feinkostladens mit eigener Schokoladenmarke, seit 2019
Tim's note: none
Output: {"reason":"du seit Jahren einen echten Laden führst.","reaction":"Und dann auch noch mit eigener Schokolade! 😄","portrait":"Ladeninhaberin"}

Profile: Mitgründer einer Agentur für Videoproduktion und Social Media Content
Tim's note: none
Output: {"reason":"ich selbst Videos produziere.","reaction":"Und ich weiß, wie viel Arbeit in gutem Content steckt.","portrait":"Macher"}

Profile: Gründerin einer Software, die Behördenprozesse vereinfacht
Tim's note: none
Output: {"reason":"du eine der wenigen bist, die etwas gegen diversen bürokratischen Wahnsinn macht. 🫠","reaction":"","portrait":"Gründerin"}

Profile: Immobilienmaklerin, nach über 20 Jahren als Angestellte seit 2017 selbstständig
Tim's note: none
Output: {"reason":"du nach vielen Jahren im Angestelltenverhältnis den Schritt in die Selbstständigkeit gewagt hast.","reaction":"Das find ich richtig stark!","portrait":"Maklerin"}

Profile:
{{profile}}

Tim's note: {{notes}}
Output:`

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
		if err := Invisible(prompt, "\n\t"); err != nil {
			return fmt.Errorf("the prompt holds %w", err)
		}
	}
	if key == HomeCity {
		var city string
		if err := json.Unmarshal(value, &city); err != nil {
			return err
		}
		if city = strings.TrimSpace(city); city != "" {
			known := place.CityOf(city)
			if _, ok := place.Km(known, known); !ok {
				return fmt.Errorf("%q is no city in NRW the app knows, like Köln, Bonn or Düsseldorf", city)
			}
			city = known
		}
		value = json.RawMessage(mustJSON(city))
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
