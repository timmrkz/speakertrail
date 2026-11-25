package main

// speakertrail-release ffmpeg builds the ffmpeg and ffprobe the app
// carries, from source, for the machine it runs on. They read a screen
// recording handed to Report a Problem, see recording.go in
// cmd/speakertrail-app, issue #49.
//
// It is FrameFairy's scripts/build-ffmpeg.sh, at its pins, written in Go,
// since Speaker Trail has no shell scripts. One difference: it builds no
// libass, and so no freetype, fribidi or harfbuzz, which FrameFairy needs
// only to burn captions into a short. The configure line is otherwise
// FrameFairy's, and so are the checks on what came out.
//
// Why our own. ffmpeg is LGPL until it is configured with --enable-gpl,
// which exists to let in GPL encoders, libx264 above all. Nearly every
// prebuilt ffmpeg has libx264, so nearly every one is GPL. This one is
// not, and the check below reads that off the binary rather than trusting
// the flags.
//
// It is static, one file with nothing to chase. make runs it when
// .build/ffmpeg is missing or this file changed, and CI keeps what it
// leaves in a cache by this file, see docs/BUILD.md.

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
)

// The ffmpeg built, pinned by its tag and the commit the tag has to be,
// since a tag can be moved and a commit cannot. FrameFairy's pin of 25
// November 2026. Raising it means raising both: git ls-remote of the tag,
// the line ending ^{}. Taken from git, not ffmpeg.org, which publishes no
// checksum to hold an archive to, and which a cloud session cannot reach.
const (
	ffmpegVersion = "9.0.2"
	ffmpegCommit  = "946fcce07b6dcd0331c8cc609192aeff5e1924f8"
	ffmpegGit     = "https://github.com/FFmpeg/FFmpeg.git"
)

// ffmpegConfigure is the configure line, the whole point of this, in one
// place. No --enable-gpl and no --enable-nonfree, which is what makes the
// result LGPL. What it may read from is cut down to what the app gives
// it, because a recording is a file somebody else may have made: a path
// to read and a path to write, so file and pipe are the only ways in and
// out. No network, and none of the formats that are lists of other files,
// hls, dash, concat, imf and webm_dash_manifest, which is how a crafted
// video has pulled somebody's own files into what they then sent. On a
// Mac, Apple's own decoders and encoders, as in FrameFairy.
func ffmpegConfigure(prefix, system string) []string {
	args := []string{
		"--prefix=" + prefix,
		"--pkg-config-flags=--static",
		"--enable-static", "--disable-shared",
		"--disable-debug",
		"--disable-doc",
		"--disable-network",
		"--disable-protocols",
		"--enable-protocol=file,pipe",
		"--disable-demuxer=hls,dash,concat,imf,webm_dash_manifest",
		"--disable-autodetect",
		"--enable-zlib",
		"--enable-iconv",
	}
	if system == "darwin" {
		// iconv is the Mac's libiconv, not part of its C library, and
		// configure finds it without saying to link it. FrameFairy's build
		// gets it through libass, which names it, and this one has no
		// libass: without it the link fails on _iconv_open.
		args = append(args, "--enable-videotoolbox", "--enable-audiotoolbox", "--extra-libs=-liconv")
	}
	return args
}

// macosMin is the oldest macOS the app runs on, the floor of its
// Info.plist, which ffmpeg is built for too.
const macosMin = "13.0"

// listFormats are the formats that read other files a file names.
var listFormats = []string{"hls", "dash", "concat", "imf", "webm_dash_manifest"}

func ffmpegCommand(args []string) error {
	fs := flag.NewFlagSet("ffmpeg", flag.ContinueOnError)
	out := fs.String("out", ".build/ffmpeg", "the folder ffmpeg is built into, bin/ffmpeg and bin/ffprobe")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return buildFFmpeg(*out, runtime.GOOS)
}

