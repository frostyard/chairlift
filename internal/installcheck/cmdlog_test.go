package installcheck

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// cmdlogImportPath is the package that announces commands in the console.
const cmdlogImportPath = "github.com/frostyard/chairlift/internal/cmdlog"

// commandTracing is what checkCommandTracing found in one file.
type commandTracing struct {
	// Commands counts the calls of exec.Command and exec.CommandContext and
	// the exec.Cmd literals, Starts the calls of cmdlog.Start.
	Commands, Starts int
	// Problems holds one line per function or expression that breaks the
	// rule.
	Problems []string
}

// checkCommandTracing parses one Go file and checks AGENTS.md's "Every
// command shows in the console" for it. Every function body, declared or
// literal, that starts commands through os/exec, under whatever name the
// file imports it as, must call cmdlog.Start at least as often. Only call
// expressions count, so a cmdlog.Start in a comment or a string does not,
// and a closure is a function of its own: the trace sits next to the
// command it announces. What the check cannot follow is reported too:
// exec.Command or exec.CommandContext taken as a value, a dot import of
// os/exec, and a command started outside any function.
func checkCommandTracing(filename string, src []byte) (commandTracing, error) {
	var res commandTracing
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.SkipObjectResolution)
	if err != nil {
		return res, err
	}
	execName, cmdlogName := "", ""
	for _, imp := range file.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			return res, err
		}
		name := path.Base(p)
		if imp.Name != nil {
			name = imp.Name.Name
		}
		switch p {
		case "os/exec":
			execName = name
		case cmdlogImportPath:
			cmdlogName = name
		}
	}
	switch execName {
	case "", "_":
		return res, nil
	case ".":
		res.Problems = append(res.Problems, filename+" dot-imports os/exec, so its commands cannot be checked")
		return res, nil
	}

	where := func(pos token.Pos) string {
		p := fset.Position(pos)
		return fmt.Sprintf("%s:%d", p.Filename, p.Line)
	}
	// pkgSel reports whether e is pkg.<one of names>.
	pkgSel := func(e ast.Expr, pkg string, names ...string) (*ast.SelectorExpr, bool) {
		sel, ok := e.(*ast.SelectorExpr)
		if !ok {
			return nil, false
		}
		id, ok := sel.X.(*ast.Ident)
		return sel, ok && id.Name == pkg && slices.Contains(names, sel.Sel.Name)
	}

	type function struct {
		node             ast.Node
		name             string
		commands, starts int
	}
	var funcs []*function // the enclosing functions, innermost last
	var stack []ast.Node
	called := map[*ast.SelectorExpr]bool{}
	command := func(pos token.Pos, what string) {
		res.Commands++
		if len(funcs) == 0 {
			res.Problems = append(res.Problems, fmt.Sprintf("%s: %s outside any function cannot be traced", where(pos), what))
			return
		}
		funcs[len(funcs)-1].commands++
	}
	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil {
			left := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if len(funcs) > 0 && funcs[len(funcs)-1].node == left {
				f := funcs[len(funcs)-1]
				funcs = funcs[:len(funcs)-1]
				if f.commands > f.starts {
					start := "cmdlog.Start"
					if cmdlogName == "" {
						start += " (" + cmdlogImportPath + " is not imported)"
					}
					res.Problems = append(res.Problems, fmt.Sprintf("%s: %s starts %d command(s) but calls %s %d time(s)",
						where(f.node.Pos()), f.name, f.commands, start, f.starts))
				}
			}
			return true
		}
		stack = append(stack, n)
		switch n := n.(type) {
		case *ast.FuncDecl:
			funcs = append(funcs, &function{node: n, name: "func " + n.Name.Name})
		case *ast.FuncLit:
			funcs = append(funcs, &function{node: n, name: "a func literal"})
		case *ast.CallExpr:
			if sel, ok := pkgSel(n.Fun, execName, "Command", "CommandContext"); ok {
				called[sel] = true
				command(n.Pos(), execName+"."+sel.Sel.Name)
			}
			if _, ok := pkgSel(n.Fun, cmdlogName, "Start"); ok {
				res.Starts++
				if len(funcs) > 0 {
					funcs[len(funcs)-1].starts++
				}
			}
		case *ast.CompositeLit:
			if _, ok := pkgSel(n.Type, execName, "Cmd"); ok {
				command(n.Pos(), execName+".Cmd literal")
			}
		case *ast.SelectorExpr:
			if _, ok := pkgSel(n, execName, "Command", "CommandContext"); ok && !called[n] {
				res.Problems = append(res.Problems, fmt.Sprintf("%s: %s.%s is taken as a value; call it directly so its trace can be checked",
					where(n.Pos()), execName, n.Sel.Name))
			}
		}
		return true
	})
	return res, nil
}

// TestEveryCommandIsTraced keeps AGENTS.md's "Every command shows in the
// console" true: each function in a production file under internal/ that
// starts an external command also announces it through internal/cmdlog
// (checkCommandTracing). The kit (internal/frostui) is exempt: it cannot
// import ChairLift, and the views trace what they launch through it
// (launchDesktopApp).
func TestEveryCommandIsTraced(t *testing.T) {
	root := RepoRoot()
	files, commands, starts := 0, 0, 0
	for _, p := range goFilesUnder(t, filepath.Join(root, "internal")) {
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if strings.HasSuffix(rel, "_test.go") || strings.HasPrefix(rel, "internal/frostui/") || strings.HasPrefix(rel, "internal/cmdlog/") {
			continue
		}
		src, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		res, err := checkCommandTracing(rel, src)
		if err != nil {
			t.Fatalf("parse %s: %v", rel, err)
		}
		for _, problem := range res.Problems {
			t.Error(problem)
		}
		if res.Commands > 0 {
			files++
		}
		commands += res.Commands
		starts += res.Starts
	}
	t.Logf("%d files start %d commands and call cmdlog.Start %d times", files, commands, starts)
	if files < 8 {
		t.Fatalf("saw %d files that start commands, too few for the scan to be working", files)
	}
}

