package updates

// Order keeps, for each channel, when the newest build the app took of it
// was made, so a build made before it is never taken again, unasked or
// picked. Whoever can write the release can put back an older entry of a
// channel, signed as it was then, with a bug fixed since, and a run of an
// old commit made again is published as its channel's build. The claim
// does not say when a build was made, so before this the app took
// whatever build its channel had when it was not the one running. See
// docs/decisions/2026-11-27-updates-reach-the-app-sooner-and-safer.md.
//
// What it keeps is in a file of its own beside updates.json, written
// whole to a new file that then replaces the old one.

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Order is the newest made time taken of each channel. Its zero value
// keeps nothing on disk.
type Order struct {
	// Path is the file it is kept in, or empty to keep it in memory.
	Path string

	mu     sync.Mutex
	loaded bool
	made   map[string]time.Time
}

// Take says whether the build may be taken, and when it may, notes when
// it was made. A build signed as made before the newest taken of its
// channel is refused. One without a signed time is taken only while
// nothing of its channel has been taken with one, so the entries from
// before orders were signed still work until their next build.
func (o *Order) Take(public ed25519.PublicKey, b Build) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.load()
	newest, known := o.made[b.Channel]
	err := VerifyOrder(public, b)
	switch {
	case errors.Is(err, errNoOrder) && !known:
		return nil
	case errors.Is(err, errNoOrder):
		return fmt.Errorf("the build of %s %s, and this app already took one that is, so it is not installed", b.Channel, err)
	case err != nil:
		return err
	case b.Made.Before(newest):
		return fmt.Errorf("the build of %s was made %s, before the one this app already took, made %s, "+
			"so it is not installed", b.Channel, b.Made.UTC().Format(time.RFC3339), newest.UTC().Format(time.RFC3339))
	}
	if !b.Made.Equal(newest) {
		o.made[b.Channel] = b.Made
		o.save()
	}
	return nil
}

func (o *Order) load() {
	if o.loaded {
		return
	}
	o.loaded = true
	o.made = map[string]time.Time{}
	if o.Path == "" {
		return
	}
	data, err := os.ReadFile(o.Path)
	if err != nil {
		return
	}
	var kept map[string]time.Time
	if json.Unmarshal(data, &kept) == nil {
		for ch, t := range kept {
			if ValidChannel(ch) {
				o.made[ch] = t
			}
		}
	}
}

// save writes what is kept, or leaves the file as it was when it cannot,
// so a failed write costs at most a step back the file would have
// stopped, never an update.
func (o *Order) save() {
	if o.Path == "" {
		return
	}
	data, err := json.MarshalIndent(o.made, "", "  ")
	if err != nil {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(o.Path), ".update-order-*")
	if err != nil {
		return
	}
	_, err = tmp.Write(data)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), o.Path)
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
	}
}
