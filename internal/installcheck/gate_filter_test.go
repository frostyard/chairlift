package installcheck

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var gateSelectableTestName = regexp.MustCompile(`^Test[^I]`)

func gateWorkflowTestCommands(text string) []string {
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

func gateMakefileCITestCommands(text string) []string {
	lines := strings.Split(text, "\n")
	inCI := false
	var commands []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(line, "\t") && !strings.HasPrefix(line, " ") && trimmed != "" {
			if strings.HasPrefix(trimmed, "ci:") {
				inCI = true
				continue
			}
			if inCI {
				inCI = false
			}
		}
		if inCI && strings.HasPrefix(line, "\t") && (strings.Contains(line, "$(GOTEST)") || strings.Contains(trimmed, "go test")) {
			commands = append(commands, strings.TrimSpace(strings.TrimPrefix(trimmed, "@")))
		}
	}
	return commands
}

func gateValidateExactFilter(command string) error {
	fields := strings.Fields(strings.ReplaceAll(command, "$(GOTEST)", "go test"))
	for i := range fields {
		fields[i] = strings.Trim(fields[i], "\"'")
	}

	validGoTest := false
	packageCount := 0
	runCount := 0
	skipCount := 0
	for i := 0; i < len(fields); i++ {
		field := fields[i]
		if i+1 < len(fields) && field == "go" && fields[i+1] == "test" {
			validGoTest = true
		}

		if field == "./internal/..." {
			packageCount++
		}
		if field == "./..." || field == "../..." || strings.HasPrefix(field, "./") || strings.HasPrefix(field, "../") {
			if field != "./internal/..." {
				return fmt.Errorf("the gate filter changed: update this guard: extra or wrong package pattern %q", field)
			}
		}

		if field == "-run" {
			runCount++
			if runCount != 1 {
				return fmt.Errorf("the gate filter changed: update this guard: duplicate -run or -test.run")
			}
			if i+1 >= len(fields) || fields[i+1] != "^Test[^I]" {
				return fmt.Errorf("the gate filter changed: update this guard: -run must be ^Test[^I]")
			}
			i++
			continue
		}
		if strings.HasPrefix(field, "-run=") || field == "--run" || strings.HasPrefix(field, "--run=") || field == "-test.run" || strings.HasPrefix(field, "-test.run=") {
			return fmt.Errorf("the gate filter changed: update this guard: rejected run filter form %q", field)
		}

		if field == "-skip" {
			skipCount++
			if skipCount != 1 {
				return fmt.Errorf("the gate filter changed: update this guard: duplicate -skip or -test.skip")
			}
			if i+1 >= len(fields) || fields[i+1] != "Integration" {
				return fmt.Errorf("the gate filter changed: update this guard: -skip must be Integration")
			}
			i++
			continue
		}
		if strings.HasPrefix(field, "-skip=") || field == "--skip" || strings.HasPrefix(field, "--skip=") || field == "-test.skip" || strings.HasPrefix(field, "-test.skip=") {
			return fmt.Errorf("the gate filter changed: update this guard: rejected skip filter form %q", field)
		}
	}

	if !validGoTest {
		return fmt.Errorf("the gate filter changed: update this guard: no go test command")
	}
	if packageCount != 1 {
		return fmt.Errorf("the gate filter changed: update this guard: package pattern must occur exactly once as ./internal/...")
	}
	if runCount != 1 {
		return fmt.Errorf("the gate filter changed: update this guard: -run must occur exactly once")
	}
	if skipCount != 1 {
		return fmt.Errorf("the gate filter changed: update this guard: -skip must occur exactly once")
	}
	return nil
}

func gateHiddenFunctionReasons(relPath, name string) []string {
	var reasons []string
	slashPath := filepath.ToSlash(relPath)
	underInternal := slashPath == "internal" || strings.HasPrefix(slashPath, "internal/")
	if !underInternal {
		reasons = append(reasons, "package is outside ./internal/...")
	}

	if strings.HasPrefix(name, "Test") {
		if !gateSelectableTestName.MatchString(name) {
			if name == "Test" {
				reasons = append(reasons, "name does not match ^Test[^I]")
			} else {
				reasons = append(reasons, `name does not match ^Test[^I]`)
			}
		}
		if strings.Contains(name, "Integration") {
			reasons = append(reasons, `name contains "Integration" and is excluded by -skip "Integration"`)
		}
	}
	if strings.HasPrefix(name, "Example") {
		reasons = append(reasons, "Example functions are not selected by -run ^Test[^I]")
	}
	if strings.HasPrefix(name, "Fuzz") {
		reasons = append(reasons, "Fuzz functions are not selected by -run ^Test[^I]")
	}
	return reasons
}

