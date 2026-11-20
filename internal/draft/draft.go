// Package draft writes a first message to a person on the Mac: it takes
// the prompt from Settings and the person's profile text, asks the
// language model, and keeps what it wrote on the person, where Tim reads
// it, changes it, copies it into LinkedIn and sends it himself. Nothing
// is ever sent from the app.
//
// A draft is work that takes a while: reading the profile from Exa when
// the app has no text for the person yet, loading the model when it is not
// loaded, and writing. It runs in the background, one at a time, since
// one model answers one question at a time, and says how far it is about
// once a second, see Job.
package draft

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/timmrkz/speakertrail/internal/engine"
	"github.com/timmrkz/speakertrail/internal/language"
	"github.com/timmrkz/speakertrail/internal/search"
	"github.com/timmrkz/speakertrail/internal/store"
)

// Job is one draft as the window shows it.
type Job struct {
	ID       string `json:"id"`
	PersonID int64  `json:"person_id"`
	// State is queued, reading (the profile from Exa), loading (the
	// model), writing, done, failed or cancelled.
	State string `json:"state"`
	// Model is the title of the model that writes.
	Model string `json:"model,omitempty"`
	// Text is what the model wrote so far, and the whole message once
	// it is done. It is untrusted, and shown as text.
	Text string `json:"text,omitempty"`
	// StartedAt is when it began.
	StartedAt time.Time `json:"started_at"`
	// Fraction is how far it is from 0 to 1, and SecondsLeft about how
	// long is left, both from how long earlier drafts took. -1 and nil
	// while there is no earlier draft to measure against.
	Fraction    float64  `json:"fraction"`
	SecondsLeft *float64 `json:"seconds_left"`
	Error       string   `json:"error,omitempty"`
	// Seq is higher for every change than for the one before it, of any
	// draft, so the window keeps the newer of an answer and an event.
	Seq int64 `json:"seq"`
}

// Ended says whether the job is over.
func (j Job) Ended() bool {
	return j.State == "done" || j.State == "failed" || j.State == "cancelled"
}

// Finder reads one profile from Exa's index, see search.Exa.Profile.
type Finder interface {
	Profile(ctx context.Context, link string) (search.Profile, error)
}

// ErrNoPerson is a draft asked for someone who is not in the file.
var ErrNoPerson = errors.New("there is no person")

// ErrNoModel is a draft asked for before a language model is downloaded.
var ErrNoModel = errors.New("download a language model in Settings first. It writes the message on this Mac")

// Drafter drafts first messages, one at a time.
type Drafter struct {
	Store *store.Store
	// Host keeps the model loaded between drafts.
	Host *language.Host
	// Model is the model that drafts and where its file is, ErrNoModel
	// when none is downloaded.
	Model func() (language.Model, string, error)
	// Check says whether the model's file may be loaded, language.Check
	// in the app. Nil checks nothing.
	Check func(m language.Model) error
	// Finder reads a profile from Exa, nil while there is no key.
	Finder func() Finder
	// Ask asks the model, language.Ask when nil. A test puts its own.
	Ask func(ctx context.Context, s *language.Server, messages []language.Message, heard func(string)) (language.Answer, error)
	// Told hears every job whenever it changes, and about once a second
	// while one runs. The app sends it to the window as the event draft.
	Told func(Job)
	// Tick is the wait between two tells while a job runs, a second
	// when nil. A test takes it in hand.
	Tick func() <-chan time.Time
	Log  *slog.Logger

	mu     sync.Mutex
	jobs   map[int64]*Job
	cancel map[int64]context.CancelFunc
	queue  []int64
	going  bool
	n      int
	// seq numbers the changes, see Job.Seq.
	seq int64
	// measure works out how far the job under way is, see draft.
	measure func(*Job)
	// idle is closed when the queue runs dry. Tests wait on it.
	idle chan struct{}
	// The changes not told yet, in the order they happened, and whether
	// a goroutine is telling them now. One tells at a time, so the
	// window never hears an older state after a newer one.
	untold  []Job
	telling bool
	told    *sync.Cond
}

func (d *Drafter) now() time.Time { return d.Store.Now() }

func (d *Drafter) tick() <-chan time.Time {
	if d.Tick != nil {
		return d.Tick()
	}
	return time.After(time.Second)
}

