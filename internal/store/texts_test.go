package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const evaText = "# Eva Beispiel\n\n## About\n\nIch töpfere in Köln."

// addEva adds an invented person with the text Exa gave of her profile.
func addEva(t *testing.T, s *Store) int64 {
	t.Helper()
	var id int64
	if err := s.Update(func(d *Data) error {
		search, _ := d.AddSearch("Keramik Köln", 0, t0)
		p, _ := d.AddFound(Found{URL: "https://www.linkedin.com/in/eva-beispiel", Name: "Eva Beispiel", Text: evaText}, search, 1, t0)
		id = p.ID
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return id
}

// The profile texts live in a file of their own, so the one file stays
// small and quick to write on every change. Opened again, each person
// has their text back.
func TestProfileTextsHaveAFileOfTheirOwn(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	id := addEva(t, s)
	main, _ := os.ReadFile(filepath.Join(dir, FileName))
	if strings.Contains(string(main), "töpfere") {
		t.Error("the one file holds the profile text")
	}
	texts, _ := os.ReadFile(filepath.Join(dir, TextsName))
	if !strings.Contains(string(texts), "töpfere") {
		t.Errorf("the texts file has no text: %s", texts)
	}
	open(t, dir).Read(func(d *Data) {
		if l := d.Person(id).LinkedIn; l.Text != evaText || !l.TextAt.Equal(t0) {
			t.Errorf("read back %q from %v", l.Text, l.TextAt)
		}
	})
}

// A change that brings no new text leaves the texts file alone.
func TestASaveWithoutANewTextLeavesTheTextsAlone(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	id := addEva(t, s)
	if err := os.Remove(filepath.Join(dir, TextsName)); err != nil {
		t.Fatal(err)
	}
	if err := s.Update(func(d *Data) error { d.Person(id).Notes = "Töpfert"; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, TextsName)); err == nil {
		t.Error("a change of notes wrote the texts")
	}
}

// A failed change puts the texts back as they were, like everything else.
func TestAFailedUpdateKeepsTheTexts(t *testing.T) {
	s := open(t, t.TempDir())
	id := addEva(t, s)
	err := s.Update(func(d *Data) error {
		d.SetProfileText(d.Person(id), "something else", t0.Add(time.Hour))
		return errors.New("no")
	})
	if err == nil {
		t.Fatal("the update did not fail")
	}
	s.Read(func(d *Data) {
		if d.Person(id).LinkedIn.Text != evaText {
			t.Errorf("after a failed update %q", d.Person(id).LinkedIn.Text)
		}
	})
}

// Builds of pull request 38 before 20 November 2026 kept the texts in the
// one file. Opening such a file moves them over, once.
func TestTextsInTheOneFileMoveOver(t *testing.T) {
	dir := t.TempDir()
	old := `{"version":1,"last_id":1,"people":[{"id":1,"name":"Eva Beispiel","linkedin":{"url":"https://www.linkedin.com/in/eva-beispiel","review":"confirmed","text":"Alt","text_at":"2026-11-19T12:00:00Z"}}]}`
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	open(t, dir)
	main, _ := os.ReadFile(filepath.Join(dir, FileName))
	if strings.Contains(string(main), `"Alt"`) {
		t.Error("the text stayed in the one file")
	}
	open(t, dir).Read(func(d *Data) {
		if l := d.Person(1).LinkedIn; l.Text != "Alt" || l.TextAt.IsZero() {
			t.Errorf("moved over %q from %v", l.Text, l.TextAt)
		}
	})
}

// A newer text already in the texts file beats an older one still in the
// one file, and once moved, opening again writes nothing.
func TestANewerTextStaysAndTheMoveIsOnce(t *testing.T) {
	dir := t.TempDir()
	old := `{"version":1,"last_id":1,"people":[{"id":1,"name":"Eva Beispiel","linkedin":{"url":"https://www.linkedin.com/in/eva-beispiel","review":"confirmed","text":"Alt","text_at":"2026-11-19T12:00:00Z"}}]}`
	texts := `{"version":1,"texts":{"https://www.linkedin.com/in/eva-beispiel":{"text":"Neu","at":"2026-11-20T12:00:00Z"}}}`
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, TextsName), []byte(texts), 0o600); err != nil {
		t.Fatal(err)
	}
	open(t, dir).Read(func(d *Data) {
		if got := d.Person(1).LinkedIn.Text; got != "Neu" {
			t.Errorf("the older text won: %q", got)
		}
	})
	// A texts file that says something else shows whether it was
	// written again.
	marker := `{"version":1,"texts":{"https://www.linkedin.com/in/eva-beispiel":{"text":"Markiert","at":"2026-11-21T12:00:00Z"}}}`
	if err := os.WriteFile(filepath.Join(dir, TextsName), []byte(marker), 0o600); err != nil {
		t.Fatal(err)
	}
	open(t, dir)
	if got, _ := os.ReadFile(filepath.Join(dir, TextsName)); string(got) != marker {
		t.Errorf("opening again wrote the texts: %s", got)
	}
}

