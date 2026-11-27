// Package draft writes a first message to a person on the Mac. It puts
// the message together from the template in Settings, one variation of
// each part picked at random, and asks the language model only for the
// hook, the portrait, the reason and the reaction, from the prompt in
// Settings and the person's profile text. The draft is kept on the
// person, where Tim reads it, changes it, copies it into LinkedIn and
// sends it himself. Nothing is ever sent from the app.
//
// A draft is work that takes a while: reading the profile from Exa when
// the app has no text for the person yet, loading the model when it is not
// loaded, and writing. It runs in the background, one at a time, since
// one model answers one question at a time, and says how far it is about
// once a second, see Job.
package draft

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/timmrkz/speakertrail/internal/engine"
	"github.com/timmrkz/speakertrail/internal/language"
	"github.com/timmrkz/speakertrail/internal/names"
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
	// Problem says why a draft that is done lacks the model's part: its
	// answer did not fit the schema, so the message has … there for Tim
	// to write.
	Problem string `json:"problem,omitempty"`
	// Seq is higher for every change than for the one before it, of any
	// draft, so the window keeps the newer of an answer and an event.
	Seq int64 `json:"seq"`
}

// Ended says whether the job is over.
func (j Job) Ended() bool {
	return j.State == "done" || j.State == "failed" || j.State == "cancelled"
}

// Finder reads one profile from Exa's index, by its link or the name in
// it, see engine.ReadProfile.
type Finder = engine.Reader

// ErrNoCopy is a draft for someone whose profile Exa's index holds
// neither under their link nor under the name in it. Tim pastes it.
var ErrNoCopy = errors.New("Exa has no copy of their profile. Open it in Chrome, copy all of the page with ⌘A and ⌘C, and paste it here with ⌘V")

// ErrNoPerson is a draft asked for someone who is not in the file.
var ErrNoPerson = errors.New("there is no person")

// errGone is a draft of someone taken out of the app while it ran, like
// by Undo of the links that added them. It ends as cancelled, since
// nobody is left to write to.
var errGone = errors.New("they were taken out of the app")

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
	// Finder reads a profile from Exa. It fails with engine.ErrNoKey
	// while no key is saved, and with why when the key could not be read.
	Finder func() (Finder, error)
	// Await waits while the going run reads a person's profile, or has
	// it still to read, and says whether it did, engine.Engine.AwaitRead
	// in the app. A draft of someone with no profile text asks it before
	// it asks Exa, so one profile is never read twice. Nil waits for
	// nothing.
	Await func(ctx context.Context, personID int64) (bool, error)
	// Ask asks the model for JSON that fits schema, language.AskFor when
	// nil. A test puts its own.
	Ask func(ctx context.Context, s *language.Server, messages []language.Message, schema json.RawMessage, heard func(string)) (language.Answer, error)
	// Pick picks one of n variations of the template, from 0 to n-1, at
	// random when nil. A test picks its own.
	Pick func(n int) int
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
	// pasted are the drafts a pasted page started, by person, with when
	// the page was kept, see Pasted and Undone.
	pasted map[int64]time.Time
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
	if j, ok := d.jobs[personID]; ok && !j.Ended() {
		out := *j
		d.mu.Unlock()
		return out, nil
	}
	// Tim asked for this one, so no undo of a paste stops it.
	delete(d.pasted, personID)
	out := d.queued(personID)
	d.mu.Unlock()
	d.tell()
	return out, nil
}

