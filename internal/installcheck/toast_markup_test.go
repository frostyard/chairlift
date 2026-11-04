package installcheck

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// toastMarkupViolations reports every libadwaita toast in src that can reach
// the screen with Pango markup still on. AdwToast parses its title as markup
// by default, so a message that contains <, > or & fails to parse and the
// toast renders blank. A toast passes only when it is assigned to a variable
// and that variable calls SetUseMarkup(false) before it is used as a call
// argument (for example AddToast(toast)). A constructor whose result is not
// assigned, such as AddToast(adw.NewToast(msg)), always fails.
func toastMarkupViolations(filename string, src []byte) ([]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, 0)
	if err != nil {
		return nil, err
	}

	var violations []string
	report := func(pos token.Pos, why string) {
		violations = append(violations, fset.Position(pos).String()+": "+why)
	}

	checkBody := func(body *ast.BlockStmt) {
		assigned := make(map[*ast.CallExpr]bool)
		ast.Inspect(body, func(n ast.Node) bool {
			// A nested function literal is checked as its own body.
			if lit, ok := n.(*ast.FuncLit); ok && lit.Body != body {
				return false
			}
			assign, ok := n.(*ast.AssignStmt)
			if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
				return true
			}
			call, ok := assign.Rhs[0].(*ast.CallExpr)
			if !ok || !isToastConstructor(call) {
				return true
			}
			ident, ok := assign.Lhs[0].(*ast.Ident)
			if !ok || ident.Name == "_" {
				return true
			}
			assigned[call] = true
			disabled := disablesMarkupAt(body, ident.Name, call.End())
			if disabled == token.NoPos {
				report(call.Pos(), ident.Name+" never calls SetUseMarkup(false)")
				return true
			}
			if used := firstArgumentUse(body, ident.Name, call.End()); used != token.NoPos && used < disabled {
				report(used, ident.Name+" is used before SetUseMarkup(false)")
			}
			return true
		})
		ast.Inspect(body, func(n ast.Node) bool {
			if lit, ok := n.(*ast.FuncLit); ok && lit.Body != body {
				return false
			}
			if call, ok := n.(*ast.CallExpr); ok && isToastConstructor(call) && !assigned[call] {
				report(call.Pos(), "toast is not assigned, so markup cannot be disabled")
			}
			return true
		})
	}

	ast.Inspect(file, func(n ast.Node) bool {
		switch fn := n.(type) {
		case *ast.FuncDecl:
			if fn.Body != nil {
				checkBody(fn.Body)
			}
		case *ast.FuncLit:
			checkBody(fn.Body)
		}
		return true
	})
	return violations, nil
}

// isToastConstructor reports whether call is adw.NewToast or another adw
// constructor whose name starts with NewToast (an AdwToastOverlay is
// NewToastOverlay and is excluded).
func isToastConstructor(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok || pkg.Name != "adw" {
		return false
	}
	return strings.HasPrefix(sel.Sel.Name, "NewToast") && !strings.HasPrefix(sel.Sel.Name, "NewToastOverlay")
}

// disablesMarkupAt returns the position of the first name.SetUseMarkup(false)
// call after from, or token.NoPos.
func disablesMarkupAt(body *ast.BlockStmt, name string, from token.Pos) token.Pos {
	found := token.NoPos
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || call.Pos() < from || found != token.NoPos {
			return found == token.NoPos
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "SetUseMarkup" || len(call.Args) != 1 {
			return true
		}
		recv, ok := sel.X.(*ast.Ident)
		arg, isIdent := call.Args[0].(*ast.Ident)
		if ok && recv.Name == name && isIdent && arg.Name == "false" {
			found = call.Pos()
		}
		return true
	})
	return found
}

// firstArgumentUse returns the position of the first call after from that
// passes name, or its address, as an argument, or token.NoPos.
func firstArgumentUse(body *ast.BlockStmt, name string, from token.Pos) token.Pos {
	found := token.NoPos
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || call.Pos() < from || found != token.NoPos {
			return found == token.NoPos
		}
		for _, arg := range call.Args {
			if unary, ok := arg.(*ast.UnaryExpr); ok && unary.Op == token.AND {
				arg = unary.X
			}
			if ident, ok := arg.(*ast.Ident); ok && ident.Name == name {
				found = arg.Pos()
				return false
			}
		}
		return true
	})
	return found
}

func TestToastMarkupViolationsFixtures(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want int
	}{
		{
			name: "markup disabled before AddToast",
			src: `package p
func f(w W, msg string) {
	toast := adw.NewToast(msg)
	toast.SetUseMarkup(false)
	toast.SetTimeout(3)
	w.AddToast(toast)
}`,
			want: 0,
		},
		{
			name: "markup left on",
			src: `package p
func f(w W, msg string) {
	toast := adw.NewToast(msg)
	toast.SetTimeout(3)
	w.AddToast(toast)
}`,
			want: 1,
		},
		{
			name: "markup disabled after the toast is shown",
			src: `package p
func f(w W, msg string) {
	toast := adw.NewToast(msg)
	w.AddToast(toast)
	toast.SetUseMarkup(false)
}`,
			want: 1,
		},
		{
			name: "markup set to true",
			src: `package p
func f(w W, msg string) {
	toast := adw.NewToast(msg)
	toast.SetUseMarkup(true)
	w.AddToast(toast)
}`,
			want: 1,
		},
		{
			name: "inline constructor",
			src: `package p
func f(w W, msg string) {
	w.AddToast(adw.NewToast(msg))
}`,
			want: 1,
		},
		{
			name: "toast in a callback",
			src: `package p
func f(w W, msg string) {
	cb := func() {
		toast := adw.NewToast(msg)
		w.AddToast(toast)
	}
	cb()
}`,
			want: 1,
		},
		{
			name: "toast overlay is not a toast",
			src: `package p
func f() {
	overlay := adw.NewToastOverlay()
	_ = overlay
}`,
			want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := toastMarkupViolations("fixture.go", []byte(tt.src))
			if err != nil {
				t.Fatalf("parse fixture: %v", err)
			}
			if len(got) != tt.want {
				t.Fatalf("toastMarkupViolations() = %v, want %d violation(s)", got, tt.want)
			}
		})
	}
}

// TestToastsRenderPlainText fails when a toast in production code can reach
// the screen with Pango markup on: a path or tool error that contains <, >
// or & would then render as a blank toast.
func TestToastsRenderPlainText(t *testing.T) {
	root := RepoRoot()
	checked := 0
	toastFiles := 0
	for _, dir := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			checked++
			if !strings.Contains(string(src), "adw.NewToast") {
				return nil
			}
			violations, err := toastMarkupViolations(path, src)
			if err != nil {
				return err
			}
			if strings.Contains(strings.ReplaceAll(string(src), "adw.NewToastOverlay", ""), "adw.NewToast") {
				toastFiles++
			}
			for _, violation := range violations {
				t.Error(violation)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}
	if toastFiles == 0 {
		t.Fatal("no production file constructs an adw toast; the check would prove nothing")
	}
	t.Logf("toast markup check: %d Go files checked, %d construct toasts", checked, toastFiles)
}