// When the texts were written and the one file then was not, the change
// is undone, and the next save writes the texts back as they are in
// memory, so the two files agree again.
func TestTheTextsAgreeAgainAfterAFailedSave(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	id := addEva(t, s)
	// A folder where the one file goes makes moving it into place fail.
	main := filepath.Join(dir, FileName)
	if err := os.Remove(main); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(main, "in-the-way"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := s.Update(func(d *Data) error {
		d.SetProfileText(d.Person(id), "Neu", t0.Add(time.Hour))
		return nil
	}); err == nil {
		t.Fatal("the save did not fail")
	}
	if err := os.RemoveAll(main); err != nil {
		t.Fatal(err)
	}
	if err := s.Update(func(d *Data) error { d.Person(id).Notes = "Töpfert"; return nil }); err != nil {
		t.Fatal(err)
	}
	open(t, dir).Read(func(d *Data) {
		if got := d.Person(id).LinkedIn.Text; got != evaText {
			t.Errorf("after a failed save the file has %q, memory had the text before", got)
		}
	})
}

// Someone whose text Exa cut, by a build that asked Exa for 8000
// characters, is read again in the next run, once, before the activity
// or inside it, where Exa lists the likes last. A page Tim pasted and a
// short text are left as they are (Tim's report of 27 November 2026).
func TestATextCutByExaIsReadAgainOnce(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	career := strings.Repeat("### Inhaberin - Beispiel GmbH\n\nBeispiel GmbH is a company in Köln.\n\n", 120)
	texts := map[string]string{
		"cut":      "# Eva Beispiel\n\n## Experience\n\n" + career,
		"active":   "# Ida Probe\n\n## Experience\n\n" + career + "## Social\n\n- [2026-11-02 · Ida Probe posted this] [Ein Beitrag](https://www.linkedin.com/posts/x)",
		"pasted":   "Ola Muster\n· 1st\n\nExperience\n" + career,
		"short":    evaText,
		"no-texts": "",
	}
	ids := map[string]int64{}
	if err := s.Update(func(d *Data) error {
		search, _ := d.AddSearch("Keramik Köln", 0, t0)
		for name, text := range texts {
			p, _ := d.AddFound(Found{URL: "https://www.linkedin.com/in/" + name, Name: "Eva Beispiel", Text: text}, search, 1, t0)
			ids[name] = p.ID
		}
		// As a file from before.
		d.CutRead = false
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	s = open(t, dir)
	if err := s.Update(func(d *Data) error {
		for name, id := range ids {
			if p := d.Person(id); p.ToRead != (name == "cut" || name == "active") {
				t.Errorf("%s: to read %v", name, p.ToRead)
			}
		}
		// The read finds the profile as before, with no activity.
		d.Person(ids["cut"]).ToRead = false
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	open(t, dir).Read(func(d *Data) {
		if d.Person(ids["cut"]).ToRead {
			t.Error("read again at the next start too")
		}
	})
}
