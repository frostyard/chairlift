package installcheck

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const (
	updexModulePath = "github.com/frostyard/updex/v2"
	updexSDKPath    = updexModulePath + "/updex"
)

func TestUpdexDependencyUsesCorrectedV2Module(t *testing.T) {
	goMod := readRepoFile(t, "go.mod")
	if !strings.Contains(goMod, "\n\t"+updexModulePath+" v2.0.1\n") {
		t.Fatalf("go.mod must require %s v2.0.1", updexModulePath)
	}
	if strings.Contains(goMod, "\n\tgithub.com/frostyard/updex v") {
		t.Fatal("go.mod still requires the Updex v1 module")
	}

	imports := activeUpdexImports(t)
	wantImports := map[string]string{
		filepath.Join("cmd", "chairlift-updex-helper", "main.go"):       updexSDKPath,
		filepath.Join("internal", "updex", "updex.go"):                  updexSDKPath,
		filepath.Join("internal", "updexhelper", "updexhelper.go"):      updexSDKPath,
		filepath.Join("internal", "updexhelper", "updexhelper_test.go"): updexSDKPath,
	}
	if len(imports) != len(wantImports) {
		t.Fatalf("active Updex import count = %d, want %d: %+v", len(imports), len(wantImports), imports)
	}
	for path, want := range wantImports {
		if got := imports[path]; got != want {
			t.Errorf("%s imports %q, want %q", path, got, want)
		}
	}
}

func activeUpdexImports(t *testing.T) map[string]string {
	t.Helper()
	imports := make(map[string]string)
	fset := token.NewFileSet()

	for _, root := range []string{"cmd", "internal"} {
		root := filepath.Join(RepoRoot(), root)
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || filepath.Ext(path) != ".go" {
				return nil
			}
			file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
			if err != nil {
				return err
			}
			for _, spec := range file.Imports {
				importPath, err := strconv.Unquote(spec.Path.Value)
				if err != nil {
					return err
				}
				if strings.HasPrefix(importPath, "github.com/frostyard/updex") {
					relative, err := filepath.Rel(RepoRoot(), path)
					if err != nil {
						return err
					}
					imports[relative] = importPath
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("scan %s imports: %v", root, err)
		}
	}

	return imports
}
