package config

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// directCallAllowlist is the complete, frozen set of unexported
// package-level functions that _test.go files in this package may name
// directly (by identifier, not merely by exercising them through an
// exported entry point). It is deliberately a permitted set rather than a
// list of forbidden name patterns: any unexported production helper, no
// matter what it is called, fails TestSourceHelperDirectCallSurface the
// moment a _test.go file names it unless it is listed here.
//
//   - parseYAMLDocument and validateSourceGraph are the two package
//     capabilities this multi-chunk slice adds and tests directly.
//     validateSourceGraph does not exist yet as of this chunk; listing it
//     now costs nothing and lets a later chunk add its direct test without
//     touching this frozen allowlist.
//   - isMergeKey and shortYAMLTag are the only two internal helpers the
//     spec singles out for direct testing (exact merge-key recognition and
//     YAML tag normalization). Like validateSourceGraph, they do not exist
//     yet as of this chunk.
//   - defaultConfig, loadFromPath, schemaPageGroups, yamlFieldNames and
//     yamlTagName are a frozen pre-existing baseline: they were already
//     named directly by config_test.go/schema_test.go before this chunk
//     and are not widened or added to here.
//   - resolveEffective and effectiveKeyIdentity are the exactly two
//     additions this slice's spec authorizes, added in this chunk before
//     either function exists so no later chunk needs to touch this frozen
//     guard again. resolveEffective does not exist yet as of this chunk;
//     effectiveKeyIdentity does, and effectivekeys_test.go names it
//     directly. Every other new resolver helper this slice adds in a later
//     chunk must be exercised only indirectly, through resolveEffective —
//     it must not be added to this allowlist.
var directCallAllowlist = map[string]bool{
	"parseYAMLDocument":    true,
	"validateSourceGraph":  true,
	"isMergeKey":           true,
	"shortYAMLTag":         true,
	"defaultConfig":        true,
	"loadFromPath":         true,
	"schemaPageGroups":     true,
	"yamlFieldNames":       true,
	"yamlTagName":          true,
	"resolveEffective":     true,
	"effectiveKeyIdentity": true,
}

// packageGoFiles lists the *.go files directly in this package's directory
// (the working directory `go test` runs in), split by whether they are
// _test.go files.
func packageGoFiles(t *testing.T, testFiles bool) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading package directory: %v", err)
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		if strings.HasSuffix(name, "_test.go") != testFiles {
			continue
		}
		files = append(files, name)
	}
	sort.Strings(files)
	return files
}

// TestSourceHelperDirectCallSurface is a frozen guard, in force from this
// chunk onward: it computes the complete set of unexported package-level
// functions declared in this package's non-_test.go files (production
// helpers, with or without a receiver), the complete set of identifiers
// named anywhere in this package's _test.go files, and fails - naming the
// offenders - if any name is in both sets but not in directCallAllowlist.
//
// Declarations in _test.go files (test fixtures - graph builders, line
// extractors, and the like) are intentionally out of scope: step 1 below
// only ever parses non-_test.go files, so a test-fixture helper can never
// enter the production set and never trips this guard, no matter what it
// is called or how many _test.go files reference it.
func TestSourceHelperDirectCallSurface(t *testing.T) {
	fset := token.NewFileSet()

	production := map[string]bool{}
	for _, name := range packageGoFiles(t, false) {
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if !fn.Name.IsExported() {
				production[fn.Name.Name] = true
			}
		}
	}

	referenced := map[string]bool{}
	for _, name := range packageGoFiles(t, true) {
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok {
				referenced[id.Name] = true
			}
			return true
		})
	}

	var offenders []string
	for name := range production {
		if referenced[name] && !directCallAllowlist[name] {
			offenders = append(offenders, name)
		}
	}
	sort.Strings(offenders)
	if len(offenders) > 0 {
		t.Fatalf("test file(s) directly name unexported production helper(s) not in directCallAllowlist: %v", offenders)
	}
}

// TestSourceConfigExportedSurface freezes this package's exported
// package-level surface: the exact set of exported funcs, methods, types
// and values declared in non-_test.go files. Adding any exported
// identifier in this chunk or a later one fails this test, since the plan
// wires no new capability into any exported API.
func TestSourceConfigExportedSurface(t *testing.T) {
	fset := token.NewFileSet()

	funcs := map[string]bool{}
	methods := map[string]bool{}
	types := map[string]bool{}
	values := map[string]bool{}

	for _, name := range packageGoFiles(t, false) {
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if !d.Name.IsExported() {
					continue
				}
				if d.Recv != nil {
					receiver := ""
					switch typ := d.Recv.List[0].Type.(type) {
					case *ast.Ident:
						receiver = typ.Name
					case *ast.StarExpr:
						if id, ok := typ.X.(*ast.Ident); ok {
							receiver = id.Name
						}
					}
					if receiver == "" {
						t.Fatalf("unsupported exported method receiver in %s: %T", name, d.Recv.List[0].Type)
					}
					methods[receiver+"."+d.Name.Name] = true
				} else {
					funcs[d.Name.Name] = true
				}
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					switch s := spec.(type) {
					case *ast.TypeSpec:
						if s.Name.IsExported() {
							types[s.Name.Name] = true
						}
					case *ast.ValueSpec:
						for _, id := range s.Names {
							if id.IsExported() {
								values[id.Name] = true
							}
						}
					}
				}
			}
		}
	}

	assertExactNameSet(t, "exported funcs", funcs,
		[]string{"Load", "SchemaActionFields", "SchemaGroupFields", "SchemaGroups", "SchemaPages"})
	assertExactNameSet(t, "exported methods", methods,
		[]string{"Config.GetGroupConfig", "Config.IsGroupEnabled", "LoadError.Error", "LoadError.Unwrap"})
	assertExactNameSet(t, "exported types", types,
		[]string{"ActionConfig", "Config", "ErrorKind", "GroupConfig", "LoadError", "PageConfig"})
	assertExactNameSet(t, "exported values", values,
		[]string{"KindParseType", "KindRead", "KindSchema"})
}

// assertExactNameSet fails the test, naming both sides, unless got's keys
// are exactly want (order-independent).
func assertExactNameSet(t *testing.T, label string, got map[string]bool, want []string) {
	t.Helper()
	gotSlice := make([]string, 0, len(got))
	for name := range got {
		gotSlice = append(gotSlice, name)
	}
	sort.Strings(gotSlice)

	wantSlice := append([]string(nil), want...)
	sort.Strings(wantSlice)

	if !reflect.DeepEqual(gotSlice, wantSlice) {
		t.Fatalf("%s: got %v, want %v", label, gotSlice, wantSlice)
	}
}