// buildFFmpeg fetches ffmpeg at its pin, builds it into out and checks
// what came out. What the build says goes to out/build.log, since it is
// thousands of lines of warnings about code that is not ours, and the end
// of it is shown when a step fails.
func buildFFmpeg(out, system string) error {
	out, err := filepath.Abs(out)
	if err != nil {
		return err
	}
	work := filepath.Join(out, "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		return err
	}
	logPath := filepath.Join(out, "build.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		return err
	}
	defer logFile.Close()
	fail := func(step string, err error) error {
		_ = logFile.Sync()
		return fmt.Errorf("%s failed: %w. The last of %s:\n%s", step, err, logPath, tail(logPath, 40))
	}
	run := func(dir string, env []string, name string, args ...string) error {
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), env...)
		cmd.Stdout, cmd.Stderr = logFile, logFile
		return cmd.Run()
	}

	src := filepath.Join(work, "ffmpeg-"+ffmpegVersion)
	// A clone from before is never built from again, since what is in it
	// now is not what was checked then.
	if err := os.RemoveAll(src); err != nil {
		return err
	}
	fmt.Printf("== fetching ffmpeg %s from git\n", ffmpegVersion)
	if err := run(work, nil, "git", "-c", "advice.detachedHead=false", "clone", "--depth", "1",
		"--branch", "n"+ffmpegVersion, "-q", ffmpegGit, src); err != nil {
		return fail("fetching ffmpeg", err)
	}
	head, err := exec.Command("git", "-C", src, "rev-parse", "HEAD").Output()
	if err != nil {
		return fail("reading the commit fetched", err)
	}
	if got := strings.TrimSpace(string(head)); got != ffmpegCommit {
		_ = os.RemoveAll(src)
		return fmt.Errorf("n%s of %s is not the commit pinned here: expected %s, got %s", ffmpegVersion, ffmpegGit, ffmpegCommit, got)
	}

	// The same floor as the app, so the two agree about which macOS they
	// run on.
	env := []string{"CFLAGS=-O2 -fPIC"}
	if system == "darwin" {
		env = []string{"MACOSX_DEPLOYMENT_TARGET=" + macosMin,
			"CFLAGS=-O2 -fPIC -mmacosx-version-min=" + macosMin,
			"LDFLAGS=-mmacosx-version-min=" + macosMin}
	}
	fmt.Printf("== building ffmpeg %s, which takes a few minutes\n", ffmpegVersion)
	if err := run(src, env, "./configure", ffmpegConfigure(out, system)...); err != nil {
		return fail("configure", err)
	}
	if err := run(src, env, "make", "-j"+strconv.Itoa(runtime.NumCPU())); err != nil {
		return fail("make", err)
	}
	if err := run(src, env, "make", "install"); err != nil {
		return fail("make install", err)
	}

	ffmpeg, ffprobe := filepath.Join(out, "bin", "ffmpeg"), filepath.Join(out, "bin", "ffprobe")
	if err := checkFFmpeg(ffmpeg, system); err != nil {
		return err
	}
	if err := checkFFmpeg(ffprobe, system); err != nil {
		return err
	}
	if err := writeFFmpegNotes(out, src, ffmpeg); err != nil {
		return err
	}
	fmt.Printf("ffmpeg and ffprobe %s, LGPL, no libx264, reading only files and pipes: %s\n", ffmpegVersion, filepath.Dir(ffmpeg))
	return nil
}

// checkFFmpeg reads what a program of the build says of itself, and
// refuses one that cannot ship: GPL, with libx264, with a way in or out
// beyond a file and a pipe, with a format that reads other files, or
// needing a library of the machine that built it.
func checkFFmpeg(program, system string) error {
	ask := func(args ...string) (string, error) {
		out, err := exec.Command(program, append([]string{"-hide_banner"}, args...)...).Output()
		if err != nil {
			return "", fmt.Errorf("%s %s: %w", filepath.Base(program), strings.Join(args, " "), err)
		}
		return string(out), nil
	}
	licence, err := ask("-L")
	if err != nil {
		return err
	}
	if !lgpl(licence) {
		return fmt.Errorf("%s came out GPL, which cannot ship. Check the configure line", program)
	}
	encoders, err := ask("-encoders")
	if err != nil {
		return err
	}
	if slices.Contains(listed(encoders), "libx264") {
		return fmt.Errorf("libx264 got into %s, which makes it GPL", program)
	}
	protocols, err := ask("-protocols")
	if err != nil {
		return err
	}
	if got := ioProtocols(protocols); !slices.Equal(got, []string{"file", "pipe"}) {
		return fmt.Errorf("%s reads and writes through %s, not only file and pipe", program, strings.Join(got, ", "))
	}
	demuxers, err := ask("-demuxers")
	if err != nil {
		return err
	}
	for _, name := range listed(demuxers) {
		if slices.Contains(listFormats, name) {
			return fmt.Errorf("the %s demuxer got into %s, which reads other files a recording names", name, program)
		}
	}
	links, err := linkedBy(program, system)
	if err != nil {
		return err
	}
	if borrowed := borrowedLibraries(links, system); len(borrowed) > 0 {
		return fmt.Errorf("%s needs libraries of this machine, so it runs only here: %s", program, strings.Join(borrowed, ", "))
	}
	// What it links, in the log of the build, which the list of the
	// release check in inputs.go is read against.
	fmt.Printf("%s links:\n%s", filepath.Base(program), links)
	return nil
}

// lgpl says whether ffmpeg -L names the Lesser GPL and not the GPL.
func lgpl(licence string) bool {
	head := strings.Join(firstLines(licence, 5), "\n")
	return strings.Contains(head, "GNU Lesser General Public") && !strings.Contains(head, "GNU General Public")
}

// listed reads the names of a list ffmpeg prints, -encoders or -demuxers:
// below the line of dashes, the second word of each line, which may be
// several names with commas between them.
func listed(text string) []string {
	var names []string
	below := false
	for line := range strings.Lines(text) {
		fields := strings.Fields(line)
		if !below {
			below = len(fields) == 1 && strings.Trim(fields[0], "-") == ""
			continue
		}
		if len(fields) >= 2 {
			names = append(names, strings.Split(fields[1], ",")...)
		}
	}
	return names
}

