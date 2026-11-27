package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/timmrkz/speakertrail/internal/updates"
)

// What publish.yml asks of the tool besides check, harden, pack and sign:
// what a finished build is a build of, the upload, and the mark on a
// commit that was not published. Shell in the workflow before, and
// FrameFairy's publishPlan, publish and notPublished here, #57.

// publishPlan is what a finished build is, from GitHub's record of the
// run and nothing the build wrote: which channel, which version, which
// commit. A run that left no zip is a commit that needed no build, and
// there is nothing to publish. A branch whose pull request closed has no
// channel any more. The commit has to be one its channel's history
// holds, see held. Unlike FrameFairy's it does not compare the commit
// with the one published: sign does that with the time the run was
// made, see orderEntry, which a run made again keeps.
type publishOf struct {
	run                         bool
	channel, name, version, zip string
	commit                      string
	notice                      string
}

// appVersionLine is how appfiles.go says the app's version.
var appVersionLine = regexp.MustCompile(`(?m)^const appVersion = "([0-9]+\.[0-9]+\.[0-9]+)"`)

func publishPlan(gh releaseHub, run, event, branch, commit string) (publishOf, error) {
	no := func(why string) (publishOf, error) { return publishOf{notice: why}, nil }
	zips, err := gh.builds(run)
	if err != nil {
		return publishOf{}, err
	}
	if zips == 0 {
		return no("The run left no build, so there is nothing to publish.")
	}
	channel, name, tag, head := "main", "main", "main", ""
	if event != "push" || branch != "main" {
		pulls, err := gh.openPulls()
		if err != nil {
			return publishOf{}, err
		}
		i := slices.IndexFunc(pulls, func(p openPull) bool { return p.branch == branch })
		if i < 0 {
			return no(branch + " has no open pull request any more, so its build has no channel.")
		}
		p := pulls[i]
		channel, tag, head = "pr-"+strconv.Itoa(p.number), "pr"+strconv.Itoa(p.number), p.head
		title, _, _ := strings.Cut(p.title, "\n")
		name = fmt.Sprintf("#%d %s", p.number, strings.TrimSpace(strings.TrimSuffix(title, "\r")))
	}
	if err := held(gh, channel, commit, head); err != nil {
		return publishOf{}, err
	}
	// The app's version at the commit built, read and not run, the way
	// builds.yml reads it.
	file, err := gh.fileAt("cmd/speakertrail-app/appfiles.go", commit)
	if err != nil {
		return publishOf{}, fmt.Errorf("the app's version at %s cannot be read: %w", commit, err)
	}
	m := appVersionLine.FindSubmatch(file)
	if m == nil {
		return publishOf{}, fmt.Errorf("the app's version at %s is not a version", commit)
	}
	version := string(m[1]) + "-" + tag + "." + commit[:min(len(commit), 7)]
	return publishOf{
		run: true, channel: channel, name: name, version: version,
		commit: commit[:min(len(commit), 12)], zip: "app-" + channel + "-" + version + ".zip",
	}, nil
}

// held says whether a build of commit belongs to its channel: main's
// history holds it, or the history of the pull request's newest commit,
// head. GitHub reports a tag pushed with the name main, or with a pull
// request's branch, as a push to that branch, at any commit, so the
// record of the run alone is not enough.
func held(gh releaseHub, channel, commit, head string) error {
	if channel == "main" {
		var err error
		if head, err = gh.mainCommit(); err != nil {
			return fmt.Errorf("main's commit cannot be read, so %s is not known to be main's: %w", commit, err)
		}
	}
	if head == "" {
		return fmt.Errorf("say the newest commit of %s, so %s can be compared with it", channel, commit)
	}
	status, err := gh.compare(commit, head)
	if err != nil {
		return fmt.Errorf("%s cannot be compared with %s's newest commit, so it is not known to be %s's: %w", commit, channel, channel, err)
	}
	if status == "identical" || status == "ahead" {
		return nil
	}
	if channel == "main" {
		return fmt.Errorf("%s is not in main's history, so it is not signed as main's build. A tag named main may have been pushed: look at the tags of the repository", commit)
	}
	// A branch rewritten after the push is the usual reason. Its newer
	// push builds by itself.
	return fmt.Errorf("%s is not in the history of %s's newest commit, %s, so it is not published for it", commit, channel, head)
}

