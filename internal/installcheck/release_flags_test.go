package installcheck

import (
	"bytes"
	"go/ast"
	"go/build"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The window ships as MyGo's production build (AGENTS.md, "Build, test,
// lint"): the mygo_noinspector tag compiles the inspector and the
// automation hooks out, and -X github.com/egoist/mygo.production=1 turns
// off MyGo's development mode. The automation plugin needs its own tag and
// never ships.
const (
	mygoModule         = "github.com/egoist/mygo"
	mygoNoInspectorTag = "mygo_noinspector"
	mygoProductionX    = mygoModule + ".production=1"
	mygoAutomationTag  = "automation"
	mygoAutomationPkg  = mygoModule + "/plugins/automation"
	chairliftMain      = "./cmd/chairlift"
)

// mygoUpdaterAPI is MyGo's self-updater (go doc github.com/egoist/mygo:
// Updater, UpdaterModule, Update, UpdateSource, ErrUpdatesDisabled).
// ChairLift is updated by its packages, never by MyGo's update feed.
var mygoUpdaterAPI = []string{"Updater", "UpdaterModule", "Update", "UpdateSource", "ErrUpdatesDisabled"}

// mygoUpdateFeedVars are the variables `mygo build` sets with -X to turn
// the update feed on (updater.go). No ChairLift build may set them.
var mygoUpdateFeedVars = []string{"packageUpdateFeed", "packageUpdateKey"}

// releaseBuild is the subset of a .goreleaser.yaml builds[] entry that
// decides how a binary is compiled.
type releaseBuild struct {
	ID      string   `yaml:"id"`
	Main    string   `yaml:"main"`
	Flags   []string `yaml:"flags"`
	Tags    []string `yaml:"tags"`
	Ldflags []string `yaml:"ldflags"`
}

// buildTags returns the build tags a builds[] entry passes, from -tags flags
// in either spelling and from its tags list.
func (b releaseBuild) buildTags() []string {
	tags := slices.Clone(b.Tags)
	for i, flag := range b.Flags {
		name, value, hasValue := strings.Cut(strings.TrimLeft(flag, "-"), "=")
		if name != "tags" {
			continue
		}
		if !hasValue && i+1 < len(b.Flags) {
			value = b.Flags[i+1]
		}
		tags = append(tags, strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ' ' })...)
	}
	return tags
}

// ldflagsXValues returns the importpath.name=value operand of every -X in
// a linker flag string.
func ldflagsXValues(ldflags string) []string {
	var values []string
	fields := strings.Fields(ldflags)
	for i, f := range fields {
		switch {
		case (f == "-X" || f == "--X") && i+1 < len(fields):
			values = append(values, fields[i+1])
		case strings.HasPrefix(f, "-X="):
			values = append(values, strings.TrimPrefix(f, "-X="))
		}
	}
	return values
}

// checkMyGoXValues reports every -X that sets a MyGo variable other than
// production=1, such as the update feed.
func checkMyGoXValues(t *testing.T, where string, values []string) {
	t.Helper()
	for _, v := range values {
		if strings.HasPrefix(v, mygoModule+".") && v != mygoProductionX {
			t.Errorf("%s sets -X %s; the only MyGo variable a build may set is %s", where, v, mygoProductionX)
		}
	}
}

// TestGoreleaserBuildsMyGoProduction pins the release build of the window
// in .goreleaser.yaml: every build of ./cmd/chairlift passes
// -tags=mygo_noinspector and -X github.com/egoist/mygo.production=1, and no
// build carries the automation tag or sets another MyGo variable.
func TestGoreleaserBuildsMyGoProduction(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(RepoRoot(), ".goreleaser.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Builds []releaseBuild `yaml:"builds"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parsing .goreleaser.yaml: %v", err)
	}
	windows := 0
	for i, b := range cfg.Builds {
		where := "builds[" + strconv.Itoa(i) + "] (" + b.ID + ")"
		if slices.Contains(b.buildTags(), mygoAutomationTag) {
			t.Errorf("%s builds with the %s tag; release builds never carry the automation plugin", where, mygoAutomationTag)
		}
		x := ldflagsXValues(strings.Join(b.Ldflags, " "))
		checkMyGoXValues(t, where, x)
		if strings.TrimSuffix(path.Clean(b.Main), "/main.go") != path.Clean(chairliftMain) {
			continue
		}
		windows++
		if !slices.Contains(b.Flags, "-tags="+mygoNoInspectorTag) {
			t.Errorf("%s flags = %q, want -tags=%s", where, b.Flags, mygoNoInspectorTag)
		}
		if !slices.Contains(x, mygoProductionX) {
			t.Errorf("%s ldflags = %q, want -X %s", where, b.Ldflags, mygoProductionX)
		}
	}
	t.Logf("%d builds, %d of %s", len(cfg.Builds), windows, chairliftMain)
	if windows == 0 {
		t.Fatalf("no build in .goreleaser.yaml has main %s", chairliftMain)
	}
}

