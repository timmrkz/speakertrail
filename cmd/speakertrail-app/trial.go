package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"
	"github.com/spf13/afero"

	"github.com/timmrkz/speakertrail/internal/store"
)

// A build put in place is on trial until it says it started. Wails'
// helper puts the build before back only when the new one cannot be
// opened at all, and removes it the moment it runs, so a build that
// opened and then crashed, or whose Updates page broke, stayed until
// make install was run in a checkout. Now the step that puts a build in
// place first starts its program once, told to end at once, so a library
// macOS refuses is seen before anything moves. It keeps the build before
// and a copy of the app's files in update-before in the app's folder.
// The new build says it started once the interface has asked where
// updates stand, and the build before and the copies go. Until then:
//
//   - On Relaunch the step opens the new build and waits a minute for it
//     to say so. When it does not, the step ends it and puts the build
//     before and the app's files back.
//   - Put in place on quit, the new build is opened later, with nobody
//     waiting, so each of its starts counts itself, and its third start
//     puts the build before and the files back, after two that did not
//     say they started.
//
// The app's files go back with the build because an older build drops
// what it does not know when it writes, and refuses a file of a newer
// store.Version. A build that did not start is written down in
// update-failed.json, and the app never takes it again, not even when
// its channel is picked. See
// docs/UPDATES.md.
const (
	// envLoadCheck asks the program only to be loaded, and to end.
	envLoadCheck = "SPEAKERTRAIL_LOAD_CHECK"
	beforeDir    = "update-before"
	trialName    = "trial.json"
	failedName   = "update-failed.json"
	// startedWithin is how long Relaunch gives the new build to say it
	// started. It opens its window in a second or two, and the first start
	// after an update also has macOS check its signature.
	startedWithin = time.Minute
	// failedStarts is how many starts of a build put in place on quit may
	// not say they started before the next one puts the build before back.
	failedStarts = 2
)

// keptFiles are the app's files that go back with the build before.
var keptFiles = []string{store.FileName, store.TextsName}

// trial is the build on trial, in update-before/trial.json.
type trial struct {
	// Version is the build put in place, and App the .app it went in.
	Version string `json:"version"`
	App     string `json:"app"`
	// Starts counts the starts of it that have not said they started, and
	// PID is the newest of them, which Relaunch ends when it gives up.
	Starts int `json:"starts"`
	PID    int `json:"pid,omitempty"`
}

func trialPath(folder string) string { return filepath.Join(folder, beforeDir, trialName) }

func readTrial(folder string) (trial, bool) {
	var t trial
	data, err := os.ReadFile(trialPath(folder))
	if err != nil || json.Unmarshal(data, &t) != nil || t.Version == "" || !filepath.IsAbs(t.App) {
		return trial{}, false
	}
	return t, true
}

func writeTrial(folder string, t trial) error { return saveJSON(trialPath(folder), t) }

// failedBuild is the build that last did not start, empty when none did.
func failedBuild(folder string) string {
	var f struct {
		Version string `json:"version"`
	}
	data, err := os.ReadFile(filepath.Join(folder, failedName))
	if err != nil || json.Unmarshal(data, &f) != nil {
		return ""
	}
	return f.Version
}

func writeFailed(folder, version string) error {
	return saveJSON(filepath.Join(folder, failedName), map[string]string{"version": version})
}

// keepBefore copies the app's files into update-before, which it makes
// anew. The build before is moved there by the step itself.
func keepBefore(folder string) error {
	before := filepath.Join(folder, beforeDir)
	if err := os.RemoveAll(before); err != nil {
		return err
	}
	if err := os.MkdirAll(before, 0o700); err != nil {
		return err
	}
	for _, name := range keptFiles {
		info, err := os.Stat(filepath.Join(folder, name))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if err := copyFile(filepath.Join(folder, name), filepath.Join(before, name), info.Mode().Perm()); err != nil {
			return err
		}
	}
	return nil
}

