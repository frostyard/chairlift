package installcheck

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// problemsPageFuncs parses internal/views/problems_page.go and returns the
// source text of each method of UserHome, by name. The view imports puregotk,
// so it cannot host a test (ADR-0007): these guards read its source.
func problemsPageFuncs(t *testing.T) map[string]string {
	t.Helper()
	path := filepath.Join(RepoRoot(), "internal", "views", "problems_page.go")
	source := readRepoFile(t, filepath.Join("internal", "views", "problems_page.go"))
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, source, 0)
	if err != nil {
		t.Fatal(err)
	}
	funcs := make(map[string]string)
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		start, end := fset.Position(fn.Body.Pos()).Offset, fset.Position(fn.Body.End()).Offset
		funcs[fn.Name.Name] = source[start:end]
	}
	return funcs
}

// mustFunc returns the body of name, or fails.
func mustFunc(t *testing.T, funcs map[string]string, name string) string {
	t.Helper()
	body, ok := funcs[name]
	if !ok {
		t.Fatalf("internal/views/problems_page.go has no function %s", name)
	}
	return body
}

// TestReportingControlsWaitForTheFirstLoad guards N2 of the T857 t19
// cross-review: the consent controls are built insensitive, their handlers
// ignore every change until the first load published the stored consent, and
// only that load sets the flag. Without it, a stale load could overwrite a
// newer user choice.
func TestReportingControlsWaitForTheFirstLoad(t *testing.T) {
	funcs := problemsPageFuncs(t)

	build := mustFunc(t, funcs, "buildReportingGroup")
	for _, fragment := range []string{
		"uh.reportingSwitch.SetSensitive(false)",
		"uh.reportingMode.SetSensitive(false)",
	} {
		if !strings.Contains(build, fragment) {
			t.Errorf("buildReportingGroup does not contain %q", fragment)
		}
	}

	for _, handler := range []string{"onReportingSwitched", "onReportingModeSelected"} {
		body := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(mustFunc(t, funcs, handler)), "{"))
		if !strings.HasPrefix(body, "if !uh.reportingLoaded ||") {
			t.Errorf("%s does not start with the load guard `if !uh.reportingLoaded || ...`", handler)
		}
	}

	setters := 0
	for name, body := range funcs {
		if n := strings.Count(body, "uh.reportingLoaded = true"); n > 0 {
			setters += n
			if name != "loadProblems" && name != "loadReportingSettings" {
				t.Errorf("%s sets reportingLoaded; only the first load may", name)
			}
		}
	}
	if setters == 0 {
		t.Error("nothing sets reportingLoaded, so the controls never become usable")
	}
}

// TestDryRunAutoChoiceStartsThePreviewRun guards N4 of the T857 t19
// cross-review: in a dry run, a new automatic choice starts the automatic run
// (which only previews), before the dry-run branch returns.
func TestDryRunAutoChoiceStartsThePreviewRun(t *testing.T) {
	body := mustFunc(t, problemsPageFuncs(t), "applyReportingConsent")
	branch := strings.Index(body, "if !decision.Persist {")
	if branch < 0 {
		t.Fatal("applyReportingConsent has no dry-run branch `if !decision.Persist {`")
	}
	rest := body[branch:]
	ret := strings.Index(rest, "return")
	start := strings.Index(rest, "uh.runAutomaticReports()")
	gate := strings.Index(rest, "reporting.StartsAutoRun(before, record)")
	if ret < 0 || start < 0 || gate < 0 || start > ret || gate > start {
		t.Errorf("the dry-run branch of applyReportingConsent must call uh.runAutomaticReports() under reporting.StartsAutoRun(before, record) before it returns (return %d, gate %d, call %d)", ret, gate, start)
	}
}

// TestLiveSendsUseTheFileFirstGate guards P1 of the T857 t19 cross-review:
// the live consent gate of the page is reporting.LiveConsent, which reads the
// file before the in-memory value.
func TestLiveSendsUseTheFileFirstGate(t *testing.T) {
	body := mustFunc(t, problemsPageFuncs(t), "reportingInput")
	if !strings.Contains(body, "reporting.LiveConsent(uh.consentCopy, reporting.FileConsent(") {
		t.Error("reportingInput does not build its live gate with reporting.LiveConsent(uh.consentCopy, reporting.FileConsent(...))")
	}
	if strings.Contains(body, "reporting.Stricter(") {
		t.Error("reportingInput calls reporting.Stricter itself; use reporting.LiveConsent, which reads the file first")
	}
}
