package browser

import (
	"errors"
	"reflect"
	"testing"
)

func TestProfilesReadsLocalState(t *testing.T) {
	got, err := Profiles("testdata/support", "Google Chrome")
	if err != nil {
		t.Fatal(err)
	}
	want := []Profile{{"Profile 1", "Arbeit"}, {"Default", "Privat"}, {"Profile 3", "Profile 3"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestProfilesWithoutTheBrowser(t *testing.T) {
	if _, err := Profiles("testdata/support", "Vivaldi"); !errors.Is(err, ErrNoBrowser) {
		t.Fatalf("got %v, want ErrNoBrowser", err)
	}
	if _, err := Profiles("testdata/support", "Safari"); err == nil {
		t.Fatal("Safari has no profiles the app can name, want an error")
	}
}

func TestProfilesOfABrokenOrEmptyFile(t *testing.T) {
	if _, err := Profiles("testdata/broken", "Google Chrome"); err == nil {
		t.Fatal("a broken Local State, want an error")
	}
	got, err := Profiles("testdata/empty", "Google Chrome")
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, %v, want no profiles and no error", got, err)
	}
}

func TestCheckLink(t *testing.T) {
	for _, c := range []struct {
		link string
		ok   bool
	}{
		{"https://www.linkedin.com/in/erika-beispiel-123", true},
		{"https://de.linkedin.com/in/max-muster?trk=x", true},
		{"--remote-debugging-port=1", false},
		{"-n", false},
		{"http://www.linkedin.com/in/x", false},
		{"javascript:alert(1)", false},
		{"file:///etc/passwd", false},
		{"https://www.linkedin.com/in/a b", false},
		{"https://user:pw@www.linkedin.com/in/x", false},
		{"https:///in/x", false},
		{"", false},
	} {
		if err := CheckLink(c.link); (err == nil) != c.ok {
			t.Errorf("CheckLink(%q) = %v, want ok %v", c.link, err, c.ok)
		}
	}
}

func TestCommand(t *testing.T) {
	link := "https://www.linkedin.com/in/x"
	got := Command("Google Chrome", "Profile 1", link, false)
	want := []string{"-n", "-a", "Google Chrome", "--args", "--profile-directory=Profile 1", link}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	got = Command("Brave Browser", "Default", link, true)
	want = []string{"-n", "-g", "-a", "Brave Browser", "--args", "--profile-directory=Default", link}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestOpenRefusesBeforeStartingAnything(t *testing.T) {
	if err := Open(t.Context(), "testdata/support", "Google Chrome", "Profile 1", "--remote-debugging-port=1"); err == nil {
		t.Fatal("an unsafe link, want an error")
	}
}
