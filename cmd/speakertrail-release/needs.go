package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os/exec"
	"slices"
	"strconv"
	"strings"

	"github.com/timmrkz/speakertrail/internal/updates"
)

// The answers needsBuild gives about a commit of a pull request, against
// the commit its channel's build was made from.
const (
	// needed: the commit changes the app, or what it changes cannot be
	// read. Every doubt answers this: a build too many costs a few minutes
	// of a runner, a build too few is a commit Tim tests without having it.
	needed = "build"
	// alreadyBuilt: it is the commit built, or has the same files.
	alreadyBuilt = "built"
	// docsOnly: it only changes the docs against what is built.
	docsOnly = "docs"
)

// trees reads which files differ between two commits. git does it in a
// checkout, and the tests hand in a repository they made.
type trees interface {
	changed(from, to string) ([]string, error)
}

// needsBuild says whether a pull request's commit needs a build of its
// own, and why. The build asks it before building, and the channel list
// asks it to know whether a pull request has a commit still to come, so
// the two never disagree about a commit: the list once said a build was
// coming that the build had decided against, and the Updates page waited
// for it for good. It was scripts/needs-build.sh, and is FrameFairy's
// needs.go, without its answer outside, for Go the app is not built from:
// Speaker Trail has no part that works that out.
//
// It compares against what is built, not against the push before. A push
// that stops the build of the one before and only changes the docs would
// otherwise leave the one before with no build at all.
func needsBuild(t trees, builtFrom, newest string) (string, string) {
	if builtFrom == "" {
		return needed, "The channel has no build yet."
	}
	files, err := t.changed(builtFrom, newest)
	if err != nil {
		return needed, fmt.Sprintf("What changed since %s cannot be read: %v", builtFrom, err)
	}
	if len(files) == 0 {
		return alreadyBuilt, fmt.Sprintf("%s has the same files as the build of %s.", newest, builtFrom)
	}
	if slices.ContainsFunc(files, func(f string) bool { return !docs(f) }) {
		return needed, fmt.Sprintf("%s changes the app against the build of %s.", newest, builtFrom)
	}
	return docsOnly, fmt.Sprintf("%s only changes the docs against the build of %s.", newest, builtFrom)
}

// docs says whether a file is only text: the docs folder, or Markdown.
func docs(name string) bool {
	return strings.HasPrefix(name, "docs/") || strings.HasSuffix(name, ".md")
}

// gitTrees asks git in the checkout the tool runs in, and fetches a
// commit it does not have, one commit deep, since comparing two commits'
// files needs nothing of their history. A commit the channel list keeps
// is shortened, which git cannot fetch, so its whole name is asked of
// GitHub first.
type gitTrees struct {
	gh  releaseHub
	dir string
}

func (g gitTrees) git(args ...string) *exec.Cmd {
	cmd := exec.Command("git", args...)
	cmd.Dir = g.dir
	return cmd
}

func (g gitTrees) changed(from, to string) ([]string, error) {
	a, err := g.full(from)
	if err != nil {
		return nil, err
	}
	b, err := g.full(to)
	if err != nil {
		return nil, err
	}
	// Both names of a file moved, since where it came from may be the
	// app, and git otherwise lists only where it went.
	out, err := g.git("diff", "--name-only", "--no-renames", a, b).Output()
	if err != nil {
		return nil, fmt.Errorf("git diff %s %s: %w", a, b, err)
	}
	return strings.Fields(string(out)), nil
}

func (g gitTrees) full(commit string) (string, error) {
	if out, err := g.git("rev-parse", "-q", "--verify", commit+"^{commit}").Output(); err == nil {
		return strings.TrimSpace(string(out)), nil
	}
	sha := commit
	if len(sha) < 40 {
		if g.gh == nil {
			return "", fmt.Errorf("%s cannot be found", commit)
		}
		var err error
		if sha, err = g.gh.commit(commit); err != nil {
			return "", fmt.Errorf("%s cannot be found: %w", commit, err)
		}
	}
	if err := g.git("fetch", "-q", "--depth=1", "origin", sha).Run(); err != nil {
		return "", fmt.Errorf("%s cannot be fetched: %w", sha, err)
	}
	return sha, nil
}

// needsBuildCommand is speakertrail-release needs-build, which the which
// job of builds.yml asks, on Linux, before a Mac is started for a pull
// request's commit. It reads the commit the channel's build was made from
// in its entry on the release, and says build=false in the step's
// outputs when the commit needs no build of its own, and why as a notice.
// main is always built.
func needsBuildCommand(args []string) error {
	fs := flag.NewFlagSet("needs-build", flag.ExitOnError)
	channel := fs.String("channel", "", "main, or pr- and the pull request's number")
	commit := fs.String("commit", "", "the commit to decide about")
	_ = fs.Parse(args)
	if *commit == "" || !updates.ValidChannel(*channel) {
		return fmt.Errorf("needs-build needs -channel and -commit")
	}
	if *channel == "main" {
		return output("build", "true")
	}
	gh := gitHubFromEnv()
	answer, why := needsBuild(gitTrees{gh: gh}, builtCommit(gh, *channel), *commit)
	fmt.Printf("::notice::%s\n", why)
	return output("build", strconv.FormatBool(answer == needed))
}

// builtCommit is the commit the channel's build was made from, as its
// entry says, or empty when there is none or it cannot be read, which
// builds.
func builtCommit(gh releaseHub, channel string) string {
	all, err := gh.assets()
	if err != nil {
		return ""
	}
	for _, a := range all {
		if a.name != "channel-"+channel+".json" {
			continue
		}
		data, err := gh.download(a)
		var b updates.Build
		if err != nil || json.Unmarshal(data, &b) != nil {
			return ""
		}
		return b.Commit
	}
	return ""
}