func (d *Drafter) log() *slog.Logger {
	if d.Log != nil {
		return d.Log
	}
	return slog.New(slog.DiscardHandler)
}

// Jobs is every draft the app knows of now, the latest of each person.
func (d *Drafter) Jobs() []Job {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]Job, 0, len(d.jobs))
	for _, j := range d.jobs {
		out = append(out, *j)
	}
	slices.SortFunc(out, func(a, b Job) int { return a.StartedAt.Compare(b.StartedAt) })
	return out
}

// Start drafts a first message to a person, after the drafts already
// asked for, and gives the job at once, so the click shows. Asked again
// while a draft of theirs is under way, it gives that one.
func (d *Drafter) Start(personID int64) (Job, error) {
	found := false
	d.Store.Read(func(data *store.Data) { found = data.Person(personID) != nil })
	if !found {
		return Job{}, fmt.Errorf("%w %d", ErrNoPerson, personID)
	}
	// Without a model there is nothing to wait for, so Draft is refused
	// at once rather than queued to fail.
	if _, _, err := d.Model(); errors.Is(err, ErrNoModel) {
		return Job{}, err
	}
	d.mu.Lock()
	if d.jobs == nil {
		d.jobs, d.cancel = map[int64]*Job{}, map[int64]context.CancelFunc{}
	}
	if j, ok := d.jobs[personID]; ok && !j.Ended() {
		out := *j
		d.mu.Unlock()
		return out, nil
	}
	d.n++
	j := &Job{ID: "draft-" + strconv.Itoa(d.n), PersonID: personID, State: "queued", StartedAt: d.now(), Fraction: -1}
	d.jobs[personID] = j
	d.queue = append(d.queue, personID)
	if !d.going {
		d.going = true
		d.idle = make(chan struct{})
		go d.work()
	}
	out := d.changed(j)
	d.mu.Unlock()
	d.tell()
	return out, nil
}

// Cancel stops a person's draft, queued or under way. What the model
// wrote so far is dropped, and the draft there was before stays.
func (d *Drafter) Cancel(personID int64) error {
	d.mu.Lock()
	j, ok := d.jobs[personID]
	if !ok || j.Ended() {
		d.mu.Unlock()
		return errors.New("no draft is under way for them")
	}
	if cancel := d.cancel[personID]; cancel != nil {
		cancel()
		d.mu.Unlock()
		return nil
	}
	// Still queued.
	d.queue = slices.DeleteFunc(d.queue, func(id int64) bool { return id == personID })
	j.State, j.Fraction, j.SecondsLeft = "cancelled", -1, nil
	d.changed(j)
	d.mu.Unlock()
	d.tell()
	return nil
}

// Wait returns once no draft is queued or under way. The app does not
// need it, a test does.
func (d *Drafter) Wait() {
	d.mu.Lock()
	idle := d.idle
	d.mu.Unlock()
	if idle != nil {
		<-idle
	}
	d.mu.Lock()
	for d.telling {
		d.cond().Wait()
	}
	d.mu.Unlock()
}

// cond is the signal that the telling ended. d.mu is held.
func (d *Drafter) cond() *sync.Cond {
	if d.told == nil {
		d.told = sync.NewCond(&d.mu)
	}
	return d.told
}

// tell tells the changes not told yet, in order. While another goroutine
// tells, it leaves them to that one, so a change never waits on the
// window to hear an earlier one.
// changed numbers a change of j and keeps it to be told. d.mu is held.
func (d *Drafter) changed(j *Job) Job {
	d.seq++
	j.Seq = d.seq
	d.untold = append(d.untold, *j)
	return *j
}

func (d *Drafter) tell() {
	d.mu.Lock()
	if d.telling {
		d.mu.Unlock()
		return
	}
	d.telling = true
	for len(d.untold) > 0 {
		jobs := d.untold
		d.untold = nil
		d.mu.Unlock()
		if d.Told != nil {
			for _, j := range jobs {
				d.Told(j)
			}
		}
		d.mu.Lock()
	}
	d.telling = false
	d.cond().Broadcast()
	d.mu.Unlock()
}

