package firstrun

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// repoRoot returns the repository root from this file's location.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller could not locate wiring_test.go")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func readSource(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// windowFirstRunFiles returns internal/window/firstrun*.go.
func windowFirstRunFiles(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(repoRoot(t), "internal", "window", "firstrun*.go"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no internal/window/firstrun*.go files (%v)", err)
	}
	return files
}

// calledFunctions returns, for each top-level function of the non-test files
// of a package directory, the names it calls (plain identifiers and
// pkg.Name selectors, joined as "pkg.Name").
func calledFunctions(t *testing.T, dir string) map[string]map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	calls := make(map[string]map[string]bool)
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			set := make(map[string]bool)
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				switch f := call.Fun.(type) {
				case *ast.Ident:
					set[f.Name] = true
				case *ast.SelectorExpr:
					if x, ok := f.X.(*ast.Ident); ok {
						set[x.Name+"."+f.Sel.Name] = true
					}
				}
				return true
			})
			calls[fn.Name.Name] = set
		}
	}
	return calls
}

// TestCompletionPathsShareOneMarkerWriter proves that the Done step and
// `chairlift-first-run --mark-done` reach the same MarkDone: main.go calls
// Gate, Gate calls Complete, the first-run window calls Complete, and
// Complete calls MarkDone.
func TestCompletionPathsShareOneMarkerWriter(t *testing.T) {
	root := repoRoot(t)
	main := readSource(t, filepath.Join(root, "cmd", "chairlift-first-run", "main.go"))
	for _, fragment := range []string{"firstrun.Gate(", "syscall.Exec(result.ExecPath, result.ExecArgv"} {
		if !strings.Contains(main, fragment) {
			t.Errorf("cmd/chairlift-first-run/main.go does not contain %q", fragment)
		}
	}
	if strings.Contains(main, "MarkDone(") || strings.Contains(main, "os.WriteFile") || strings.Contains(main, "os.OpenFile") {
		t.Error("cmd/chairlift-first-run/main.go writes files itself; it must only call firstrun.Gate")
	}

	calls := calledFunctions(t, filepath.Join(root, "internal", "firstrun"))
	if !calls["Gate"]["Complete"] {
		t.Error("Gate does not call Complete for --mark-done")
	}
	if !calls["Complete"]["MarkDone"] || !calls["Complete"]["DecideCompletion"] {
		t.Error("Complete does not call DecideCompletion and MarkDone")
	}
	for fn, set := range calls {
		if set["MarkDone"] && fn != "Complete" {
			t.Errorf("%s calls MarkDone directly; only Complete may", fn)
		}
	}

	var window strings.Builder
	for _, path := range windowFirstRunFiles(t) {
		window.WriteString(readSource(t, path))
	}
	for _, fragment := range []string{"firstrun.Complete(", "firstrun.IsDryRun()"} {
		if !strings.Contains(window.String(), fragment) {
			t.Errorf("internal/window/firstrun*.go does not contain %q", fragment)
		}
	}
	if strings.Contains(window.String(), "firstrun.MarkDone(") {
		t.Error("internal/window/firstrun*.go calls MarkDone directly; it must call Complete (dry-run decision)")
	}
}

