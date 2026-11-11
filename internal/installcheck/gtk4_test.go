package installcheck

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

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
