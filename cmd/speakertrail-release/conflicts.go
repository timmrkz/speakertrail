package main

// conflicts tells a pull request that no longer merges into main so, in a
// comment that names the files in conflict, once for each commit of it
// and each commit of main it fails to merge into. GitHub sends no event
// when a pull request starts to conflict, and runs no CI on one, so
// without the comment nothing wakes the session that owns it. A comment
// can get lost on its way to the session, so the next push to main that
// still leaves the pull request in conflict tells it again.
//
// After a push to main every open pull request is tried, and after a push
// to another branch, the pull request of that branch, at the commit
// pushed. git merge-tree merges in memory, without a checkout of either
// side, and no code of the pull request runs. A pull request from a fork
// is left alone.
//
// Ported from FrameFairy's .github/workflows/conflicts.yml, where it is a
// loop in shell. Speaker Trail keeps no logic in shell, so the same steps
// are here, tested, and the workflow only calls them. See When main moves
// in docs/WORKFLOW.md.

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// pull is an open pull request as gh pr list writes it.
type pull struct {
	Number int    `json:"number"`
	Branch string `json:"headRefName"`
	Head   string `json:"headRefOid"`
	Fork   bool   `json:"isCrossRepository"`
}

// gitHub is what telling needs from git and GitHub, so a test can stand
// in for both.
type gitHub interface {
	// pulls lists the open pull requests into main.
	pulls() ([]pull, error)
	// main is the commit of main checked out.
	main() (string, error)
	// fetch brings a commit of the pull request into the checkout.
	fetch(commit string) error
	// merge merges commit into main in memory and names the files in
	// conflict, none when it merges.
	merge(commit string) ([]string, error)
	// comments are the bodies of the pull request's comments.
	comments(number int) (string, error)
	// comment writes a comment on the pull request.
	comment(number int, body string) error
}

// conflictsCommand is speakertrail-release conflicts.
func conflictsCommand(args []string) error {
	fs := flag.NewFlagSet("conflicts", flag.ContinueOnError)
	repo := fs.String("repo", os.Getenv("GITHUB_REPOSITORY"), "the repository, owner/name")
	branch := fs.String("branch", "", "the branch pushed, empty after a push to main")
	pushed := fs.String("pushed", "", "the commit pushed to it, which the pull request may not show as its head yet")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *repo == "" {
		return errors.New("conflicts needs -repo or GITHUB_REPOSITORY")
	}
	if (*branch == "") != (*pushed == "") {
		return errors.New("conflicts needs -branch and -pushed together, or neither after a push to main")
	}
	return tellConflicts(tools{repo: *repo}, *branch, *pushed, os.Stdout)
}

// tellConflicts tries each pull request that the push concerns and
// comments on each one that no longer merges and was not told about this
// pair of commits yet. A pull request it cannot try is a warning in the
// log, and the others are still tried.
func tellConflicts(g gitHub, branch, pushed string, log io.Writer) error {
	pulls, err := g.pulls()
	if err != nil {
		return err
	}
	base, err := g.main()
	if err != nil {
		return err
	}
	for _, p := range pulls {
		if p.Fork {
			continue
		}
		head := p.Head
		if branch != "" {
			if p.Branch != branch {
				continue
			}
			head = pushed
		}
		mark := conflictMark(head, base)
		if err := g.fetch(head); err != nil {
			fmt.Fprintf(log, "::warning::#%d: could not fetch it: %v\n", p.Number, err)
			continue
		}
		files, err := g.merge(head)
		if err != nil {
			fmt.Fprintf(log, "::warning::#%d: git merge-tree could not merge it: %v\n", p.Number, err)
			continue
		}
		if len(files) == 0 {
			fmt.Fprintf(log, "#%d at %s merges\n", p.Number, short(head))
			continue
		}
		said, err := g.comments(p.Number)
		if err != nil {
			fmt.Fprintf(log, "::warning::#%d: could not read its comments: %v\n", p.Number, err)
			continue
		}
		if strings.Contains(said, mark) {
			fmt.Fprintf(log, "#%d at %s conflicts, told already\n", p.Number, short(head))
			continue
		}
		fmt.Fprintf(log, "#%d at %s conflicts, telling it\n", p.Number, short(head))
		if err := g.comment(p.Number, conflictComment(head, base, files)); err != nil {
			fmt.Fprintf(log, "::warning::#%d: could not comment: %v\n", p.Number, err)
		}
	}
	return nil
}

