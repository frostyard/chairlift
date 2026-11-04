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
// the screen with Pango markup still on, and counts the toast constructors it
// found in the syntax tree (comments and strings do not count). AdwToast
// parses its title as markup by default, so a message that contains <, > or &
// fails to parse and the toast renders blank.
//
// The rule is strict so that it can be proven without control-flow analysis:
// the constructor result must be assigned to a variable, and the very next
// statement in the same block must be name.SetUseMarkup(false). No other
// SetUseMarkup reference may appear in the file, on any receiver and in any
// form (defer, go, method value, alias, helper). Anything else is a
// violation, including an inline constructor such as
// AddToast(adw.NewToast(msg)).
func toastMarkupViolations(filename string, src []byte) ([]string, int, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, 0)
	if err != nil {
		return nil, 0, err
	}

	var violations []string
	report := func(pos token.Pos, why string) {
		violations = append(violations, fset.Position(pos).String()+": "+why)
	}

	constructors := 0
	accepted := make(map[*ast.CallExpr]bool)
	acceptedSetters := make(map[*ast.SelectorExpr]bool)
	checkBody := func(body *ast.BlockStmt) {
		ast.Inspect(body, func(n ast.Node) bool {
			// A nested function literal is checked as its own body.
			if lit, ok := n.(*ast.FuncLit); ok && lit.Body != body {
				return false
			}
			block, ok := n.(*ast.BlockStmt)
			if !ok {
				return true
			}
			for i, stmt := range block.List {
				assign, ok := stmt.(*ast.AssignStmt)
				if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
					continue
				}
				call, ok := assign.Rhs[0].(*ast.CallExpr)
				if !ok || !isToastConstructor(call) {
					continue
				}
				ident, ok := assign.Lhs[0].(*ast.Ident)
				if !ok || ident.Name == "_" {
					continue
				}
				accepted[call] = true
				if i+1 >= len(block.List) || !isMarkupSetter(block.List[i+1], ident.Name) {
					report(call.Pos(), ident.Name+" is not followed at once by "+ident.Name+".SetUseMarkup(false)")
					continue
				}
				setter := block.List[i+1].(*ast.ExprStmt).X.(*ast.CallExpr)
				acceptedSetters[setter.Fun.(*ast.SelectorExpr)] = true
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
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CallExpr:
			if isToastConstructor(node) {
				constructors++
				if !accepted[node] {
					report(node.Pos(), "toast is not assigned to a variable, so markup cannot be disabled")
				}
			}
		case *ast.SelectorExpr:
			// Any other SetUseMarkup reference, in any form (a plain,
			// deferred or goroutine call, a method value, or a call on an
			// alias or another receiver), could turn markup on again.
			if node.Sel.Name == "SetUseMarkup" && !acceptedSetters[node] {
				report(node.Pos(), "SetUseMarkup is used outside the setter that directly follows a toast constructor")
			}
		}
		return true
	})
	return violations, constructors, nil
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

// isMarkupSetter reports whether stmt is the statement
// name.SetUseMarkup(false).
func isMarkupSetter(stmt ast.Stmt, name string) bool {
	expr, ok := stmt.(*ast.ExprStmt)
	if !ok {
		return false
	}
	call, ok := expr.X.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "SetUseMarkup" {
		return false
	}
	recv, ok := sel.X.(*ast.Ident)
	arg, isIdent := call.Args[0].(*ast.Ident)
	return ok && recv.Name == name && isIdent && arg.Name == "false"
}

