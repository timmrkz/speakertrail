// Package browser opens a link in one profile of a Chromium browser on
// the Mac, the profile where Tim is logged in to LinkedIn. A login lives
// in the profile's cookies, so naming the profile is enough, and the
// browser picks one of its windows or opens one. See
// docs/open-in-profile.md for why and how this was tested.
package browser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// Apps are the browsers whose profiles the app can name, and the folder
// under ~/Library/Application Support each keeps its Local State in.
var Apps = []struct{ App, Folder string }{
	{"Google Chrome", "Google/Chrome"},
	{"Brave Browser", "BraveSoftware/Brave-Browser"},
	{"Microsoft Edge", "Microsoft Edge"},
	{"Vivaldi", "Vivaldi"},
	{"Chromium", "Chromium"},
}

// Profile is one profile of a browser: the folder the browser knows it
// by, which never changes, and the name Tim gave it, which may.
type Profile struct {
	Folder string `json:"folder"`
	Name   string `json:"name"`
}

func folderOf(app string) (string, bool) {
	for _, a := range Apps {
		if a.App == app {
			return a.Folder, true
		}
	}
	return "", false
}

// ErrNoBrowser says the browser has no Local State, so it is not on this
// Mac or was never started.
var ErrNoBrowser = errors.New("this browser has no profiles on this Mac")

// Profiles reads the profiles of app from its Local State under support,
// which is ~/Library/Application Support on a Mac, sorted by name.
func Profiles(support, app string) ([]Profile, error) {
	folder, ok := folderOf(app)
	if !ok {
		return nil, fmt.Errorf("%s is not a browser the app can open links in", app)
	}
	raw, err := os.ReadFile(filepath.Join(support, folder, "Local State"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNoBrowser
	}
	if err != nil {
		return nil, err
	}
	var state struct {
		Profile struct {
			InfoCache map[string]struct {
				Name string `json:"name"`
			} `json:"info_cache"`
		} `json:"profile"`
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, fmt.Errorf("the Local State of %s cannot be read: %w", app, err)
	}
	out := make([]Profile, 0, len(state.Profile.InfoCache))
	for dir, p := range state.Profile.InfoCache {
		name := p.Name
		if name == "" {
			name = dir
		}
		out = append(out, Profile{Folder: dir, Name: name})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Folder < out[j].Folder
	})
	return out, nil
}

// CheckLink passes only an https link without spaces. A link is untrusted,
// it came from Exa, and one that started with -- would reach the browser
// as a switch on its command line.
func CheckLink(link string) error {
	if link == "" || strings.ContainsAny(link, " \t\r\n") || strings.HasPrefix(link, "-") {
		return errors.New("the link is not one the app opens")
	}
	u, err := url.Parse(link)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return errors.New("the app only opens https links")
	}
	return nil
}

// Command is what open is run with: a second copy of the browser, -n,
// that hands the link to the running one in the named profile and quits.
// -g leaves the browser behind the app.
func Command(app, folder, link string, background bool) []string {
	args := []string{"-n"}
	if background {
		args = append(args, "-g")
	}
	return append(args, "-a", app, "--args", "--profile-directory="+folder, link)
}

// Open opens link in the profile folder of app. It checks the link, and
// that the folder is still one of the browser's profiles, because Chrome
// makes a new, logged out profile for a folder it does not know.
func Open(ctx context.Context, support, app, folder, link string) error {
	if err := CheckLink(link); err != nil {
		return err
	}
	if runtime.GOOS != "darwin" {
		return errors.New("only the Mac app opens links in a browser profile")
	}
	profiles, err := Profiles(support, app)
	if err != nil {
		return err
	}
	known := false
	for _, p := range profiles {
		known = known || p.Folder == folder
	}
	if !known {
		return fmt.Errorf("%s has no profile %q any more. Choose another in Settings", app, folder)
	}
	// Never through a shell: every argument goes to open as it is.
	if out, err := exec.CommandContext(ctx, "open", Command(app, folder, link, false)...).CombinedOutput(); err != nil {
		return fmt.Errorf("%s did not open: %s", app, strings.TrimSpace(string(out)))
	}
	return nil
}

// Support is ~/Library/Application Support, where the browsers keep
// their Local State on a Mac.
func Support() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Library", "Application Support")
}
