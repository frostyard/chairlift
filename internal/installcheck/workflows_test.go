package installcheck

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

var commitActionPattern = regexp.MustCompile(`^[^@]+@[0-9a-f]{40}$`)

func isImmutableActionReference(ref string) bool {
	return strings.HasPrefix(ref, "./") || commitActionPattern.MatchString(ref)
}

func TestActionReferenceClassification(t *testing.T) {
	sha := strings.Repeat("a", 40)
	for _, tc := range []struct {
		name string
		ref  string
		want bool
	}{
		{name: "commit pinned action", ref: "actions/checkout@" + sha, want: true},
		{name: "commit pinned nested action", ref: "frostyard/repogen/.github/actions/publish-to-r2@" + sha, want: true},
		{name: "local action", ref: "./.github/actions/check", want: true},
		{name: "version tag", ref: "actions/checkout@v6", want: false},
		{name: "branch", ref: "frostyard/repogen/.github/actions/publish-to-r2@main", want: false},
		{name: "short commit", ref: "actions/checkout@deadbeef", want: false},
		{name: "expression", ref: "actions/checkout@${{inputs.ref}}", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := isImmutableActionReference(tc.ref); got != tc.want {
				t.Errorf("isImmutableActionReference(%q) = %v, want %v", tc.ref, got, tc.want)
			}
		})
	}
}

func TestWorkflowActionsUseImmutableCommitSHAs(t *testing.T) {
	workflowDir := filepath.Join(RepoRoot(), ".github", "workflows")
	entries, err := os.ReadDir(workflowDir)
	if err != nil {
		t.Fatalf("read workflow directory: %v", err)
	}

	workflowCount := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := filepath.Ext(entry.Name())
		if ext != ".yml" && ext != ".yaml" {
			continue
		}
		workflowCount++

		path := filepath.Join(workflowDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("read %s: %v", path, err)
			continue
		}
		var document yaml.Node
		if err := yaml.Unmarshal(data, &document); err != nil {
			t.Errorf("parse %s: %v", path, err)
			continue
		}

		var inspectUses func(*yaml.Node)
		inspectUses = func(node *yaml.Node) {
			if node.Kind == yaml.MappingNode {
				for i := 0; i+1 < len(node.Content); i += 2 {
					key, value := node.Content[i], node.Content[i+1]
					if key.Kind == yaml.ScalarNode && key.Tag == "!!str" && key.Value == "uses" {
						if value.Kind != yaml.ScalarNode || value.Tag != "!!str" {
							t.Errorf("%s:%d: uses value must be a string", entry.Name(), value.Line)
						} else if !isImmutableActionReference(value.Value) {
							t.Errorf("%s:%d: external action %q must use a full 40-character commit SHA", entry.Name(), value.Line, value.Value)
						}
					}
					inspectUses(value)
				}
				return
			}
			for _, child := range node.Content {
				inspectUses(child)
			}
		}
		inspectUses(&document)
	}
	if workflowCount == 0 {
		t.Fatal("no workflow files found")
	}
}

// workflowMappingValue returns a direct mapping entry, or nil when absent.
func workflowMappingValue(node *yaml.Node, name string) *yaml.Node {
	if node == nil {
		return nil
	}
	if node.Kind == yaml.DocumentNode && len(node.Content) == 1 {
		node = node.Content[0]
	}
	if node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i]
		if key.Kind == yaml.ScalarNode && key.Tag == "!!str" && key.Value == name {
			return node.Content[i+1]
		}
	}
	return nil
}

func workflowHasReadOnlyDefault(node *yaml.Node) bool {
	permissions := workflowMappingValue(node, "permissions")
	if permissions == nil || permissions.Kind != yaml.MappingNode {
		return false
	}
	for i := 1; i < len(permissions.Content); i += 2 {
		value := permissions.Content[i]
		if value.Kind != yaml.ScalarNode || value.Tag != "!!str" || value.Value != "read" {
			return false
		}
	}
	return true
}

func TestWorkflowReadOnlyDefaultClassification(t *testing.T) {
	for _, tc := range []struct {
		name string
		yaml string
		want bool
	}{
		{name: "absent", yaml: "name: test", want: false},
		{name: "empty", yaml: "permissions: {}", want: true},
		{name: "read", yaml: "permissions: {contents: read}", want: true},
		{name: "mixed", yaml: "permissions: {contents: read, issues: write}", want: false},
		{name: "write", yaml: "permissions: {contents: write}", want: false},
		{name: "read all", yaml: "permissions: read-all", want: false},
		{name: "write all", yaml: "permissions: write-all", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var document yaml.Node
			if err := yaml.Unmarshal([]byte(tc.yaml), &document); err != nil {
				t.Fatal(err)
			}
			if got := workflowHasReadOnlyDefault(&document); got != tc.want {
				t.Errorf("workflowHasReadOnlyDefault = %v, want %v", got, tc.want)
			}
		})
	}
}

// checkWorkflowDocuments visits every workflow file, like the immutable-action
// check, and fails when no workflow exists so the guard cannot pass vacuously.
func checkWorkflowDocuments(t *testing.T, check func(*testing.T, *yaml.Node)) {
	t.Helper()
	workflowDir := filepath.Join(RepoRoot(), ".github", "workflows")
	entries, err := os.ReadDir(workflowDir)
	if err != nil {
		t.Fatalf("read workflow directory: %v", err)
	}
	workflowCount := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := filepath.Ext(entry.Name())
		if ext != ".yml" && ext != ".yaml" {
			continue
		}
		workflowCount++
		t.Run(entry.Name(), func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(workflowDir, entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			var document yaml.Node
			if err := yaml.Unmarshal(data, &document); err != nil {
				t.Fatalf("parse workflow: %v", err)
			}
			check(t, &document)
		})
	}
	if workflowCount == 0 {
		t.Fatal("no workflow files found")
	}
}

