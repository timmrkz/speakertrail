package main

import (
	"archive/zip"
	"bytes"
	"io"
	"testing"

	"github.com/spf13/afero"
)

// box is the start of an MP4 or a QuickTime file, its first box of kind
// with brand, then what follows.
func box(kind, brand, rest string) []byte {
	return []byte("\x00\x00\x00\x18" + kind + brand + "\x00\x00\x00\x00" + rest)
}

// ebml is the start of a Matroska or a WebM file, then what follows.
func ebml(rest string) []byte {
	return append([]byte{0x1a, 0x45, 0xdf, 0xa3, 0x9f, 0x42, 0x86, 0x81}, rest...)
}

// What a recording is, is told by its first bytes: a MOV, an MP4 and an
// MKV are read as a recording, a HEIC or AVIF picture in the same box is
// not, and neither is anything else.
func TestWhatARecordingIsIsToldByItsFirstBytes(t *testing.T) {
	for _, c := range []struct {
		name string
		data []byte
		want string
	}{
		{"Screen Recording.mov", box("ftyp", "qt  ", ""), "mov"},
		{"phone.mp4", box("ftyp", "isom", ""), "mov"},
		{"old.mov", box("wide", "\x00\x00\x00\x00", ""), "mov"},
		{"recording.webm", ebml(""), "matroska"},
		{"screen.heic", box("ftyp", "heic", ""), ""},
		{"screen.avif", box("ftyp", "avif", ""), ""},
		{"notes.txt", []byte("#EXTM3U\nhttp://example.com/a.ts\n"), ""},
		{"short", []byte("ftyp"), ""},
	} {
		if got := sniffRecording(c.data); got != c.want {
			t.Errorf("%s is read as %q, want %q", c.name, got, c.want)
		}
	}
}

// A screen recording of each kind the usual recorders make is attached
// byte for byte, under the ending its name had when that fits what it
// is, and otherwise under the one every file of its kind can have. A
// report holds it as it came, and once sent it leaves no file behind.
func TestScreenRecordingsAreAttachedAsTheyCame(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name   string
		data   []byte
		ending string
	}{
		{"Screen Recording 2026-11-25 at 14.02.11.mov", box("ftyp", "qt  ", "the picture and the sound"), "mov"},
		{"phone.MP4", box("ftyp", "isom", "the picture and the sound"), "mp4"},
		{"clip.m4v", box("ftyp", "M4V ", "the picture and the sound"), "m4v"},
		// An MP4 named for another kind is kept as what it is.
		{"renamed.webm", box("ftyp", "isom", "the picture and the sound"), "mov"},
		{"browser.webm", ebml("the picture and the sound"), "webm"},
		{"obs.mkv", ebml("the picture and the sound"), "mkv"},
		{"no ending", ebml("the picture and the sound"), "mkv"},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := newReporting(afero.NewMemMapFs(), "/config/report")
			d := r.attach("drop", []handed{{name: c.name, data: c.data}})
			if d.Error != "" || len(d.Pictures) != 1 {
				t.Fatalf("%s gave %+v", c.name, d)
			}
			p := d.Pictures[0]
			if p.Recording != c.ending || p.Name != c.name {
				t.Fatalf("%s is kept as %+v, want a recording kept as %s", c.name, p, c.ending)
			}
			ids, sent := r.reportPictures()
			if len(sent) != 1 || sent[0].ending != c.ending || !bytes.Equal(sent[0].data, c.data) {
				t.Fatalf("a report would hold %+v, want the file as it came", sent)
			}
			r.clear(ids)
			if files, _ := afero.ReadDir(r.files, "/config/report"); len(files) > 1 {
				t.Fatalf("after the report the folder still holds %d files", len(files))
			}
		})
	}
}

// A recording counts toward the ten a report takes, the way a picture
// does, and one past them is left out with the reason.
func TestRecordingsCountTowardTheTen(t *testing.T) {
	r := newReporting(afero.NewMemMapFs(), "/config/report")
	var all []handed
	for range picturesMax {
		all = append(all, handed{name: "a.mov", data: box("ftyp", "qt  ", "")})
	}
	all = append(all, handed{name: "one too many.mov", data: box("ftyp", "qt  ", "")})
	d := r.attach("drop", all)
	if len(d.Pictures) != picturesMax {
		t.Fatalf("the draft holds %d, want %d", len(d.Pictures), picturesMax)
	}
	if want := "one too many.mov cannot be attached because it would be more than the 10 pictures and recordings a report takes"; d.Error != want {
		t.Fatalf("the reason is %q, want %q", d.Error, want)
	}
}

// A report holds a recording under recordings/, stored as it is, and the
// pictures under pictures/, each counted on its own.
//
// On the real disk: the report is a zip in a folder, with the log
// lumberjack keeps on the disk.
func TestAReportHoldsTheRecordings(t *testing.T) {
	folder := t.TempDir()
	testLog(t)
	r := newReporting(afero.NewMemMapFs(), "/config/report")
	m := reportMaker{report: r, folder: func() string { return folder }, show: func(string) error { return nil }}
	film := box("ftyp", "qt  ", "the picture and the sound")
	d := r.attach("drop", []handed{{name: "a.png", data: pngOf(t, 12, 8)}, {name: "Screen Recording.mov", data: film}})
	if d.Error != "" {
		t.Fatal(d.Error)
	}
	made, err := m.ReportProblem("It froze.")
	if err != nil {
		t.Fatal(err)
	}
	z, err := zip.OpenReader(made)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	got := map[string]*zip.File{}
	for _, file := range z.File {
		got[file.Name] = file
	}
	if got["Speaker Trail report/pictures/1.png"] == nil {
		t.Fatalf("the report holds no picture: %v", zipNames(got))
	}
	rec := got["Speaker Trail report/recordings/1.mov"]
	if rec == nil || rec.Method != zip.Store {
		t.Fatalf("the report holds no recording stored as it is: %v", zipNames(got))
	}
	rc, err := rec.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	body, _ := io.ReadAll(rc)
	if !bytes.Equal(body, film) {
		t.Fatal("the recording in the report is not the file that was attached")
	}
}

// A draft kept from before names its recording by an ending of its own,
// and one that names any other, or whose recording is gone, is left out.
func TestADraftKeepsOnlyRecordingsItCanName(t *testing.T) {
	files := afero.NewMemMapFs()
	for _, name := range []string{"p1.mov", "p2.exe", "p3.png"} {
		if err := afero.WriteFile(files, "/config/report/"+name, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	draft := `{"text":"","next":3,"pictures":[
		{"id":"p1","name":"a.mov","recording":"mov"},
		{"id":"p2","name":"b.exe","recording":"exe"},
		{"id":"p3","name":"c.mp4","recording":"mp4"}]}`
	if err := afero.WriteFile(files, "/config/report/draft.json", []byte(draft), 0o600); err != nil {
		t.Fatal(err)
	}
	d := newReporting(files, "/config/report").ReportDraft()
	if len(d.Pictures) != 1 || d.Pictures[0].ID != "p1" || d.Pictures[0].Recording != "mov" {
		t.Fatalf("the draft kept %+v, want only p1", d.Pictures)
	}
}

func zipNames(files map[string]*zip.File) []string {
	var all []string
	for name := range files {
		all = append(all, name)
	}
	return all
}
