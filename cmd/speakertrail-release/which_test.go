package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Every way a run of builds.yml can come in, and what it builds. Above
// all, a commit pushed before its pull request was opened is built by the
// opening and not by the push, and one pushed to a pull request open
// before it by the push, so no commit is built twice and none is left out,
// #93 and FrameFairy #642. The pull request of #93 was opened at 12:49:11,
// seven seconds after its push.
func TestEachCommitIsBuiltByItsPushOrItsOpening(t *testing.T) {
	t.Parallel()
	opened := time.Date(2026, 11, 24, 12, 49, 11, 0, time.UTC)
	at := func(d time.Duration) string { return strconv.FormatInt(opened.Add(d).Unix(), 10) }
	ours := []branchPull{{Number: 93, Created: opened}}
	cases := []struct {
		name  string
		run   run
		pulls []branchPull
		want  buildOf
	}{
		{"a push to main builds main",
			run{event: "push", branch: "main", sha: "a1", pushed: at(0)}, nil,
			buildOf{build: true, channel: "main", sha: "a1"}},
		{"a push before its pull request was opened is left to the opening",
			run{event: "push", branch: "work", sha: "b2", pushed: at(-7 * time.Second)}, ours,
			buildOf{}},
		{"a push in the same second as the opening is left to the opening",
			run{event: "push", branch: "work", sha: "b2", pushed: at(0)}, ours,
			buildOf{}},
		{"a push to a pull request open before it builds",
			run{event: "push", branch: "work", sha: "c3", pushed: at(time.Second)}, ours,
			buildOf{build: true, channel: "pr-93", number: "93", sha: "c3"}},
		{"a push time that is no number builds",
			run{event: "push", branch: "work", sha: "c3", pushed: "soon"}, ours,
			buildOf{build: true, channel: "pr-93", number: "93", sha: "c3"}},
		{"a push with no push time builds",
			run{event: "push", branch: "work", sha: "c3"}, ours,
			buildOf{build: true, channel: "pr-93", number: "93", sha: "c3"}},
		{"a push to a branch with no pull request builds nothing",
			run{event: "push", branch: "work", sha: "d4", pushed: at(0)}, nil,
			buildOf{}},
		{"a fork's pull request of the same branch name is not ours",
			run{event: "push", branch: "work", sha: "d4", pushed: at(time.Hour)}, []branchPull{{Number: 7, Fork: true}},
			buildOf{}},
		{"ours is found beside a fork's",
			run{event: "push", branch: "work", sha: "d4", pushed: at(time.Hour)}, []branchPull{{Number: 7, Fork: true}, {Number: 93, Created: opened}},
			buildOf{build: true, channel: "pr-93", number: "93", sha: "d4"}},
		{"an opening builds its head",
			run{event: "pull_request", action: "opened", sha: "e5", number: "93"}, nil,
			buildOf{build: true, channel: "pr-93", number: "93", sha: "e5"}},
		{"a reopening builds its head",
			run{event: "pull_request", action: "reopened", sha: "e5", number: "93"}, nil,
			buildOf{build: true, channel: "pr-93", number: "93", sha: "e5"}},
		{"a close builds nothing",
			run{event: "pull_request", action: "closed", sha: "e5", number: "93"}, nil,
			buildOf{channel: "pr-93", number: "93", sha: "e5"}},
		{"any other event builds nothing",
			run{event: "workflow_dispatch", branch: "main", sha: "f6"}, nil,
			buildOf{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			asked := false
			got, err := whichBuild(c.run, func(branch string) ([]branchPull, error) {
				asked = true
				if branch != c.run.branch {
					t.Errorf("asked for the pull requests of %q, not of %q", branch, c.run.branch)
				}
				return c.pulls, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			got.notice = ""
			if got != c.want {
				t.Errorf("got %+v, want %+v", got, c.want)
			}
			if asked != (c.run.event == "push" && c.run.branch != "main") {
				t.Errorf("asked GitHub for the branch's pull requests: %v", asked)
			}
		})
	}
}

// A run that cannot tell which pull requests a branch has fails, rather
// than building nothing and leaving the pull request without a build.
func TestWhichFailsWhenGitHubCannotSay(t *testing.T) {
	t.Parallel()
	_, err := whichBuild(run{event: "push", branch: "work", sha: "a1", pushed: "1"}, func(string) ([]branchPull, error) {
		return nil, errors.New("gh: no answer")
	})
	if err == nil {
		t.Fatal("a branch whose pull requests could not be listed was decided anyway")
	}
	if _, err := whichBuild(run{event: "pull_request", action: "opened", sha: "a1"}, nil); err == nil {
		t.Fatal("a pull request's run with no number was decided anyway")
	}
}

// What builds.yml reads: every output on a line of its own, build false
// when nothing is built, and the notice where the run shows it.
func TestWhichWritesWhatTheWorkflowReads(t *testing.T) {
	t.Parallel()
	outputs := filepath.Join(t.TempDir(), "outputs")
	var log bytes.Buffer
	b := buildOf{build: true, channel: "pr-93", number: "93", sha: "b2bbfe1"}
	if err := b.write(&log, outputs); err != nil {
		t.Fatal(err)
	}
	if err := (buildOf{notice: "nothing to build"}).write(&log, outputs); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(outputs)
	if err != nil {
		t.Fatal(err)
	}
	want := "channel=pr-93\nnumber=93\nsha=b2bbfe1\nbuild=true\nchannel=\nnumber=\nsha=\nbuild=false\n"
	if string(got) != want {
		t.Errorf("the outputs are\n%s\nwant\n%s", got, want)
	}
	if !strings.Contains(log.String(), "b2bbfe1 is a build of pr-93") || !strings.Contains(log.String(), "::notice::nothing to build") {
		t.Errorf("the log says %q", log.String())
	}
	if err := b.write(&log, ""); err == nil {
		t.Error("a run with no GITHUB_OUTPUT was told nothing went wrong")
	}
}