func TestGateValidateExactFilter(t *testing.T) {
	for _, tc := range []struct {
		name    string
		command string
		wantErr bool
	}{
		{name: "workflow unit command", command: `go test -v ./internal/... -coverprofile=coverage.out -covermode=atomic -run "^Test[^I]" -skip "Integration"`},
		{name: "makefile ci race command", command: `CGO_ENABLED=1 $(GOTEST) -race -short ./internal/... -run "^Test[^I]" -skip "Integration"`},
		{name: "duplicate skip", command: `go test ./internal/... -run "^Test[^I]" -skip "Integration" -skip "Goreleaser"`, wantErr: true},
		{name: "duplicate run", command: `go test ./internal/... -run "^Test[^I]" -run "^TestFoo" -skip "Integration"`, wantErr: true},
		{name: "skip equals", command: `go test ./internal/... -run "^Test[^I]" -skip=Goreleaser`, wantErr: true},
		{name: "test skip pair", command: `go test ./internal/... -run "^Test[^I]" -test.skip Goreleaser`, wantErr: true},
		{name: "test skip equals", command: `go test ./internal/... -run "^Test[^I]" -test.skip=Goreleaser`, wantErr: true},
		{name: "run wrong value", command: `go test ./internal/... -run "^TestFoo" -skip "Integration"`, wantErr: true},
		{name: "missing skip", command: `go test ./internal/... -run "^Test[^I]"`, wantErr: true},
		{name: "wrong package", command: `go test ./... -run "^Test[^I]" -skip "Integration"`, wantErr: true},
		{name: "extra package", command: `go test ./internal/... ./cmd/... -run "^Test[^I]" -skip "Integration"`, wantErr: true},
		{name: "run equals", command: `go test ./internal/... -run=^TestFoo -skip "Integration"`, wantErr: true},
		{name: "long run", command: `go test ./internal/... --run "^Test[^I]" -skip "Integration"`, wantErr: true},
		{name: "test run pair", command: `go test ./internal/... -test.run "^TestFoo" -skip "Integration"`, wantErr: true},
		{name: "test run equals", command: `go test ./internal/... -test.run=^TestFoo -skip "Integration"`, wantErr: true},
		{name: "long skip", command: `go test ./internal/... -run "^Test[^I]" --skip "Integration"`, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := gateValidateExactFilter(tc.command)
			if tc.wantErr && err == nil {
				t.Errorf("gateValidateExactFilter(%q) = nil, want error", tc.command)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("gateValidateExactFilter(%q) = %v, want nil", tc.command, err)
			}
			if tc.wantErr && err != nil && !strings.Contains(err.Error(), "the gate filter changed: update this guard") {
				t.Errorf("gateValidateExactFilter(%q) error = %q, want guard-change wording", tc.command, err)
			}
		})
	}
}

