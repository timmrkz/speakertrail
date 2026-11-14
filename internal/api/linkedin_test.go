package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/timmrkz/speakertrail/internal/browser"
	"github.com/timmrkz/speakertrail/internal/store"
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

// lea adds an invented person with a LinkedIn profile and gives their id.
func lea(t *testing.T, st *store.Store, review string) string {
	t.Helper()
	var id int64
	err := st.Update(func(d *store.Data) error {
		id = d.NewID()
		d.People = append(d.People, &store.Person{ID: id, Name: "Lea Beispiel",
			LinkedIn: store.LinkedIn{URL: "https://www.linkedin.com/in/lea-beispiel", Review: review}})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return itoa(id)
}

func TestLinkedInOpensInTheChosenProfile(t *testing.T) {
	a, st, _ := newAPI(t)
	m := &macs{}
	a.Browsers = m
	open := "/api/people/" + lea(t, st, "confirmed") + "/linkedin"

	var problem struct {
		Error  string
		Choose bool
	}
	if code := call(t, a, "POST", open, `{}`, &problem); code != http.StatusConflict || !problem.Choose {
		t.Errorf("open before a profile is chosen: %d %+v", code, problem)
	}
	if code := call(t, a, "POST", "/api/people/"+lea(t, st, "rejected")+"/linkedin", `{}`, nil); code != http.StatusNotFound {
		t.Errorf("a rejected profile: %d", code)
	}
	if code := call(t, a, "POST", "/api/people/999/linkedin", `{}`, nil); code != http.StatusNotFound {
		t.Errorf("nobody: %d", code)
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
	if code := call(t, a, "POST", open, `{}`, nil); code != http.StatusNoContent {
		t.Errorf("open: %d", code)
	}
	if len(m.opened) != 1 || m.opened[0] != "Google Chrome|Profile 1|https://www.linkedin.com/in/lea-beispiel" {
		t.Errorf("opened %v", m.opened)
	}
	var list struct{ Settings []struct{ Key string } }
	call(t, a, "GET", "/api/settings", "", &list)
	for _, s := range list.Settings {
		if strings.HasPrefix(s.Key, "linkedin") {
			t.Errorf("%s shows among the settings", s.Key)
		}
	}
}

// solo is a Mac with one browser and one profile in it.
type solo struct{ macs }

func (m *solo) Profiles(app string) ([]browser.Profile, error) {
	if app != "Google Chrome" {
		return nil, browser.ErrNoBrowser
	}
	return []browser.Profile{{Folder: "Default", Name: "Privat"}}, nil
}

// One browser with one profile counts as chosen, the same for where
// Settings says LinkedIn opens and for where it opens.
func TestLinkedInWithOneProfileOpensThere(t *testing.T) {
	a, st, _ := newAPI(t)
	m := &solo{}
	a.Browsers = m
	var where struct{ App, Folder string }
	call(t, a, "GET", "/api/linkedin", "", &where)
	if where.App != "Google Chrome" || where.Folder != "Default" {
		t.Errorf("where = %+v", where)
	}
	if code := call(t, a, "POST", "/api/people/"+lea(t, st, "confirmed")+"/linkedin", `{}`, nil); code != http.StatusNoContent {
		t.Errorf("open: %d", code)
	}
	if len(m.opened) != 1 || m.opened[0] != "Google Chrome|Default|https://www.linkedin.com/in/lea-beispiel" {
		t.Errorf("opened %v", m.opened)
	}
}

func TestLinkedInFromSeveralAtOnce(t *testing.T) {
	a, st, _ := newAPI(t)
	m := &macs{}
	a.Browsers = m
	open := "/api/people/" + lea(t, st, "confirmed") + "/linkedin"
	call(t, a, "PUT", "/api/linkedin", `{"app":"Google Chrome","folder":"Default"}`, nil)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			folder := []string{"Default", "Profile 1"}[i%2]
			call(t, a, "PUT", "/api/linkedin", fmt.Sprintf(`{"app":"Google Chrome","folder":%q}`, folder), nil)
			if code := call(t, a, "POST", open, `{}`, nil); code != http.StatusNoContent {
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
