package installcheck

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// frostuiAllowedImports are the only module paths internal/frostui may
// import besides the standard library: it is written to leave ChairLift as
// frostyard's shared desktop UI kit (docs/design/frostui.md). Its own
// packages move with it.
var frostuiAllowedImports = []string{
	"github.com/egoist/mygo",
	"github.com/ebitengine/purego",
	"github.com/frostyard/chairlift/internal/frostui",
}

// frostuiImportProblems checks one file of internal/frostui, named by its
// slash-separated path relative to that directory, against the kit's import
// rule: the standard library, MyGo, purego and the kit's own packages, and
// the standard library alone in its test helpers (frostuitest/). It returns
// the number of imports outside the standard library and one line per
// import that breaks the rule.
func frostuiImportProblems(rel string, src []byte) (external int, problems []string, err error) {
	file, err := parser.ParseFile(token.NewFileSet(), rel, src, parser.ImportsOnly)
	if err != nil {
		return 0, nil, err
	}
	stdlibOnly := strings.HasPrefix(rel, "frostuitest/")
	for _, imp := range file.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			return 0, nil, err
		}
		if !strings.Contains(strings.SplitN(p, "/", 2)[0], ".") {
			continue // standard library
		}
		external++
		if stdlibOnly {
			problems = append(problems, fmt.Sprintf("%s imports %s; frostuitest may import only the standard library", rel, p))
			continue
		}
		ok := false
		for _, allowed := range frostuiAllowedImports {
			ok = ok || p == allowed || strings.HasPrefix(p, allowed+"/")
		}
		if !ok {
			problems = append(problems, fmt.Sprintf("%s imports %s; frostui may import only the standard library, MyGo and purego", rel, p))
		}
	}
	return external, problems, nil
}

// TestFrostUIImportsNoChairLift keeps internal/frostui extractable: none of
// its files, in any of its packages and tests included, may import a
// ChairLift package outside the kit or any module other than MyGo and
// purego. Its test helpers (frostuitest) import the standard library only,
// so an app's tests can use them whatever UI they draw.
func TestFrostUIImportsNoChairLift(t *testing.T) {
	dir := filepath.Join(RepoRoot(), "internal", "frostui")
	checked, helpers, external := 0, 0, 0
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		rel = filepath.ToSlash(rel)
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		n, problems, err := frostuiImportProblems(rel, src)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		checked++
		external += n
		if strings.HasPrefix(rel, "frostuitest/") {
			helpers++
		}
		for _, problem := range problems {
			t.Error(problem)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d files in internal/frostui (%d in frostuitest) import %d packages outside the standard library", checked, helpers, external)
	if checked < 5 || helpers == 0 || external == 0 {
		t.Fatalf("saw %d files in internal/frostui (%d in frostuitest, %d imports outside the standard library), too few", checked, helpers, external)
	}
}

// TestFrostUIImportFixtures proves frostuiImportProblems can fail: each
// fixture that breaks the kit's import rule must be reported, and the one
// that keeps it must pass.
func TestFrostUIImportFixtures(t *testing.T) {
	fixtures := []struct {
		name, rel string
		imports   []string
		fail      bool
	}{
		{"a ChairLift import", "shell.go", []string{"fmt", "github.com/frostyard/chairlift/internal/config"}, true},
		{"a third-party module", "theme.go", []string{"gopkg.in/yaml.v3"}, true},
		{"a module that only shares MyGo's prefix", "viz.go", []string{"github.com/egoist/mygoextra"}, true},
		{"frostuitest importing MyGo", "frostuitest/loop.go", []string{"sync", "github.com/egoist/mygo/ui"}, true},
		{"frostuitest importing the kit", "frostuitest/loop.go", []string{"github.com/frostyard/chairlift/internal/frostui"}, true},
		{"the standard library, MyGo, purego and the kit", "jobs.go", []string{
			"context", "os/exec", "github.com/egoist/mygo", "github.com/egoist/mygo/ui",
			"github.com/ebitengine/purego", "github.com/frostyard/chairlift/internal/frostui/frostuitest",
		}, false},
	}
	reported := 0
	for _, fx := range fixtures {
		t.Run(fx.name, func(t *testing.T) {
			var src strings.Builder
			src.WriteString("package frostui\n\nimport (\n")
			for _, imp := range fx.imports {
				fmt.Fprintf(&src, "\t%q\n", imp)
			}
			src.WriteString(")\n")
			external, problems, err := frostuiImportProblems(fx.rel, []byte(src.String()))
			if err != nil {
				t.Fatalf("parse: %v\n%s", err, src.String())
			}
			switch {
			case fx.fail && len(problems) == 0:
				t.Errorf("%s imports %q, which breaks the rule, but was not reported", fx.rel, fx.imports)
			case !fx.fail && len(problems) > 0:
				t.Errorf("the allowed imports were reported: %q", problems)
			}
			if fx.fail {
				reported += len(problems)
			}
			t.Logf("external=%d problems=%q", external, problems)
		})
	}
	t.Logf("%d fixtures, %d problems reported", len(fixtures), reported)
}

// TestNoGTK4Bindings checks that the GTK 4 bindings are gone: MyGo runs on
// GTK 3, and GTK 3 and GTK 4 cannot share a process.
func TestNoGTK4Bindings(t *testing.T) {
	root := RepoRoot()
	fset := token.NewFileSet()
	for _, top := range []string{"cmd", "internal", "test"} {
		for _, path := range goFilesUnder(t, filepath.Join(root, top)) {
			file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			for _, imp := range file.Imports {
				p, _ := strconv.Unquote(imp.Path.Value)
				if strings.HasPrefix(p, "codeberg.org/puregotk/") || strings.HasPrefix(p, "github.com/frostyard/snowkit") {
					rel, _ := filepath.Rel(root, path)
					t.Errorf("%s imports %s", rel, p)
				}
			}
		}
	}
	gomod := readRepoFile(t, "go.mod")
	for _, mod := range []string{"codeberg.org/puregotk/puregotk", "github.com/frostyard/snowkit"} {
		if strings.Contains(gomod, mod) {
			t.Errorf("go.mod still requires %s", mod)
		}
	}
}
