// Package store keeps everything the app knows in one plain JSON file in
// the user's own folder: the people, the searches, the runs, the settings
// and how much of each search provider's budget is spent. There is no
// database. The whole of it is held in memory, every change is written
// at once, to a new file that then takes the old one's place, so a crash
// in the middle of a write leaves the last good file behind.
//
// Each person's profile text, which is most of the size, lives in a
// second file, TextsName, written only when a text arrives, so the
// changes made all day stay quick to write.
//
// It is safe to use from several goroutines. Read and Update hand out the
// data under one lock, and nothing handed out may be kept after they
// return.
package store

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/timmrkz/speakertrail/internal/rubric"
)

// Version is the shape of the file. A file of a newer version is not
// opened, so an older build never writes over what a newer one kept.
const Version = 1

// FileName is the file in the app's folder.
const FileName = "speakertrail.json"

// TextsName is the file of the profile texts beside it, by profile link.
// Tim chose it on 20 November 2026 over the texts in the one file, which
// made every save about ten times slower.
const TextsName = "profile-texts.json"

// textsFile is what TextsName holds.
type textsFile struct {
	Version int                    `json:"version"`
	Texts   map[string]profileText `json:"texts"`
}

type profileText struct {
	Text string    `json:"text"`
	At   time.Time `json:"at"`
}

// Data is everything the app keeps.
type Data struct {
	Version int `json:"version"`
	// LastID is the last id given to a person, a search or a run. Ids are
	// never given twice.
	LastID   int64     `json:"last_id"`
	People   []*Person `json:"people"`
	Searches []*Search `json:"searches"`
	// Exclusions are Tim's words and phrases that leave a person out, see
	// exclusions.go. Missing from a file before they existed, and then
	// the starting ones are added.
	Exclusions []*Exclusion `json:"exclusions"`
	Runs       []*Run       `json:"runs"`
	// Settings holds the settings changed from their defaults, see
	// settings.go.
	Settings map[string]SettingValue `json:"settings"`
	// LinkedInOpens is the browser profile LinkedIn opens in, chosen in
	// Settings, see internal/browser. Empty until Tim chooses one.
	LinkedInOpens Where `json:"linkedin_opens"`
	// Calls counts the calls made to each search provider, by month, like
	// "exa" -> "2026-11" -> 12. Every call counts, a failed one too,
	// because it may be billed.
	Calls map[string]map[string]int `json:"calls"`
	// Started says the starting searches were added once, so a search
	// removed later does not come back.
	Started bool `json:"started"`
	// DraftTimes are how long the last drafts took, newest last, so the
	// next one says about how long is left from earlier work rather than
	// from a guess. See internal/draft.
	DraftTimes []DraftTime `json:"draft_times,omitempty"`
	// LanguageModel is the file name of the language model picked in
	// Settings to draft with, downloaded or not. Empty until Tim picks one,
	// and then the largest on the Mac drafts.
	LanguageModel string `json:"language_model,omitempty"`
	// CutRead says the people whose profile text Exa cut were marked once
	// to be read again, see readCutAgain.
	CutRead bool `json:"cut_read,omitempty"`
	// Template is the template a first message is put together from, as
	// Tim changed it in Settings, nil while it is his first one. See
	// template.go.
	Template *Template `json:"first_message_template,omitempty"`

	// textsChanged says a profile text arrived since the texts were
	// written, see SetProfileText.
	textsChanged bool
}

// DraftTime is how long one draft of a first message took: loading the
// model, when it was not loaded, and writing the message.
type DraftTime struct {
	LoadMS  int64 `json:"load_ms,omitempty"`
	WriteMS int64 `json:"write_ms"`
}

// KeptDraftTimes is how many of the last drafts are kept to tell the time
// of the next.
const KeptDraftTimes = 10

// Drafted keeps how long a draft took, and lets the oldest go.
func (d *Data) Drafted(t DraftTime) {
	d.DraftTimes = append(d.DraftTimes, t)
	if n := len(d.DraftTimes); n > KeptDraftTimes {
		d.DraftTimes = append([]DraftTime(nil), d.DraftTimes[n-KeptDraftTimes:]...)
	}
}

