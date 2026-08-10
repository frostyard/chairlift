package installcheck

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func readRepoFile(t *testing.T, relative string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(RepoRoot(), relative))
	if err != nil {
		t.Fatalf("read %s: %v", relative, err)
	}
	return string(data)
}

func TestCurrentDocumentationMatchesSourceFacts(t *testing.T) {
	t.Run("updex version comes from go.mod", func(t *testing.T) {
		goMod := readRepoFile(t, "go.mod")
		match := regexp.MustCompile(`(?m)^\s*github\.com/frostyard/updex\s+(v\S+)`).FindStringSubmatch(goMod)
		if len(match) != 2 {
			t.Fatal("go.mod does not contain a parseable github.com/frostyard/updex version")
		}
		overview := readRepoFile(t, filepath.Join("yeti", "OVERVIEW.md"))
		want := "currently pinned to " + match[1] + " in go.mod"
		if !strings.Contains(overview, want) {
			t.Errorf("yeti/OVERVIEW.md does not contain %q", want)
		}
	})

	t.Run("bootc event contract has no duplicate error event", func(t *testing.T) {
		for _, path := range []string{
			filepath.Join("internal", "bootc", "stage.go"),
			filepath.Join("internal", "views", "updates_page.go"),
			filepath.Join("yeti", "OVERVIEW.md"),
			filepath.Join("yeti", "package-managers.md"),
		} {
			if strings.Contains(readRepoFile(t, path), "EventError") {
				t.Errorf("%s still documents or implements removed EventError", path)
			}
		}
	})

	t.Run("historical port guide is marked", func(t *testing.T) {
		if !strings.Contains(readRepoFile(t, "README-go-port.md"), "**Historical document.**") {
			t.Error("README-go-port.md is not clearly marked historical")
		}
	})

	t.Run("configuration fallback wording is consistent", func(t *testing.T) {
		for _, path := range []string{
			"README.md",
			"CONFIG.md",
			filepath.Join("docs", "reference.md"),
		} {
			document := readRepoFile(t, path)
			for _, fact := range []string{
				"beside the",
				"executable",
				"current working",
				"development fallback",
			} {
				if !strings.Contains(document, fact) {
					t.Errorf("%s does not describe %q", path, fact)
				}
			}
		}
	})

	t.Run("optional visibility and install prefix are current", func(t *testing.T) {
		index := readRepoFile(t, filepath.Join("docs", "index.md"))
		if strings.Contains(index, "Groups for unavailable tools are hidden automatically") {
			t.Error("docs/index.md retains the false uniform runtime-visibility claim")
		}
		if !strings.Contains(index, "default `/usr`") {
			t.Error("docs/index.md does not document the /usr install default")
		}
	})

	t.Run("public metrics catalog stays auditable", func(t *testing.T) {
		catalog := strings.Join(strings.Fields(readRepoFile(t, filepath.Join("docs", "metrics", "README.md"))), " ")
		for _, required := range []string{
			"../metrics.md",
			"actions/workflows/test.yml",
			"actions/workflows/nightly-compliance.yml",
			"app.codecov.io/gh/frostyard/chairlift",
			"does not currently attach a reliable provenance marker",
			"does not collect application usage telemetry",
		} {
			if !strings.Contains(catalog, required) {
				t.Errorf("docs/metrics/README.md does not contain %q", required)
			}
		}
	})

	t.Run("known stale claims stay removed", func(t *testing.T) {
		current := strings.Join([]string{
			readRepoFile(t, "README.md"),
			readRepoFile(t, "CONFIG.md"),
			readRepoFile(t, filepath.Join("docs", "index.md")),
			readRepoFile(t, filepath.Join("docs", "reference.md")),
		}, "\n")
		for _, stale := range []string{
			"updates_status_group",
			"Help page coming soon",
			"Help is coming soon",
			"Groups for unavailable tools are hidden automatically",
		} {
			if strings.Contains(current, stale) {
				t.Errorf("current documentation still contains stale claim %q", stale)
			}
		}
	})
}

func TestAIFixRequestedWorkflowIsLabelScoped(t *testing.T) {
	path := filepath.Join(".github", "workflows", "ai-fix-requested.yml")
	workflow := readRepoFile(t, path)

	var document yaml.Node
	if err := yaml.Unmarshal([]byte(workflow), &document); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	for _, required := range []string{
		"issues:",
		"types: [labeled]",
		"issues: write",
		"github.event.issue.state == 'open'",
		"github.event.label.name == 'ai-fix-requested'",
		"actions/github-script@ed597411d8f924073f98dfc5c65a23a2325f34cd",
		"<!-- ai-fix-requested:${issueNumber} -->",
		"comments.some((comment) => comment.body?.includes(marker))",
		"@copilot Please implement",
	} {
		if !strings.Contains(workflow, required) {
			t.Errorf("%s does not contain required contract %q", path, required)
		}
	}

	for _, unsafe := range []string{
		"pull_request_target:",
		"actions/checkout@",
		"github.event.issue.body",
		"github.event.issue.title",
		"context.payload.issue.body",
		"context.payload.issue.title",
		"contents: write",
		"\n        run:",
	} {
		if strings.Contains(workflow, unsafe) {
			t.Errorf("%s contains unsafe or unnecessary workflow surface %q", path, unsafe)
		}
	}

	quality := readRepoFile(t, filepath.Join("docs", "quality.md"))
	if !strings.Contains(quality, "`.github/workflows/ai-fix-requested.yml`") {
		t.Error("docs/quality.md does not document the AI-fix-requested workflow")
	}
}