func TestToastMarkupViolationsFixtures(t *testing.T) {
	tests := []struct {
		name   string
		src    string
		unsafe bool
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
			unsafe: false,
		},
		{
			name: "markup left on",
			src: `package p
func f(w W, msg string) {
	toast := adw.NewToast(msg)
	toast.SetTimeout(3)
	w.AddToast(toast)
}`,
			unsafe: true,
		},
		{
			name: "markup disabled after the toast is shown",
			src: `package p
func f(w W, msg string) {
	toast := adw.NewToast(msg)
	w.AddToast(toast)
	toast.SetUseMarkup(false)
}`,
			unsafe: true,
		},
		{
			name: "markup set to true",
			src: `package p
func f(w W, msg string) {
	toast := adw.NewToast(msg)
	toast.SetUseMarkup(true)
	w.AddToast(toast)
}`,
			unsafe: true,
		},
		{
			name: "inline constructor",
			src: `package p
func f(w W, msg string) {
	w.AddToast(adw.NewToast(msg))
}`,
			unsafe: true,
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
			unsafe: true,
		},
		{
			name: "markup disabled only inside an uncalled callback",
			src: `package p
func f(w W, msg string) {
	toast := adw.NewToast(msg)
	_ = func() { toast.SetUseMarkup(false) }
	w.AddToast(toast)
}`,
			unsafe: true,
		},
		{
			name: "markup turned on again before AddToast",
			src: `package p
func f(w W, msg string) {
	toast := adw.NewToast(msg)
	toast.SetUseMarkup(false)
	toast.SetUseMarkup(true)
	w.AddToast(toast)
}`,
			unsafe: true,
		},
		{
			name: "markup turned on again by a deferred call",
			src: `package p
func f(w W, msg string) {
	toast := adw.NewToast(msg)
	toast.SetUseMarkup(false)
	defer w.AddToast(toast)
	defer toast.SetUseMarkup(true)
}`,
			unsafe: true,
		},
		{
			name: "markup turned on again in a goroutine",
			src: `package p
func f(w W, msg string) {
	toast := adw.NewToast(msg)
	toast.SetUseMarkup(false)
	go toast.SetUseMarkup(true)
	w.AddToast(toast)
}`,
			unsafe: true,
		},
		{
			name: "markup turned on again through an alias",
			src: `package p
func f(w W, msg string) {
	toast := adw.NewToast(msg)
	toast.SetUseMarkup(false)
	same := toast
	same.SetUseMarkup(true)
	w.AddToast(toast)
}`,
			unsafe: true,
		},
		{
			name: "markup turned on again through a method value",
			src: `package p
func f(w W, msg string) {
	toast := adw.NewToast(msg)
	toast.SetUseMarkup(false)
	set := toast.SetUseMarkup
	set(true)
	w.AddToast(toast)
}`,
			unsafe: true,
		},
		{
			name: "markup turned on again in a helper",
			src: `package p
func f(w W, msg string) {
	toast := adw.NewToast(msg)
	toast.SetUseMarkup(false)
	enable(toast)
	w.AddToast(toast)
}
func enable(t *Toast) { t.SetUseMarkup(true) }`,
			unsafe: true,
		},
		{
			name: "markup disabled only on one branch",
			src: `package p
func f(w W, msg string, plain bool) {
	toast := adw.NewToast(msg)
	if plain {
		toast.SetUseMarkup(false)
	}
	w.AddToast(toast)
}`,
			unsafe: true,
		},
		{
			name: "toast in a callback with markup disabled",
			src: `package p
func f(w W, msg string) {
	cb := func() {
		toast := adw.NewToast(msg)
		toast.SetUseMarkup(false)
		w.AddToast(toast)
	}
	cb()
}`,
			unsafe: false,
		},
		{
			name: "toast overlay is not a toast",
			src: `package p
func f() {
	overlay := adw.NewToastOverlay()
	_ = overlay
}`,
			unsafe: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, err := toastMarkupViolations("fixture.go", []byte(tt.src))
			if err != nil {
				t.Fatalf("parse fixture: %v", err)
			}
			if (len(got) > 0) != tt.unsafe {
				t.Fatalf("toastMarkupViolations() = %v, want unsafe = %v", got, tt.unsafe)
			}
		})
	}
}

func TestToastMarkupViolationsCountsOnlyRealConstructors(t *testing.T) {
	src := `package p
// adw.NewToast(msg) in a comment is not a toast.
const doc = "adw.NewToast(msg)"
func f(w W, msg string) {
	overlay := adw.NewToastOverlay()
	toast := adw.NewToast(msg)
	toast.SetUseMarkup(false)
	w.AddToast(toast)
	_ = overlay
}`
	violations, constructors, err := toastMarkupViolations("fixture.go", []byte(src))
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("violations = %v, want none", violations)
	}
	if constructors != 1 {
		t.Fatalf("constructors = %d, want 1 (comments, strings and the overlay do not count)", constructors)
	}
}

// TestToastsRenderPlainText fails when a toast in production code can reach
// the screen with Pango markup on: a path or tool error that contains <, >
// or & would then render as a blank toast.
func TestToastsRenderPlainText(t *testing.T) {
	root := RepoRoot()
	checked := 0
	constructors := 0
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
			violations, found, err := toastMarkupViolations(path, src)
			if err != nil {
				return err
			}
			checked++
			constructors += found
			for _, violation := range violations {
				t.Error(violation)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}
	if constructors == 0 {
		t.Fatal("no production code constructs an adw toast; the check would prove nothing")
	}
	t.Logf("toast markup check: %d Go files parsed, %d toast constructors checked", checked, constructors)
}
