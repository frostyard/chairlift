package installcheck

import (
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestClaudeReviewWorkflowIsMaintainerScoped(t *testing.T) {
	path := filepath.Join(".github", "workflows", "claude-code-review.yml")
	workflow := readRepoFile(t, path)

	var config struct {
		Permissions map[string]string `yaml:"permissions"`
		Jobs        map[string]struct {
			Permissions map[string]string `yaml:"permissions"`
			Steps       []struct {
				Uses string `yaml:"uses"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal([]byte(workflow), &config); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	if len(config.Permissions) != 0 {
		t.Errorf("top-level permissions = %v, want none", config.Permissions)
	}

	analyze, ok := config.Jobs["analyze"]
	if !ok {
		t.Fatal("workflow does not define analyze job")
	}
	if len(analyze.Permissions) != 2 ||
		analyze.Permissions["contents"] != "read" ||
		analyze.Permissions["pull-requests"] != "read" {
		t.Errorf("analyze permissions = %v, want contents/read and pull-requests/read", analyze.Permissions)
	}

	publish, ok := config.Jobs["publish"]
	if !ok {
		t.Fatal("workflow does not define publish job")
	}
	if len(publish.Permissions) != 1 || publish.Permissions["pull-requests"] != "write" {
		t.Errorf("publish permissions = %v, want only pull-requests/write", publish.Permissions)
	}

	var analyzeUsesClaude, publishUsesGitHubScript bool
	for _, step := range analyze.Steps {
		analyzeUsesClaude = analyzeUsesClaude ||
			strings.HasPrefix(step.Uses, "anthropics/claude-code-action@")
		if strings.HasPrefix(step.Uses, "actions/github-script@") {
			t.Error("analyze job must not have a write-capable publishing step")
		}
	}
	for _, step := range publish.Steps {
		publishUsesGitHubScript = publishUsesGitHubScript ||
			strings.HasPrefix(step.Uses, "actions/github-script@")
		if strings.HasPrefix(step.Uses, "anthropics/claude-code-action@") {
			t.Error("publish job must not run Claude with a write-capable token")
		}
		if strings.HasPrefix(step.Uses, "actions/checkout@") {
			t.Error("publish job must not check out repository content")
		}
	}
	if !analyzeUsesClaude {
		t.Error("analyze job does not run the Claude Code action")
	}
	if !publishUsesGitHubScript {
		t.Error("publish job does not use the fixed GitHub publishing step")
	}

	for _, required := range []string{
		"workflow_dispatch:",
		"pull_request_number:",
		"github.ref_name == github.event.repository.default_branch",
		"persist-credentials: false",
		"ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}",
		"github_token: ${{ secrets.GITHUB_TOKEN }}",
		"anthropics/claude-code-action@",
		"Treat the pull request title, body, comments, commits, diff, and",
		"Do not modify files, commit, push, approve, merge, release, deploy,",
		`--allowedTools "Read,Glob,Grep,Bash(gh pr view:*),Bash(gh pr diff:*)"`,
		"--json-schema",
		"github.rest.issues.createComment",
		"show_full_output: false",
	} {
		if !strings.Contains(workflow, required) {
			t.Errorf("%s does not contain required contract %q", path, required)
		}
	}

	for _, unsafe := range []string{
		"pull_request_target:",
		"\n  pull_request:",
		"contents: write",
		"issues: write",
		"id-token: write",
		"allowed_non_write_users:",
		"show_full_output: true",
		"track_progress: true",
		"Bash(gh pr comment:",
		"Bash(git:",
		"Bash(go:",
		"Bash(make:",
		"github.event.pull_request.head",
	} {
		if strings.Contains(workflow, unsafe) {
			t.Errorf("%s contains unsafe review surface %q", path, unsafe)
		}
	}

	quality := readRepoFile(t, filepath.Join("docs", "quality.md"))
	for _, required := range []string{
		"`.github/workflows/claude-code-review.yml`",
		"`ANTHROPIC_API_KEY`",
		"does not check out the pull request head",
		"separate publishing job",
	} {
		if !strings.Contains(quality, required) {
			t.Errorf("docs/quality.md does not document %q", required)
		}
	}
}