// TestOnlyMarkDoneWritesFiles fails when a function of internal/firstrun
// other than MarkDone creates, writes or removes a file.
func TestOnlyMarkDoneWritesFiles(t *testing.T) {
	writers := []string{"os.WriteFile", "os.OpenFile", "os.Create", "os.Mkdir", "os.MkdirAll", "os.Remove", "os.RemoveAll", "os.Rename", "os.Symlink", "os.Link", "os.Chmod"}
	calls := calledFunctions(t, filepath.Join(repoRoot(t), "internal", "firstrun"))
	found := false
	for fn, set := range calls {
		for _, writer := range writers {
			if !set[writer] {
				continue
			}
			if fn != "MarkDone" {
				t.Errorf("%s calls %s; only MarkDone may write files", fn, writer)
			} else {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("MarkDone writes no file; the AST scan no longer matches the source")
	}
}

// TestOnlyMarkersFileNamesTheMarkers fails when a production Go file other
// than internal/firstrun/markers.go spells a marker file name, so no second
// writer of the markers can appear.
func TestOnlyMarkersFileNamesTheMarkers(t *testing.T) {
	root := repoRoot(t)
	allowed := filepath.Join(root, "internal", "firstrun", "markers.go")
	names := []string{markerName, legacyMarkerName, legacyAutostartName}
	seen := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			base := d.Name()
			if path != root && (strings.HasPrefix(base, ".") || strings.HasPrefix(base, "_") || base == "testdata" || base == "build") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		source := readSource(t, path)
		for _, name := range names {
			if strings.Contains(source, name) {
				if path == allowed {
					seen++
				} else {
					t.Errorf("%s spells the marker name %q; only internal/firstrun/markers.go may", path, name)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if seen != len(names) {
		t.Fatalf("markers.go spells %d of %d marker names", seen, len(names))
	}
}

// TestFirstRunSourcesStayUnprivilegedAndTranslated checks every first-run
// source file: no pkexec and no sudo (the first run is user-level,
// ADR-0001), and the window logs its readiness marker and translates its
// text.
func TestFirstRunSourcesStayUnprivilegedAndTranslated(t *testing.T) {
	root := repoRoot(t)
	files := append([]string{filepath.Join(root, "cmd", "chairlift-first-run", "main.go")}, windowFirstRunFiles(t)...)
	for _, dir := range []string{filepath.Join(root, "internal", "firstrun"), filepath.Join(root, "internal", "i18n")} {
		matches, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range matches {
			if !strings.HasSuffix(match, "_test.go") {
				files = append(files, match)
			}
		}
	}
	for _, path := range files {
		source := readSource(t, path)
		for _, word := range []string{"pkexec", "sudo"} {
			if strings.Contains(source, word) {
				t.Errorf("%s contains %q; the first run is user-level", path, word)
			}
		}
	}

	var window strings.Builder
	for _, path := range windowFirstRunFiles(t) {
		window.WriteString(readSource(t, path))
	}
	marker := strconv.Quote("first-run: window presented")
	if !strings.Contains(window.String(), "log.Println("+marker+")") {
		t.Errorf("internal/window/firstrun*.go does not log %s", marker)
	}
	if !strings.Contains(window.String(), "i18n.T(") {
		t.Error("internal/window/firstrun*.go does not translate through i18n.T")
	}
	if !strings.Contains(window.String(), "sgtk.RunOnMainThread(") {
		t.Error("internal/window/firstrun*.go has no sgtk.RunOnMainThread; worker results must return to the main thread")
	}
}

// TestAppSelectsFirstRunMode reads internal/app/app.go (a puregotk package
// that cannot host a test) for the first-run wiring: the separate
// application ID chosen before the GObject exists, the registered option,
// and the dry-run flag.
func TestAppSelectsFirstRunMode(t *testing.T) {
	app := readSource(t, filepath.Join(repoRoot(t), "internal", "app", "app.go"))
	for _, fragment := range []string{
		`const firstRunAppID = "org.frostyard.ChairLift.FirstRun"`,
		`const appID = "org.frostyard.ChairLift"`,
		`if arg == "--first-run" {`,
		`id = firstRunAppID`,
		`gobject.NewObject(gTypeApplication, "application_id", id,`,
		`"first-run",`,
		`firstrun.SetDryRun(true)`,
		`a.firstRunWindow = window.NewFirstRun(a.Application)`,
		`a.firstRunWindow.Present()`,
	} {
		if !strings.Contains(app, fragment) {
			t.Errorf("internal/app/app.go does not contain %q", fragment)
		}
	}
	if strings.Index(app, `if arg == "--first-run" {`) > strings.Index(app, `gobject.NewObject(gTypeApplication`) {
		t.Error("internal/app/app.go detects --first-run after it creates the application object")
	}
}