// Where is a browser and the folder of one of its profiles, like
// "Google Chrome" and "Profile 1".
type Where struct {
	App    string `json:"app"`
	Folder string `json:"folder"`
}

// SettingValue is a setting Tim changed, and when.
type SettingValue struct {
	Value     json.RawMessage `json:"value"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// Person is someone a profile search found, or Tim connected with.
type Person struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Headline string `json:"headline"`
	City     string `json:"city"`
	// Connected says Tim connected with the person on LinkedIn, and
	// since when. Whoever he did not connect with is simply open.
	Connected   bool      `json:"connected"`
	ConnectedAt time.Time `json:"connected_at,omitzero"`
	// Decision and DecidedAt are what a file from before connections
	// says: kept became connected, skipped open again. They are read
	// once, then dropped.
	Decision  string    `json:"decision,omitempty"`
	DecidedAt time.Time `json:"decided_at,omitzero"`
	Notes     string    `json:"notes"`
	// Evidence says how the person was found, like "Found by the profile
	// search Yogalehrerin Köln".
	Evidence   string    `json:"evidence"`
	FirstSeen  time.Time `json:"first_seen"`
	UpdatedAt  time.Time `json:"updated_at"`
	FirstRunID int64     `json:"first_run_id,omitempty"`
	// LinkedIn is the profile that found them. The engine never opens it,
	// it is what the search provider's own index says.
	LinkedIn LinkedIn `json:"linkedin"`
	// Draft is the first message to them, as the language model wrote it
	// at DraftedAt and as Tim changed it since. Tim copies it into
	// LinkedIn and sends it himself. See internal/draft.
	Draft     string    `json:"draft,omitempty"`
	DraftedAt time.Time `json:"drafted_at,omitzero"`
	// Signals are the fit rubric's reasons for and against, and Score
	// what they add up to.
	Signals []Signal `json:"signals"`
	Score   int      `json:"score"`
	// Rubric is the version of the rubric that scored them.
	Rubric int `json:"rubric"`
	// FoundBy are the searches that found them, each with the last time.
	FoundBy []Sighting `json:"found_by"`
	// ToRead says the next run reads their profile from Exa: Tim added
	// them from a link and no run has read it yet, and until one does they
	// are named by the handle in the link, or their text was cut, see
	// readCutAgain.
	ToRead bool `json:"to_read,omitempty"`
	// ReadError says why the last read of their profile failed, like a
	// profile Exa's index does not hold yet.
	ReadError string `json:"read_error,omitempty"`
}

// LinkedIn is what a LinkedIn profile says, as the search provider's
// index has it.
type LinkedIn struct {
	URL      string `json:"url"`
	Handle   string `json:"handle"`
	Headline string `json:"headline"`
	// Review is confirmed, or rejected when Tim said it is not theirs.
	Review string `json:"review"`
	// BornAbout is the likely year of birth, zero when unknown, and
	// AgeReason the step on the profile that shows it.
	BornAbout int    `json:"born_about,omitempty"`
	AgeReason string `json:"age_reason,omitempty"`
	// Roles are the roles held now, like "Inhaberin, Studio Beispiel".
	Roles string `json:"roles,omitempty"`
	// LastActive is the newest post, comment or reaction.
	LastActive time.Time `json:"last_active,omitzero"`
	// Text is the whole profile as Exa's index had it at TextAt, or as Tim
	// pasted it from his browser, see Data.Paste: the headline, the About,
	// the jobs, the education and the latest activity, at most 8000
	// characters. A first message is written from it. Exa's index is not
	// LinkedIn now, so its copy can be weeks old. It is
	// kept in the file TextsName, never in the one file, and changed only
	// with Data.SetProfileText.
	Text   string    `json:"-"`
	TextAt time.Time `json:"-"`
	// LastWrote is the newest post or comment Text lists, and Wrote says
	// whether Text lists any activity at all, so its kinds are known. Both
	// are read from Text whenever it is set, never kept.
	LastWrote time.Time `json:"-"`
	Wrote     bool      `json:"-"`
}

// Signal is one reason for or against a fit, with the passage that shows
// it.
type Signal struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	For     bool   `json:"for"`
	Passage string `json:"passage"`
	FoundIn string `json:"found_in"`
}

// Sighting is a search that found a person, and when it last did.
type Sighting struct {
	SearchID int64     `json:"search_id"`
	At       time.Time `json:"at"`
}

// Search is a profile search: a kind of person and a place, like
// "Yogalehrerin Köln".
type Search struct {
	ID    int64  `json:"id"`
	Query string `json:"query"`
	// Status is candidate before its first run, then active while it
	// finds people, probation when it found nobody yet, and retired when
	// it stopped finding anyone or Tim paused it.
	Status        string    `json:"status"`
	StatusChanged time.Time `json:"status_changed"`
	// Points put a search first among those due at the same time.
	Points    float64   `json:"points"`
	CreatedAt time.Time `json:"created_at"`
	Notes     string    `json:"notes,omitempty"`
	// From is the search a run made this one from, in another city, see
	// Spread. Empty for a search Tim added or one the app began with.
	From string `json:"from,omitempty"`
	// LastRun and NextRun are when the search ran last and is due next.
	LastRun    time.Time `json:"last_run,omitzero"`
	NextRun    time.Time `json:"next_run,omitzero"`
	Runs       int       `json:"runs"`
	EmptyInRow int       `json:"empty_in_row"`
	// Last is what its last run brought.
	Last *Check `json:"last,omitempty"`
}

// Run is one pass over the due searches, started by hand.
type Run struct {
	ID         int64     `json:"id"`
	Kind       string    `json:"kind"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at,omitzero"`
	// Ended is empty for a run that went to its end, stopped when Tim
	// stopped it, and interrupted when the app closed under it.
	Ended string `json:"ended,omitempty"`
	// Searches are the ids the run takes, in order.
	Searches []int64 `json:"searches"`
	Checks   []Check `json:"checks"`
	// Links are the people whose profiles the run reads, by id, those Tim
	// added from links. Reads is what came of each read so far.
	Links []int64 `json:"links,omitempty"`
	Reads []Read  `json:"reads,omitempty"`
}

