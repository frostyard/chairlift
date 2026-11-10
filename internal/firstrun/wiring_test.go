package firstrun

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
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

// windowFirstRunFiles returns the production files
// internal/window/firstrun*.go, without their tests.
func windowFirstRunFiles(t *testing.T) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(repoRoot(t), "internal", "window", "firstrun*.go"))
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, match := range matches {
		if !strings.HasSuffix(match, "_test.go") {
			files = append(files, match)
		}
	}
	if len(files) == 0 {
		t.Fatal("no internal/window/firstrun*.go files")
	}
	return files
}

// windowFunc returns the source of the function or method name in
// internal/window/firstrun*.go.
func windowFunc(t *testing.T, name string) string {
	t.Helper()
	fset := token.NewFileSet()
	for _, path := range windowFirstRunFiles(t) {
		source := readSource(t, path)
		file, err := parser.ParseFile(fset, path, source, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == name && fn.Body != nil {
				return source[fset.Position(fn.Pos()).Offset:fset.Position(fn.End()).Offset]
			}
		}
	}
	t.Fatalf("internal/window/firstrun*.go has no function %s", name)
	return ""
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
// Gate, Gate calls Complete, the first-run window calls CompleteAt,
// CompleteAt calls Complete, and Complete calls MarkDone.
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
	if !calls["CompleteAt"]["Complete"] {
		t.Error("CompleteAt does not call Complete")
	}
	if !calls["RunProgress"]["Execute"] || !calls["RunProgress"]["DoneState"] {
		t.Error("RunProgress does not call Execute and DoneState")
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
	for _, fragment := range []string{"firstrun.CompleteAt(", "firstrun.IsDryRun()", "firstrun.RunProgress(", "firstrun.GateStateAt(paths, pathsErr)", "done.Warning", "done.Entries"} {
		if !strings.Contains(window.String(), fragment) {
			t.Errorf("internal/window/firstrun*.go does not contain %q", fragment)
		}
	}
	if strings.Contains(window.String(), "firstrun.MarkDone(") || strings.Contains(window.String(), "firstrun.Complete(") {
		t.Error("internal/window/firstrun*.go calls MarkDone or Complete directly; it must call CompleteAt, which also carries the path error")
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
	// Worker results return to the UI thread through the window's post
	// (MyGo's Window.Update): both workers hand their results back with it.
	for _, fn := range []string{"startConnectivityCheck", "startProgress"} {
		body := windowFunc(t, fn)
		if !strings.Contains(body, "go func() {") || !strings.Contains(body, "post(func() {") {
			t.Errorf("%s does not hand its worker's result back through post", fn)
		}
	}
	for _, toolkit := range []string{"puregotk", "snowkit", "sgtk."} {
		if strings.Contains(window.String(), toolkit) {
			t.Errorf("internal/window/firstrun*.go still names %q", toolkit)
		}
	}
}

// TestAppSelectsFirstRunMode reads internal/app/app.go for the first-run
// wiring: the separate application ID chosen from the command line before
// the single-instance lock and before the app runs, the --help listing,
// the dry-run flag, and the first-run window with its close guard and
// readiness marker.
func TestAppSelectsFirstRunMode(t *testing.T) {
	root := repoRoot(t)
	app := readSource(t, filepath.Join(root, "internal", "app", "app.go"))
	for _, fragment := range []string{
		`const FirstRunAppID = "org.frostyard.ChairLift.FirstRun"`,
		`const AppID = "org.frostyard.ChairLift"`,
		"case FlagFirstRun:",
		"o.FirstRun = true",
		"return FirstRunAppID",
		`frostui.SetAppID(o.ID(), "ChairLift")`,
		"mygo.App.SetName(o.ID())",
		"firstrun.SetDryRun(true)",
		"fr := window.NewFirstRun(p.post)",
		"win.OnClose(func(e *mygo.CloseEvent) {",
		"if !fr.CanClose() {",
		"e.PreventDefault()",
		"fr.Presented()",
	} {
		if !strings.Contains(app, fragment) {
			t.Errorf("internal/app/app.go does not contain %q", fragment)
		}
	}
	if !regexp.MustCompile(`FlagFirstRun\s+= "--first-run"`).MatchString(app) {
		t.Error(`internal/app/app.go does not define FlagFirstRun as "--first-run"`)
	}
	setName := strings.Index(app, "mygo.App.SetName(o.ID())")
	lock := strings.Index(app, "mygo.App.RequestSingleInstanceLock()")
	run := strings.Index(app, "return mygo.App.Run()")
	if setName < 0 || lock < setName || run < lock {
		t.Error("internal/app/app.go does not choose the application ID before the single-instance lock and the run")
	}
	newWindow := strings.Index(app, "fr := window.NewFirstRun(p.post)")
	guard := strings.Index(app, "if !fr.CanClose() {")
	presented := strings.Index(app, "fr.Presented()")
	if newWindow < 0 || guard < newWindow || presented < guard {
		t.Error("internal/app/app.go does not guard the first-run window's close before it logs the window presented")
	}

	main := readSource(t, filepath.Join(root, "cmd", "chairlift", "main.go"))
	if !strings.Contains(main, "--first-run") {
		t.Error("cmd/chairlift/main.go does not list --first-run in its --help")
	}
}

// windowFirstRunSource returns internal/window/firstrun*.go joined.
func windowFirstRunSource(t *testing.T) string {
	t.Helper()
	var window strings.Builder
	for _, path := range windowFirstRunFiles(t) {
		window.WriteString(readSource(t, path))
	}
	return window.String()
}

// TestWindowGuardsCloseDuringProgress proves the first-run window keeps
// itself open while the Progress actions run: CanClose is Flow.CanClose,
// which the app's close handler vetoes on (mygo.App.Quit and SIGTERM close
// through it too); the flow is on Progress before the worker starts, and
// FinishProgress runs only in finishProgress, which the worker posts after
// RunProgress returned.
func TestWindowGuardsCloseDuringProgress(t *testing.T) {
	if body := windowFunc(t, "CanClose"); !strings.Contains(body, "return w.flow.CanClose()") {
		t.Errorf("FirstRun.CanClose is not Flow.CanClose:\n%s", body)
	}
	next := windowFunc(t, "goNext")
	move := strings.Index(next, "w.flow.Next()")
	start := strings.Index(next, "w.startProgress()")
	if move < 0 || start < move {
		t.Error("goNext starts Progress before the flow is on it, so the window could close while the worker starts")
	}
	progress := windowFunc(t, "startProgress")
	worker := strings.Index(progress, "go func() {")
	run := strings.Index(progress, "firstrun.RunProgress(actions, steps)")
	finish := strings.Index(progress, "post(func() { w.finishProgress(done) })")
	if worker < 0 || run < worker || finish < run {
		t.Error("startProgress does not post finishProgress after RunProgress returns on its worker")
	}
	if body := windowFunc(t, "finishProgress"); !strings.Contains(body, "w.flow.FinishProgress()") {
		t.Error("finishProgress does not end the Progress step")
	}
	if n := strings.Count(windowFirstRunSource(t), "FinishProgress()"); n != 1 {
		t.Errorf("internal/window/firstrun*.go calls FinishProgress %d times; only finishProgress may", n)
	}
}

// TestWindowShowsConfigErrorToast proves a configuration error stays on
// screen as a persistent plain-text toast (AGENTS.md, ADR-0003), as the main
// window does: MyGo toasts are plain text, and a negative timeout keeps it.
func TestWindowShowsConfigErrorToast(t *testing.T) {
	window := windowFirstRunSource(t)
	for _, fragment := range []string{
		"cfg, cfgErr := config.Load()",
		"selfreport.Error(selfreport.CodeConfigLoadFailed, cfgErr)",
		"if cfgErr := w.cfgErr; cfgErr != nil {",
		`ui.Toast{ID: "config-error", Title: cfgErr.ToastMessage(), Type: "error", Timeout: -1}`,
	} {
		if !strings.Contains(window, fragment) {
			t.Errorf("internal/window/firstrun*.go does not contain %q", fragment)
		}
	}
	if !regexp.MustCompile(`cfgErr:\s+cfgErr,`).MatchString(windowFunc(t, "NewFirstRun")) {
		t.Error("NewFirstRun does not hand the configuration error to the window")
	}
	if !strings.Contains(windowFunc(t, "View"), "c.AddToast(toast)") {
		t.Error("View does not show the queued toasts")
	}
}

// TestWindowWritesColorSchemeSerially proves the Style step goes through the
// one SchemeWriter, so writes cannot land out of order.
func TestWindowWritesColorSchemeSerially(t *testing.T) {
	window := windowFirstRunSource(t)
	for _, fragment := range []string{
		"firstrun.NewSchemeWriter(firstrun.ApplyColorScheme)",
		"w.scheme.Request(scheme)",
	} {
		if !strings.Contains(window, fragment) {
			t.Errorf("internal/window/firstrun*.go does not contain %q", fragment)
		}
	}
	if strings.Contains(window, "firstrun.ApplyColorScheme(scheme)") {
		t.Error("internal/window/firstrun*.go calls ApplyColorScheme directly; it must use the SchemeWriter")
	}
}

// TestWindowDrainsSchemeWriterBeforeCompletion proves the Progress worker
// goes through RunProgress with the window's SchemeWriter as its drain, so
// the last style choice is written before the completion is recorded and
// the window may close. It also fails when the window runs the actions or
// records the completion outside RunProgress.
func TestWindowDrainsSchemeWriterBeforeCompletion(t *testing.T) {
	progress := windowFunc(t, "startProgress")
	steps := strings.Index(progress, "steps := firstrun.ProgressSteps{")
	drain := strings.Index(progress, "Drain: w.scheme.Wait,")
	complete := strings.Index(progress, "return firstrun.CompleteAt(dryRun, paths, pathsErr)")
	run := strings.Index(progress, "firstrun.RunProgress(actions, steps)")
	if steps < 0 || drain < steps || complete < steps || run < drain || run < complete {
		t.Fatalf("the Progress worker does not pass Drain: w.scheme.Wait and CompleteAt to firstrun.RunProgress (steps %d, drain %d, complete %d, run %d)", steps, drain, complete, run)
	}
	window := windowFirstRunSource(t)
	if strings.Count(window, "firstrun.CompleteAt(") != 1 || strings.Contains(window, "firstrun.Execute(") {
		t.Error("internal/window/firstrun*.go records completion or runs actions outside RunProgress")
	}
	if strings.Count(window, "firstrun.RunProgress(") != 1 {
		t.Error("internal/window/firstrun*.go runs RunProgress more than once")
	}
}
