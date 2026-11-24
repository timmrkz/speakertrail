package workflows

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// A pull request is built when it is opened, not only on a push. A
// session pushes its branch first and opens the pull request seconds
// later, so that push finds no pull request and builds nothing. A pull
// request of that one commit then had no build and never showed on the
// Updates page, as #93 did on 24 November 2026. So builds.yml runs when a
// pull request is opened or reopened, and publish.yml publishes what such
// a run built, as it does a push's.
//
// On the real disk: what is read is the repository's own workflows.
func TestAPullRequestIsBuiltWhenItOpens(t *testing.T) {
	t.Parallel()
	dir := filepath.Join("..", "..", ".github", "workflows")

	b, err := os.ReadFile(filepath.Join(dir, "builds.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var builds struct {
		On struct {
			PullRequest struct {
				Types []string `yaml:"types"`
			} `yaml:"pull_request"`
		} `yaml:"on"`
	}
	if err := yaml.Unmarshal(b, &builds); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"opened", "reopened", "closed"} {
		if !slices.Contains(builds.On.PullRequest.Types, want) {
			t.Errorf("builds.yml does not run when a pull request is %s, it runs on %v", want, builds.On.PullRequest.Types)
		}
	}

	b, err = os.ReadFile(filepath.Join(dir, "publish.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var publish struct {
		Jobs map[string]struct {
			If string `yaml:"if"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(b, &publish); err != nil {
		t.Fatal(err)
	}
	cond := publish.Jobs["publish"].If
	if !strings.Contains(cond, "github.event.workflow_run.event == 'pull_request'") {
		t.Errorf("publish.yml publishes no build of a pull request that was opened, its if is %q", cond)
	}
	// What keeps a branch's build from passing as another's stays.
	for _, want := range []string{
		"github.event.workflow_run.path == '.github/workflows/builds.yml'",
		"github.event.workflow_run.head_repository.full_name == github.repository",
	} {
		if !strings.Contains(cond, want) {
			t.Errorf("publish.yml's publish job no longer checks %s", want)
		}
	}
}