func TestWorkflowsDefaultToReadOnlyPermissions(t *testing.T) {
	checkWorkflowDocuments(t, func(t *testing.T, document *yaml.Node) {
		if !workflowHasReadOnlyDefault(document) {
			t.Error("top-level permissions must be present and an empty or read-only mapping (core ADR-0021)")
		}
	})
}

// workflowUsesGoModVersion rejects every go-version key, including keys outside
// setup-go steps. Each setup-go invocation must read the sole Go pin in go.mod.
func workflowUsesGoModVersion(node *yaml.Node) bool {
	if node == nil || node.Kind == yaml.AliasNode {
		return false
	}
	if node.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value == "go-version" {
				return false
			}
		}
		uses := workflowMappingValue(node, "uses")
		if uses != nil && strings.HasPrefix(uses.Value, "actions/setup-go@") {
			version := workflowMappingValue(workflowMappingValue(node, "with"), "go-version-file")
			if version == nil || version.Kind != yaml.ScalarNode || version.Tag != "!!str" || version.Value != "go.mod" {
				return false
			}
		}
	}
	for _, child := range node.Content {
		if !workflowUsesGoModVersion(child) {
			return false
		}
	}
	return true
}

func TestWorkflowGoVersionClassification(t *testing.T) {
	for _, tc := range []struct {
		name string
		yaml string
		want bool
	}{
		{name: "go mod", yaml: "uses: actions/setup-go@sha\nwith: {go-version-file: go.mod}", want: true},
		{name: "stable", yaml: "uses: actions/setup-go@sha\nwith: {go-version: stable}", want: false},
		{name: "explicit version", yaml: "uses: actions/setup-go@sha\nwith: {go-version: '1.27.4'}", want: false},
		{name: "missing version", yaml: "uses: actions/setup-go@sha", want: false},
		{name: "other file", yaml: "uses: actions/setup-go@sha\nwith: {go-version-file: other.mod}", want: false},
		{name: "no setup go", yaml: "jobs: {check: {steps: [{run: echo ok}]}}", want: true},
		{name: "version elsewhere", yaml: "env: {go-version: stable}", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var document yaml.Node
			if err := yaml.Unmarshal([]byte(tc.yaml), &document); err != nil {
				t.Fatal(err)
			}
			if got := workflowUsesGoModVersion(&document); got != tc.want {
				t.Errorf("workflowUsesGoModVersion = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestWorkflowsUseGoModVersion(t *testing.T) {
	checkWorkflowDocuments(t, func(t *testing.T, document *yaml.Node) {
		if !workflowUsesGoModVersion(document) {
			t.Error("workflow must have no go-version key and every setup-go step must use go-version-file: go.mod (core ADR-0043)")
		}
	})
}

func TestWorkflowUsesLeastPrivilege(t *testing.T) {
	path := filepath.Join(".github", "workflows", "ci.yml")
	workflow := readRepoFile(t, path)

	var config struct {
		Permissions *map[string]string `yaml:"permissions"`
		Jobs        map[string]struct {
			Permissions map[string]string `yaml:"permissions"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal([]byte(workflow), &config); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	if config.Permissions == nil || len(*config.Permissions) != 0 {
		t.Errorf("top-level permissions = %v, want explicit empty permissions", config.Permissions)
	}

	expected := map[string]map[string]string{
		"lint":      {"contents": "read"},
		"unit-test": {"contents": "read", "id-token": "write"},
		"race-test": {"contents": "read"},
		"e2e":       {"contents": "read"},
		"verify":    {"contents": "read"},
		"build":     {"contents": "read"},
	}
	if len(config.Jobs) != len(expected) {
		t.Errorf("workflow has %d jobs, want %d", len(config.Jobs), len(expected))
	}
	for name, want := range expected {
		job, ok := config.Jobs[name]
		if !ok {
			t.Errorf("workflow does not define %s job", name)
			continue
		}
		if len(job.Permissions) != len(want) {
			t.Errorf("%s permissions = %v, want %v", name, job.Permissions, want)
			continue
		}
		for permission, access := range want {
			if job.Permissions[permission] != access {
				t.Errorf("%s permissions = %v, want %v", name, job.Permissions, want)
				break
			}
		}
	}
}

func TestSnapshotWorkflowUsesRollingDevConcurrency(t *testing.T) {
	path := filepath.Join(".github", "workflows", "snapshot.yml")
	workflow := readRepoFile(t, path)

	var config struct {
		Concurrency struct {
			Group            string `yaml:"group"`
			CancelInProgress bool   `yaml:"cancel-in-progress"`
		} `yaml:"concurrency"`
	}
	if err := yaml.Unmarshal([]byte(workflow), &config); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	if config.Concurrency.Group != "goreleaser-nightly" {
		t.Errorf("concurrency group = %q, want goreleaser-nightly", config.Concurrency.Group)
	}
	if !config.Concurrency.CancelInProgress {
		t.Error("cancel-in-progress = false, want true")
	}
}