// queued puts a new draft of a person in the queue, while d.mu is held,
// and gives it. The caller tells it once it lets go.
func (d *Drafter) queued(personID int64) Job {
	if d.jobs == nil {
		d.jobs, d.cancel = map[int64]*Job{}, map[int64]context.CancelFunc{}
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
	return d.changed(j)
}

// Pasted starts the draft a page Tim pasted as a person's profile was
// missing for, so a paste does the whole thing (Tim's report of 24
// November 2026, #88): for someone with no first message yet whose
// draft failed because Exa has no copy, or who is connected, since a
// draft is what Tim does next for them. ok says one started, or was
// under way. Without a model none starts, and Draft says why.
func (d *Drafter) Pasted(personID int64) (j Job, ok bool) {
	empty, connected := false, false
	var kept time.Time
	d.Store.Read(func(data *store.Data) {
		if p := data.Person(personID); p != nil {
			empty, connected, kept = p.Draft == "", p.Connected, p.LinkedIn.TextAt
		}
	})
	if _, _, err := d.Model(); errors.Is(err, ErrNoModel) {
		return Job{}, false
	}
	// What the draft there is says, and the new one starts, under one
	// hold, so pastes at once start one draft, and none after a draft
	// that just wrote a message.
	d.mu.Lock()
	was, there := d.jobs[personID]
	switch {
	case there && !was.Ended():
		out := *was
		d.mu.Unlock()
		return out, true
	case there && was.State == "done":
		d.mu.Unlock()
		return Job{}, false
	}
	noCopy := there && was.State == "failed" && was.Error == Sentence(ErrNoCopy.Error())
	if !empty || !connected && !noCopy {
		d.mu.Unlock()
		return Job{}, false
	}
	out := d.queued(personID)
	if d.pasted == nil {
		d.pasted = map[int64]time.Time{}
	}
	d.pasted[personID] = kept
	d.mu.Unlock()
	d.tell()
	return out, true
}

// Undone hears of an undo in the Edit menu that changed a person. When
// it took back the page whose paste started their draft, the draft
// stops if it still runs, since nothing asks for it any more, as the
// paste's Undo toast did before Undo moved into the Edit menu. An undo
// of anything else on them, like their notes, leaves it.
func (d *Drafter) Undone(personID int64) {
	d.mu.Lock()
	kept, ok := d.pasted[personID]
	d.mu.Unlock()
	if !ok {
		return
	}
	var now time.Time
	d.Store.Read(func(data *store.Data) {
		if p := data.Person(personID); p != nil {
			now = p.LinkedIn.TextAt
		}
	})
	if now.Equal(kept) {
		return
	}
	d.mu.Lock()
	delete(d.pasted, personID)
	d.mu.Unlock()
	// A draft that already ended has nothing to stop.
	_ = d.Cancel(personID)
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

// changed numbers a change of j and keeps it to be told. d.mu is held.
func (d *Drafter) changed(j *Job) Job {
	d.seq++
	j.Seq = d.seq
	d.untold = append(d.untold, *j)
	return *j
}

// tell tells the changes not told yet, in order. While another goroutine
// tells, it leaves them to that one, so a change never waits on the
// window to hear an earlier one.
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
		case ctx.Err() != nil, errors.Is(err, errGone):
			j.State, j.Text = "cancelled", ""
		default:
			j.State, j.Error = "failed", Sentence(err.Error())
		}
	})
	if err != nil && ctx.Err() == nil && !errors.Is(err, errGone) {
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
	// When it was asked for, so a read a run made since is not made again.
	var asked time.Time
	d.mu.Lock()
	if j, ok := d.jobs[id]; ok {
		asked = j.StartedAt
	}
	d.mu.Unlock()
	var name, notes, link, text string
	var earlier []store.DraftTime
	var prompt string
	var template store.Template
	found, missed := false, false
	person := func() {
		d.Store.Read(func(data *store.Data) {
			p := data.Person(id)
			if found = p != nil; found {
				name, notes, link, text = p.Name, p.Notes, p.LinkedIn.URL, p.LinkedIn.Text
				missed = text == "" && slices.ContainsFunc(data.Runs, func(r *store.Run) bool {
					return slices.ContainsFunc(r.Reads, func(x store.Read) bool {
						return x.PersonID == id && x.Error == engine.NotIndexed && !x.At.Before(asked)
					})
				})
			}
			earlier = slices.Clone(data.DraftTimes)
			prompt = data.Text(store.FirstMessagePrompt)
			template = data.FirstMessageTemplate()
		})
	}
	person()
	if !found {
		return errGone
	}
	if link == "" && text == "" {
		return errors.New("this person has no LinkedIn profile to write from")
	}
	if err := template.Check(); err != nil {
		return fmt.Errorf("the first message template in Settings cannot be used: %w", err)
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
		// The run that reads their profile now, or has it still to read,
		// is waited for rather than asked again of Exa. What it read
		// names them too, so the message greets the name it found. It
		// may have read it a moment ago, so the person is read again
		// whether it waited or not.
		if d.Await != nil {
			if _, err = d.Await(ctx, id); err != nil {
				return err
			}
		}
		person()
		if !found {
			return errGone
		}
		switch {
		case text != "":
		// What a run found missing since the draft was asked for is not
		// asked for again.
		case missed:
			return ErrNoCopy
		default:
			if text, err = d.read(ctx, id, link); err != nil {
				return err
			}
		}
	}
	// The message is put together before the model writes, so it is on
	// screen at once, with … where the model's part goes.
	message := d.put(template, name, link)
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
	d.update(id, func(j *Job) { j.State = "writing"; j.Text = message.with(Hook{}); progress(j) })
	ask := d.Ask
	if ask == nil {
		ask = func(ctx context.Context, s *language.Server, messages []language.Message, schema json.RawMessage, heard func(string)) (language.Answer, error) {
			return language.AskFor(ctx, d.Host.Client, s, messages, schema, heard)
		}
	}
	// An answer that does not fit is asked for once more. The model
	// samples, so the next answer is another one, and one cut off at a
	// limit of the schema, or with a letter of another script, rarely
	// comes twice. One that still does
	// not fit makes a draft with … in its place, and the window says
	// why. Never the answer itself: it is untrusted and may name the
	// person.
	var answer language.Answer
	var read, wrote int
	var hook Hook
	var problem string
	for try := 1; try <= hookTries; try++ {
		// What the model writes so far is JSON, which is nothing to read,
		// so the window sees only that it grows.
		answer, err = ask(ctx, s, Messages(prompt, name, link, notes, text), json.RawMessage(HookSchema), func(string) {
			d.update(id, progress)
		})
		if err != nil {
			stalled = errors.Is(err, language.ErrStalled)
			return err
		}
		read, wrote = read+answer.PromptTokens, wrote+answer.Written
		if hook, err = ParseHook(answer.Text); err == nil {
			problem = ""
			break
		}
		problem = Sentence("the language model's answer did not fit, so … marks what it should have written: " + err.Error())
		d.log().Warn("the language model's answer did not fit the schema", "person", id, "model", m.Name, "try", try, "error", err)
		hook = Hook{}
	}
	took.WriteMS = d.now().Sub(writeStart).Milliseconds()
	written := message.with(hook)
	// Tim pressed Draft, so the message is his change, and Undo in the
	// Edit menu puts back the one before. How long it took is not.
	err = d.Store.Edit(func(data *store.Data) error {
		p := data.Person(id)
		if p == nil {
			return errGone
		}
		p.Draft, p.DraftedAt, p.UpdatedAt = written, d.now(), d.now()
		data.Drafted(took)
		return nil
	}, store.PersonThing(id))
	if err != nil {
		return err
	}
	d.update(id, func(j *Job) { j.Text, j.Problem = written, problem })
	// By ids and counts, never by whom or what, which is personal data.
	d.log().Info("a first message was drafted", "person", id, "model", m.Name,
		"load_ms", took.LoadMS, "write_ms", took.WriteMS, "read", read, "wrote", wrote)
	return nil
}

