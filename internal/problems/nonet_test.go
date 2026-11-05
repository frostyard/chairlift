package problems

import (
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// networkPackage reports whether an import path is network code: net,
// net/..., crypto/tls, or the vendored x/net of the standard library.
func networkPackage(path string) bool {
	return path == "net" || strings.HasPrefix(path, "net/") || path == "crypto/tls" ||
		strings.HasPrefix(path, "vendor/golang.org/x/net/") || strings.HasPrefix(path, "golang.org/x/net/")
}

// TestNoNetworkImports guards phase 1: internal/problems sends nothing, so
// no file of it, test files included, imports a network package.
func TestNoNetworkImports(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("glob: %v, %d files", err, len(files))
	}
	fset := token.NewFileSet()
	for _, name := range files {
		f, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			path, _ := strconv.Unquote(imp.Path.Value)
			if networkPackage(path) {
				t.Errorf("%s imports the network package %s", name, path)
			}
		}
	}
}

// TestNoNetworkDependencies checks the whole build graph of the package
// (std's problems package included) with go list -deps.
func TestNoNetworkDependencies(t *testing.T) {
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Skip("the go tool is not on PATH; cannot list the dependencies")
	}
	cmd := exec.Command(goTool, "list", "-deps", ".")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -deps: %v", err)
	}
	deps := strings.Fields(string(out))
	if len(deps) < 10 {
		t.Fatalf("go list -deps gave %d packages", len(deps))
	}
	for _, dep := range deps {
		if networkPackage(dep) {
			t.Errorf("internal/problems depends on the network package %s", dep)
		}
	}
	for _, p := range []string{"net", "net/http", "crypto/tls", "vendor/golang.org/x/net/dns/dnsmessage"} {
		if !networkPackage(p) {
			t.Errorf("networkPackage(%q) = false", p)
		}
	}
	if networkPackage("os/exec") || networkPackage("netlink") {
		t.Error("networkPackage matched a package that is not network code")
	}
}
