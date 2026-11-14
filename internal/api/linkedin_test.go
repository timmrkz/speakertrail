package api

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"

	"github.com/timmrkz/speakertrail/internal/browser"
)

// macs is a Mac with Google Chrome and two invented profiles. It keeps
// what it was asked to open.
type macs struct {
	mu     sync.Mutex
	opened []string
}

func (m *macs) Profiles(app string) ([]browser.Profile, error) {
	if app != "Google Chrome" {
		return nil, browser.ErrNoBrowser
	}
	return []browser.Profile{{Folder: "Default", Name: "Privat"}, {Folder: "Profile 1", Name: "Arbeit"}}, nil
}

func (m *macs) Open(ctx context.Context, app, folder, link string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.opened = append(m.opened, app+"|"+folder+"|"+link)
	return nil
}

func TestLinkedInOpensInTheChosenProfile(t *testing.T) {
	a, _, _ := newAPI(t)
	m := &macs{}
	a.Browsers = m
	link := `{"url":"https://www.linkedin.com/in/lea-beispiel"}`

	if code := call(t, a, "POST", "/api/open", link, nil); code != http.StatusConflict {
		t.Errorf("open before a profile is chosen: %d", code)
	}
	if code := call(t, a, "POST", "/api/open", `{"url":"file:///etc/passwd"}`, nil); code != http.StatusBadRequest {
		t.Errorf("a link that is not https: %d", code)
	}
	if code := call(t, a, "PUT", "/api/linkedin", `{"app":"Google Chrome","folder":"Profile 9"}`, nil); code != http.StatusBadRequest {
		t.Errorf("a profile that is not there: %d", code)
	}
	if code := call(t, a, "PUT", "/api/linkedin", `{"app":"Netscape","folder":"Default"}`, nil); code != http.StatusBadRequest {
		t.Errorf("a browser that is not known: %d", code)
	}
	var where struct {
		App, Folder string
		Known       bool
		Browsers    []browserJSON
	}
	if code := call(t, a, "PUT", "/api/linkedin", `{"app":"Google Chrome","folder":"Profile 1"}`, &where); code != http.StatusOK || !where.Known || where.Folder != "Profile 1" {
		t.Errorf("choose: %d %+v", code, where)
	}
	if code := call(t, a, "POST", "/api/open", link, nil); code != http.StatusNoContent {
		t.Errorf("open: %d", code)
	}
	if len(m.opened) != 1 || m.opened[0] != "Google Chrome|Profile 1|https://www.linkedin.com/in/lea-beispiel" {
		t.Errorf("opened %v", m.opened)
	}
	var list struct{ Settings []struct{ Key string } }
	call(t, a, "GET", "/api/settings", "", &list)
	for _, s := range list.Settings {
		if s.Key == "linkedin_profile" {
			t.Error("the profile shows among the plain settings")
		}
	}
	// Where LinkedIn opens is chosen only through PUT /api/linkedin, which
	// checks the profile is there. A plain setting would skip that check.
	if code := call(t, a, "PATCH", "/api/settings/linkedin_profile", `{"value":"Profile 9"}`, nil); code != http.StatusNotFound {
		t.Errorf("the profile set as a plain setting: %d", code)
	}
	call(t, a, "GET", "/api/linkedin", "", &where)
	if where.Folder != "Profile 1" {
		t.Errorf("the profile after a plain setting: %+v", where)
	}
}

func TestLinkedInFromSeveralAtOnce(t *testing.T) {
	a, _, _ := newAPI(t)
	m := &macs{}
	a.Browsers = m
	call(t, a, "PUT", "/api/linkedin", `{"app":"Google Chrome","folder":"Default"}`, nil)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			folder := []string{"Default", "Profile 1"}[i%2]
			call(t, a, "PUT", "/api/linkedin", fmt.Sprintf(`{"app":"Google Chrome","folder":%q}`, folder), nil)
			if code := call(t, a, "POST", "/api/open", `{"url":"https://www.linkedin.com/in/max-muster"}`, nil); code != http.StatusNoContent {
				t.Errorf("open: %d", code)
			}
			call(t, a, "GET", "/api/linkedin", "", nil)
		})
	}
	wg.Wait()
	if len(m.opened) != 8 {
		t.Errorf("opened %d times", len(m.opened))
	}
}