// conflictMark is the hidden mark a comment carries. It holds both
// commits, so a new commit of either tells the pull request again, and a
// run stopped halfway never tells it twice about the same two.
func conflictMark(head, base string) string {
	return "<!-- conflicts " + head + " " + base + " -->"
}

// conflictComment says what is so and asks nothing: a session reads it as
// news and checks on GitHub itself.
func conflictComment(head, base string, files []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "This pull request at %s no longer merges into main at %s. These files conflict:\n\n", short(head), short(base))
	for _, f := range files {
		fmt.Fprintf(&b, "- `%s`\n", f)
	}
	b.WriteString("\n" + conflictMark(head, base))
	return b.String()
}

func short(commit string) string {
	if len(commit) > 7 {
		return commit[:7]
	}
	return commit
}

// objectID is a commit or a tree, in SHA-1 or SHA-256.
var objectID = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)

// mergeTreeFiles reads what git merge-tree --write-tree --name-only
// --no-messages wrote and how it exited. 0 is a merge, 1 and a tree is a
// conflict and the files after the tree, and anything else is git unable
// to try.
func mergeTreeFiles(out string, code int) ([]string, error) {
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	switch {
	case code == 0:
		return nil, nil
	case code == 1 && objectID.MatchString(lines[0]):
		var files []string
		for _, l := range lines[1:] {
			if l != "" {
				files = append(files, l)
			}
		}
		if len(files) == 0 {
			return nil, errors.New("git merge-tree says it conflicts and names no file")
		}
		return files, nil
	default:
		return nil, fmt.Errorf("exit %d: %s", code, strings.TrimSpace(out))
	}
}

// tools is gitHub with the real git, in the checkout of main, and gh.
type tools struct {
	repo string
	// dir is the checkout, the working directory when empty.
	dir string
}

// run runs a program in the checkout and gives what it wrote, and its
// exit code when it ran and failed.
func (t tools) run(name string, args ...string) (string, int, error) {
	var out, errOut bytes.Buffer
	cmd := exec.Command(name, args...)
	cmd.Dir = t.dir
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	err := cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return out.String(), exit.ExitCode(), fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(errOut.String()))
	}
	return out.String(), 0, err
}

func (t tools) pulls() ([]pull, error) {
	out, _, err := t.run("gh", "pr", "list", "--repo", t.repo, "--base", "main", "--state", "open", "--limit", "100",
		"--json", "number,headRefName,headRefOid,isCrossRepository")
	if err != nil {
		return nil, err
	}
	var pulls []pull
	if err := json.Unmarshal([]byte(out), &pulls); err != nil {
		return nil, fmt.Errorf("gh pr list: %w", err)
	}
	return pulls, nil
}

func (t tools) main() (string, error) {
	out, _, err := t.run("git", "rev-parse", "HEAD")
	return strings.TrimSpace(out), err
}

func (t tools) fetch(commit string) error {
	_, _, err := t.run("git", "fetch", "-q", "origin", commit)
	return err
}

func (t tools) merge(commit string) ([]string, error) {
	out, code, err := t.run("git", "merge-tree", "--write-tree", "--name-only", "--no-messages", "HEAD", commit)
	if err != nil && code == 0 {
		return nil, err
	}
	return mergeTreeFiles(out, code)
}

func (t tools) comments(number int) (string, error) {
	out, _, err := t.run("gh", "api", "--paginate", "repos/"+t.repo+"/issues/"+strconv.Itoa(number)+"/comments", "--jq", ".[].body")
	return out, err
}

func (t tools) comment(number int, body string) error {
	_, _, err := t.run("gh", "pr", "comment", strconv.Itoa(number), "--repo", t.repo, "--body", body)
	return err
}
