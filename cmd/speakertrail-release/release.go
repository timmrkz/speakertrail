package main

import (
	"cmp"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/timmrkz/speakertrail/internal/updates"
)

// Which files of the release dev the builds an app may run still reach.
// pack patches against those builds, and unreachable names everything
// else, so the publish workflow can remove it. The workflow hands the
// tool the channel list and the release's files, as gh writes them, and
// gh does the uploading and removing. See .github/workflows/publish.yml
// and docs/UPDATES.md.

// leftBases is how many builds that left the channel list a new build is
// patched against. A build leaves the list when its pull request is
// merged or a newer commit replaces it, and the app that runs it is then
// the one most likely to update next, to main after a merge above all.
// Each costs the publish a bsdiff of a few seconds for each file that
// changed, four at a time.
const leftBases = 8

// keptFor is how long a file no build reaches stays on the release: a
// publish that runs beside the clean-up may have uploaded files whose
// entry the list has not seen yet.
const keptFor = time.Hour

// asset is a file of the dev, as
// `gh release view dev --json assets` writes it.
type asset struct {
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

// dev is what the release dev holds: the channel list and its files.
type dev struct {
	list   updates.List
	assets []asset
}

// readRelease reads the channel list and the release's files from the
// files the workflow wrote.
//
// On the real disk: the workflow writes both with gh.
func readRelease(listPath, assetsPath string) (dev, error) {
	var r dev
	data, err := os.ReadFile(listPath)
	if err != nil {
		return r, err
	}
	if err := json.Unmarshal(data, &r.list); err != nil {
		return r, fmt.Errorf("the channel list is not JSON: %w", err)
	}
	data, err = os.ReadFile(assetsPath)
	if err != nil {
		return r, err
	}
	var files struct {
		Assets []asset `json:"assets"`
	}
	if err := json.Unmarshal(data, &files); err != nil {
		return r, fmt.Errorf("the release's files are not JSON: %w", err)
	}
	r.assets = files.Assets
	return r, nil
}

// zips are the names of the zips the channel list points at.
func (r dev) zips() []string {
	var out []string
	for _, b := range r.list.Channels {
		u, err := url.Parse(b.URL)
		if err != nil {
			continue
		}
		if name := path.Base(u.Path); strings.HasPrefix(name, "app-") && strings.HasSuffix(name, ".zip") {
			out = append(out, name)
		}
	}
	return out
}

func (r dev) holds(name string) bool {
	return slices.ContainsFunc(r.assets, func(a asset) bool { return a.Name == name })
}

// bases names the parts files of the builds a new build is patched
// against: every build the channel list names, and the newest leftBases
// that left it. Only what the release holds, each once.
func (r dev) bases() []string {
	var out []string
	add := func(name string) {
		if r.holds(name) && !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	for _, zip := range r.zips() {
		add(updates.PartsName(zip))
	}
	listed := len(out)
	newest := slices.SortedFunc(slices.Values(r.assets), func(a, b asset) int {
		return b.CreatedAt.Compare(a.CreatedAt)
	})
	for _, a := range newest {
		if len(out)-listed == leftBases {
			break
		}
		if strings.HasPrefix(a.Name, "app-") && strings.HasSuffix(a.Name, ".parts.json") {
			add(a.Name)
		}
	}
	return out
}

// unreachable names the files of the release that no build an app may
// run reaches, older than keptFor at now: a build's zip and its packed
// zip when the list does not point at it, the parts of every build that
// is not one of bases, and every piece and patch that no parts in parts
// name. parts holds the parts of bases that could be read. When one could
// not, every piece and patch stays, since nothing then says which may go.
// When the release's files cannot be the whole of them, because a zip the
// list points at is not among them or a file has no time, nothing goes.
func (r dev) unreachable(parts map[string]updates.Parts, now time.Time) []string {
	for _, zip := range r.zips() {
		if !r.holds(zip) {
			return nil
		}
	}
	for _, a := range r.assets {
		if a.CreatedAt.IsZero() {
			return nil
		}
	}
	kept := map[string]bool{}
	for _, zip := range r.zips() {
		kept[zip], kept[updates.PackedName(zip)] = true, true
	}
	known := true
	for _, name := range r.bases() {
		kept[name] = true
		p, ok := parts[name]
		if !ok {
			known = false
			continue
		}
		for _, part := range p.Parts {
			if part.Bytes != nil {
				continue
			}
			kept[updates.PieceName(part.SHA256)] = true
			for _, pt := range part.Patches {
				kept[updates.PatchName(pt.From, part.SHA256)] = true
			}
		}
	}
	var out []string
	for _, a := range r.assets {
		if kept[a.Name] || now.Sub(a.CreatedAt) <= keptFor {
			continue
		}
		switch {
		case strings.HasPrefix(a.Name, "app-") && (strings.HasSuffix(a.Name, ".zip") ||
			strings.HasSuffix(a.Name, ".zip.zst") || strings.HasSuffix(a.Name, ".parts.json")):
		case known && (strings.HasPrefix(a.Name, "piece-") || strings.HasPrefix(a.Name, "patch-")):
		default:
			continue
		}
		out = append(out, a.Name)
	}
	slices.SortFunc(out, cmp.Compare)
	return out
}

// readParts reads the parts files named from the dev, by the app's
// own rules, against the size and checksum they say they are of, since
// nothing else here says it. One that cannot be read is left out and
// said, as a warning on the workflow's run.
func (b bases) readParts(names []string, log io.Writer) map[string]updates.Parts {
	out := map[string]updates.Parts{}
	for _, name := range names {
		data, err := b.get(name, updates.MaxList)
		var p updates.Parts
		if err == nil {
			err = json.Unmarshal(data, &p)
		}
		if err == nil {
			p, err = updates.ParseParts(data, p.Size, p.SHA256)
		}
		if err != nil {
			fmt.Fprintf(log, "::warning::%s could not be read: %v\n", name, err)
			continue
		}
		out[name] = p
	}
	return out
}

// unreachableCommand writes the names of the files of the release that no
// build an app may run reaches, one a line, for gh to remove.
func unreachableCommand(args []string) error {
	fs := flag.NewFlagSet("unreachable", flag.ExitOnError)
	listPath := fs.String("list", "", "the channel list")
	assetsPath := fs.String("assets", "", "the release's files, as gh release view dev --json assets writes them")
	from := fs.String("from", updates.EntriesURL, "the folder of the release the parts are read from")
	_ = fs.Parse(args)
	if *listPath == "" || *assetsPath == "" {
		return errors.New("unreachable needs -list and -assets")
	}
	r, err := readRelease(*listPath, *assetsPath)
	if err != nil {
		return err
	}
	b := bases{from: *from, client: &http.Client{}}
	for _, name := range r.unreachable(b.readParts(r.bases(), os.Stderr), time.Now()) {
		fmt.Fprintln(os.Stderr, "removing", name)
		fmt.Println(name)
	}
	return nil
}