// ioProtocols reads the protocols ffmpeg -protocols lists, for input and
// output together, once each, in order.
func ioProtocols(text string) []string {
	var names []string
	for line := range strings.Lines(text) {
		fields := strings.Fields(line)
		if len(fields) != 1 || fields[0] == "Input:" || fields[0] == "Output:" || strings.HasSuffix(fields[0], ":") {
			continue
		}
		names = append(names, fields[0])
	}
	slices.Sort(names)
	return slices.Compact(names)
}

// linkedBy names the libraries a program needs: otool -L on a Mac, ldd
// elsewhere.
func linkedBy(program, system string) (string, error) {
	tool, args := "ldd", []string{program}
	if system == "darwin" {
		tool, args = "otool", []string{"-L", program}
	}
	out, err := exec.Command(tool, args...).Output()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w", tool, program, err)
	}
	return string(out), nil
}

// linuxOwn are the libraries every Linux has that a static ffmpeg may
// still name: the C runtime, its loader, and zlib, asked for on purpose.
var linuxOwn = regexp.MustCompile(`^(linux-vdso|ld-linux[^.]*|libc|libm|libdl|libpthread|librt|libz|libgcc_s|libstdc\+\+)\.so`)

// borrowedLibraries are the libraries a program names that are not the
// system's own, from what otool -L or ldd said. A static build may still
// name one of the machine that built it, as FrameFairy's once named
// Homebrew's libunibreak, and would then fail on every other Mac.
func borrowedLibraries(links, system string) []string {
	var borrowed []string
	lines := strings.Split(strings.TrimSpace(links), "\n")
	if system == "darwin" && len(lines) > 0 {
		lines = lines[1:] // otool names the program first
	}
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		lib := fields[0]
		if system == "darwin" {
			if !strings.HasPrefix(lib, "/usr/lib/") && !strings.HasPrefix(lib, "/System/Library/") {
				borrowed = append(borrowed, lib)
			}
			continue
		}
		if !linuxOwn.MatchString(filepath.Base(lib)) {
			borrowed = append(borrowed, lib)
		}
	}
	return borrowed
}

// writeFFmpegNotes writes beside the programs what travels with them:
// ffmpeg's licence, LICENSE-ffmpeg.txt, which the LGPL asks for with
// every copy, and BUILD-ffmpeg.txt, what the build is, read back off the
// binary rather than echoed from here, so it cannot drift from what was
// done.
func writeFFmpegNotes(out, src, ffmpeg string) error {
	lgplText, err := os.ReadFile(filepath.Join(src, "COPYING.LGPLv2.1"))
	if err != nil {
		return err
	}
	licence := fmt.Sprintf("ffmpeg %s, GNU Lesser General Public License 2.1 or later\n\n%s", ffmpegVersion, lgplText)
	if err := os.WriteFile(filepath.Join(out, "bin", "LICENSE-ffmpeg.txt"), []byte(licence), 0o644); err != nil {
		return err
	}
	said := func(args ...string) string {
		b, _ := exec.Command(ffmpeg, append([]string{"-hide_banner"}, args...)...).Output()
		return string(b)
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, "ffmpeg, built for Speaker Trail\n\n")
	fmt.Fprintf(&b, "built    %s on %s %s\n", time.Now().UTC().Format("2006-01-02 15:04 UTC"), runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(&b, "recipe   cmd/speakertrail-release/ffmpeg.go\n")
	fmt.Fprintf(&b, "sources  ffmpeg %s, commit %s\n\n", ffmpegVersion, ffmpegCommit)
	fmt.Fprintf(&b, "version\n  %s\n\n", strings.Join(firstLines(said("-version"), 1), ""))
	fmt.Fprintf(&b, "licence, as the binary itself reports it\n")
	for _, line := range firstLines(said("-L"), 4) {
		fmt.Fprintf(&b, "  %s\n", line)
	}
	fmt.Fprintf(&b, "\nconfigure, as the binary itself reports it\n")
	for line := range strings.Lines(said("-buildconf")) {
		if strings.TrimSpace(line) != "" {
			b.WriteString(line)
		}
	}
	return os.WriteFile(filepath.Join(out, "bin", "BUILD-ffmpeg.txt"), b.Bytes(), 0o644)
}

// firstLines are the first n lines of text that hold anything.
func firstLines(text string, n int) []string {
	var lines []string
	for line := range strings.Lines(text) {
		if line = strings.TrimSpace(line); line != "" && len(lines) < n {
			lines = append(lines, line)
		}
	}
	return lines
}

// tail is the last n lines of the file at path.
func tail(path string, n int) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	var lines []string
	s := bufio.NewScanner(io.LimitReader(f, 64<<20))
	s.Buffer(make([]byte, 64<<10), 1<<20)
	for s.Scan() {
		lines = append(lines, s.Text())
		if len(lines) > n {
			lines = lines[1:]
		}
	}
	return strings.Join(lines, "\n")
}