// Read is one profile read from Exa's index in a run.
type Read struct {
	PersonID   int64     `json:"person_id"`
	At         time.Time `json:"at"`
	DurationMS int64     `json:"duration_ms"`
	Error      string    `json:"error,omitempty"`
}

// Check is one search's part in a run.
type Check struct {
	SearchID    int64     `json:"search_id"`
	At          time.Time `json:"at"`
	DurationMS  int64     `json:"duration_ms"`
	PeopleFound int       `json:"people_found"`
	PeopleNew   int       `json:"people_new"`
	Error       string    `json:"error,omitempty"`
}

// Store is the open file.
type Store struct {
	path, texts string
	mu          sync.Mutex
	data        *Data
	now         func() time.Time
	// hist is the undo and redo of Tim's changes, see history.go.
	hist history
}

// Open reads the file in dir, or starts a new one there. now is the clock,
// nil for the real one.
func Open(dir string, now func() time.Time) (*Store, error) {
	if now == nil {
		now = time.Now
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	s := &Store{path: filepath.Join(dir, FileName), texts: filepath.Join(dir, TextsName), now: now}
	raw, err := os.ReadFile(s.path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		s.data = &Data{Version: Version}
	case err != nil:
		return nil, err
	default:
		d := &Data{}
		if err := json.Unmarshal(raw, d); err != nil {
			return nil, fmt.Errorf("%s cannot be read: %w", s.path, err)
		}
		if d.Version > Version {
			return nil, fmt.Errorf("%s was written by a newer Speaker Trail, version %d. Update the app", s.path, d.Version)
		}
		s.data = d
	}
	s.data.fill()
	if err := s.readTexts(raw); err != nil {
		return nil, err
	}
	s.data.readCutAgain()
	return s, nil
}

// readTexts gives each person their profile text from TextsName. Builds
// of pull request 38 before 20 November 2026 kept the texts in the one
// file, raw. Those move over, and both files are written at once.
func (s *Store) readTexts(raw []byte) error {
	texts := textsFile{}
	got, err := os.ReadFile(s.texts)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return err
	default:
		if err := json.Unmarshal(got, &texts); err != nil {
			return fmt.Errorf("%s cannot be read: %w", s.texts, err)
		}
	}
	var old struct {
		People []struct {
			LinkedIn struct {
				URL    string    `json:"url"`
				Text   string    `json:"text"`
				TextAt time.Time `json:"text_at"`
			} `json:"linkedin"`
		} `json:"people"`
	}
	if raw != nil {
		_ = json.Unmarshal(raw, &old)
	}
	moved := false
	for _, p := range old.People {
		l := p.LinkedIn
		if l.Text == "" {
			continue
		}
		if t, ok := texts.Texts[l.URL]; !ok || t.At.Before(l.TextAt) {
			if texts.Texts == nil {
				texts.Texts = map[string]profileText{}
			}
			texts.Texts[l.URL] = profileText{Text: l.Text, At: l.TextAt}
		}
		moved = true
	}
	s.data.attach(texts.Texts)
	if !moved {
		return nil
	}
	s.data.textsChanged = true
	return s.save()
}