// work drafts what is queued, one after the other, until nothing is.
func (d *Drafter) work() {
	for {
		d.mu.Lock()
		if len(d.queue) == 0 {
			d.going = false
			close(d.idle)
			d.mu.Unlock()
			return
		}
		id := d.queue[0]
		d.queue = d.queue[1:]
		ctx, cancel := context.WithCancel(context.Background())
		d.cancel[id] = cancel
		d.mu.Unlock()
		d.run(ctx, id)
		cancel()
		d.mu.Lock()
		delete(d.cancel, id)
		d.mu.Unlock()
	}
}

// run drafts one, and says how it ended.
func (d *Drafter) run(ctx context.Context, id int64) {
	stopTicking := make(chan struct{})
	ticked := make(chan struct{})
	go func() {
		defer close(ticked)
		for {
			select {
			case <-stopTicking:
				return
			case <-d.tick():
				d.update(id, func(j *Job) {
					if d.measure != nil {
						d.measure(j)
					}
				})
			}
		}
	}()
	err := d.draft(ctx, id)
	close(stopTicking)
	<-ticked
	d.update(id, func(j *Job) {
		d.measure = nil
		j.Fraction, j.SecondsLeft = -1, nil
		switch {
		case err == nil:
			j.State, j.Fraction = "done", 1
		case ctx.Err() != nil:
			j.State, j.Text = "cancelled", ""
		default:
			j.State, j.Error = "failed", Sentence(err.Error())
		}
	})
	if err != nil && ctx.Err() == nil {
		d.log().Warn("a first message was not drafted", "person", id, "error", err)
	}
}

// update changes a job and tells it, with how far it is worked out anew.
func (d *Drafter) update(id int64, change func(*Job)) {
	d.mu.Lock()
	j, ok := d.jobs[id]
	if !ok {
		d.mu.Unlock()
		return
	}
	change(j)
	d.changed(j)
	d.mu.Unlock()
	d.tell()
}

// draft is the work of one draft: the profile, the prompt, the model and
// the message.
func (d *Drafter) draft(ctx context.Context, id int64) error {
	var name, notes, link, text string
	var earlier []store.DraftTime
	var prompt string
	d.Store.Read(func(data *store.Data) {
		if p := data.Person(id); p != nil {
			name, notes, link, text = p.Name, p.Notes, p.LinkedIn.URL, p.LinkedIn.Text
		}
		earlier = slices.Clone(data.DraftTimes)
		prompt = data.Text(store.FirstMessagePrompt)
	})
	if link == "" && text == "" {
		return errors.New("this person has no LinkedIn profile to write from")
	}
	m, path, err := d.Model()
	if err != nil {
		return err
	}
	loaded := d.Host.Loaded(path)
	began := d.now()
	expected, known := expect(earlier, loaded, text == "")
	// How far the draft is, from how long earlier drafts took, worked out
	// each time it is told.
	progress := func(j *Job) {
		if !known {
			j.Fraction, j.SecondsLeft = -1, nil
			return
		}
		spent := d.now().Sub(began).Seconds()
		// It never says it is done before it is.
		j.Fraction = min(spent/expected, 0.95)
		left := max(expected-spent, 0)
		j.SecondsLeft = &left
	}
	d.mu.Lock()
	d.measure = progress
	d.mu.Unlock()

	if text == "" {
		d.update(id, func(j *Job) { j.State, j.Model = "reading", m.Title; progress(j) })
		if text, err = d.read(ctx, id, link); err != nil {
			return err
		}
	}
	if d.Check != nil {
		if err := d.Check(m); err != nil {
			return err
		}
	}
	d.update(id, func(j *Job) {
		j.State, j.Model = "writing", m.Title
		if !loaded {
			j.State = "loading"
		}
		progress(j)
	})
	loadStart := d.now()
	s, release, err := d.Host.Hold(ctx, path)
	if err != nil {
		return err
	}
	// A server that stopped answering goes at once, the rest stays for
	// the next draft.
	var stalled bool
	defer func() {
		if stalled {
			release(0)
			return
		}
		release(language.Keep)
	}()
	var took store.DraftTime
	if !loaded {
		took.LoadMS = d.now().Sub(loadStart).Milliseconds()
	}
	writeStart := d.now()
	d.update(id, func(j *Job) { j.State = "writing"; progress(j) })
	ask := d.Ask
	if ask == nil {
		ask = func(ctx context.Context, s *language.Server, messages []language.Message, heard func(string)) (language.Answer, error) {
			return language.Ask(ctx, d.Host.Client, s, messages, heard)
		}
	}
	answer, err := ask(ctx, s, []language.Message{{Role: "user", Content: Fill(prompt, name, notes, text)}}, func(so string) {
		d.update(id, func(j *Job) { j.Text = so; progress(j) })
	})
	if err != nil {
		stalled = errors.Is(err, language.ErrStalled)
		return err
	}
	took.WriteMS = d.now().Sub(writeStart).Milliseconds()
	message := strings.TrimSpace(answer.Text)
	err = d.Store.Update(func(data *store.Data) error {
		p := data.Person(id)
		if p == nil {
			return errors.New("they were removed while the message was written")
		}
		p.Draft, p.DraftedAt, p.UpdatedAt = message, d.now(), d.now()
		data.Drafted(took)
		return nil
	})
	if err != nil {
		return err
	}
	d.update(id, func(j *Job) { j.Text = message })
	// By ids and counts, never by whom or what, which is personal data.
	d.log().Info("a first message was drafted", "person", id, "model", m.Name,
		"load_ms", took.LoadMS, "write_ms", took.WriteMS, "read", answer.PromptTokens, "wrote", answer.Written)
	return nil
}

