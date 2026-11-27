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

// Started by the build before, to put a build in place or to see that
// this one loads, the program does that and nothing of the app: no log,
// no app's file, and so no language model loaded. Wails' helper, which
// Relaunch used before, got as far as loading it, ended inside
// application.New and left llama-server running, once for every Relaunch
// Tim pressed, until a draft found no memory left (21 November 2026).
//
// On the real disk: what is asked is that the program writes nothing to
// it, the app's folder and its log.
func TestTheStepsBeforeTheAppStartNothingOfIt(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		env  []string
		exit int
	}{
		// The app to replace is not an app, so the step ends at once,
		// without waiting for anything.
		{"the step that puts a build in place", []string{
			envInstallTarget + "=Speaker Trail.app",
			envInstallFrom + "=/tmp/wails-update-1/Speaker Trail.app",
			envInstallPID + "=1", envInstallVersion + "=0.3.0-main.5", envInstallOpen + "=1",
		}, 1},
		{"the check that it loads", []string{envLoadCheck + "=1"}, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			tmp := t.TempDir()
			appDir := filepath.Join(tmp, "app")
			cmd := exec.Command(self, "-test.run=^$")
			cmd.Env = append(os.Environ(),
				"SPEAKERTRAIL_TEST_MAIN=1",
				"SPEAKERTRAIL_DIR="+appDir,
				"HOME="+tmp,
				"TMPDIR="+tmp,
				"XDG_CACHE_HOME="+filepath.Join(tmp, "cache"),
				"XDG_CONFIG_HOME="+filepath.Join(tmp, "config"),
			)
			cmd.Env = append(cmd.Env, c.env...)
			err := cmd.Run()
			var exit *exec.ExitError
			switch {
			case c.exit == 0 && err != nil:
				t.Fatalf("it ended with %v", err)
			case c.exit != 0 && (!errors.As(err, &exit) || exit.ExitCode() != c.exit):
				t.Fatalf("it ended with %v, want %d", err, c.exit)
			}
			for _, p := range []string{appDir, filepath.Join(tmp, "cache"), filepath.Join(tmp, "Library")} {
				if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
					t.Errorf("it made %s, as the app does when it starts", p)
				}
			}
		})
	}
}