// attach puts the texts on the people they are of.
func (d *Data) attach(texts map[string]profileText) {
	for _, p := range d.People {
		if t, ok := texts[p.LinkedIn.URL]; ok && p.LinkedIn.URL != "" {
			p.LinkedIn.Text, p.LinkedIn.TextAt = t.Text, t.At
			p.LinkedIn.LastWrote, p.LinkedIn.Wrote = rubric.Writing(t.Text)
		}
	}
}

// profileTexts are the texts of the people here now, by profile link.
func (d *Data) profileTexts() map[string]profileText {
	out := map[string]profileText{}
	for _, p := range d.People {
		if l := p.LinkedIn; l.Text != "" && l.URL != "" {
			out[l.URL] = profileText{Text: l.Text, At: l.TextAt}
		}
	}
	return out
}

// SetProfileText keeps the text of a person's profile as Exa's index had
// it at at, to be written to TextsName with the change it is part of. The
// same text again changes nothing, so a check that finds people already
// here does not write the texts, and at stays when the text was first
// read as it is.
func (d *Data) SetProfileText(p *Person, text string, at time.Time) {
	if p.LinkedIn.Text == text {
		return
	}
	p.LinkedIn.Text, p.LinkedIn.TextAt = text, at
	p.LinkedIn.LastWrote, p.LinkedIn.Wrote = rubric.Writing(text)
	d.textsChanged = true
}

// fill makes the maps and lists that are missing, so the rest of the code
// never meets a nil map.
func (d *Data) fill() {
	d.Version = Version
	if d.Settings == nil {
		d.Settings = map[string]SettingValue{}
	}
	// Builds of pull request 12 before 14 November 2026 kept where
	// LinkedIn opens as two settings. Their choice moves over once.
	if v, ok := d.Settings["linkedin_profile"]; ok {
		var w Where
		_ = json.Unmarshal(v.Value, &w.Folder)
		w.App = "Google Chrome"
		if b, ok := d.Settings["linkedin_browser"]; ok {
			_ = json.Unmarshal(b.Value, &w.App)
		}
		if d.LinkedInOpens.Folder == "" && w.Folder != "" {
			d.LinkedInOpens = w
		}
	}
	delete(d.Settings, "linkedin_browser")
	delete(d.Settings, "linkedin_profile")
	// Builds of pull request 38 asked the model for a whole message with
	// first_message_prompt. Since 21 November 2026 it writes the hook
	// only, from hook_prompt, so a prompt kept under the old name goes.
	delete(d.Settings, "first_message_prompt")
	if d.Calls == nil {
		d.Calls = map[string]map[string]int{}
	}
	if d.People == nil {
		d.People = []*Person{}
	}
	if d.Searches == nil {
		d.Searches = []*Search{}
	}
	if d.Runs == nil {
		d.Runs = []*Run{}
	}
	if d.Exclusions == nil {
		d.Exclusions = []*Exclusion{}
		for _, phrase := range startingExclusions {
			d.Exclusions = append(d.Exclusions, &Exclusion{ID: d.NewID(), Phrase: phrase})
		}
	}
	for _, p := range d.People {
		if p.Decision == "kept" {
			p.Connected, p.ConnectedAt = true, p.DecidedAt
		}
		p.Decision, p.DecidedAt = "", time.Time{}
	}
}