// write says p as the outputs of the step, and its notice where the run
// shows it.
func (p publishOf) write(out io.Writer, outputs string) error {
	if p.notice != "" {
		fmt.Fprintf(out, "::notice::%s\n", p.notice)
	}
	if outputs == "" {
		return errors.New("GITHUB_OUTPUT is not set, so there is nowhere to say what this is a build of")
	}
	f, err := os.OpenFile(outputs, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	for _, kv := range [][2]string{
		{"run", strconv.FormatBool(p.run)}, {"channel", p.channel}, {"name", p.name},
		{"version", p.version}, {"commit", p.commit}, {"zip", p.zip},
	} {
		if _, err := fmt.Fprintf(f, "%s=%s\n", kv[0], kv[1]); err != nil {
			f.Close()
			return err
		}
	}
	if p.run {
		fmt.Fprintf(out, "%s is %s, for %s\n", p.commit, p.version, p.channel)
	}
	return f.Close()
}

// publish uploads a signed build and its entry to the release dev, marks
// a pull request's commit ready to update to, and then uploads the
// patches.
//
// First every piece the release does not hold yet, then the zip, the
// stored zip packed with zstd and the parts, and the entry only once
// they are there, because the app reads the entry of the channel it
// follows and would be offered a build it cannot download yet. The stored
// zip itself is never uploaded: the app puts it together or unpacks it.
// A piece or a patch is named by the SHA-256 of what it makes, so one
// that is there already is the same and stays.
//
// The commit is marked the moment the entry is up: this workflow runs
// from main, so its checks show on main's commit and not on the pull
// request, which learns its build is ready from a status on the commit
// that was built.
//
// The patches go up last, after the entry and the status, so a build is
// on Updates without waiting for them, about a minute and a half on 27
// November 2026. Its parts name them already, and an app that finds one
// missing takes that file's piece instead. A patch that cannot go up, on
// a release that is full, costs the same piece, never the build its
// place on Updates, so it is a warning and no failure.
func publish(gh releaseHub, zipPath, zipName, packed, entryPath, channel, commit string, log io.Writer) error {
	zip, err := os.ReadFile(zipPath)
	if err != nil {
		return err
	}
	entry, err := os.ReadFile(entryPath)
	if err != nil {
		return err
	}
	parts, err := os.ReadFile(filepath.Join(packed, partsFile))
	if err != nil {
		return err
	}
	var p updates.Parts
	if err := json.Unmarshal(parts, &p); err != nil {
		return err
	}
	whole, err := os.ReadFile(filepath.Join(packed, packedWhole))
	if err != nil {
		return err
	}
	there, err := assetNames(gh)
	if err != nil {
		return err
	}
	for _, part := range p.Parts {
		if part.Bytes != nil {
			continue
		}
		if err := uploadOnce(gh, packed, updates.PieceName(part.SHA256), there); err != nil {
			return err
		}
	}
	for _, f := range []struct {
		name string
		data []byte
	}{
		{zipName, zip},
		{updates.PackedName(zipName), whole},
		{updates.PartsName(zipName), parts},
		{"channel-" + channel + ".json", entry},
	} {
		if err := gh.upload(f.name, f.data); err != nil {
			return err
		}
	}
	if channel != "main" {
		if err := gh.setReady(commit, "success", "On the Updates page as "+channel, ""); err != nil {
			return err
		}
	}
	for _, part := range p.Parts {
		for _, pt := range part.Patches {
			name := updates.PatchName(pt.From, part.SHA256)
			if err := uploadOnce(gh, packed, name, there); err != nil {
				fmt.Fprintf(log, "::warning::%s did not go up, so an app takes the piece of that file instead: %v\n", name, err)
			}
		}
	}
	return nil
}

// assetNames are the names of the release's files.
func assetNames(gh releaseHub) (map[string]bool, error) {
	all, err := gh.assets()
	if err != nil {
		return nil, err
	}
	there := map[string]bool{}
	for _, a := range all {
		there[a.name] = true
	}
	return there, nil
}

// uploadOnce uploads a piece or a patch the release does not hold yet.
// Another channel's publish may upload the same file at the same moment.
// It holds the same bytes, so one that is there once the upload failed
// is uploaded.
func uploadOnce(gh releaseHub, packed, name string, there map[string]bool) error {
	if there[name] {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(packed, name))
	if err != nil {
		return err
	}
	if err := gh.add(name, data); err != nil {
		now, aerr := assetNames(gh)
		if aerr != nil || !now[name] {
			return err
		}
	}
	there[name] = true
	return nil
}

// notPublished marks a commit whose build was not published, whatever
// failed: the check, harden, pack or the signer refused it, or a step on
// the way went wrong. The pull request shows why on the commit, and the
// channel list reads the mark and stops saying a build is coming.
// refusal is what a command wrote when it refused, if anything.
func notPublished(gh releaseHub, commit string, refusal []byte, run string) error {
	why := "Not published: a step of its publishing failed, see the run"
	if text := strings.TrimSpace(string(refusal)); text != "" {
		why = "Not signed: " + firstFinding(text)
	}
	return gh.setReady(commit, "failure", why[:min(len(why), 140)], run)
}

// firstFinding is the first thing a refusal names, the first indented
// line of the list check gives, or its first line.
func firstFinding(text string) string {
	s := bufio.NewScanner(strings.NewReader(text))
	first := ""
	for s.Scan() {
		line := s.Text()
		if first == "" {
			first = line
		}
		if strings.HasPrefix(line, "  ") {
			return strings.TrimSpace(line)
		}
	}
	return strings.TrimPrefix(first, "speakertrail-release: ")
}

func publishPlanCommand(args []string) error {
	fs := flag.NewFlagSet("publish-plan", flag.ExitOnError)
	run := fs.String("run", "", "the run of builds.yml that made the build")
	event := fs.String("event", "", "the event that started it, push or pull_request")
	branch := fs.String("branch", "", "the branch it built")
	commit := fs.String("commit", "", "the commit it built")
	_ = fs.Parse(args)
	if *run == "" || *commit == "" || *branch == "" {
		return errors.New("publish-plan needs -run, -branch and -commit")
	}
	p, err := publishPlan(gitHubFromEnv(), *run, *event, *branch, *commit)
	if err != nil {
		return err
	}
	return p.write(os.Stdout, os.Getenv("GITHUB_OUTPUT"))
}

func publishCommand(args []string) error {
	fs := flag.NewFlagSet("publish", flag.ExitOnError)
	zipPath := fs.String("zip", "", "the signed zip")
	as := fs.String("as", "", "the name it is published under")
	entry := fs.String("entry", "", "its entry for the channel list")
	channel := fs.String("channel", "", "main, or pr- and the pull request's number")
	commit := fs.String("commit", "", "the whole commit it was built from")
	packed := fs.String("packed", "", "the folder pack wrote, whose parts, pieces and patches go up too")
	_ = fs.Parse(args)
	if *zipPath == "" || *as == "" || *entry == "" || *packed == "" || !updates.ValidChannel(*channel) || *commit == "" {
		return errors.New("publish needs -zip, -as, -entry, -packed, -channel and -commit")
	}
	return publish(gitHubFromEnv(), *zipPath, *as, *packed, *entry, *channel, *commit, os.Stdout)
}

func notPublishedCommand(args []string) error {
	fs := flag.NewFlagSet("not-published", flag.ExitOnError)
	commit := fs.String("commit", "", "the whole commit that was built")
	run := fs.String("run", "", "the address of the run, which the mark links to")
	refusal := fs.String("refusal", "", "where a command kept why it refused, if it did")
	_ = fs.Parse(args)
	if *commit == "" {
		return errors.New("not-published needs -commit")
	}
	text, _ := os.ReadFile(*refusal)
	return notPublished(gitHubFromEnv(), *commit, text, *run)
}
