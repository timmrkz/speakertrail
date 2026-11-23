package workflows

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// read is a path a Go test names to read a file of the interface, the
// docs or the skills, relative to the repository's root however many ../
// lead there. A folder read whole ends in a slash.
var read = regexp.MustCompile(`"(?:\.\./)*((?:web|docs|\.claude/skills)/[^"]*)"`)

// A pull request starts the Go tests only when it changes Go, by the
// filters of the job changes in ci.yml, and Go is no file of web/ and
// none of the docs. So a Go test that reads one would not run on the pull
// request that changes it, and main would find out after the merge.
// Every file of web/ a Go test reads or web/embed.go embeds is in the
// filter goread, beside go, a skill a Go test reads is in the filter
// skills, which starts the Go job and not the Mac's, and no Go test reads
// the docs.
//
// On the real disk: what is read is the repository's own tests and CI.
func TestWhatAGoTestReadsStartsIt(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	b, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var ci struct {
		Jobs map[string]struct {
			Steps []struct {
				ID   string            `yaml:"id"`
				With map[string]string `yaml:"with"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(b, &ci); err != nil {
		t.Fatal(err)
	}
	var filters map[string][]string
	for _, s := range ci.Jobs["changes"].Steps {
		if s.ID == "filter" {
			if err := yaml.Unmarshal([]byte(s.With["filters"]), &filters); err != nil {
				t.Fatal(err)
			}
		}
	}
	if len(filters["goread"]) == 0 {
		t.Fatal("ci.yml's job changes has no filter goread, so this test reads it wrong")
	}
	if len(filters["skills"]) == 0 {
		t.Fatal("ci.yml's job changes has no filter skills, so a change to a skill starts no Go test")
	}
	starts := func(file string) bool {
		for _, p := range append(filters["goread"], filters["skills"]...) {
			for _, glob := range braces(p) {
				if ok, _ := path.Match(glob, file); ok {
					return true
				}
			}
		}
		return false
	}
	// The Go of web/ embeds a folder that only make fills, and the go job
	// does not run make: the file committed there, .keep, is what Go reads.
	// Each folder embedded is also what this test checks itself on, since
	// no Go test reads a file of web/ yet.
	src, err := os.ReadFile(filepath.Join(root, "web", "embed.go"))
	if err != nil {
		t.Fatal(err)
	}
	embeds := regexp.MustCompile(`(?m)^//go:embed (.+)$`).FindAllStringSubmatch(string(src), -1)
	if len(embeds) == 0 {
		t.Fatal("web/embed.go embeds nothing, so this test reads it wrong")
	}
	for _, e := range embeds {
		for _, p := range strings.Fields(e[1]) {
			file := "web/" + strings.TrimPrefix(p, "all:") + "/.keep"
			if !starts(file) {
				t.Errorf("web/embed.go embeds %s, and ci.yml's filter goread leaves out %s: a pull request that removes it starts no Go test. Add it there", p, file)
			}
		}
	}
	if m := read.FindStringSubmatch(`os.ReadFile("../../web/embed.go")`); m == nil || !starts(m[1]) {
		t.Fatalf("this test does not see that a Go test reading web/embed.go is started by goread, %q", m)
	}
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "node_modules" || strings.HasPrefix(d.Name(), ".")) && p != root {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(p, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		if filepath.ToSlash(rel) == "internal/workflows/workflows_test.go" {
			return nil
		}
		for _, m := range read.FindAllStringSubmatch(string(src), -1) {
			file := m[1]
			switch {
			case strings.HasPrefix(file, "docs/"):
				t.Errorf("%s reads %s, and a pull request of the docs starts no Go test", rel, file)
			case !starts(file):
				t.Errorf("%s reads %s, which ci.yml's filters goread and skills leave out: a pull request that changes it starts no Go test. Add it there", rel, file)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// braces spells out a glob's one {a,b} the way the filter reads it.
func braces(glob string) []string {
	i, j := strings.Index(glob, "{"), strings.Index(glob, "}")
	if i < 0 || j < i {
		return []string{glob}
	}
	var out []string
	for _, alt := range strings.Split(glob[i+1:j], ",") {
		out = append(out, glob[:i]+alt+glob[j+1:])
	}
	return out
}
