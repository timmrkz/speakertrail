package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// toolPrefix starts the name of each build of the tool in the release
// release-tool, one for each system it runs on, as
// speakertrail-release-<system>-<processor>, the way GitHub's runners
// name them in runner.os and runner.arch, see
// .github/actions/release-tool.
const toolPrefix = "speakertrail-release-"

// toolStore is what keeping the tool asks of GitHub.
type toolStore interface {
	// assets are the files of the release.
	assets() ([]hubAsset, error)
	// upload puts a file into the release, in place of one of the same
	// name, and makes the release first if there is none.
	upload(name string, data []byte) error
	// remove takes a file away from the release.
	remove(a hubAsset) error
}

// keepTool puts main's new builds of the tool into the release
// release-tool and takes every other file away. What makes a job trust a
// build is main's signature and its key, see self.go, never which file
// the release holds. FrameFairy's keeps one build, for Linux. Speaker
// Trail's publish signs on a Mac, so it keeps one for each.
func keepTool(gh toolStore, files map[string][]byte, out io.Writer) error {
	for _, name := range slices.Sorted(maps.Keys(files)) {
		if !strings.HasPrefix(name, toolPrefix) {
			return fmt.Errorf("%s is no build of the tool", name)
		}
		if err := gh.upload(name, files[name]); err != nil {
			return err
		}
		fmt.Fprintf(out, "Kept %s in the release release-tool.\n", name)
	}
	all, err := gh.assets()
	if err != nil {
		return err
	}
	var errs []error
	for _, a := range all {
		if _, kept := files[a.name]; !kept {
			errs = append(errs, gh.remove(a))
		}
	}
	return errors.Join(errs...)
}

// keepCommand keeps the builds, for release-tool.yml on main, each under
// the name of its file.
func keepCommand(args []string, gh toolStore, out io.Writer) error {
	fs := flag.NewFlagSet("keep", flag.ExitOnError)
	_ = fs.Parse(args)
	if fs.NArg() == 0 {
		return errors.New("keep needs the builds")
	}
	files := map[string][]byte{}
	for _, path := range fs.Args() {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[filepath.Base(path)] = data
	}
	return keepTool(gh, files, out)
}
