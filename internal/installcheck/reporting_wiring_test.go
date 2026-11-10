package installcheck

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// problemsSources are the files of the Problems page and its reporting
// (ADR-0015): the state, the reporting logic and the view.
var problemsSources = []string{
	filepath.Join("internal", "views", "problems.go"),
	filepath.Join("internal", "views", "problems_reporting.go"),
	filepath.Join("internal", "views", "problems_view.go"),
}

// problemsPageFuncs parses the Problems page's files and returns the source
// text of each function and method, by name. internal/views tests the
// page's behavior headlessly (problems_test.go); these guards keep the
// shape of the code that the T857 t19 cross-review asked for, so a
// refactor cannot drop it while the behavior tests happen to pass.
func problemsPageFuncs(t *testing.T) map[string]string {
	t.Helper()
	funcs := make(map[string]string)
	for _, relative := range problemsSources {
		path := filepath.Join(RepoRoot(), relative)
		source := readRepoFile(t, relative)
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, source, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			if _, dup := funcs[fn.Name.Name]; dup {
				t.Fatalf("%s is declared twice in the Problems page's files", fn.Name.Name)
			}
			start, end := fset.Position(fn.Body.Pos()).Offset, fset.Position(fn.Body.End()).Offset
			funcs[fn.Name.Name] = source[start:end]
		}
	}
	return funcs
}

// mustFunc returns the body of name, or fails.
func mustFunc(t *testing.T, funcs map[string]string, name string) string {
	t.Helper()
	body, ok := funcs[name]
	if !ok {
		t.Fatalf("the Problems page's files (%s) have no function %s", strings.Join(problemsSources, ", "), name)
	}
	return body
}

// TestReportingControlsWaitForTheFirstLoad guards N2 of the T857 t19
// cross-review: the consent controls are drawn disabled until the first load
// published the stored consent, their handlers ignore every change until
// then, and only that load sets the flag. Without it, a stale load could
// overwrite a newer user choice.
func TestReportingControlsWaitForTheFirstLoad(t *testing.T) {
	funcs := problemsPageFuncs(t)

	view := mustFunc(t, funcs, "reportsCard")
	for _, fragment := range []string{
		"Label(pageview.ReportingSwitchTitle).Disabled(!p.reportingLoaded)",
		"Label(pageview.ReportingModeTitle).Disabled(!p.reportingLoaded ||",
	} {
		if !strings.Contains(view, fragment) {
			t.Errorf("reportsCard does not contain %q", fragment)
		}
	}

	for _, handler := range []string{"onReportingSwitched", "onReportingModeSelected"} {
		body := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(mustFunc(t, funcs, handler)), "{"))
		if !strings.HasPrefix(body, "if !p.reportingLoaded ||") {
			t.Errorf("%s does not start with the load guard `if !p.reportingLoaded || ...`", handler)
		}
	}

	setters := 0
	for name, body := range funcs {
		if n := strings.Count(body, "p.reportingLoaded = true"); n > 0 {
			setters += n
			if name != "loadProblems" {
				t.Errorf("%s sets reportingLoaded; only the first load may", name)
			}
		}
	}
	if setters == 0 {
		t.Error("nothing sets reportingLoaded, so the controls never become usable")
	}
	load := mustFunc(t, funcs, "loadProblems")
	comment := strings.Index(load, "Only the first load publishes the consent")
	guard := -1
	if comment >= 0 {
		guard = strings.LastIndex(load[:comment], "if first && reportingOn {")
	}
	set := strings.Index(load, "p.reportingLoaded = true")
	if guard < 0 || set < comment {
		t.Error("loadProblems sets reportingLoaded outside its `if first && reportingOn` branch: a reload could publish a stale consent")
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
	start := strings.Index(rest, "p.runAutomaticReports(")
	gate := strings.Index(rest, "reporting.StartsAutoRun(before, record)")
	if ret < 0 || start < 0 || gate < 0 || start > ret || gate > start {
		t.Errorf("the dry-run branch of applyReportingConsent must call p.runAutomaticReports(...) under reporting.StartsAutoRun(before, record) before it returns (return %d, gate %d, call %d)", ret, gate, start)
	}
}

// TestLiveSendsUseTheFileFirstGate guards P1 of the T857 t19 cross-review:
// the live consent gate of the page is reporting.LiveConsent, which reads the
// file before the in-memory value.
func TestLiveSendsUseTheFileFirstGate(t *testing.T) {
	body := mustFunc(t, problemsPageFuncs(t), "reportingInput")
	if !strings.Contains(body, "reporting.LiveConsent(p.consentCopy, reporting.FileConsent(") {
		t.Error("reportingInput does not build its live gate with reporting.LiveConsent(p.consentCopy, reporting.FileConsent(...))")
	}
	if strings.Contains(body, "reporting.Stricter(") {
		t.Error("reportingInput calls reporting.Stricter itself; use reporting.LiveConsent, which reads the file first")
	}
}

// TestReportDialogSendsTheReportItShows guards ADR-0015's one-buffer rule
// in the page: the dialog shows Report.Text() and Send passes the same
// Report value to reporting.SendReport, after actionmsg.Report decided it.
func TestReportDialogSendsTheReportItShows(t *testing.T) {
	funcs := problemsPageFuncs(t)
	if view := mustFunc(t, funcs, "dialogView"); !strings.Contains(view, "d.report.Text()") {
		t.Error("dialogView does not show the report's Text()")
	}
	send := mustFunc(t, funcs, "sendDialogReport")
	for _, fragment := range []string{
		"report, onSent := p.dialog.report, p.dialog.onSent",
		"actionmsg.Report(p.env.DryRun, report.Kind)",
		"send(context.Background(), in, report)",
	} {
		if !strings.Contains(send, fragment) {
			t.Errorf("sendDialogReport does not contain %q", fragment)
		}
	}
	if strings.Index(send, "if !decision.Send {") > strings.Index(send, "send(context.Background(), in, report)") {
		t.Error("sendDialogReport sends before actionmsg.Report's dry-run decision")
	}
}
