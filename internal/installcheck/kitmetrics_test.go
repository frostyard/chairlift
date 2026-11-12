package installcheck

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The views draw in the kit's metrics and text styles (frostui ADR-0011,
// docs/design/frostui.md): a literal corner radius misses the brand's
// nearly square corners, a literal font size misses the desktop's text
// size (frostui.TextSize), and the brand separates with hairlines, never
// shadows.

// kitMetricProblems parses one Go file and reports, one line each, every
// .Radius or .FontSize call and every FontSize field (a
// frostui.KeyValuesStyle) whose value is a number literal, and every
// .Shadow call.
func kitMetricProblems(name string, src []byte) ([]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, name, src, 0)
	if err != nil {
		return nil, err
	}
	var problems []string
	report := func(at token.Pos, format string, args ...any) {
		problems = append(problems, fmt.Sprintf("%s: %s", fset.Position(at), fmt.Sprintf(format, args...)))
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.CallExpr:
			sel, ok := n.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			switch sel.Sel.Name {
			case "Shadow":
				report(n.Pos(), "a shadow: the brand separates with a hairline (Border)")
			case "Radius", "FontSize":
				for _, arg := range n.Args {
					if isNumberLiteral(arg) {
						report(arg.Pos(), "%s with a number: use the kit's metric or text style (frostui.RadiusCard, RadiusChip, the theme's Radius, frostui.TextSize)", sel.Sel.Name)
					}
				}
			}
		case *ast.KeyValueExpr:
			if key, ok := n.Key.(*ast.Ident); ok && key.Name == "FontSize" && isNumberLiteral(n.Value) {
				report(n.Value.Pos(), "FontSize field with a number: scale it with frostui.TextSize")
			}
		}
		return true
	})
	return problems, nil
}

// isNumberLiteral reports whether e is a number literal, signed or in
// parentheses.
func isNumberLiteral(e ast.Expr) bool {
	switch e := e.(type) {
	case *ast.BasicLit:
		return e.Kind == token.INT || e.Kind == token.FLOAT
	case *ast.UnaryExpr:
		return (e.Op == token.SUB || e.Op == token.ADD) && isNumberLiteral(e.X)
	case *ast.ParenExpr:
		return isNumberLiteral(e.X)
	}
	return false
}

// TestViewsDrawInKitMetrics scans every non-test Go file under internal/
// for a literal radius, a literal font size and a shadow.
func TestViewsDrawInKitMetrics(t *testing.T) {
	root := RepoRoot()
	scanned, ui := 0, 0
	for _, p := range goFilesUnder(t, filepath.Join(root, "internal")) {
		if strings.HasSuffix(p, "_test.go") {
			continue
		}
		src, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		rel, _ := filepath.Rel(root, p)
		problems, err := kitMetricProblems(filepath.ToSlash(rel), src)
		if err != nil {
			t.Fatalf("parse %s: %v", rel, err)
		}
		for _, problem := range problems {
			t.Error(problem)
		}
		scanned++
		if strings.Contains(string(src), "github.com/frostyard/frostui") {
			ui++
		}
	}
	t.Logf("scanned %d files, %d of them draw with frostui", scanned, ui)
	if ui < 10 {
		t.Fatalf("saw %d files that draw with frostui, too few for the scan to be working", ui)
	}
}

// TestKitMetricFixtures proves kitMetricProblems can fail: each fixture
// that breaks the rule is reported, and the kit's metrics pass.
func TestKitMetricFixtures(t *testing.T) {
	const header = "package p\n\nfunc f() {\n"
	fixtures := []struct {
		name, body string
		problems   int
	}{
		{"literal radius", "\tui.Box(c).Radius(16)\n", 1},
		{"literal float radius", "\tui.Box(c).Radius(2.5)\n", 1},
		{"literal font size", "\tui.Text(c, s).FontSize(13)\n", 1},
		{"signed literal in parentheses", "\tui.Text(c, s).FontSize((-1))\n", 1},
		{"style field", "\tfrostui.KeyValues(c, rows, frostui.KeyValuesStyle{KeyWidth: 72, FontSize: 12.5})\n", 1},
		{"shadow", "\tui.Column(c).Shadow(0, 8, 28, 0, color)\n", 1},
		{"two in one chain", "\tui.Box(c).Radius(10).Child(ui.Text(c, s).FontSize(11))\n", 2},
		{"kit metrics", "\tui.Box(c).Radius(frostui.RadiusChip).Radius(t.Radius)\n\tui.Text(c, s).FontSize(frostui.TextSize(c, 13))\n", 0},
		{"derived from a constant", "\tfrostui.Icon(c, n).FontSize(heroLogo * 9 / 16)\n", 0},
		{"scaled style field", "\t_ = frostui.KeyValuesStyle{KeyWidth: 72, FontSize: frostui.TextSize(c, 12.5)}\n", 0},
	}
	for _, fx := range fixtures {
		t.Run(fx.name, func(t *testing.T) {
			problems, err := kitMetricProblems("fixture.go", []byte(header+fx.body+"}\n"))
			if err != nil {
				t.Fatal(err)
			}
			if len(problems) != fx.problems {
				t.Errorf("got %d problems, want %d: %q", len(problems), fx.problems, problems)
			}
		})
	}
}
