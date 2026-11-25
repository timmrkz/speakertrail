package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"image/png"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/afero"
)

// The recordings in testdata/ are FrameFairy's fixtures, a second of
// ffmpeg's test picture at 160 by 90 and a tone, written the way a phone,
// a recorder and the Mac's own write one, with a place, a title and a
// comment: in a MOV, in an MP4 held upright, in Matroska the way OBS
// writes it, and in WebM the way a browser does. Their title and comment
// name nobody. tone-then-silence.m4a is a sound with no picture.

// needMedia is the ffmpeg and ffprobe on the search path, for a test that
// reads a recording. On a machine of one's own a test without them is
// skipped with the reason. In CI it fails, because a skip reads as a pass
// there, and a run without ffmpeg would be as green as one that read the
// recordings. As FrameFairy's ffmpegtest.Need.
func needMedia(t *testing.T) func() (media, error) {
	t.Helper()
	var found media
	for _, p := range []*string{&found.ffmpeg, &found.ffprobe} {
		name := "ffmpeg"
		if p == &found.ffprobe {
			name = "ffprobe"
		}
		path, err := exec.LookPath(name)
		if err != nil {
			if os.Getenv("CI") != "" {
				t.Fatalf("%s is not installed. In CI that fails, because a skipped test reads as a pass", name)
			}
			t.Skipf("%s is not installed", name)
		}
		*p = path
	}
	return func() (media, error) { return found, nil }
}

// fixture reads a file of testdata/.
func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// What a file is, is told by its first bytes: a MOV or an MP4, a WebM or
// an MKV is read as a recording, a HEIC or AVIF picture in the same box is
// not, and neither is anything else.
func TestWhatARecordingIsIsToldByItsFirstBytes(t *testing.T) {
	box := func(kind, brand string) []byte { return []byte("\x00\x00\x00\x18" + kind + brand + "\x00\x00\x00\x00") }
	for _, c := range []struct {
		name string
		data []byte
		want string
	}{
		{"Screen Recording.mov", box("ftyp", "qt  "), "mov"},
		{"phone.mp4", box("ftyp", "isom"), "mov"},
		{"old.mov", box("wide", "\x00\x00\x00\x00"), "mov"},
		{"recording.webm", []byte{0x1a, 0x45, 0xdf, 0xa3, 0x9f, 0x42, 0x86, 0x81}, "matroska"},
		{"screen.heic", box("ftyp", "heic"), ""},
		{"screen.avif", box("ftyp", "avif"), ""},
		{"notes.txt", []byte("#EXTM3U\nhttp://example.com/a.ts\n"), ""},
		{"short", []byte("ftyp"), ""},
	} {
		if got := sniffRecording(c.data); got != c.want {
			t.Errorf("%s is read as %q, want %q", c.name, got, c.want)
		}
	}
}

// A screen recording of each kind the usual recorders make is attached:
// its size and length are known, the tile has a still of its first
// frame, and the copy kept holds its picture and its sound and nothing
// the file said about itself, not the place, the title or the device. A
// report holds the copy under recordings/, and a recording removed and
// let go of leaves no file behind.
//
// On the real disk: ffprobe and ffmpeg read the recordings.
func TestScreenRecordingsAreAttachedWithoutWhatElseTheyHeld(t *testing.T) {
	tools := needMedia(t)
	t.Parallel()
	for _, c := range []struct {
		name    string
		fixture string
		ending  string
	}{
		{"Screen Recording 2026-11-25 at 14.02.11.mov", "recording.mov", "mov"},
		// A phone held upright, whose picture lies on its side.
		{"phone.mp4", "recording-upright.mp4", "mp4"},
		{"browser.webm", "recording.webm", "webm"},
		{"obs.mkv", "recording.mkv", "mkv"},
	} {
		t.Run(c.ending, func(t *testing.T) {
			data := fixture(t, c.fixture)
			if !bytes.Contains(data, []byte("52.52")) {
				t.Fatalf("the test recording %s does not hold the place it should be copied without", c.name)
			}
			r := newReporting(afero.NewMemMapFs(), "/config/report")
			r.media = tools
			d := r.attach("drop", []handed{{name: c.name, data: data}})
			if d.Error != "" || len(d.Pictures) != 1 {
				t.Fatalf("%s gave %+v", c.name, d)
			}
			p := d.Pictures[0]
			want := [2]int{160, 90}
			if c.ending == "mp4" {
				want = [2]int{90, 160}
			}
			if p.Recording != c.ending || [2]int{p.Width, p.Height} != want || math.Abs(p.Seconds-1) > 0.1 {
				t.Fatalf("%s is kept as %+v, want a recording of %v kept as %s, one second long", c.name, p, want, c.ending)
			}
			still, err := afero.ReadFile(r.files, "/config/report/"+p.ID+".png")
			if err != nil {
				t.Fatal(err)
			}
			cfg, err := png.DecodeConfig(bytes.NewReader(still))
			if err != nil || [2]int{cfg.Width, cfg.Height} != want {
				t.Fatalf("the still of %s is %dx%d, %v, want %v", c.name, cfg.Width, cfg.Height, err, want)
			}
			ids, sent := r.reportPictures()
			if len(sent) != 1 || sent[0].ending != c.ending {
				t.Fatalf("a report would hold %+v", sent)
			}
			copied := filepath.Join(t.TempDir(), "copy."+c.ending)
			if err := os.WriteFile(copied, sent[0].data, 0o600); err != nil {
				t.Fatal(err)
			}
			kinds, tags := probeTags(t, copied)
			if kinds != "video audio" {
				t.Fatalf("the copy of %s holds %q, want its picture and its sound", c.name, kinds)
			}
			if bytes.Contains(sent[0].data, []byte("52.52")) || bytes.Contains(sent[0].data, []byte("A lab desk")) {
				t.Fatalf("the copy of %s still holds the place or the title it was recorded with", c.name)
			}
			for _, tag := range []string{"location", "title", "comment", "com.apple.quicktime.location.ISO6709"} {
				if _, ok := tags[tag]; ok {
					t.Fatalf("the copy of %s still says its %s: %v", c.name, tag, tags)
				}
			}
			// Sent, it is let go of with its still.
			r.clear(ids)
			for _, name := range []string{p.ID + ".png", p.ID + "." + c.ending} {
				if ok, _ := afero.Exists(r.files, "/config/report/"+name); ok {
					t.Fatalf("%s is still kept after the report", name)
				}
			}
		})
	}
}

