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

func TestWorkflowUsesLeastPrivilege(t *testing.T) {
	path := filepath.Join(".github", "workflows", "test.yml")
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

// workflowGoTestCommands extracts go test commands from workflow run: lines,
// including commands in run: | blocks.
func workflowGoTestCommands(text string) []string {
	lines := strings.Split(text, "\n")
	var commands []string
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "run:") {
			continue
		}
		runIndent := len(line) - len(strings.TrimLeft(line, " \t"))
		remainder := strings.TrimSpace(strings.TrimPrefix(trimmed, "run:"))
		if remainder != "" && remainder != "|" && !strings.HasPrefix(remainder, "|") && !strings.HasPrefix(remainder, ">") {
			if strings.Contains(remainder, "go test") {
				commands = append(commands, remainder)
			}
			continue
		}
		for j := i + 1; j < len(lines); j++ {
			blockLine := lines[j]
			if strings.TrimSpace(blockLine) == "" {
				continue
			}
			blockIndent := len(blockLine) - len(strings.TrimLeft(blockLine, " \t"))
			if blockIndent <= runIndent {
				break
			}
			if strings.Contains(strings.TrimSpace(blockLine), "go test") {
				commands = append(commands, strings.TrimSpace(blockLine))
			}
		}
	}
	return commands
}

// makefileGoTestCommands finds go test commands and their Make target. It
// treats the ci target and the rest of the Makefile separately for validation.
func makefileGoTestCommands(text string) []struct {
	Target  string
	Command string
} {
	lines := strings.Split(text, "\n")
	target := ""
	var commands []struct {
		Target  string
		Command string
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "\t") && !strings.HasPrefix(line, " ") {
			if colon := strings.Index(line, ":"); colon >= 0 {
				candidate := strings.TrimSpace(line[:colon])
				if candidate != "" && !strings.ContainsAny(candidate, "= \t") {
					target = candidate
				}
			}
		}
		if strings.HasPrefix(line, "\t") && (strings.Contains(line, "$(GOTEST)") || strings.Contains(trimmed, "go test")) {
			commands = append(commands, struct {
				Target  string
				Command string
			}{Target: target, Command: strings.TrimSpace(strings.TrimPrefix(trimmed, "@"))})
		}
	}
	return commands
}

func validateUnitTestGoCommand(t *testing.T, source, target, command string) {
	t.Helper()
	fields := strings.Fields(strings.ReplaceAll(command, "$(GOTEST)", "go test"))
	for _, field := range fields {
		field = strings.Trim(field, "\"'")
		if field == "-run" || strings.HasPrefix(field, "-run=") || field == "-skip" || strings.HasPrefix(field, "-skip=") {
			t.Errorf("%s: go test command contains test filtering in %q", source, command)
		}
	}

	packagePatterns := 0
	skipNext := false
	goTestIndex := -1
	for i := 0; i+1 < len(fields); i++ {
		if fields[i] == "go" && fields[i+1] == "test" {
			goTestIndex = i
			break
		}
	}
	if goTestIndex < 0 {
		t.Errorf("%s: no go test command found in %q", source, command)
		return
	}
	for i := goTestIndex + 2; i < len(fields); i++ {
		field := fields[i]
		field = strings.Trim(field, "\"'")
		if skipNext {
			skipNext = false
			continue
		}
		if field == "-run" || field == "-skip" || field == "-coverprofile" || field == "-covermode" || field == "-coverpkg" || field == "-timeout" || field == "-count" {
			skipNext = true
			continue
		}
		if strings.HasPrefix(field, "-") {
			continue
		}
		packagePatterns++
		allowed := field == "./..."
		if target == "e2e" {
			allowed = allowed || field == "./test/e2e"
		}
		if !allowed {
			t.Errorf("%s: go test command uses package pattern %q; want ./... (e2e target may use ./test/e2e): %q", source, field, command)
		}
	}
	if packagePatterns == 0 {
		t.Errorf("%s: go test command has no package pattern; want ./... (e2e target may use ./test/e2e): %q", source, command)
	}
}

func TestUnitTestGatesRunEveryTest(t *testing.T) {
	root := RepoRoot()
	if override := os.Getenv("CHAIRLIFT_TEST_REPO_ROOT"); override != "" {
		root = override
	}

	workflowPath := filepath.Join(".github", "workflows", "test.yml")
	workflowData, err := os.ReadFile(filepath.Join(root, workflowPath))
	if err != nil {
		t.Fatalf("read %s: %v", workflowPath, err)
	}
	workflow := string(workflowData)
	workflowCommands := workflowGoTestCommands(workflow)
	if len(workflowCommands) < 2 {
		t.Errorf("%s: found %d go test commands, want at least 2", workflowPath, len(workflowCommands))
	}
	for _, command := range workflowCommands {
		validateUnitTestGoCommand(t, workflowPath, "", command)
	}

	makefilePath := "Makefile"
	makefileData, err := os.ReadFile(filepath.Join(root, makefilePath))
	if err != nil {
		t.Fatalf("read %s: %v", makefilePath, err)
	}
	makefile := string(makefileData)
	makefileCommands := makefileGoTestCommands(makefile)
	ciCount := 0
	for _, command := range makefileCommands {
		if command.Target == "ci" {
			ciCount++
		}
		validateUnitTestGoCommand(t, makefilePath+"["+command.Target+"]", command.Target, command.Command)
	}
	if ciCount < 2 {
		t.Errorf("%s ci target: found %d go test commands, want at least 2", makefilePath, ciCount)
	}

	t.Logf("unit-test gate command counts: test.yml=%d, Makefile ci=%d, Makefile all=%d", len(workflowCommands), ciCount, len(makefileCommands))
}
