package draft

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/timmrkz/speakertrail/internal/language"
	"github.com/timmrkz/speakertrail/internal/search"
	"github.com/timmrkz/speakertrail/internal/store"
)

var t0 = time.Date(2026, 11, 19, 14, 0, 0, 0, time.UTC)

// clock moves only when the test moves it.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.now }
func (c *clock) Add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// finder is Exa's index with one invented profile.
type finder struct{ asked atomic.Int32 }

func (f *finder) Profile(_ context.Context, link string) (search.Profile, error) {
	f.asked.Add(1)
	return search.Profile{URL: link, Name: "Mara Beispiel", Text: "# Mara Beispiel\n\nInhaberin | Beispiel Keramik\n\n## About\n\nIch töpfere in Köln."}, nil
}

type rig struct {
	st     *store.Store
	clock  *clock
	d      *Drafter
	finder *finder
	// asks counts the questions to the model, and most how many were
	// asked at once.
	asks, now, most atomic.Int32
	launched        atomic.Int32
	mu              sync.Mutex
	told            []Job
}

func newRig(t *testing.T) *rig {
	t.Helper()
	r := &rig{clock: &clock{now: t0}, finder: &finder{}}
	st, err := store.Open(t.TempDir(), r.clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	r.st = st
	host := &language.Host{
		Launch: func(context.Context, string) (*language.Server, error) {
			r.launched.Add(1)
			r.clock.Add(20 * time.Second)
			return &language.Server{URL: "http://127.0.0.1:1"}, nil
		},
		// A model nobody holds stays loaded for the whole test.
		Idle: func(time.Duration, func()) func() bool { return func() bool { return true } },
	}
	t.Cleanup(host.Close)
	m := language.Models()[0]
	r.d = &Drafter{
		Store: st, Host: host,
		Model:  func() (language.Model, string, error) { return m, "/models/" + m.Name, nil },
		Finder: func() Finder { return r.finder },
		Ask: func(ctx context.Context, _ *language.Server, msgs []language.Message, schema json.RawMessage, heard func(string)) (language.Answer, error) {
			r.asks.Add(1)
			n := r.now.Add(1)
			defer r.now.Add(-1)
			for {
				most := r.most.Load()
				if n <= most || r.most.CompareAndSwap(most, n) {
					break
				}
			}
			if !strings.Contains(msgs[0].Content, "## About") {
				return language.Answer{}, errors.New("the prompt has no profile in it")
			}
			if string(schema) != HookSchema {
				return language.Answer{}, errors.New("the model was not held to the schema")
			}
			heard(`{"reason":"du`)
			r.clock.Add(10 * time.Second)
			heard(`{"reason":"du in Köln töpferst.",`)
			return language.Answer{Text: ` {"reason":"du in Köln töpferst.","reaction":"Toll!","portrait":"Töpferin"}` + "\n", Finish: "stop"}, nil
		},
		Told: func(j Job) {
			r.mu.Lock()
			r.told = append(r.told, j)
			r.mu.Unlock()
		},
		// No tick between tells: the test sees every change as it happens.
		Tick: func() <-chan time.Time { return nil },
		// The last variation of each part, so the test knows the message.
		Pick: func(n int) int { return n - 1 },
	}
	err = st.Update(func(d *store.Data) error {
		return d.SetTemplate(template(t, `{"origin":["#greeting# #first_name#, du bist mir aufgefallen, weil #reason##reaction_part#\n\nDeine Geschichte als #portrait#?"],"greeting":["Hi","Hallo"]}`))
	})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func template(t *testing.T, raw string) store.Template {
	t.Helper()
	var out store.Template
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// written is the message the rig's model makes for Mara.
const written = "Hallo Mara, du bist mir aufgefallen, weil du in Köln töpferst. Toll!\n\nDeine Geschichte als Töpferin?"

// person adds an invented person, with the profile text Exa gave or none.
func (r *rig) person(t *testing.T, name, text string) int64 {
	t.Helper()
	var id int64
	err := r.st.Update(func(d *store.Data) error {
		s, _ := d.AddSearch("Keramik Köln "+name, 0, t0)
		p, _ := d.AddFound(store.Found{URL: "https://www.linkedin.com/in/" + strings.ToLower(strings.ReplaceAll(name, " ", "-")), Name: name, Text: text}, s, 1, t0)
		id = p.ID
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (r *rig) states(id int64) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, j := range r.told {
		if j.PersonID == id && (len(out) == 0 || out[len(out)-1] != j.State) {
			out = append(out, j.State)
		}
	}
	return out
}

func TestADraftIsWrittenAndKept(t *testing.T) {
	r := newRig(t)
	id := r.person(t, "Mara Beispiel", "# Mara Beispiel\n\n## About\n\nIch töpfere in Köln.")
	j, err := r.d.Start(id)
	if err != nil || j.State != "queued" {
		t.Fatalf("start %+v %v", j, err)
	}
	r.d.Wait()
	if got := strings.Join(r.states(id), " "); got != "queued loading writing done" {
		t.Errorf("states %s", got)
	}
	r.st.Read(func(d *store.Data) {
		p := d.Person(id)
		if p.Draft != written || !p.DraftedAt.Equal(t0.Add(30*time.Second)) {
			t.Errorf("draft %q at %v", p.Draft, p.DraftedAt)
		}
		if len(d.DraftTimes) != 1 || d.DraftTimes[0] != (store.DraftTime{LoadMS: 20000, WriteMS: 10000}) {
			t.Errorf("times %+v", d.DraftTimes)
		}
	})
	if r.finder.asked.Load() != 0 {
		t.Error("Exa was asked for a profile the app had")
	}
	// The message is on screen while the model writes, with … where its
	// part goes, and never the JSON it writes.
	r.mu.Lock()
	var shown []string
	for _, j := range r.told {
		if j.State == "writing" {
			shown = append(shown, j.Text)
		}
	}
	end := r.told[len(r.told)-1]
	r.mu.Unlock()
	if len(shown) < 3 || slices.ContainsFunc(shown[:len(shown)-1], func(s string) bool {
		return s != "Hallo Mara, du bist mir aufgefallen, weil …\n\nDeine Geschichte als …?"
	}) {
		t.Errorf("while it was written the window saw %q", shown)
	}
	if end.State != "done" || end.Text != written || end.Problem != "" {
		t.Errorf("at the end the window saw %+v", end)
	}
}

// The second draft finds the model loaded, and says how far it is from
// how long the first took.
func TestTheNextDraftKnowsHowLongItTakes(t *testing.T) {
	r := newRig(t)
	a := r.person(t, "Mara Beispiel", "# Mara\n\n## About\n\nA")
	b := r.person(t, "Jonas Probe", "# Jonas\n\n## About\n\nB")
	r.d.Start(a)
	r.d.Wait()
	r.d.Start(b)
	r.d.Wait()
	if r.launched.Load() != 1 {
		t.Errorf("the model was loaded %d times", r.launched.Load())
	}
	if got := strings.Join(r.states(b), " "); got != "queued writing done" {
		t.Errorf("states %s", got)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, j := range r.told {
		if j.PersonID == b && j.State == "writing" {
			if j.SecondsLeft == nil || *j.SecondsLeft != 10 || j.Fraction != 0 {
				t.Errorf("at the start of writing: %v left, %v done", j.SecondsLeft, j.Fraction)
			}
			return
		}
	}
	t.Error("the second draft was never told writing")
}

// A person the app has no profile text for has it read from Exa once,
// counted against the month's budget.
func TestAMissingProfileIsReadFromExa(t *testing.T) {
	r := newRig(t)
	id := r.person(t, "Mara Beispiel", "")
	r.d.Start(id)
	r.d.Wait()
	if got := strings.Join(r.states(id), " "); got != "queued reading loading writing done" {
		t.Errorf("states %s", got)
	}
	r.st.Read(func(d *store.Data) {
		if p := d.Person(id); !strings.Contains(p.LinkedIn.Text, "töpfere") || p.Draft == "" {
			t.Errorf("text %q, draft %q", p.LinkedIn.Text, p.Draft)
		}
		if d.Used("exa", t0) != 1 {
			t.Errorf("%d calls to Exa counted", d.Used("exa", t0))
		}
	})
}

func TestADraftWithoutAKeyOrAModelFails(t *testing.T) {
	r := newRig(t)
	id := r.person(t, "Mara Beispiel", "")
	r.d.Finder = func() Finder { return nil }
	r.d.Start(id)
	r.d.Wait()
	j := r.d.Jobs()[0]
	if j.State != "failed" || !strings.Contains(j.Error, "Exa key") {
		t.Errorf("without a key: %+v", j)
	}
	// Without a model, Draft is refused at once and nothing is queued,
	// so the window never shows a draft that waits for nothing.
	r.d.Model = func() (language.Model, string, error) { return language.Model{}, "", ErrNoModel }
	other := r.person(t, "Jonas Probe", "# Jonas\n\n## About\n\nB")
	if _, err := r.d.Start(other); !errors.Is(err, ErrNoModel) {
		t.Errorf("without a model: %v", err)
	}
	for _, j := range r.d.Jobs() {
		if j.PersonID == other {
			t.Errorf("a job without a model: %+v", j)
		}
	}
	if r.asks.Load() != 0 {
		t.Error("the model was asked")
	}
}

// Drafts asked for from several goroutines at once are written one at a
// time, each once, and one cancelled while queued is not written.
func TestDraftsFromSeveralGoroutines(t *testing.T) {
	r := newRig(t)
	var ids []int64
	for _, n := range []string{"Mara Beispiel", "Jonas Probe", "Lena Muster", "Emil Beispiel", "Ida Probe", "Paul Muster"} {
		ids = append(ids, r.person(t, n, "# "+n+"\n\n## About\n\nText"))
	}
	var wg sync.WaitGroup
	for _, id := range ids {
		for range 3 {
			wg.Go(func() {
				if _, err := r.d.Start(id); err != nil {
					t.Error(err)
				}
				r.d.Jobs()
			})
		}
	}
	wg.Wait()
	_ = r.d.Cancel(ids[len(ids)-1])
	r.d.Wait()
	if r.most.Load() != 1 {
		t.Errorf("%d questions to the model at once", r.most.Load())
	}
	// A Start after a person's draft is done drafts again, so a person
	// can be drafted twice here. Each job asked the model once.
	done := map[string]bool{}
	people := map[int64]bool{}
	r.mu.Lock()
	for _, j := range r.told {
		if j.State == "done" {
			done[j.ID], people[j.PersonID] = true, true
		}
	}
	r.mu.Unlock()
	// The last may have been under way already when it was cancelled.
	if int(r.asks.Load()) != len(done) || len(people) < len(ids)-1 {
		t.Errorf("%d asked, %d done, for %d of %d people", r.asks.Load(), len(done), len(people), len(ids))
	}
}

func TestFill(t *testing.T) {
	got := Fill("Hi {{first_name}} ({{name}}). {{notes}}\n{{profile}}", "Mara Beispiel", "", "Profil mit {{name}} darin")
	if got != "Hi Mara (Mara Beispiel). None.\nProfil mit {{name}} darin" {
		t.Errorf("%q", got)
	}
}

// A model that stopped answering is let go of at once, so the next draft
// loads one that works. One that answered stays loaded for the next.
func TestAStalledModelIsLetGo(t *testing.T) {
	r := newRig(t)
	id := r.person(t, "Mara Beispiel", "# Mara\n\n## About\n\nA")
	model := "/models/" + language.Models()[0].Name
	r.d.Ask = func(context.Context, *language.Server, []language.Message, json.RawMessage, func(string)) (language.Answer, error) {
		return language.Answer{}, fmt.Errorf("%w: nothing came from it for 2m0s", language.ErrStalled)
	}
	r.d.Start(id)
	r.d.Wait()
	if j := r.d.Jobs()[0]; j.State != "failed" || r.d.Host.Loaded(model) {
		t.Errorf("after a stall: %s, loaded %v", j.State, r.d.Host.Loaded(model))
	}
	r.d.Ask = func(context.Context, *language.Server, []language.Message, json.RawMessage, func(string)) (language.Answer, error) {
		return language.Answer{Text: `{"reason":"du töpferst.","reaction":"","portrait":"Töpferin"}`, Finish: "stop"}, nil
	}
	r.d.Start(id)
	r.d.Wait()
	if !r.d.Host.Loaded(model) || r.launched.Load() != 2 {
		t.Errorf("after an answer: loaded %v, %d loads", r.d.Host.Loaded(model), r.launched.Load())
	}
}

// The window is told a person's changes in the order they happened, so
// it never ends on an older state than the draft is in. Here the window
// is slow to hear the first change, Start's queued, while the draft
// runs to its end.
func TestTheWindowHearsChangesInOrder(t *testing.T) {
	r := newRig(t)
	id := r.person(t, "Mara Beispiel", "# Mara Beispiel\n\n## About\n\nIch töpfere in Köln.")
	told := r.d.Told
	r.d.Told = func(j Job) {
		if j.State == "queued" {
			for !slices.ContainsFunc(r.d.Jobs(), func(j Job) bool { return j.State == "done" }) {
				runtime.Gosched()
			}
		}
		told(j)
	}
	r.d.Start(id)
	r.d.Wait()
	if got := strings.Join(r.states(id), " "); got != "queued loading writing done" {
		t.Errorf("told %s", got)
	}
}

// Every change a person's draft goes through has a higher number than the
// one before it, so the window can tell an older answer from a newer one.
func TestEachChangeHasAHigherNumber(t *testing.T) {
	r := newRig(t)
	id := r.person(t, "Mara Beispiel", "# Mara Beispiel\n\n## About\n\nIch töpfere in Köln.")
	started, _ := r.d.Start(id)
	r.d.Wait()
	r.mu.Lock()
	defer r.mu.Unlock()
	last := int64(0)
	for _, j := range r.told {
		if j.Seq <= last {
			t.Errorf("%s told with %d after %d", j.State, j.Seq, last)
		}
		last = j.Seq
	}
	if started.Seq != r.told[0].Seq || r.told[len(r.told)-1].State != "done" {
		t.Errorf("Start answered %d, first told %d", started.Seq, r.told[0].Seq)
	}
}
