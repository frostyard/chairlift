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

func gateRequireExactFilter(t *testing.T, source, command string) {
	t.Helper()
	fields := strings.Fields(strings.ReplaceAll(command, "$(GOTEST)", "go test"))
	for i := range fields {
		fields[i] = strings.Trim(fields[i], "\"'")
	}

	packageCount := 0
	runCount := 0
	skipCount := 0
	valid := false
	for i := 0; i+1 < len(fields); i++ {
		if fields[i] == "go" && fields[i+1] == "test" {
			valid = true
		}
		if fields[i] == "./internal/..." {
			packageCount++
		}
		if fields[i] == "-run" && fields[i+1] == "^Test[^I]" {
			runCount++
		}
		if fields[i] == "-skip" && fields[i+1] == "Integration" {
			skipCount++
		}
	}
	for _, field := range fields {
		if field != "./internal/..." && (strings.HasPrefix(field, "./") || strings.Contains(field, "/...")) {
			valid = false
		}
	}
	if !valid || packageCount != 1 || runCount != 1 || skipCount != 1 {
		t.Errorf("the gate filter changed: update this guard: %s: %q", source, command)
	}
}

func gateHiddenFunctionReasons(relPath, name string) []string {
	var reasons []string
	slashPath := filepath.ToSlash(relPath)
	underInternal := slashPath == "internal" || strings.HasPrefix(slashPath, "internal/")
	if !underInternal {
		reasons = append(reasons, "package is outside ./internal/...")
	}
	if strings.HasPrefix(name, "TestI") {
		reasons = append(reasons, `name begins with "TestI" and is not selected by ^Test[^I]`)
	}
	if strings.HasPrefix(name, "Test") && strings.Contains(name, "Integration") {
		reasons = append(reasons, `name contains "Integration" and is excluded by -skip "Integration"`)
	}
	if strings.HasPrefix(name, "Example") {
		reasons = append(reasons, "Example functions are not selected by -run ^Test[^I]")
	}
	if strings.HasPrefix(name, "Fuzz") {
		reasons = append(reasons, "Fuzz functions are not selected by -run ^Test[^I]")
	}
	return reasons
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
		gateRequireExactFilter(t, workflowPath, command)
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
		gateRequireExactFilter(t, makefilePath+" ci target", command)
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

			var reasons []string
			reasons = gateHiddenFunctionReasons(slashPath, name)
			if strings.HasPrefix(name, "Test") {
				underInternal := slashPath == "internal" || strings.HasPrefix(slashPath, "internal/")
				if underInternal && gateSelectableTestName.MatchString(name) && !strings.Contains(name, "Integration") {
					reasons = nil
				}
			}
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
