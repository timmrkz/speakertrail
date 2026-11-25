package main

// which says what a run of builds.yml is a build of: main, the open pull
// request of the branch pushed or opened, or nothing. It was shell in the
// workflow, and is here, tested, since the rule of who builds a pull
// request opened after its push came in. builds.yml only calls it. See
// What happens on a push in docs/UPDATES.md.
//
// A pull request is built by its push or by its opening, never by both.
// A session pushes its branch and opens the pull request seconds later, so
// that push finds no pull request, and a pull request of that one push had
// no build at all, #93. Building on the opening as well built one commit
// twice when the push and the opening came in the same second, as
// Dependabot does them, and the second build stopped the first, FrameFairy
// #642. So a push builds only for a pull request that was open before it,
// by the time GitHub says it was pushed, and the opening builds the rest.

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// branchPull is an open pull request of a branch, as gh pr list writes it.
type branchPull struct {
	Number  int       `json:"number"`
	Fork    bool      `json:"isCrossRepository"`
	Created time.Time `json:"createdAt"`
}

// run is a run of builds.yml as GitHub tells it.
type run struct {
	event  string // push or pull_request
	action string // for a pull request: opened, reopened or closed
	branch string // the branch pushed
	sha    string // the commit pushed, or the pull request's head
	number string // the pull request's number, for a pull request
	pushed string // for a push, when it happened, in seconds since 1970
}

// buildOf is what a run is a build of. channel, number and sha are empty
// when it is a build of nothing.
type buildOf struct {
	build   bool
	channel string
	number  string
	sha     string
	notice  string
}

// whichBuild says what r is a build of. pullsOf lists the open pull
// requests of a branch.
func whichBuild(r run, pullsOf func(branch string) ([]branchPull, error)) (buildOf, error) {
	switch {
	case r.event == "pull_request":
		if r.number == "" {
			return buildOf{}, errors.New("a pull request's run names no pull request")
		}
		b := buildOf{build: r.action != "closed", channel: "pr-" + r.number, number: r.number, sha: r.sha}
		if !b.build {
			b.notice = "A pull request that closes builds nothing. Its build only leaves the list."
		}
		return b, nil
	case r.event != "push":
		return buildOf{notice: "Only a push or the opening of a pull request builds, not " + r.event + "."}, nil
	case r.branch == "main":
		return buildOf{build: true, channel: "main", sha: r.sha}, nil
	}
	pulls, err := pullsOf(r.branch)
	if err != nil {
		return buildOf{}, err
	}
	// A branch's pull request is only one opened from this repository: a
	// fork can have a branch of the same name.
	var own *branchPull
	for i := range pulls {
		if !pulls[i].Fork {
			own = &pulls[i]
			break
		}
	}
	if own == nil {
		return buildOf{notice: r.branch + " has no open pull request, so it has no channel to build for."}, nil
	}
	number := strconv.Itoa(own.Number)
	// A push time that is no number builds, since a doubt builds.
	if pushed, err := strconv.ParseInt(r.pushed, 10, 64); err == nil && pushed > 0 && own.Created.Unix() >= pushed {
		return buildOf{notice: fmt.Sprintf("Pull request %s was opened at or after this push, so its opening builds it.", number)}, nil
	}
	return buildOf{build: true, channel: "pr-" + number, number: number, sha: r.sha}, nil
}

// write says b as the outputs of the step, one name and one value a line,
// to the file GitHub reads them from, and its notice where the run shows
// it.
func (b buildOf) write(out io.Writer, outputs string) error {
	if b.notice != "" {
		fmt.Fprintf(out, "::notice::%s\n", b.notice)
	}
	if outputs == "" {
		return errors.New("GITHUB_OUTPUT is not set, so there is nowhere to say what this is a build of")
	}
	f, err := os.OpenFile(outputs, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	lines := [][2]string{
		{"channel", b.channel},
		{"number", b.number},
		{"sha", b.sha},
		{"build", strconv.FormatBool(b.build)},
	}
	for _, kv := range lines {
		if _, err := fmt.Fprintf(f, "%s=%s\n", kv[0], kv[1]); err != nil {
			f.Close()
			return err
		}
	}
	if b.build {
		fmt.Fprintf(out, "%s is a build of %s\n", b.sha, b.channel)
	}
	return f.Close()
}

// whichCommand is speakertrail-release which.
func whichCommand(args []string) error {
	fs := flag.NewFlagSet("which", flag.ContinueOnError)
	var r run
	repo := fs.String("repo", os.Getenv("GITHUB_REPOSITORY"), "the repository, owner/name")
	fs.StringVar(&r.event, "event", "", "the event that started the run, push or pull_request")
	fs.StringVar(&r.action, "action", "", "for a pull request, what happened to it")
	fs.StringVar(&r.branch, "branch", "", "the branch pushed")
	fs.StringVar(&r.sha, "sha", "", "the commit pushed, or the pull request's head")
	fs.StringVar(&r.number, "number", "", "for a pull request, its number")
	fs.StringVar(&r.pushed, "pushed", "", "for a push, when it happened, in seconds since 1970")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *repo == "" {
		return errors.New("which needs -repo or GITHUB_REPOSITORY")
	}
	if r.event == "" || r.sha == "" {
		return errors.New("which needs -event and -sha")
	}
	b, err := whichBuild(r, func(branch string) ([]branchPull, error) { return branchPulls(*repo, branch) })
	if err != nil {
		return err
	}
	return b.write(os.Stdout, os.Getenv("GITHUB_OUTPUT"))
}

// branchPulls asks gh for the open pull requests of a branch.
func branchPulls(repo, branch string) ([]branchPull, error) {
	cmd := exec.Command("gh", "pr", "list", "--repo", repo, "--head", branch, "--state", "open",
		"--json", "number,isCrossRepository,createdAt")
	var errOut strings.Builder
	cmd.Stderr = &errOut
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gh pr list: %w: %s", err, strings.TrimSpace(errOut.String()))
	}
	var pulls []branchPull
	if err := json.Unmarshal(out, &pulls); err != nil {
		return nil, fmt.Errorf("gh pr list: %w", err)
	}
	return pulls, nil
}
