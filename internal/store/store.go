// Package store keeps everything the app knows in one plain JSON file in
// the user's own folder: the people, the searches, the runs, the settings
// and how much of each search provider's budget is spent. There is no
// database. The whole of it is held in memory, every change is written
// at once, to a new file that then takes the old one's place, so a crash
// in the middle of a write leaves the last good file behind.
//
// It is safe to use from several goroutines. Read and Update hand out the
// data under one lock, and nothing handed out may be kept after they
// return.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Version is the shape of the file. A file of a newer version is not
// opened, so an older build never writes over what a newer one kept.
const Version = 1

// FileName is the file in the app's folder.
const FileName = "speakertrail.json"

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
	// Calls counts the calls made to each search provider, by month, like
	// "exa" -> "2026-11" -> 12. Every call counts, a failed one too,
	// because it may be billed.
	Calls map[string]map[string]int `json:"calls"`
	// Started says the starting searches were added once, so a search
	// removed later does not come back.
	Started bool `json:"started"`
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
	// Signals are the fit rubric's reasons for and against, and Score
	// what they add up to.
	Signals []Signal `json:"signals"`
	Score   int      `json:"score"`
	// Rubric is the version of the rubric that scored them.
	Rubric int `json:"rubric"`
	// FoundBy are the searches that found them, each with the last time.
	FoundBy []Sighting `json:"found_by"`
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
	path string
	mu   sync.Mutex
	data *Data
	now  func() time.Time
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
	s := &Store{path: filepath.Join(dir, FileName), now: now}
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
	return s, nil
}

// fill makes the maps and lists that are missing, so the rest of the code
// never meets a nil map.
func (d *Data) fill() {
	d.Version = Version
	if d.Settings == nil {
		d.Settings = map[string]SettingValue{}
	}
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
// back as it was, so memory and the file never disagree.
func (s *Store) Update(fn func(d *Data) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	before, err := json.Marshal(s.data)
	if err != nil {
		return err
	}
	undo := func() {
		d := &Data{}
		if json.Unmarshal(before, d) == nil {
			d.fill()
			s.data = d
		}
	}
	if err := fn(s.data); err != nil {
		undo()
		return err
	}
	if err := s.write(); err != nil {
		undo()
		return err
	}
	return nil
}

// write puts the data in a new file beside the old one, then moves it into
// place, which on one disk is all or nothing.
func (s *Store) write() error {
	raw, err := json.MarshalIndent(s.data, "", "\t")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".speakertrail-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(raw); err != nil {
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
	return os.Rename(tmp.Name(), s.path)
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