func TestGateHiddenFunctionReasons(t *testing.T) {
	for _, tc := range []struct {
		name       string
		relPath    string
		function   string
		wantHidden bool
		wantReason string
	}{
		{name: "exact Test hidden", relPath: "internal/x/test_test.go", function: "Test", wantHidden: true, wantReason: "name does not match ^Test[^I]"},
		{name: "TestI hidden", relPath: "internal/x/test_test.go", function: "TestIsValid", wantHidden: true, wantReason: "name does not match ^Test[^I]"},
		{name: "Integration hidden", relPath: "internal/x/test_test.go", function: "TestFooIntegration", wantHidden: true, wantReason: `name contains "Integration"`},
		{name: "outside internal hidden", relPath: "cmd/x/test_test.go", function: "TestFoo", wantHidden: true, wantReason: "package is outside ./internal/..."},
		{name: "Example hidden", relPath: "internal/x/test_test.go", function: "ExampleFoo", wantHidden: true, wantReason: "Example functions are not selected"},
		{name: "Fuzz hidden", relPath: "internal/x/test_test.go", function: "FuzzFoo", wantHidden: true, wantReason: "Fuzz functions are not selected"},
		{name: "normal internal Test selected", relPath: "internal/x/test_test.go", function: "TestFoo", wantHidden: false},
		{name: "TestMain internal selected", relPath: "internal/x/test_test.go", function: "TestMain", wantHidden: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reasons := gateHiddenFunctionReasons(tc.relPath, tc.function)
			if tc.wantHidden && len(reasons) == 0 {
				t.Fatalf("gateHiddenFunctionReasons(%q, %q) = nil, want hidden", tc.relPath, tc.function)
			}
			if !tc.wantHidden && len(reasons) != 0 {
				t.Fatalf("gateHiddenFunctionReasons(%q, %q) = %v, want selected", tc.relPath, tc.function, reasons)
			}
			if tc.wantReason != "" {
				joined := strings.Join(reasons, "; ")
				if !strings.Contains(joined, tc.wantReason) {
					t.Errorf("gateHiddenFunctionReasons(%q, %q) = %v, want reason containing %q", tc.relPath, tc.function, reasons, tc.wantReason)
				}
			}
		})
	}
}

func TestGateFilterHidesNoHermeticTest(t *testing.T) {
	workflowPath := filepath.Join(".github", "workflows", "test.yml")
	workflowData, err := os.ReadFile(filepath.Join(RepoRoot(), workflowPath))
	if err != nil {
		t.Fatalf("read %s: %v", workflowPath, err)
	}
	workflowCommands := gateWorkflowTestCommands(string(workflowData))
	if len(workflowCommands) < 2 {
		t.Errorf("the gate filter changed: update this guard: %s has %d go test commands, want at least 2", workflowPath, len(workflowCommands))
	}
	for _, command := range workflowCommands {
		if err := gateValidateExactFilter(command); err != nil {
			t.Errorf("%s: %v: %q", workflowPath, err, command)
		}
	}

	makefilePath := "Makefile"
	makefileData, err := os.ReadFile(filepath.Join(RepoRoot(), makefilePath))
	if err != nil {
		t.Fatalf("read %s: %v", makefilePath, err)
	}
	makefileCommands := gateMakefileCITestCommands(string(makefileData))
	if len(makefileCommands) < 2 {
		t.Errorf("the gate filter changed: update this guard: %s ci target has %d go test commands, want at least 2", makefilePath, len(makefileCommands))
	}
	for _, command := range makefileCommands {
		if err := gateValidateExactFilter(command); err != nil {
			t.Errorf("%s ci target: %v: %q", makefilePath, err, command)
		}
	}

	checked := 0
	exemptE2E := 0
	hidden := 0
	var hiddenFunctions []string

	err = filepath.WalkDir(RepoRoot(), func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || !strings.HasSuffix(path, "_test.go") {
			return nil
		}

		relPath, err := filepath.Rel(RepoRoot(), path)
		if err != nil {
			return err
		}
		slashPath := filepath.ToSlash(relPath)
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return fmt.Errorf("parse %s: %w", slashPath, err)
		}
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv != nil {
				continue
			}
			name := function.Name.Name
			if !strings.HasPrefix(name, "Test") && !strings.HasPrefix(name, "Example") && !strings.HasPrefix(name, "Fuzz") {
				continue
			}

			checked++
			if slashPath == "test/e2e" || strings.HasPrefix(slashPath, "test/e2e/") {
				exemptE2E++
				continue
			}

			reasons := gateHiddenFunctionReasons(slashPath, name)
			// -skip "Integration" is matched against top-level test names, not t.Run subtest names.
			if len(reasons) > 0 {
				hidden++
				hiddenFunctions = append(hiddenFunctions, fmt.Sprintf("%s: %s (%s)", slashPath, name, strings.Join(reasons, "; ")))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk test functions: %v", err)
	}

	if checked == 0 {
		t.Fatal("gate filter guard checked 0 test functions")
	}
	if hidden > 0 {
		t.Errorf("hidden by the chairlift gate filter:\n- %s", strings.Join(hiddenFunctions, "\n- "))
	}
	t.Logf("gate filter counts: functions checked=%d, exempt e2e=%d, hidden=%d", checked, exemptE2E, hidden)
}
