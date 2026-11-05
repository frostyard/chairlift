package installcheck

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// reportingPackage is the only package that may send a report (ADR-0015).
const reportingPackage = "internal/reporting"

// goFilesUnder returns every .go file under dir, recursively, skipping the
// folders that the go tool's ./... pattern skips and the build folder.
func goFilesUnder(t *testing.T, dir string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			base := d.Name()
			if path != dir && (strings.HasPrefix(base, ".") || strings.HasPrefix(base, "_") ||
				base == "testdata" || base == "build" || base == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// TestOnlyReportingImportsNetHTTP proves that internal/reporting is the only
// package whose production code imports net/http: one sender per host, and
// no other page or package can reach the network.
func TestOnlyReportingImportsNetHTTP(t *testing.T) {
	root := RepoRoot()
	fset := token.NewFileSet()
	checked := 0
	for _, path := range goFilesUnder(t, root) {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		if strings.HasPrefix(path, filepath.Join(root, "test")+string(filepath.Separator)) {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		checked++
		relative := strings.TrimPrefix(strings.TrimPrefix(path, root), string(filepath.Separator))
		for _, imp := range file.Imports {
			imported, _ := strconv.Unquote(imp.Path.Value)
			if imported != "net/http" {
				continue
			}
			if !strings.HasPrefix(relative, filepath.Join(reportingPackage)+string(filepath.Separator)) {
				t.Errorf("%s imports net/http; only %s may send a report", relative, reportingPackage)
			}
		}
	}
	if checked < 50 {
		t.Fatalf("the scan saw %d files, which is too few to be the repository", checked)
	}
}

// TestOnlyReportingNamesTheReportHost proves that the intake host name appears
// in non-test Go code only in internal/reporting, so no other package can
// send a report to it.
func TestOnlyReportingNamesTheReportHost(t *testing.T) {
	root := RepoRoot()
	const host = "report.frostyard.org"
	found := 0
	for _, path := range goFilesUnder(t, root) {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), host) {
			continue
		}
		relative := strings.TrimPrefix(strings.TrimPrefix(path, root), string(filepath.Separator))
		if !strings.HasPrefix(relative, filepath.Join(reportingPackage)+string(filepath.Separator)) {
			t.Errorf("%s names %s; only %s may", relative, host, reportingPackage)
			continue
		}
		found++
	}
	if found == 0 {
		t.Fatalf("no file of %s names %s, so the scan no longer matches the source", reportingPackage, host)
	}
}

// TestProblemsStaysFreeOfTheSender proves internal/problems never imports
// the sender: the page's data package stays network-free (ADR-0015).
func TestProblemsStaysFreeOfTheSender(t *testing.T) {
	root := RepoRoot()
	problemsDir := filepath.Join(root, "internal", "problems")
	fset := token.NewFileSet()
	checked := 0
	for _, path := range goFilesUnder(t, problemsDir) {
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		checked++
		for _, imp := range file.Imports {
			imported, _ := strconv.Unquote(imp.Path.Value)
			if imported == "github.com/frostyard/chairlift/"+filepath.ToSlash(reportingPackage) {
				t.Errorf("%s imports %s; internal/problems must not", path, reportingPackage)
			}
		}
	}
	if checked < 5 {
		t.Fatalf("the scan saw %d files of internal/problems, which is too few", checked)
	}
}

// TestSendPendingReportsModeIsWindowless proves the windowless mode runs
// before the application object exists, and that the option is listed, so
// `chairlift --help` shows it.
func TestSendPendingReportsModeIsWindowless(t *testing.T) {
	source := readRepoFile(t, filepath.Join("cmd", "chairlift", "main.go"))
	call := strings.Index(source, "os.Exit(sendPendingReports())")
	application := strings.Index(source, "application := app.New()")
	if call < 0 || application < 0 {
		t.Fatalf("cmd/chairlift/main.go does not hold the mode and the application (call %d, application %d)", call, application)
	}
	if call > application {
		t.Error("cmd/chairlift/main.go creates the application before the windowless mode runs")
	}
	if !strings.Contains(source, "reporting.SendPending(") {
		t.Error("cmd/chairlift/main.go does not call reporting.SendPending")
	}
	if strings.Contains(source, "gtk.") || strings.Contains(source, "adw.") {
		t.Error("cmd/chairlift/main.go builds GTK widgets itself")
	}

	app := readRepoFile(t, filepath.Join("internal", "app", "app.go"))
	for _, fragment := range []string{
		`"send-pending-reports",`,
		`"Send the problem reports that your consent allows, then exit; no window."`,
	} {
		if !strings.Contains(app, fragment) {
			t.Errorf("internal/app/app.go does not contain %q", fragment)
		}
	}
}
