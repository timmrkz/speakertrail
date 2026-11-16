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
		return d.SetSetting("profile_searches_per_run", json.RawMessage("7"), time.Now())
	}); err != nil {
		t.Fatal(err)
	}
	var shown []string
	m := reportMaker{report: newReporting(afero.NewMemMapFs(), "/config/report"), st: st, show: func(p string) error { shown = append(shown, p); return nil }}
	first, err := m.ReportProblem("  The run stopped in " + folder + ".  ")
	if err != nil {
		t.Fatal(err)
	}
	second, err := m.ReportProblem("")
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
	want := []string{"about.json", "logs/app-2026-10-09-10-00-00.000.log", "logs/app.log", "logs/recent.log", "settings.json", "what-happened.txt"}
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
