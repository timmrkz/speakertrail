package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The test program is the app too: started with SPEAKERTRAIL_TEST_MAIN
// set, it runs main, so a test sees what the app does when it is started
// the way the system starts it.
func TestMain(m *testing.M) {
	if os.Getenv("SPEAKERTRAIL_TEST_MAIN") == "1" {
		main()
		return
	}
	os.Exit(m.Run())
}

// Started by Relaunch as Wails' updater, the program does the updater's
// step and nothing of the app: no log, no app's file, and so no language
// model loaded. A helper that got as far as loading it ended inside
// application.New and left llama-server running, once for every Relaunch
// Tim pressed, until a draft found no memory left (21 November 2026).
//
// On the real disk: what is asked is that the program writes nothing to
// it, the app's folder and its log.
func TestTheUpdatersHelperStartsNothingOfTheApp(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	appDir := filepath.Join(tmp, "app")
	cmd := exec.Command(self, "-test.run=^$")
	cmd.Env = append(os.Environ(),
		"SPEAKERTRAIL_TEST_MAIN=1",
		"SPEAKERTRAIL_DIR="+appDir,
		"HOME="+tmp,
		"XDG_CACHE_HOME="+filepath.Join(tmp, "cache"),
		"XDG_CONFIG_HOME="+filepath.Join(tmp, "config"),
		// What Wails' updater gives the helper. The app to replace is not
		// there, so the step ends at once, without waiting for anything.
		"WAILS_UPDATER_HELPER=1",
		"WAILS_UPDATER_HELPER_TARGET="+filepath.Join(tmp, "Speaker Trail.app"),
		"WAILS_UPDATER_HELPER_NEW="+filepath.Join(tmp, "wails-update-1", "Speaker Trail.app"),
	)
	err = cmd.Run()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 10 {
		t.Fatalf("the helper ended with %v, want the updater's 10 for an app that is not there", err)
	}
	for _, p := range []string{appDir, filepath.Join(tmp, "cache"), filepath.Join(tmp, "Library")} {
		if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("the helper made %s, as the app does when it starts", p)
		}
	}
}
