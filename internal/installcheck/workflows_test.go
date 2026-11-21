package installcheck

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

var (
	commitActionPattern = regexp.MustCompile(`^[^@]+@[0-9a-f]{40}$`)

	// actionPin and versionComment are the repository-wide pin check of
	// core ADR-0090: every OWNER/REPO[/PATH]@SHA carries a # vX.Y.Z or
	// # vX.Y comment, and each action has 1 SHA across all files.
	actionPin      = regexp.MustCompile(`^([^@]+)@([0-9a-f]{40})$`)
	versionComment = regexp.MustCompile(`^# v\d+\.\d+(\.\d+)?$`)
)

// corePinPrefix names a frostyard/core reusable workflow. Core has no release
// tags, so its pin needs only a comment (which names the core commit on main).
const corePinPrefix = "frostyard/core/.github/workflows/"

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

// pinSHAs maps each action (OWNER/REPO[/PATH]) to its SHAs, and each SHA to
// the first file:line that pins it.
type pinSHAs map[string]map[string]string

// checkPin checks the value node of 1 uses: key (ADR-0090) and records its SHA.
// A value that is not OWNER/REPO[/PATH]@SHA (./local or a mutable ref) is left
// to isImmutableActionReference.
func checkPin(path string, value *yaml.Node, shas pinSHAs) error {
	m := actionPin.FindStringSubmatch(value.Value)
	if m == nil {
		return nil
	}
	action, sha := m[1], m[2]
	where := fmt.Sprintf("%s:%d", path, value.Line)
	if shas[action] == nil {
		shas[action] = map[string]string{}
	}
	if _, seen := shas[action][sha]; !seen {
		shas[action][sha] = where
	}
	switch {
	case strings.HasPrefix(action, corePinPrefix):
		if value.LineComment == "" {
			return fmt.Errorf("%s: %s has no comment; name the core commit on main", where, action)
		}
	case !versionComment.MatchString(value.LineComment):
		return fmt.Errorf("%s: %s has comment %q, want # vX.Y.Z or # vX.Y", where, action, value.LineComment)
	}
	return nil
}

// checkOneSHA runs once, after every file is walked: each action has 1 SHA.
func checkOneSHA(shas pinSHAs) []error {
	var problems []error
	for action, set := range shas {
		if len(set) == 1 {
			continue
		}
		var at []string
		for sha, where := range set {
			at = append(at, sha+" ("+where+")")
		}
		sort.Strings(at)
		problems = append(problems, fmt.Errorf("%s has %d SHAs across the workflows, want 1: %s",
			action, len(set), strings.Join(at, ", ")))
	}
	sort.Slice(problems, func(i, j int) bool { return problems[i].Error() < problems[j].Error() })
	return problems
}

// pinReport is what 1 walk of workflow and action files found.
type pinReport struct {
	files    int
	uses     int
	shas     pinSHAs
	problems []error
}

