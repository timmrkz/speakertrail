package workflows

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"go.yaml.in/yaml/v3"
)

// No workflow runs on pull_request_target, and conflicts.yml hears every
// push from main's own copy of it. pull_request_target runs main's
// workflow for a stranger's pull request with a token that writes, so one
// step that reads the pull request's code hands that token over.
// pull_request runs on no pull request that conflicts. So conflicts.yml
// runs on a push to main, and on a push to any other branch through
// builds.yml, whose start GitHub reports to main's copy. A push to a
// branch must not start it directly, because then the branch's own copy,
// code nobody has reviewed, would get the token that comments. Ported
// from FrameFairy.
//
// On the real disk: what is read is the repository's own workflows.
func TestNoPullRequestTarget(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob(filepath.Join("..", "..", ".github", "workflows", "*.yml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("found no workflows: %v", err)
	}
	names := map[string]string{}
	seen, waitsOn := false, ""
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(f)
		var wf struct {
			Name string `yaml:"name"`
			On   any    `yaml:"on"`
		}
		if err := yaml.Unmarshal(b, &wf); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		names[name] = wf.Name
		// The events are a word, a list or a map.
		events := map[string]any{}
		switch on := wf.On.(type) {
		case string:
			events[on] = nil
		case []any:
			for _, e := range on {
				events[fmt.Sprint(e)] = nil
			}
		case map[string]any:
			events = on
		}
		if _, ok := events["pull_request_target"]; ok {
			t.Errorf("%s runs on pull_request_target, which hands a token that writes to a stranger's pull request: use pull_request, or workflow_run as conflicts.yml does", name)
		}
		if name != "conflicts.yml" {
			continue
		}
		seen = true
		push, _ := events["push"].(map[string]any)
		if b, _ := push["branches"].([]any); len(push) != 1 || len(b) != 1 || b[0] != "main" {
			t.Errorf("%s runs on a push to %v, where only main may start it: a branch's own copy would get the token that comments", name, events["push"])
		}
		run, _ := events["workflow_run"].(map[string]any)
		builds, _ := run["workflows"].([]any)
		types, _ := run["types"].([]any)
		if len(builds) != 1 || len(types) != 1 || types[0] != "requested" {
			t.Errorf("%s does not start when builds.yml starts, so a pull request that starts to conflict on its own push is never told", name)
			continue
		}
		waitsOn = fmt.Sprint(builds[0])
	}
	if !seen {
		t.Error("found no conflicts.yml, so nothing tells a pull request that it conflicts")
	}
	if waitsOn != "" && waitsOn != names["builds.yml"] {
		t.Errorf("conflicts.yml waits on the workflow %q, but builds.yml is called %q", waitsOn, names["builds.yml"])
	}
}
