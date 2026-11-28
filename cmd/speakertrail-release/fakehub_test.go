package main

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// fakeHub is GitHub as far as publishing asks it, in memory: a release
// with its files, the open and the ended pull requests, the commits'
// statuses and runs, and what was done to them, in order.
type fakeHub struct {
	mu      sync.Mutex
	files   map[string][]byte
	created map[string]time.Time
	updated map[string]time.Time
	ids     map[string]int64
	nextID  int64
	now     time.Time

	main     string
	open     []openPull
	gone     []endedPull
	conflict map[int]bool
	// histories holds, for each newest commit, the commits its history
	// holds.
	histories map[string][]string
	statuses  map[string]string
	runs      map[string][2]string
	zips      map[string]int
	tree      map[string][]byte
	waits     []waitingRun
	// refuse makes an upload of a name fail.
	refuse map[string]bool

	did []string
}

func newFakeHub(now time.Time) *fakeHub {
	return &fakeHub{files: map[string][]byte{}, created: map[string]time.Time{}, updated: map[string]time.Time{},
		ids: map[string]int64{}, now: now, conflict: map[int]bool{}, histories: map[string][]string{},
		statuses: map[string]string{}, runs: map[string][2]string{}, zips: map[string]int{}, tree: map[string][]byte{},
		refuse: map[string]bool{}}
}

// put is a file the release holds, uploaded at.
func (f *fakeHub) put(name string, data []byte, at time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	f.files[name], f.created[name], f.updated[name], f.ids[name] = data, at, at, f.nextID
}

func (f *fakeHub) done(format string, args ...any) {
	f.did = append(f.did, fmt.Sprintf(format, args...))
}

// doneWith are the steps done that start with prefix, in order.
func (f *fakeHub) doneWith(prefix string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, d := range f.did {
		if strings.HasPrefix(d, prefix) {
			out = append(out, d)
		}
	}
	return out
}

func (f *fakeHub) mainCommit() (string, error) { return f.main, nil }

func (f *fakeHub) commit(short string) (string, error) { return "", errors.New("no such commit") }

func (f *fakeHub) compare(base, head string) (string, error) {
	if base == head {
		return "identical", nil
	}
	for _, c := range f.histories[head] {
		if c == base {
			return "ahead", nil
		}
	}
	return "diverged", nil
}

func (f *fakeHub) buildRun(commit string) (string, string, error) {
	r := f.runs[commit]
	return r[0], r[1], nil
}

func (f *fakeHub) ready(commit string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.statuses[commit], nil
}

func (f *fakeHub) setReady(commit, state, description, target string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statuses[commit] = state
	f.done("ready %s %s %s", commit, state, description)
	return nil
}

func (f *fakeHub) openPulls() ([]openPull, error)   { return f.open, nil }
func (f *fakeHub) conflicts(n int) (bool, error)    { return f.conflict[n], nil }
func (f *fakeHub) endedPulls() ([]endedPull, error) { return f.gone, nil }
func (f *fakeHub) builds(run string) (int, error)   { return f.zips[run], nil }

func (f *fakeHub) fileAt(name, commit string) ([]byte, error) {
	data, ok := f.tree[commit+":"+name]
	if !ok {
		return nil, errNotFound
	}
	return data, nil
}

func (f *fakeHub) assets() ([]hubAsset, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var all []hubAsset
	for name := range f.files {
		all = append(all, hubAsset{id: f.ids[name], name: name, created: f.created[name], updated: f.updated[name]})
	}
	return all, nil
}

func (f *fakeHub) download(a hubAsset) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, ok := f.files[a.name]
	if !ok {
		return nil, errNotFound
	}
	return data, nil
}

func (f *fakeHub) upload(name string, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.refuse[name] {
		return errors.New("the release is full")
	}
	f.nextID++
	f.now = f.now.Add(time.Second)
	f.files[name], f.created[name], f.updated[name], f.ids[name] = data, f.now, f.now, f.nextID
	f.done("upload %s", name)
	return nil
}

func (f *fakeHub) add(name string, data []byte) error {
	f.mu.Lock()
	_, there := f.files[name]
	f.mu.Unlock()
	if there {
		return errors.New("already_exists")
	}
	return f.upload(name, data)
}

func (f *fakeHub) remove(a hubAsset) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.files, a.name)
	f.done("remove %s", a.name)
	return nil
}

func (f *fakeHub) rename(a hubAsset, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.files[name], f.created[name], f.updated[name], f.ids[name] = f.files[a.name], f.created[a.name], f.updated[a.name], f.ids[a.name]
	delete(f.files, a.name)
	f.done("rename %s %s", a.name, name)
	return nil
}

func (f *fakeHub) waiting() ([]waitingRun, error) { return f.waits, nil }

func (f *fakeHub) cancel(run int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.done("cancel %d", run)
	return nil
}

// fakeTrees says which files differ between two commits, from a table.
type fakeTrees map[[2]string][]string

func (t fakeTrees) changed(from, to string) ([]string, error) {
	files, ok := t[[2]string{from, to}]
	if !ok {
		return nil, errors.New("unknown commits")
	}
	return files, nil
}
