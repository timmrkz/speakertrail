package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
)

// The release tool built once, on main, signed there with a build
// attestation, and downloaded by every job that holds a key or a token
// that writes the release, instead of each building it again with no
// cache. Ported from FrameFairy, see
// docs/decisions/2026-11-24-the-release-tool-built-once.md and
// docs/decisions/2026-11-25-the-release-tool-keys-itself.md.
//
// A build made by self carries the key of what it was built from, a
// fingerprint of its source, and the folders it was built from. fits asks
// whether a checkout is still that, and the job builds the tool itself
// when it is not. The key opens nothing: trust comes from main's
// signature alone, which the action checks first.
//
// FrameFairy works the folders out of the commit's imports with git
// archive, since the action asks before Go is set up. Here self asks Go
// with go list, for both systems the tool runs on, and stamps the folders
// in beside the key. That is enough: a package the tool comes to import
// is named by an import line in a folder it already reads, so the key of
// those folders changes with it.

// stamped is the key of what this build of the tool was built from, and
// stampedFrom the inputs, comma separated, both set by self with -ldflags
// -X. Any other build leaves them empty and never fits.
var stamped, stampedFrom string

// toolSystems are the systems the tool runs on: Linux for the list jobs,
// macOS for publish, which signs with codesign.
var toolSystems = []string{"linux", "darwin"}

// toolInputs are what the program in pkg is built from, as go list says
// in the module at root for each of toolSystems: go.mod, go.sum and the
// folders of every package of this module it imports, sorted.
func toolInputs(root, pkg string) ([]string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	dirs := map[string]bool{}
	for _, goos := range toolSystems {
		cmd := exec.Command("go", "list", "-deps", "-f", "{{if not .Standard}}{{.Dir}}{{end}}", pkg)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GOOS="+goos, "CGO_ENABLED=0")
		out, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("go list %s for %s: %w", pkg, goos, err)
		}
		for dir := range strings.FieldsSeq(string(out)) {
			rel, err := filepath.Rel(root, dir)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				continue
			}
			dirs[filepath.ToSlash(rel)] = true
		}
	}
	if len(dirs) == 0 {
		return nil, fmt.Errorf("go list names no folder of the module for %s", pkg)
	}
	return append([]string{"go.mod", "go.sum"}, slices.Sorted(maps.Keys(dirs))...), nil
}

// gitKey is the key of the inputs at a commit of the repository at root:
// the first 16 hex digits of a hash over each one's git object, which
// changes with any file in it, the files a package embeds and its folders
// below included.
func gitKey(root, commit string, inputs []string) (string, error) {
	args := []string{"rev-parse"}
	for _, in := range inputs {
		if in == "." {
			in = ""
		}
		args = append(args, commit+":"+in)
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	objects, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse of what the tool is built from: %w", err)
	}
	found := strings.Fields(string(objects))
	if len(found) != len(inputs) {
		return "", fmt.Errorf("git named %d objects for %d inputs", len(found), len(inputs))
	}
	return keyOf(inputs, found), nil
}

// keyOf is the key of the inputs by their git objects: the first 16 hex
// digits of a hash over each input's name and object, in order.
func keyOf(inputs, objects []string) string {
	h := sha256.New()
	for i, o := range objects {
		fmt.Fprintf(h, "%s %s\n", inputs[i], o)
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// gitRoot is the top of the checkout the tool runs in.
func gitRoot() (string, error) {
	top, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse --show-toplevel: %w", err)
	}
	return strings.TrimSpace(string(top)), nil
}

// fits says whether a build stamped with one key is the build of the
// checkout, whose key is key, or why not. Every doubt builds the tool.
func fits(stamped, key string, keyErr error) (bool, string) {
	switch {
	case stamped == "":
		return false, "The release tool says nothing of what it was built from, so it is built here."
	case keyErr != nil:
		return false, "What the release tool is built from cannot be read here, so it is built here: " + keyErr.Error()
	case stamped != key:
		return false, "The signed release tool was built from other code than this commit's, so it is built here."
	}
	return true, "The signed release tool is built from this commit's code."
}

// fitsCommand answers fits=true or fits=false for the action, and puts
// this program's folder on the job's path when it fits.
func fitsCommand(out io.Writer) error {
	key, keyErr := "", error(nil)
	if stampedFrom != "" {
		var root string
		if root, keyErr = gitRoot(); keyErr == nil {
			key, keyErr = gitKey(root, "HEAD", strings.Split(stampedFrom, ","))
		}
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return answerFits(out, stamped, key, keyErr, filepath.Dir(exe), os.Getenv("GITHUB_OUTPUT"), os.Getenv("GITHUB_PATH"))
}

// answerFits says in the job's outputs whether a build stamped with stamp
// is the build of the checkout's key, and puts the build's folder on the
// job's path only when it is. A build that does not fit never reaches a
// later step.
func answerFits(out io.Writer, stamp, key string, keyErr error, dir, outputs, path string) error {
	ok, why := fits(stamp, key, keyErr)
	fmt.Fprintln(out, why)
	if ok {
		if err := onPath(path, dir); err != nil {
			return err
		}
	}
	if outputs == "" {
		return errors.New("GITHUB_OUTPUT is not set, so there is nowhere to say whether the tool fits")
	}
	f, err := os.OpenFile(outputs, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(f, "fits=%s\n", strconv.FormatBool(ok)); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// selfCommand builds this program from the checkout, stamped with the key
// of what it is built from, with no C, for the system given or this one,
// and puts it on the job's path when it is for this one. A checkout
// whose key cannot be worked out fails here, on the pull request that
// made it so, rather than leaving main a build that never fits.
func selfCommand(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("self", flag.ExitOnError)
	to := fs.String("out", "", "where to write the build")
	goos := fs.String("goos", runtime.GOOS, "the system it is for")
	goarch := fs.String("goarch", runtime.GOARCH, "the processor it is for")
	_ = fs.Parse(args)
	if *to == "" {
		return errors.New("self needs -out")
	}
	root, err := gitRoot()
	if err != nil {
		return err
	}
	const pkg = "./cmd/speakertrail-release"
	inputs, err := toolInputs(root, pkg)
	if err != nil {
		return fmt.Errorf("what the release tool is built from cannot be worked out, so no build of it could ever fit: %w", err)
	}
	key, err := gitKey(root, "HEAD", inputs)
	if err != nil {
		return err
	}
	abs, err := filepath.Abs(*to)
	if err != nil {
		return err
	}
	build := exec.Command("go", "build", "-trimpath",
		"-ldflags=-X main.stamped="+key+" -X main.stampedFrom="+strings.Join(inputs, ","), "-o", abs, pkg)
	build.Dir = root
	build.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+*goos, "GOARCH="+*goarch)
	build.Stdout, build.Stderr = out, os.Stderr
	if err := build.Run(); err != nil {
		return fmt.Errorf("go build %s: %w", pkg, err)
	}
	fmt.Fprintf(out, "Built the release tool for %s/%s, key %q, from %s.\n", *goos, *goarch, key, strings.Join(inputs, ", "))
	if *goos != runtime.GOOS || *goarch != runtime.GOARCH {
		return nil
	}
	return onPath(os.Getenv("GITHUB_PATH"), filepath.Dir(abs))
}

// onPath adds a folder to the path of the job's later steps, through the
// file GitHub names in GITHUB_PATH.
func onPath(file, dir string) error {
	if file == "" {
		return errors.New("GITHUB_PATH is not set, so there is no path to put the tool on")
	}
	f, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(f, dir); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