// makeAssignment matches a Makefile variable assignment in any flavor.
var makeAssignment = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\s*(?:[:?+!]|::)?=\s*(.*?)\s*$`)

// makeRecipe returns the prerequisites and recipe lines of target in a
// Makefile's source, after joining backslash-continued lines.
func makeRecipe(src, target string) (prereqs string, recipe []string, found bool) {
	lines := strings.Split(strings.ReplaceAll(src, "\\\n", " "), "\n")
	for i, line := range lines {
		rest, ok := strings.CutPrefix(line, target+":")
		if !ok || strings.HasPrefix(rest, "=") {
			continue
		}
		for _, r := range lines[i+1:] {
			if !strings.HasPrefix(r, "\t") {
				break
			}
			recipe = append(recipe, strings.TrimSpace(r))
		}
		return rest, recipe, true
	}
	return "", nil, false
}

// TestMakefileBuildsMyGoProduction pins the same production build in the
// Makefile: APP_TAGS and APP_LDFLAGS hold the tag and the -X, and
// build-app and both cross-compile targets use them, as make expands them.
// make build builds build-app, never the automation build.
func TestMakefileBuildsMyGoProduction(t *testing.T) {
	src := readRepoFile(t, "Makefile")
	want := map[string]string{
		"APP_TAGS":    mygoNoInspectorTag,
		"APP_LDFLAGS": "-X " + mygoProductionX,
	}
	assigned := map[string][]string{}
	for line := range strings.SplitSeq(src, "\n") {
		if m := makeAssignment.FindStringSubmatch(line); m != nil {
			assigned[m[1]] = append(assigned[m[1]], m[2])
		}
	}
	for name, value := range want {
		if got := assigned[name]; !slices.Equal(got, []string{value}) {
			t.Errorf("Makefile assigns %s %q, want exactly once %q", name, got, value)
		}
	}
	checkMyGoXValues(t, "Makefile", ldflagsXValues(strings.Join(assigned["APP_LDFLAGS"], " ")))
	for _, m := range regexp.MustCompile(regexp.QuoteMeta(mygoModule)+`\.\S+`).FindAllString(src, -1) {
		if m != mygoProductionX {
			t.Errorf("Makefile names %s; the only MyGo variable it may set is %s", m, mygoProductionX)
		}
	}

	targets := []string{"build-app", "build-linux-amd64", "build-linux-arm64"}
	for _, target := range targets {
		_, recipe, found := makeRecipe(src, target)
		if !found {
			t.Errorf("Makefile has no %s target", target)
			continue
		}
		var builds []string
		for _, line := range recipe {
			if strings.HasSuffix(line, " "+chairliftMain) {
				builds = append(builds, line)
			}
		}
		if len(builds) != 1 {
			t.Errorf("%s has %d recipe lines that build %s, want 1: %q", target, len(builds), chairliftMain, recipe)
			continue
		}
		for _, use := range []string{"-tags $(APP_TAGS)", `-ldflags "$(APP_LDFLAGS)"`} {
			if !strings.Contains(builds[0], use) {
				t.Errorf("%s builds %s without %s: %q", target, chairliftMain, use, builds[0])
			}
		}
		if strings.Contains(builds[0], mygoAutomationTag) {
			t.Errorf("%s builds with the automation plugin: %q", target, builds[0])
		}
	}
	prereqs, _, found := makeRecipe(src, "build")
	if fields := strings.Fields(prereqs); !found || !slices.Contains(fields, "build-app") || slices.Contains(fields, "build-automation") {
		t.Errorf("make build depends on %q, want build-app and not build-automation", fields)
	}

	t.Run("as make expands it", func(t *testing.T) {
		if _, err := exec.LookPath("make"); err != nil {
			t.Skip("GNU make is not installed")
		}
		cmd := exec.Command("make", append([]string{"-n"}, targets...)...)
		cmd.Dir = RepoRoot()
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("make -n %s: %v\n%s", strings.Join(targets, " "), err, out)
		}
		builds := 0
		for line := range strings.SplitSeq(string(out), "\n") {
			if !strings.HasSuffix(strings.TrimSpace(line), " "+chairliftMain) {
				continue
			}
			builds++
			for _, flag := range []string{"-tags " + mygoNoInspectorTag, `-ldflags "-X ` + mygoProductionX + `"`} {
				if !strings.Contains(line, flag) {
					t.Errorf("make runs %q without %s", line, flag)
				}
			}
		}
		t.Logf("make -n runs %d builds of %s", builds, chairliftMain)
		if builds != len(targets) {
			t.Errorf("make -n %s ran %d builds of %s, want %d:\n%s", strings.Join(targets, " "), builds, chairliftMain, len(targets), out)
		}
	})
}

// buildConstraint returns the //go:build expression of a Go file, or nil.
func buildConstraint(src []byte) (constraint.Expr, error) {
	for line := range strings.SplitSeq(string(src), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "package ") {
			break
		}
		if constraint.IsGoBuild(line) {
			return constraint.Parse(line)
		}
	}
	return nil, nil
}

// releaseContext is the build context of the release: linux/amd64, pure
// Go, with the given tags.
func releaseContext(tags ...string) build.Context {
	ctx := build.Default
	ctx.GOOS, ctx.GOARCH, ctx.CgoEnabled = "linux", "amd64", false
	ctx.BuildTags = tags
	return ctx
}

// TestAutomationPluginNeedsItsTag keeps MyGo's automation plugin out of
// every build without -tags automation: cmd/chairlift/automation.go keeps
// its `//go:build automation` line, and every non-test file under cmd/ and
// internal/ that imports the plugin is compiled only with that tag.
func TestAutomationPluginNeedsItsTag(t *testing.T) {
	root := RepoRoot()
	src, err := os.ReadFile(filepath.Join(root, "cmd", "chairlift", "automation.go"))
	if err != nil {
		t.Fatal(err)
	}
	expr, err := buildConstraint(src)
	if err != nil || expr == nil || expr.String() != mygoAutomationTag {
		t.Errorf("cmd/chairlift/automation.go has build constraint %v (%v), want //go:build %s", expr, err, mygoAutomationTag)
	}

	release, automation := releaseContext(mygoNoInspectorTag), releaseContext(mygoAutomationTag)
	fset := token.NewFileSet()
	importers := 0
	for _, top := range []string{"cmd", "internal"} {
		for _, p := range goFilesUnder(t, filepath.Join(root, top)) {
			if strings.HasSuffix(p, "_test.go") {
				continue
			}
			file, err := parser.ParseFile(fset, p, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatalf("parse %s: %v", p, err)
			}
			rel, _ := filepath.Rel(root, p)
			for _, imp := range file.Imports {
				if ip, _ := strconv.Unquote(imp.Path.Value); ip != mygoAutomationPkg {
					continue
				}
				importers++
				inRelease, err := release.MatchFile(filepath.Dir(p), filepath.Base(p))
				if err != nil {
					t.Fatal(err)
				}
				inAutomation, err := automation.MatchFile(filepath.Dir(p), filepath.Base(p))
				if err != nil {
					t.Fatal(err)
				}
				if inRelease || !inAutomation {
					t.Errorf("%s imports %s and is built without -tags %s (release build: %v, automation build: %v)", rel, mygoAutomationPkg, mygoAutomationTag, inRelease, inAutomation)
				}
			}
		}
	}
	t.Logf("%d files import %s", importers, mygoAutomationPkg)
	if importers == 0 {
		t.Fatalf("no file imports %s; the automation build lost its plugin", mygoAutomationPkg)
	}
}

