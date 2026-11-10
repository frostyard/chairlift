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
// frostyard's shared desktop UI kit (docs/design/frostui.md).
var frostuiAllowedImports = []string{
	"github.com/egoist/mygo",
	"github.com/ebitengine/purego",
}

// TestFrostUIImportsNoChairLift keeps internal/frostui extractable: none of
// its files, tests included, may import a ChairLift package or any module
// other than MyGo and purego.
func TestFrostUIImportsNoChairLift(t *testing.T) {
	dir := filepath.Join(RepoRoot(), "internal", "frostui")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	checked := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		checked++
		for _, imp := range file.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			if !strings.Contains(strings.SplitN(p, "/", 2)[0], ".") {
				continue // standard library
			}
			ok := false
			for _, allowed := range frostuiAllowedImports {
				ok = ok || p == allowed || strings.HasPrefix(p, allowed+"/")
			}
			if !ok {
				t.Errorf("%s imports %s; frostui may import only the standard library, MyGo and purego", e.Name(), p)
			}
		}
	}
	if checked < 5 {
		t.Fatalf("saw %d files in internal/frostui, too few", checked)
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
