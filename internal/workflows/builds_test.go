package workflows

import (
	"maps"
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
// a run built, as it does a push's. A push leaves a pull request opened
// after it to the opening, by the time GitHub says it was pushed, so the
// two never build one commit twice, FrameFairy #642.
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

	var steps struct {
		Jobs map[string]struct {
			Steps []struct {
				ID  string            `yaml:"id"`
				Env map[string]string `yaml:"env"`
				Run string            `yaml:"run"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(b, &steps); err != nil {
		t.Fatal(err)
	}
	// What is decided is speakertrail-release build-plan's, with its tests
	// in cmd/speakertrail-release/plan_test.go. Here, that the workflow
	// hands it every fact it decides by, each from where GitHub says it.
	// The pull request's head and not github.sha, which for a pull
	// request is the merge. The branch, or main is no longer built as
	// main. The push's time, or one commit is built twice or not at all.
	wantEnv := map[string]string{
		"GH_TOKEN": "${{ github.token }}",
		"EVENT":    "${{ github.event_name }}",
		"ACTION":   "${{ github.event.action }}",
		"BRANCH":   "${{ github.ref_name }}",
		"NUMBER":   "${{ github.event.pull_request.number }}",
		"SHA":      "${{ github.event.pull_request.head.sha || github.sha }}",
		"PUSHED":   "${{ github.event.repository.pushed_at }}",
	}
	wantRun := `go run ./cmd/speakertrail-release build-plan -event "$EVENT" -action "$ACTION" -branch "$BRANCH" -commit "$SHA" -number "$NUMBER" -pushed "$PUSHED"`
	found := false
	for _, s := range steps.Jobs["which"].Steps {
		if s.ID != "which" {
			continue
		}
		found = true
		if !maps.Equal(s.Env, wantEnv) {
			t.Errorf("builds.yml's which hands build-plan %v, want %v", s.Env, wantEnv)
		}
		if got := strings.Join(strings.Fields(s.Run), " "); got != wantRun {
			t.Errorf("builds.yml's which runs\n%s\nwant\n%s", got, wantRun)
		}
	}
	if !found {
		t.Error("builds.yml has no step which in its job which, so nothing says what a run is a build of")
	}

	b, err = os.ReadFile(filepath.Join(dir, "publish.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var publish struct {
		Jobs map[string]struct {
			If     string `yaml:"if"`
			Needs  any    `yaml:"needs"`
			RunsOn string `yaml:"runs-on"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(b, &publish); err != nil {
		t.Fatal(err)
	}
	// A run that left no zip, as every close now is, starts no Mac: a
	// job on Linux asks first.
	if publish.Jobs["publish"].Needs != "built" || !strings.Contains(publish.Jobs["publish"].If, "needs.built.outputs.zip == 'true'") ||
		!strings.HasPrefix(publish.Jobs["built"].RunsOn, "ubuntu") {
		t.Errorf("publish.yml's publish does not wait for a job on Linux to find a zip, so a run that built nothing starts a Mac")
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