// A file that starts like a recording and is none, and a recording with
// no picture in it, are left out with the reason, and a picture beside
// them is still attached.
//
// On the real disk: ffprobe reads what was handed over.
func TestWhatIsNoRecordingIsLeftOut(t *testing.T) {
	tools := needMedia(t)
	t.Parallel()
	r := newReporting(afero.NewMemMapFs(), "/config/report")
	r.media = tools
	d := r.attach("drop", []handed{
		{name: "broken.mov", data: []byte("\x00\x00\x00\x18ftypqt  \x00\x00\x00\x00 and then nothing")},
		{name: "voice.m4a", data: fixture(t, "tone-then-silence.m4a")},
		{name: "fine.png", data: pngOf(t, 8, 8)},
	})
	for _, why := range []string{"broken.mov cannot be attached because it is not a picture or a screen recording", "voice.m4a cannot be attached because it is not a picture or a screen recording"} {
		if !strings.Contains(d.Error, why) {
			t.Errorf("the reason %q does not say %q", d.Error, why)
		}
	}
	if len(d.Pictures) != 1 || d.Pictures[0].Name != "fine.png" {
		t.Fatalf("the one good picture among them gave %+v", d.Pictures)
	}
}

// An app without its ffmpeg says so for a recording, and still attaches
// a picture beside it.
func TestARecordingWithoutFFmpegSaysWhy(t *testing.T) {
	testLog(t)
	r := newReporting(afero.NewMemMapFs(), "/config/report")
	r.media = findMedia
	t.Setenv("SPEAKERTRAIL_FFMPEG", "")
	d := r.attach("drop", []handed{
		{name: "Screen Recording.mov", data: []byte("\x00\x00\x00\x18ftypqt  \x00\x00\x00\x00")},
		{name: "fine.png", data: pngOf(t, 8, 8)},
	})
	if want := "Screen Recording.mov cannot be attached because it needs ffmpeg, which the app is missing"; d.Error != want {
		t.Errorf("the reason is %q, want %q", d.Error, want)
	}
	if len(d.Pictures) != 1 || d.Pictures[0].Name != "fine.png" {
		t.Fatalf("the picture beside it gave %+v", d.Pictures)
	}
}

// A report holds a recording under recordings/, stored as it is, and the
// pictures under pictures/, each counted on its own.
//
// On the real disk: ffmpeg reads the recording, and the report is a zip
// in a folder with the log lumberjack keeps on the disk.
func TestAReportHoldsTheRecordings(t *testing.T) {
	tools := needMedia(t)
	folder := t.TempDir()
	testLog(t)
	r := newReporting(afero.NewMemMapFs(), "/config/report")
	r.media = tools
	m := reportMaker{report: r, folder: func() string { return folder }, show: func(string) error { return nil }}
	d := r.attach("drop", []handed{{name: "a.png", data: pngOf(t, 12, 8)}, {name: "Screen Recording.mov", data: fixture(t, "recording.mov")}})
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
	if sniffRecording(body) != "mov" {
		t.Fatal("the recording in the report is not a MOV")
	}
}

// A draft kept from before names its recording by an ending of its own,
// and one that names any other, or whose recording is gone, is left out.
func TestADraftKeepsOnlyRecordingsItCanName(t *testing.T) {
	files := afero.NewMemMapFs()
	for _, name := range []string{"p1.png", "p1.mov", "p2.png", "p2.exe", "p3.png"} {
		if err := afero.WriteFile(files, "/config/report/"+name, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	draft := `{"text":"","next":3,"pictures":[
		{"id":"p1","name":"a.mov","recording":"mov","seconds":2},
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

// probeTags says what kinds of streams a file holds, in order, and the
// tags of the file.
func probeTags(t *testing.T, path string) (string, map[string]string) {
	t.Helper()
	out, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "stream=codec_type:format_tags",
		"-of", "json", path).Output()
	if err != nil {
		t.Fatalf("ffprobe %s: %v", path, err)
	}
	var data struct {
		Streams []struct {
			Kind string `json:"codec_type"`
		} `json:"streams"`
		Format struct {
			Tags map[string]string `json:"tags"`
		} `json:"format"`
	}
	if err := json.Unmarshal(out, &data); err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, s := range data.Streams {
		kinds = append(kinds, s.Kind)
	}
	return strings.Join(kinds, " "), data.Format.Tags
}

func zipNames(files map[string]*zip.File) []string {
	var all []string
	for name := range files {
		all = append(all, name)
	}
	return all
}
