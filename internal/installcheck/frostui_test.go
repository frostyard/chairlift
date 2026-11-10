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

// frostuiAllowedImports are the only module paths internal/frostui may
// import besides the standard library: it is written to leave ChairLift as
// frostyard's shared desktop UI kit (docs/design/frostui.md). Its own
// packages move with it.
var frostuiAllowedImports = []string{
	"github.com/egoist/mygo",
	"github.com/ebitengine/purego",
	"github.com/frostyard/chairlift/internal/frostui",
}

// TestFrostUIImportsNoChairLift keeps internal/frostui extractable: none of
// its files, in any of its packages and tests included, may import a
// ChairLift package outside the kit or any module other than MyGo and
// purego. Its test helpers (frostuitest) import the standard library only,
// so an app's tests can use them whatever UI they draw.
func TestFrostUIImportsNoChairLift(t *testing.T) {
	dir := filepath.Join(RepoRoot(), "internal", "frostui")
	fset := token.NewFileSet()
	checked, helpers := 0, 0
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		checked++
		stdlibOnly := strings.HasPrefix(filepath.ToSlash(rel), "frostuitest/")
		if stdlibOnly {
			helpers++
		}
		for _, imp := range file.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			if !strings.Contains(strings.SplitN(p, "/", 2)[0], ".") {
				continue // standard library
			}
			if stdlibOnly {
				t.Errorf("%s imports %s; frostuitest may import only the standard library", rel, p)
				continue
			}
			ok := false
			for _, allowed := range frostuiAllowedImports {
				ok = ok || p == allowed || strings.HasPrefix(p, allowed+"/")
			}
			if !ok {
				t.Errorf("%s imports %s; frostui may import only the standard library, MyGo and purego", rel, p)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked < 5 || helpers == 0 {
		t.Fatalf("saw %d files in internal/frostui (%d in frostuitest), too few", checked, helpers)
	}
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