// Path is where the file is.
func (s *Store) Path() string { return s.path }

// Now is the store's clock.
func (s *Store) Now() time.Time { return s.now() }

// Read hands the data to fn under the lock. fn must not change it.
func (s *Store) Read(fn func(d *Data)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(s.data)
}

// Update hands the data to fn under the lock and writes it when fn
// returns nil. When fn fails, or the write does, the data in memory is put
// back as it was, so memory and the files never disagree for long. The
// texts, written first, are written again at the next save.
func (s *Store) Update(fn func(d *Data) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.update(fn)
}

// update is Update with the lock held.
func (s *Store) update(fn func(d *Data) error) error {
	before, err := json.Marshal(s.data)
	if err != nil {
		return err
	}
	// The texts are not in before. Strings are never changed in place,
	// so holding them is cheap.
	texts := s.data.profileTexts()
	undo := func() {
		d := &Data{}
		if json.Unmarshal(before, d) == nil {
			d.fill()
			d.attach(texts)
			s.data = d
		}
	}
	if err := fn(s.data); err != nil {
		undo()
		return err
	}
	if err := s.save(); err != nil {
		wrote := s.data.textsChanged
		undo()
		// The texts may have been written before the one file failed.
		// The next save writes them again as memory has them, so the two
		// files agree again.
		s.data.textsChanged = wrote
		return err
	}
	return nil
}

// save writes the texts when one arrived, then the one file. The texts
// go by profile link, so texts written before a failed write of the one
// file never land on the wrong person.
func (s *Store) save() error {
	if s.data.textsChanged {
		// Not indented: nobody reads it by hand, and it is most of the
		// bytes.
		if err := write(s.texts, textsFile{Version: Version, Texts: s.data.profileTexts()}, ""); err != nil {
			return err
		}
	}
	if err := write(s.path, s.data, "\t"); err != nil {
		return err
	}
	s.data.textsChanged = false
	return nil
}

// write puts v in a new file beside the one at path, indented by indent,
// then moves it into place, which on one disk is all or nothing.
func write(path string, v any, indent string) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".speakertrail-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	// Encoded straight into the file, so megabytes of profile texts are
	// never held twice.
	buf := bufio.NewWriterSize(tmp, 1<<16)
	enc := json.NewEncoder(buf)
	enc.SetIndent("", indent)
	if err := enc.Encode(v); err != nil {
		tmp.Close()
		return err
	}
	if err := buf.Flush(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// NewID gives the next id.
func (d *Data) NewID() int64 {
	d.LastID++
	return d.LastID
}

// Person finds a person by id.
func (d *Data) Person(id int64) *Person {
	for _, p := range d.People {
		if p.ID == id {
			return p
		}
	}
	return nil
}

// PersonByProfile finds a person by their LinkedIn profile.
func (d *Data) PersonByProfile(url string) *Person {
	for _, p := range d.People {
		if p.LinkedIn.URL == url {
			return p
		}
	}
	return nil
}

// Search finds a search by id.
func (d *Data) Search(id int64) *Search {
	for _, s := range d.Searches {
		if s.ID == id {
			return s
		}
	}
	return nil
}

// Run finds a run by id.
func (d *Data) Run(id int64) *Run {
	for _, r := range d.Runs {
		if r.ID == id {
			return r
		}
	}
	return nil
}