// read reads a person's profile from Exa's index, once, and keeps it on
// them. It is one call to Exa, counted against the month's budget like a
// check.
func (d *Drafter) read(ctx context.Context, id int64, link string) (string, error) {
	var finder Finder
	if d.Finder != nil {
		finder = d.Finder()
	}
	if finder == nil {
		return "", errors.New("the app has no profile text for them yet, and reading it needs the Exa key. Add it in Settings")
	}
	err := d.Store.Update(func(data *store.Data) error {
		if !data.Reserve(engine.Provider, d.now()) {
			return engine.ErrNoBudget
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	prof, err := finder.Profile(ctx, link)
	if err != nil {
		return "", fmt.Errorf("their profile could not be read from Exa: %w", err)
	}
	err = d.Store.Update(func(data *store.Data) error {
		if p := data.Person(id); p != nil {
			data.SetProfileText(p, prof.Text, d.now())
		}
		return nil
	})
	return prof.Text, err
}

// expect is how long a draft is expected to take, in seconds, from
// earlier drafts: the middle of their writing times, and of their loading
// times when the model is not loaded, and a few seconds for reading the
// profile from Exa. false when there is nothing to measure against.
func expect(earlier []store.DraftTime, loaded, reading bool) (float64, bool) {
	var writes, loads []int64
	for _, t := range earlier {
		writes = append(writes, t.WriteMS)
		if t.LoadMS > 0 {
			loads = append(loads, t.LoadMS)
		}
	}
	if len(writes) == 0 || (!loaded && len(loads) == 0) {
		return 0, false
	}
	ms := middle(writes)
	if !loaded {
		ms += middle(loads)
	}
	if reading {
		// One call to Exa's index, which answers in a few seconds.
		ms += 3000
	}
	return float64(ms) / 1000, true
}

// middle is the median of ns.
func middle(ns []int64) int64 {
	s := slices.Clone(ns)
	slices.Sort(s)
	return s[len(s)/2]
}

// Fill puts a person into the prompt: {{profile}}, {{name}},
// {{first_name}} and {{notes}}. The profile goes in last, so nothing it
// says is read as a field.
func Fill(prompt, name, notes, profile string) string {
	first, _, _ := strings.Cut(strings.TrimSpace(name), " ")
	if strings.TrimSpace(notes) == "" {
		notes = "None."
	}
	out := strings.NewReplacer("{{name}}", name, "{{first_name}}", first, "{{notes}}", notes).Replace(prompt)
	return strings.ReplaceAll(out, store.ProfileField, profile)
}

// Sentence starts an error with a capital and ends it with a full stop.
func Sentence(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = []rune(strings.ToUpper(string(r[0])))[0]
	s = string(r)
	if !strings.HasSuffix(s, ".") {
		s += "."
	}
	return s
}