// undo puts the build before and the app's files back in place of the
// build on trial, writes that build down as one that did not start, and
// opens the build before. stop ends the build on trial when it still
// runs, nil when it is the one undoing itself.
func undo(folder string, stop func(pid int), launch func(app string) error) error {
	t, ok := readTrial(folder)
	if !ok {
		return nil
	}
	if stop != nil && t.PID > 0 {
		stop(t.PID)
		// It may have said it started as it was ended. Then it stays, and
		// is opened again.
		if _, still := readTrial(folder); !still {
			return launch(t.App)
		}
	}
	before := filepath.Join(folder, beforeDir)
	old := filepath.Join(before, filepath.Base(t.App))
	if _, err := os.Stat(old); err != nil {
		return fmt.Errorf("the build before is not there to put back: %w", err)
	}
	failed := t.App + ".failed"
	_ = os.RemoveAll(failed)
	if err := os.Rename(t.App, failed); err != nil {
		return fmt.Errorf("the build that did not start could not be moved aside: %w", err)
	}
	if err := move(old, t.App); err != nil {
		if back := os.Rename(failed, t.App); back != nil {
			return fmt.Errorf("the build before did not go back (%v), and the app is %s: %w", err, failed, back)
		}
		return fmt.Errorf("the build before did not go back, the app is as it was: %w", err)
	}
	_ = os.RemoveAll(failed)
	var errs []error
	for _, name := range keptFiles {
		kept := filepath.Join(before, name)
		data, err := os.ReadFile(kept)
		if errors.Is(err, fs.ErrNotExist) {
			// It was not there before, so it goes.
			if err := os.Remove(filepath.Join(folder, name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
				errs = append(errs, fmt.Errorf("%s could not be removed: %w", name, err))
			}
			continue
		}
		if err == nil {
			err = writeAtomic(afero.NewOsFs(), filepath.Join(folder, name), data)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s did not go back: %w", name, err))
		}
	}
	if err := writeFailed(folder, t.Version); err != nil {
		errs = append(errs, err)
	}
	// The copies stay when one did not go back, so it can be put back by
	// hand.
	if len(errs) == 0 {
		_ = os.RemoveAll(before)
	} else {
		_ = os.Remove(trialPath(folder))
	}
	if err := launch(t.App); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// startTrial is the first thing a start does once its log is open, before
// it reads the app's file. When this build is on trial it counts the
// start, and on the start after failedStarts that did not say they
// started it puts the build before back and says so, and the app ends.
// A trial of another build is over: whatever undid it already ran.
func startTrial(folder, version string, pid int, launch func(app string) error) (undone bool, err error) {
	t, ok := readTrial(folder)
	if !ok {
		return false, nil
	}
	if t.Version != version {
		return false, os.RemoveAll(filepath.Join(folder, beforeDir))
	}
	if t.Starts >= failedStarts {
		return true, undo(folder, nil, launch)
	}
	t.Starts++
	t.PID = pid
	return false, writeTrial(folder, t)
}

// endTrial is the build on trial saying it started: the build before and
// the copies of the app's files go. Any other build has nothing to end.
func endTrial(folder, version string) error {
	t, ok := readTrial(folder)
	if !ok || t.Version != version {
		return nil
	}
	return os.RemoveAll(filepath.Join(folder, beforeDir))
}

// saysStarted waits until the build on trial said it started, or the
// time is up. It looks four times a second: it runs in the step, after
// the app has quit, where nothing else would tell it.
func saysStarted(folder string, within time.Duration) bool {
	deadline := time.Now().Add(within)
	for {
		if _, err := os.Stat(trialPath(folder)); errors.Is(err, fs.ErrNotExist) {
			return true
		}
		if !time.Now().Before(deadline) {
			return false
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// loads starts a program told only to be loaded and to end, so the
// system loads it and every library it links. A library macOS refuses
// under the hardened runtime ends a program before any of our code runs,
// so only another program can see it.
func loads(program string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, program)
	cmd.Env = append(os.Environ(), envLoadCheck+"=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		if len(out) > 600 {
			out = out[:600]
		}
		return fmt.Errorf("%w: %s", err, bytes.TrimSpace(out))
	}
	return nil
}

// launchApp opens an app the way Finder does, as a new instance.
func launchApp(app string) error {
	return exec.Command("open", "-n", app).Run()
}

// undoIfUnstarted is startTrial for main, in the app's folder.
func undoIfUnstarted() bool {
	folder, err := Folder()
	if err != nil {
		return false
	}
	undone, err := startTrial(folder, runningVersion(), os.Getpid(), launchApp)
	if err != nil {
		theLog.line(zerolog.ErrorLevel, "updates").Err(err).Msg("the build on trial could not be counted or put back")
	}
	if undone {
		theLog.line(zerolog.WarnLevel, "updates").Str("build", runningVersion()).
			Msg("this build did not start twice, so the build before it and the app's files were put back")
	}
	return undone
}