// TestMyGoProductionSwitchExists guards against a silent no-op after a
// MyGo bump: the linker ignores -X for a variable that does not exist, and
// the go tool ignores a tag no file mentions. In the MyGo module go.mod
// selects, the package github.com/egoist/mygo must still declare
// `var production string`, its ui package must still compile the
// inspector out under mygo_noinspector, and the updater API this package
// keeps out of ChairLift must still have the names it looks for. The
// release build of ./cmd/chairlift must link github.com/egoist/mygo (or
// the -X sets nothing) and not the automation plugin.
func TestMyGoProductionSwitchExists(t *testing.T) {
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Skip("the go tool is not on PATH; cannot locate the MyGo module")
	}
	cmd := exec.Command(goTool, "list", "-m", "-f", "{{.Dir}}", mygoModule)
	cmd.Dir = RepoRoot()
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go list -m %s: %v\n%s", mygoModule, err, stderr.String())
	}
	dir := strings.TrimSpace(stdout.String())
	if dir == "" {
		t.Fatalf("%s is not in the module cache (run go mod download)", mygoModule)
	}

	release := releaseContext(mygoNoInspectorTag)
	declared, production := map[string]bool{}, false
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if ok, err := release.MatchFile(dir, name); err != nil || !ok {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		if file.Name.Name != "mygo" {
			continue
		}
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil {
					declared[d.Name.Name] = true
				}
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					switch s := spec.(type) {
					case *ast.TypeSpec:
						declared[s.Name.Name] = true
					case *ast.ValueSpec:
						for _, id := range s.Names {
							declared[id.Name] = true
							if id.Name == "production" && d.Tok == token.VAR && isStringType(s) {
								production = true
							}
						}
					}
				}
			}
		}
	}
	if !production {
		t.Errorf("%s (%s) declares no package-level `var production string`: -X %s would set nothing", mygoModule, dir, mygoProductionX)
	}
	for _, name := range append(slices.Clone(mygoUpdaterAPI), mygoUpdateFeedVars...) {
		if !declared[name] {
			t.Errorf("%s no longer declares %s: reread its updater and update mygoUpdaterAPI and mygoUpdateFeedVars", mygoModule, name)
		}
	}

	uiDir := filepath.Join(dir, "ui")
	uiEntries, err := os.ReadDir(uiDir)
	if err != nil {
		t.Fatal(err)
	}
	dev := releaseContext()
	dropped, added := 0, 0
	for _, e := range uiEntries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		inRelease, err := release.MatchFile(uiDir, name)
		if err != nil {
			t.Fatal(err)
		}
		inDev, err := dev.MatchFile(uiDir, name)
		if err != nil {
			t.Fatal(err)
		}
		switch {
		case inDev && !inRelease:
			dropped++
		case inRelease && !inDev:
			added++
		}
	}
	t.Logf("%s/ui: -tags %s drops %d files (the inspector) and adds %d", mygoModule, mygoNoInspectorTag, dropped, added)
	if dropped == 0 || added == 0 {
		t.Errorf("-tags %s no longer changes %s/ui (drops %d files, adds %d): the release build may carry the inspector", mygoNoInspectorTag, mygoModule, dropped, added)
	}

	list := exec.Command(goTool, "list", "-deps", "-tags", mygoNoInspectorTag, chairliftMain)
	list.Dir = RepoRoot()
	list.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH=amd64")
	stdout.Reset()
	stderr.Reset()
	list.Stdout, list.Stderr = &stdout, &stderr
	if err := list.Run(); err != nil {
		t.Fatalf("go list -deps %s: %v\n%s", chairliftMain, err, stderr.String())
	}
	deps := strings.Fields(stdout.String())
	if !slices.Contains(deps, mygoModule) {
		t.Errorf("the release build of %s does not link %s, so -X %s sets nothing", chairliftMain, mygoModule, mygoProductionX)
	}
	if slices.Contains(deps, mygoAutomationPkg) {
		t.Errorf("the release build of %s links %s", chairliftMain, mygoAutomationPkg)
	}
}