// read reads a person's profile from Exa's index, once, and keeps it on
// them. It is one call to Exa, counted against the month's budget like a
// check.
func (d *Drafter) read(ctx context.Context, id int64, link string) (string, error) {
	finder, err := Finder(nil), engine.ErrNoKey
	if d.Finder != nil {
		finder, err = d.Finder()
	}
	switch {
	case errors.Is(err, engine.ErrNoKey):
		return "", errors.New("the app has no profile text for them yet, and reading it needs the Exa key. Add it in Settings")
	case err != nil:
		return "", fmt.Errorf("the app reads their profile text from Exa first, and %w", err)
	}
	reserve := func() (bool, error) {
		var ok bool
		err := d.Store.Update(func(data *store.Data) error {
			ok = data.Reserve(engine.Provider, d.now())
			return nil
		})
		return ok, err
	}
	switch ok, err := reserve(); {
	case err != nil:
		return "", err
	case !ok:
		return "", engine.ErrNoBudget
	}
	prof, err := engine.ReadProfile(ctx, finder, link, reserve)
	if errors.Is(err, search.ErrNotIndexed) {
		return "", ErrNoCopy
	}
	if err != nil {
		return "", fmt.Errorf("their profile could not be read from Exa: %w", err)
	}
	text := prof.Text
	err = d.Store.Update(func(data *store.Data) error {
		p := data.Person(id)
		switch {
		case p == nil:
		// A page Tim pasted while Exa was asked wins.
		case p.LinkedIn.Text != "":
			text = p.LinkedIn.Text
		default:
			data.SetProfileText(p, prof.Text, d.now())
		}
		return nil
	})
	return text, err
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

// marks stand where the model's parts go in a message put together before
// the model wrote them. No template holds them, because Check refuses
// characters that cannot be written.
const (
	portraitMark = "\x00portrait\x00"
	reasonMark   = "\x00reason\x00"
	reactionMark = "\x00reaction\x00"
)

// message is a first message put together from the template, waiting for
// the model's part.
type message string

// put puts a message together from the template, with the variations
// picked at random, and marks where the model's part goes. The link is
// the person's profile, which says the order of their name.
func (d *Drafter) put(t store.Template, name, link string) message {
	pick := d.Pick
	if pick == nil {
		pick = rand.IntN
	}
	// Where the name holds no first name for sure, … stands for Tim to
	// write, as it does for the model's part.
	first := names.First(name, link)
	if first == "" {
		first = Missing
	}
	return message(t.Expand(map[string]string{
		store.FirstNameField: first,
		store.PortraitField:  portraitMark,
		store.ReasonField:    reasonMark,
		store.ReactionField:  reactionMark,
	}, pick))
}

// with puts the model's part in. Where it has none, … stands for Tim to
// write, and the reaction is left out.
func (m message) with(h Hook) string {
	or := func(s string) string {
		if s == "" {
			return Missing
		}
		return s
	}
	reaction := ""
	if h.Reaction != "" {
		reaction = " " + h.Reaction
	}
	return strings.NewReplacer(portraitMark, or(h.Portrait), reasonMark, or(h.Reason), reactionMark, reaction).Replace(string(m))
}

// Messages is the prompt as the model is asked it: the paragraphs before
// the one of its first field as a system turn, the same for every draft, and
// the rest, filled in for the person, as the user's turn. llama-server
// keeps a checkpoint where the last user turn starts, so the next draft
// reads only the person's part again, whatever the model's sliding window
// let go of. A prompt that starts with a field is one user turn.
func Messages(prompt, name, link, notes, profile string) []language.Message {
	first := len(prompt)
	for _, f := range []string{"{{name}}", "{{first_name}}", "{{notes}}", store.ProfileField} {
		if i := strings.Index(prompt, f); i >= 0 && i < first {
			first = i
		}
	}
	cut := max(strings.LastIndex(prompt[:first], "\n\n"), 0)
	system := strings.TrimSpace(prompt[:cut])
	user := Fill(strings.TrimSpace(prompt[cut:]), name, link, notes, profile)
	if system == "" {
		return []language.Message{{Role: "user", Content: user}}
	}
	return []language.Message{{Role: "system", Content: system}, {Role: "user", Content: user}}
}

// Fill puts a person into the prompt: {{profile}}, {{name}},
// {{first_name}} and {{notes}}. The profile goes in last, so nothing it
// says is read as a field. The first name is read with the profile's
// link, and where the name holds none for sure, the model gets the whole
// name.
func Fill(prompt, name, link, notes, profile string) string {
	first := names.First(name, link)
	if first == "" {
		first = strings.TrimSpace(name)
	}
	if strings.TrimSpace(notes) == "" {
		notes = "none"
	}
	out := strings.NewReplacer("{{name}}", name, "{{first_name}}", first, "{{notes}}", notes).Replace(prompt)
	return strings.ReplaceAll(out, store.ProfileField, profile)
}

// hookTries is how often the model is asked for a hook that fits.
const hookTries = 2

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
