package main

import (
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/spf13/afero"

	"github.com/timmrkz/speakertrail/internal/store"
)

// oneWay is how every name in a report is written.
var oneWay = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*(\.[a-z0-9]+)*$`)

// A report of a problem is one zip on the Desktop, shown in Finder, with
// what Tim wrote, the log with its older files unpacked and the trace
// lines kept in memory, what the app and the Mac are and the settings. The
// home folder is written as ~ in all of it. A second report on the same
// minute does not take the first one's place.
//
// On the real disk: the report holds the log lumberjack keeps on the disk,
// and lands on the Desktop.
func TestAReportHoldsTheLogAndTheSettings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	desk := filepath.Join(home, "Desktop")
	if err := os.Mkdir(desk, 0o755); err != nil {
		t.Fatal(err)
	}
	path, _ := testLog(t)
	folder := filepath.Join(home, "Library", "Application Support", "Speaker Trail")
	theLog.line(zerolog.WarnLevel, "files").Str("path", folder).Msg("the file could not be written")
	theLog.line(zerolog.TraceLevel, "files").Str("path", folder).Msg("written in 3 ms")
	// An older file of the log, the way lumberjack keeps it.
	older, err := os.Create(filepath.Join(filepath.Dir(path), "app-2026-10-09T10-00-00.000.log.gz"))
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(older)
	_, _ = gz.Write([]byte(`{"level":"info","msg":"started yesterday"}` + "\n"))
	_ = gz.Close()
	_ = older.Close()

	st, err := store.Open(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Update(func(d *store.Data) error {
		if err := d.SetTemplatePart("greeting", []string{"Servus", ""}, -1); err != nil {
			return err
		}
		return d.SetSetting("profile_searches_per_run", json.RawMessage("7"), time.Now())
	}); err != nil {
		t.Fatal(err)
	}
	var shown []string
	m := reportMaker{report: newReporting(afero.NewMemMapFs(), "/config/report"), st: st, show: func(p string) error { shown = append(shown, p); return nil }}
	first, err := m.ReportProblem("  The run stopped in "+folder+".  ", false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := m.ReportProblem("", false)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(first) != desk || first == second {
		t.Errorf("the reports are %s and %s, want two files on the Desktop", first, second)
	}
	if len(shown) != 2 || shown[0] != first {
		t.Errorf("Finder was asked to show %v, want each report", shown)
	}

	files := unzipped(t, first)
	var names []string
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	want := []string{"about.json", "first-message-template.json", "logs/app-2026-10-09-10-00-00.000.log", "logs/app.log", "logs/recent.log", "settings.json", "what-happened.txt"}
	sort.Strings(want)
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("the report holds %v, want %v", names, want)
	}
	// Every name in it is written one way, small letters with a hyphen
	// between words.
	for _, name := range names {
		for _, part := range strings.Split(name, "/") {
			if !oneWay.MatchString(part) {
				t.Errorf("%s in the report is not written in small letters with hyphens", name)
			}
		}
	}
	if got := files["what-happened.txt"]; got != "The run stopped in ~/Library/Application Support/Speaker Trail.\n" {
		t.Errorf("what happened reads %q", got)
	}
	if !strings.Contains(files["logs/app.log"], `"path":"~/Library/Application Support/Speaker Trail"`) {
		t.Errorf("the log in the report does not name the folder under ~:\n%s", files["logs/app.log"])
	}
	if !strings.Contains(files["logs/recent.log"], `"msg":"written in 3 ms"`) {
		t.Errorf("the trace lines kept in memory are not in the report:\n%s", files["logs/recent.log"])
	}
	if !strings.Contains(files["logs/app-2026-10-09-10-00-00.000.log"], "started yesterday") {
		t.Error("the older file of the log is not in the report, unpacked")
	}
	if !strings.Contains(files["settings.json"], `"key": "profile_searches_per_run",`+"\n    \"value\": 7") {
		t.Errorf("the settings in the report do not hold the changed setting:\n%s", files["settings.json"])
	}
	// The template as Tim changed it, in the shape of the one in the code.
	var tpl store.Template
	if err := json.Unmarshal([]byte(files["first-message-template.json"]), &tpl); err != nil {
		t.Fatalf("the template in the report cannot be read: %v", err)
	}
	if g, _ := tpl.Part("greeting"); strings.Join(g.Variations, "|") != "Servus|" || len(tpl.Parts) != len(store.DefaultTemplate().Parts) {
		t.Errorf("the template in the report:\n%s", files["first-message-template.json"])
	}
	if !strings.Contains(files["first-message-template.json"], "\n  \"origin\": [\n") {
		t.Errorf("the template is not written one part to a line:\n%s", files["first-message-template.json"])
	}
	if !strings.Contains(files["about.json"], `"start": "`+theLog.start+`"`) {
		t.Errorf("about.json does not name the start of the app:\n%s", files["about.json"])
	}
	for name, text := range files {
		if strings.Contains(text, home) {
			t.Errorf("%s names the home folder", name)
		}
	}
	if _, ok := unzipped(t, second)["what-happened.txt"]; ok {
		t.Error("a report with nothing written holds an empty what-happened.txt")
	}
}

// With People switched on, a report holds people.json, every person
// with the profile text and without their name or link. Without it, it
// holds no person at all, and the next report starts with People off.
func TestAReportTakesPeopleAlongOnlyWhenAsked(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	testLog(t)
	st, err := store.Open(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Update(func(d *store.Data) error {
		s, err := d.AddSearch("Floristin Münster", 1, time.Now())
		if err != nil {
			return err
		}
		d.AddFound(store.Found{URL: "https://www.linkedin.com/in/oda-brinkhege", Handle: "oda-brinkhege", Name: "Oda Brinkhege",
			Headline: "Floristin und Inhaberin, Blumen Brinkhege", City: "Münster", Text: "Ich bin Oda, Floristin aus Münster."}, s, 1, time.Now())
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	r := newReporting(afero.NewMemMapFs(), "/config/report")
	r.TakePeople(true)
	m := reportMaker{report: r, st: st, folder: func() string { return home }, show: func(string) error { return nil }}
	with, err := m.ReportProblem("Why is she first?", true)
	if err != nil {
		t.Fatal(err)
	}
	people := unzipped(t, with)["people.json"]
	var shared []store.Shared
	if err := json.Unmarshal([]byte(people), &shared); err != nil || len(shared) != 1 {
		t.Fatalf("people.json is %v, %s", err, people)
	}
	if shared[0].Headline != "Floristin und Inhaberin, Blumen [name]" || shared[0].Text != "Ich bin [name], Floristin aus Münster." || shared[0].City != "Münster" {
		t.Errorf("the person went as %+v", shared[0])
	}
	for _, never := range []string{"Oda", "Brinkhege", "linkedin"} {
		if strings.Contains(people, never) {
			t.Errorf("%q went along in people.json", never)
		}
	}
	if r.ReportDraft().People {
		t.Error("People is still on for the next report")
	}
	without, err := m.ReportProblem("", false)
	if err != nil {
		t.Fatal(err)
	}
	for name, text := range unzipped(t, without) {
		if name == "people.json" || strings.Contains(text, "Floristin und") {
			t.Errorf("a report without People holds a person in %s", name)
		}
	}
}

func unzipped(t *testing.T, path string) map[string]string {
	t.Helper()
	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	files := map[string]string{}
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(rc)
		_ = rc.Close()
		files[strings.TrimPrefix(f.Name, "Speaker Trail report/")] = string(b)
	}
	return files
}
