// Package carried finds a program the app carries beside its own, in
// Contents/MacOS of the bundle: llama-server, which runs the language
// model, and ffmpeg and ffprobe, which read a screen recording handed to
// Report a Problem. make builds them, see docs/BUILD.md.
//
// Nothing is taken from the search path, so what runs is what was built
// and shipped, never whatever Homebrew put on the Mac.
package carried

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// ErrMissing is a program that is not beside the app's own.
var ErrMissing = errors.New("is not beside the app")

// Find is the program called name: the one the environment variable env
// names, for a try, or else the one beside the app's program.
func Find(env, name string) (string, error) {
	if named := os.Getenv(env); named != "" {
		if _, err := exec.LookPath(named); err != nil {
			return "", fmt.Errorf("%s names %s, which cannot be run", env, named)
		}
		return named, nil
	}
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	return In(filepath.Dir(self), name)
}

// In is the program called name in dir, when it is a file that runs.
func In(dir, name string) (string, error) {
	path := filepath.Join(dir, name)
	if info, err := os.Stat(path); err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
		return "", fmt.Errorf("%s %w", name, ErrMissing)
	}
	return path, nil
}