// checkWorkflowFile checks every uses: key of 1 file: ADR-0021's immutable
// commit SHA and ADR-0090's version comment.
func checkWorkflowFile(path string, data []byte, report *pinReport) {
	report.files++
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		report.problems = append(report.problems, fmt.Errorf("parse %s: %w", path, err))
		return
	}
	var inspectUses func(*yaml.Node)
	inspectUses = func(node *yaml.Node) {
		if node.Kind == yaml.MappingNode {
			for i := 0; i+1 < len(node.Content); i += 2 {
				key, value := node.Content[i], node.Content[i+1]
				if key.Kind == yaml.ScalarNode && key.Tag == "!!str" && key.Value == "uses" {
					report.uses++
					switch {
					case value.Kind != yaml.ScalarNode || value.Tag != "!!str":
						report.problems = append(report.problems, fmt.Errorf("%s:%d: uses value must be a string", path, value.Line))
					case !isImmutableActionReference(value.Value):
						report.problems = append(report.problems, fmt.Errorf("%s:%d: external action %q must use a full 40-character commit SHA", path, value.Line, value.Value))
					default:
						if err := checkPin(path, value, report.shas); err != nil {
							report.problems = append(report.problems, err)
						}
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

// checkPinTree walks every .yml and .yaml file below dirs. A missing directory
// is skipped: .github/actions exists only in some repositories.
func checkPinTree(dirs ...string) (*pinReport, error) {
	report := &pinReport{shas: pinSHAs{}}
	for _, dir := range dirs {
		if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			if ext := filepath.Ext(path); ext != ".yml" && ext != ".yaml" {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(RepoRoot(), path)
			if err != nil || strings.HasPrefix(rel, "..") {
				rel = path
			}
			checkWorkflowFile(rel, data, report)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	report.problems = append(report.problems, checkOneSHA(report.shas)...)
	return report, nil
}

// TestWorkflowActionsUseImmutableCommitSHAs is the repository's ADR-0021 and
// ADR-0090 pin check over .github/workflows and .github/actions.
func TestWorkflowActionsUseImmutableCommitSHAs(t *testing.T) {
	workflowDir := filepath.Join(RepoRoot(), ".github", "workflows")
	actionDir := filepath.Join(RepoRoot(), ".github", "actions")
	if _, err := os.Stat(workflowDir); err != nil {
		t.Fatalf("read workflow directory: %v", err)
	}

	report, err := checkPinTree(workflowDir, actionDir)
	if err != nil {
		t.Fatalf("walk workflow directories: %v", err)
	}
	for _, problem := range report.problems {
		t.Error(problem)
	}
	if report.files == 0 {
		t.Fatal("no workflow files found")
	}
	if report.uses == 0 {
		t.Fatal("no action references found")
	}
	if len(report.shas) == 0 {
		t.Fatal("no OWNER/REPO@SHA action pins found: the ADR-0090 check would pass on nothing")
	}
}

// Placeholder SHAs for synthetic fixtures (core ADR-0088): not live pins.
const (
	fixtureSHA  = "0123456789abcdef0123456789abcdef01234567"
	fixtureSHA2 = "89abcdef0123456789abcdef0123456789abcdef"
)

func TestPinCheckSelfTest(t *testing.T) {
	step := func(uses string) string {
		return "jobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: " + uses + "\n"
	}
	call := func(uses string) string {
		return "jobs:\n  triage:\n    uses: " + uses + "\n    secrets: inherit\n"
	}
	const corePin = corePinPrefix + "pr-triage.yml@" + fixtureSHA
	for _, tc := range []struct {
		name  string
		files []string
		want  []string // substrings, 1 for each expected problem; none means pass
	}{
		{name: "three-part comment", files: []string{step("actions/checkout@" + fixtureSHA + " # v7.0.1")}},
		{name: "two-part comment", files: []string{step("github/issue-labeler@" + fixtureSHA + " # v3.5")}},
		{name: "quoted value", files: []string{step(`"actions/checkout@` + fixtureSHA + `" # v7.0.1`)}},
		{name: "nested action path", files: []string{step("frostyard/repogen/.github/actions/publish-to-r2@" + fixtureSHA + " # v1.2.3")}},
		{name: "local action needs no comment", files: []string{step("./.github/actions/check")}},
		{name: "core pin with comment", files: []string{call(corePin + " # core#246 (frostyard/core main, 2026-11-14; core has no release tags)")}},
		{name: "same SHA in 2 files", files: []string{
			step("actions/checkout@" + fixtureSHA + " # v7.0.1"),
			step("actions/checkout@" + fixtureSHA + " # v7.0.1"),
		}},
		{name: "missing comment", files: []string{step("actions/checkout@" + fixtureSHA)},
			want: []string{`actions/checkout has comment "", want # vX.Y.Z or # vX.Y`}},
		{name: "major-only comment", files: []string{step("actions/checkout@" + fixtureSHA + " # v7")},
			want: []string{`actions/checkout has comment "# v7"`}},
		{name: "comment with extra text", files: []string{step("actions/checkout@" + fixtureSHA + " # v7.0.1 pinned")},
			want: []string{`has comment "# v7.0.1 pinned"`}},
		{name: "branch comment on a non-core action", files: []string{step("systemd/mkosi@" + fixtureSHA + " # main 2026-11-14")},
			want: []string{`systemd/mkosi has comment "# main 2026-11-14"`}},
		{name: "core pin with no comment", files: []string{call(corePin)},
			want: []string{"pr-triage.yml has no comment"}},
		{name: "2 SHAs for 1 action in 1 file", files: []string{
			"jobs:\n  a:\n    runs-on: x\n    steps:\n      - uses: actions/checkout@" + fixtureSHA + " # v7.0.1\n" +
				"  b:\n    runs-on: x\n    steps:\n      - uses: actions/checkout@" + fixtureSHA2 + " # v7.0.2\n",
		}, want: []string{"actions/checkout has 2 SHAs across the workflows, want 1"}},
		{name: "2 SHAs for 1 action across files", files: []string{
			step("actions/checkout@" + fixtureSHA + " # v7.0.1"),
			step("actions/checkout@" + fixtureSHA2 + " # v7.0.2"),
		}, want: []string{"actions/checkout has 2 SHAs across the workflows, want 1"}},
		{name: "2 SHAs for 1 core workflow", files: []string{
			call(corePin + " # core#246"),
			call(corePinPrefix + "pr-triage.yml@" + fixtureSHA2 + " # core#250"),
		}, want: []string{"pr-triage.yml has 2 SHAs across the workflows, want 1"}},
		{name: "different actions may differ", files: []string{
			step("actions/checkout@" + fixtureSHA + " # v7.0.1"),
			step("actions/setup-go@" + fixtureSHA2 + " # v7.1.0"),
		}},
		{name: "tag ref", files: []string{step("actions/checkout@v7")},
			want: []string{`external action "actions/checkout@v7" must use a full 40-character commit SHA`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			report := &pinReport{shas: pinSHAs{}}
			for i, file := range tc.files {
				checkWorkflowFile(fmt.Sprintf("w%d.yml", i), []byte(file), report)
			}
			report.problems = append(report.problems, checkOneSHA(report.shas)...)
			if len(report.problems) != len(tc.want) {
				t.Fatalf("got %d problems %v, want %d %q", len(report.problems), report.problems, len(tc.want), tc.want)
			}
			for i, want := range tc.want {
				if !strings.Contains(report.problems[i].Error(), want) {
					t.Errorf("problem %d = %q, want it to contain %q", i, report.problems[i], want)
				}
			}
		})
	}
}

func TestPinCheckTreeSkipsMissingActionsDirectory(t *testing.T) {
	dir := t.TempDir()
	workflows := filepath.Join(dir, "workflows")
	if err := os.Mkdir(workflows, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "jobs:\n  j:\n    runs-on: x\n    steps:\n      - uses: actions/checkout@" + fixtureSHA + " # v7.0.1\n"
	if err := os.WriteFile(filepath.Join(workflows, "ci.yml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	actions := filepath.Join(dir, "actions", "setup")
	if err := os.MkdirAll(actions, 0o755); err != nil {
		t.Fatal(err)
	}
	metadata := "runs:\n  using: composite\n  steps:\n    - uses: actions/checkout@" + fixtureSHA2 + " # v7.0.2\n"
	if err := os.WriteFile(filepath.Join(actions, "action.yml"), []byte(metadata), 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := checkPinTree(workflows, filepath.Join(dir, "missing"))
	if err != nil {
		t.Fatalf("a missing directory must be skipped: %v", err)
	}
	if report.files != 1 || len(report.problems) != 0 {
		t.Fatalf("files = %d, problems = %v; want 1 file and no problem", report.files, report.problems)
	}

	report, err = checkPinTree(workflows, filepath.Join(dir, "actions"))
	if err != nil {
		t.Fatal(err)
	}
	if len(report.problems) != 1 || !strings.Contains(report.problems[0].Error(), "actions/checkout has 2 SHAs") {
		t.Fatalf("problems = %v, want the 1-SHA check to span workflows and action metadata", report.problems)
	}
}

// usesOf returns the value node of every uses: key in document that names
// action (OWNER/REPO[/PATH]), at any ref.
func usesOf(document *yaml.Node, action string) []*yaml.Node {
	var found []*yaml.Node
	var walk func(*yaml.Node)
	walk = func(node *yaml.Node) {
		if node.Kind == yaml.MappingNode {
			for i := 0; i+1 < len(node.Content); i += 2 {
				key, value := node.Content[i], node.Content[i+1]
				if key.Value == "uses" && (value.Value == action || strings.HasPrefix(value.Value, action+"@")) {
					found = append(found, value)
				}
			}
		}
		for _, child := range node.Content {
			walk(child)
		}
	}
	walk(document)
	return found
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