// isStringType reports whether a var spec declares a string, the only kind
// of variable -X can set.
func isStringType(s *ast.ValueSpec) bool {
	if id, ok := s.Type.(*ast.Ident); ok {
		return id.Name == "string"
	}
	if s.Type == nil && len(s.Values) == 1 {
		lit, ok := s.Values[0].(*ast.BasicLit)
		return ok && lit.Kind == token.STRING
	}
	return false
}

// TestNoMyGoUpdater keeps MyGo's self-updater out of ChairLift, which its
// packages update: no non-test Go file under cmd/ or internal/ names
// mygo.Updater or the rest of mygoUpdaterAPI through whatever name it
// imports github.com/egoist/mygo as. The ldflags checks above keep the feed
// variables unset.
func TestNoMyGoUpdater(t *testing.T) {
	root := RepoRoot()
	fset := token.NewFileSet()
	importers := 0
	for _, top := range []string{"cmd", "internal"} {
		for _, p := range goFilesUnder(t, filepath.Join(root, top)) {
			if strings.HasSuffix(p, "_test.go") {
				continue
			}
			file, err := parser.ParseFile(fset, p, nil, parser.SkipObjectResolution)
			if err != nil {
				t.Fatalf("parse %s: %v", p, err)
			}
			name := ""
			for _, imp := range file.Imports {
				if ip, _ := strconv.Unquote(imp.Path.Value); ip == mygoModule {
					name = "mygo"
					if imp.Name != nil {
						name = imp.Name.Name
					}
				}
			}
			if name == "" {
				continue
			}
			importers++
			rel, _ := filepath.Rel(root, p)
			if name == "." {
				t.Errorf("%s dot-imports %s, so its use of the updater cannot be checked", rel, mygoModule)
				continue
			}
			ast.Inspect(file, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if id, ok := sel.X.(*ast.Ident); ok && id.Name == name && slices.Contains(mygoUpdaterAPI, sel.Sel.Name) {
					t.Errorf("%s:%d uses %s.%s; ChairLift is updated by its packages, not MyGo's update feed", rel, fset.Position(sel.Pos()).Line, name, sel.Sel.Name)
				}
				return true
			})
		}
	}
	t.Logf("%d non-test files import %s", importers, mygoModule)
	if importers == 0 {
		t.Fatalf("no file imports %s; the scan is not working", mygoModule)
	}
}