// TestCommandTracingFixtures proves checkCommandTracing can fail: each
// fixture that breaks the rule must be reported, and the traced one must
// pass.
func TestCommandTracingFixtures(t *testing.T) {
	const header = "package p\n\nimport (\n\t\"context\"\n\t\"os/exec\"\n\n\t\"" + cmdlogImportPath + "\"\n)\n\n"
	fixtures := []struct {
		name     string
		src      string
		commands int
		fail     bool
	}{
		{
			name:     "untraced command",
			src:      "package p\n\nimport \"os/exec\"\n\nfunc run() error { return exec.Command(\"brew\", \"update\").Run() }\n",
			commands: 1,
			fail:     true,
		},
		{
			name: "aliased os/exec, untraced",
			src: "package p\n\nimport (\n\tosexec \"os/exec\"\n\n\t\"" + cmdlogImportPath + "\"\n)\n\n" +
				"func run() error { return osexec.Command(\"brew\", \"update\").Run() }\n\n" +
				"func traced() { trace := cmdlog.Start(\"brew\"); trace.End(nil) }\n",
			commands: 1,
			fail:     true,
		},
		{
			name: "cmdlog.Start only in a comment",
			src: header + "func run(ctx context.Context) error {\n" +
				"\t// trace := cmdlog.Start(\"brew\", \"update\")\n" +
				"\t_ = \"cmdlog.Start(\"\n" +
				"\treturn exec.CommandContext(ctx, \"brew\", \"update\").Run()\n}\n",
			commands: 1,
			fail:     true,
		},
		{
			name: "two commands, one Start",
			src: header + "func run(ctx context.Context) error {\n" +
				"\ttrace := cmdlog.Start(\"brew\", \"update\")\n" +
				"\terr := exec.CommandContext(ctx, \"brew\", \"update\").Run()\n" +
				"\ttrace.End(err)\n" +
				"\treturn exec.CommandContext(ctx, \"brew\", \"upgrade\").Run()\n}\n",
			commands: 2,
			fail:     true,
		},
		{
			name: "Start in the enclosing function, command in a closure",
			src: header + "func run(ctx context.Context) func() error {\n" +
				"\ttrace := cmdlog.Start(\"brew\", \"update\")\n" +
				"\tdefer trace.End(nil)\n" +
				"\treturn func() error { return exec.CommandContext(ctx, \"brew\", \"update\").Run() }\n}\n",
			commands: 1,
			fail:     true,
		},
		{
			name: "exec.CommandContext taken as a value",
			src: header + "var command = exec.CommandContext\n\n" +
				"func run(ctx context.Context) error {\n" +
				"\ttrace := cmdlog.Start(\"brew\")\n" +
				"\tdefer trace.End(nil)\n" +
				"\treturn command(ctx, \"brew\").Run()\n}\n",
			commands: 0,
			fail:     true,
		},
		{
			name:     "exec.Cmd literal, untraced",
			src:      header + "func run() error { return (&exec.Cmd{Path: \"/usr/bin/brew\"}).Run() }\n",
			commands: 1,
			fail:     true,
		},
		{
			name:     "command at package level",
			src:      header + "var cmd = exec.Command(\"brew\")\n",
			commands: 1,
			fail:     true,
		},
		{
			name: "dot import of os/exec",
			src: "package p\n\nimport . \"os/exec\"\n\n" +
				"func run() error { return Command(\"brew\").Run() }\n",
			fail: true,
		},
		{
			name: "traced, with aliases and a closure",
			src: "package p\n\nimport (\n\t\"context\"\n\tosexec \"os/exec\"\n\n\ttrace \"" + cmdlogImportPath + "\"\n)\n\n" +
				"func run(ctx context.Context) error {\n" +
				"\tt := trace.Start(\"brew\", \"update\")\n" +
				"\terr := osexec.CommandContext(ctx, \"brew\", \"update\").Run()\n" +
				"\tt.End(err)\n" +
				"\tgo func() {\n" +
				"\t\tt := trace.Start(\"brew\", \"cleanup\")\n" +
				"\t\tt.End(osexec.Command(\"brew\", \"cleanup\").Run())\n" +
				"\t}()\n" +
				"\t_, lookErr := osexec.LookPath(\"brew\")\n" +
				"\treturn lookErr\n}\n",
			commands: 2,
		},
		{
			name:     "os/exec for LookPath only",
			src:      "package p\n\nimport \"os/exec\"\n\nfunc have() bool { _, err := exec.LookPath(\"brew\"); return err == nil }\n",
			commands: 0,
		},
	}
	reported := 0
	for _, fx := range fixtures {
		t.Run(fx.name, func(t *testing.T) {
			res, err := checkCommandTracing("fixture.go", []byte(fx.src))
			if err != nil {
				t.Fatalf("parse: %v\n%s", err, fx.src)
			}
			if res.Commands != fx.commands {
				t.Errorf("counted %d commands, want %d", res.Commands, fx.commands)
			}
			switch {
			case fx.fail && len(res.Problems) == 0:
				t.Errorf("the fixture breaks the rule but was not reported:\n%s", fx.src)
			case !fx.fail && len(res.Problems) > 0:
				t.Errorf("the traced fixture was reported: %q", res.Problems)
			}
			if fx.fail {
				reported += len(res.Problems)
			}
			t.Logf("commands=%d starts=%d problems=%q", res.Commands, res.Starts, res.Problems)
		})
	}
	t.Logf("%d fixtures, %d problems reported", len(fixtures), reported)
}
