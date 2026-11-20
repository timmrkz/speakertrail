package main

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/gabstv/go-bsdiff/pkg/bsdiff"

	"github.com/timmrkz/speakertrail/internal/updates"
)

// A file that changed between two builds changes by a few bytes: a change
// to the code moves every address after it and leaves the rest. So pack
// also writes a patch of each file against the same file of every build
// the channel list holds, bsdiff's, and the parts name it by the file it
// starts from. The app downloads the patch when it runs one of those
// builds, see internal/updates/patch.go.
//
// The builds patched against are read from the release as an app reads
// them: their entries, the parts beside their zips and the pieces those
// name. Whatever of them cannot be had costs only its patches, never the
// publish.

// bases is what pack patches against: the entries of the builds the
// release holds, and the folder of the release they are read from.
type bases struct {
	entries []updates.Build
	from    string
	client  *http.Client
}

// patchTakes is the longest reading one file of the release may take.
const patchTakes = 5 * time.Minute

// readBases reads the entries pack patches against.
//
// On the real disk: the workflow hands the release tool the entries as files.
func readBases(paths []string, log io.Writer) []updates.Build {
	var out []updates.Build
	for _, p := range paths {
		data, err := os.ReadFile(p)
		var b updates.Build
		if err == nil {
			err = json.Unmarshal(data, &b)
		}
		if err == nil {
			err = b.Check()
		}
		if err != nil {
			fmt.Fprintf(log, "no patches against %s: %v\n", p, err)
			continue
		}
		out = append(out, b)
	}
	return out
}

// addPatches writes the patches of the files of p, which lie in stored,
// against the same files of the bases, to out, where they weigh less than
// the file's piece, and names them in p.
func addPatches(p *updates.Parts, stored io.ReaderAt, b bases, out string, log io.Writer) {
	if len(b.entries) == 0 {
		return
	}
	// The files of every build patched against, by where they are in the
	// app, each once.
	olds := map[string]map[string]updates.Part{}
	for _, e := range b.entries {
		if e.Stored == nil {
			fmt.Fprintf(log, "no patches against %s, published without parts\n", e.Version)
			continue
		}
		bp, err := b.parts(e)
		if err != nil {
			fmt.Fprintf(log, "no patches against %s: %v\n", e.Version, err)
			continue
		}
		for _, part := range bp.Parts {
			if part.Bytes != nil {
				continue
			}
			inside := inApp(part.Name)
			if olds[inside] == nil {
				olds[inside] = map[string]updates.Part{}
			}
			olds[inside][part.SHA256] = part
		}
	}
	type job struct {
		part  int
		at    int64
		old   updates.Part
		patch updates.Patch
		ok    bool
	}
	var jobs []*job
	at := int64(0)
	for i, part := range p.Parts {
		if part.Bytes != nil {
			at += int64(len(part.Bytes))
			continue
		}
		for sum, old := range olds[inApp(part.Name)] {
			if sum != part.SHA256 {
				jobs = append(jobs, &job{part: i, at: at, old: old})
			}
		}
		at += part.Size
	}
	work := make(chan *job)
	var wg sync.WaitGroup
	for range runtime.NumCPU() {
		wg.Go(func() {
			for j := range work {
				part := p.Parts[j.part]
				patch, err := b.patch(stored, j.at, part, j.old)
				if err == nil && int64(len(patch)) >= part.Packed {
					err = fmt.Errorf("it is %d bytes, the piece %d", len(patch), part.Packed)
				}
				if err == nil {
					err = os.WriteFile(filepath.Join(out, updates.PatchName(j.old.SHA256, part.SHA256)), patch, 0o644)
				}
				if err != nil {
					fmt.Fprintf(log, "no patch of %s from %.12s: %v\n", part.Name, j.old.SHA256, err)
					continue
				}
				j.patch, j.ok = updates.Patch{From: j.old.SHA256, Size: int64(len(patch))}, true
			}
		})
	}
	// The largest first, so the last to finish is a small one.
	for _, j := range slices.SortedFunc(slices.Values(jobs), func(a, c *job) int {
		return cmp.Compare(p.Parts[c.part].Size, p.Parts[a.part].Size)
	}) {
		work <- j
	}
	close(work)
	wg.Wait()
	for _, j := range jobs {
		if j.ok {
			p.Parts[j.part].Patches = append(p.Parts[j.part].Patches, j.patch)
		}
	}
	for i := range p.Parts {
		slices.SortFunc(p.Parts[i].Patches, func(a, c updates.Patch) int { return cmp.Compare(a.From, c.From) })
		if len(p.Parts[i].Patches) > updates.MaxPatches {
			p.Parts[i].Patches = p.Parts[i].Patches[:updates.MaxPatches]
		}
	}
}

// inApp is where a file of the zip is inside the app, the name without the
// app's own folder.
func inApp(name string) string {
	_, rest, _ := strings.Cut(name, "/")
	return rest
}

// parts reads the parts beside a build's zip.
func (b bases) parts(e updates.Build) (updates.Parts, error) {
	data, err := b.get(updates.PartsName(path.Base(e.URL)), updates.MaxList)
	if err != nil {
		return updates.Parts{}, err
	}
	return updates.ParseParts(data, e.Stored.Size, e.Stored.SHA256)
}

// patch is the patch from the file old names to the file part names,
// which lies at at in stored, once it is known to make that file.
func (b bases) patch(stored io.ReaderAt, at int64, part, old updates.Part) ([]byte, error) {
	packed, err := b.get(updates.PieceName(old.SHA256), old.Packed)
	if err != nil {
		return nil, err
	}
	from, err := updates.UnpackPiece(packed, old)
	if err != nil {
		return nil, err
	}
	to := make([]byte, part.Size)
	if _, err := stored.ReadAt(to, at); err != nil {
		return nil, err
	}
	patch, err := bsdiff.Bytes(from, to)
	if err != nil {
		return nil, err
	}
	// The app reads a patch with code of its own, so what is published is
	// what that code makes the file from.
	made, err := updates.ApplyPatch(from, patch, part.Size)
	if err != nil {
		return nil, fmt.Errorf("the app cannot apply it: %w", err)
	}
	if !bytes.Equal(made, to) {
		return nil, errors.New("the app would make another file from it")
	}
	return patch, nil
}

// get reads a file of the release, at most limit bytes.
func (b bases) get(name string, limit int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), patchTakes)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.from+name, nil)
	if err != nil {
		return nil, err
	}
	client := b.client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s answered %s", name, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s is larger than it can be", name)
	}
	return data, nil
}
