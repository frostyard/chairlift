package app

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo"

	"github.com/frostyard/chairlift/internal/bootc"
	"github.com/frostyard/chairlift/internal/cmdlog"
	"github.com/frostyard/chairlift/internal/config"
	"github.com/frostyard/chairlift/internal/firstrun"
	"github.com/frostyard/chairlift/internal/flatpak"
	"github.com/frostyard/chairlift/internal/fwupd"
	"github.com/frostyard/chairlift/internal/homebrew"
	"github.com/frostyard/chairlift/internal/sysupdate"
	"github.com/frostyard/chairlift/internal/updex"
	"github.com/frostyard/chairlift/internal/views"
	"github.com/frostyard/chairlift/internal/window"
	"github.com/frostyard/frostui"
	"github.com/frostyard/frostui/frostuitest"
)

type fakeWindow struct {
	mu sync.Mutex
}

func (f *fakeWindow) Update(fn func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn()
}

func TestPosterKeepsWhatArrivesBeforeTheWindow(t *testing.T) {
	p := &poster{}
	w := &fakeWindow{}
	var got []string
	p.post(func() { got = append(got, "early") })
	p.attach(w)
	p.post(func() { got = append(got, "late") })
	if len(got) != 2 || got[0] != "early" || got[1] != "late" {
		t.Errorf("ran %v, want early then late", got)
	}
}

func TestPosterIsSafeFromGoroutines(t *testing.T) {
	p := &poster{}
	w := &fakeWindow{}
	var mu sync.Mutex
	n := 0
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.post(func() { mu.Lock(); n++; mu.Unlock() })
		}()
		if i == 25 {
			p.attach(w)
		}
	}
	wg.Wait()
	if n != 50 {
		t.Errorf("ran %d of 50 posts", n)
	}
}

// closableWindow is a window that keeps the close listener it is given.
type closableWindow struct {
	fakeWindow
	onClose func(*mygo.CloseEvent)
}

func (w *closableWindow) OnClose(fn func(*mygo.CloseEvent)) func() {
	w.onClose = fn
	return func() {}
}

func TestMainWindowCloseGuard(t *testing.T) {
	held := func(release <-chan struct{}) func(context.Context, *frostui.Progress) error {
		return func(ctx context.Context, _ *frostui.Progress) error {
			select {
			case <-release:
			case <-ctx.Done():
			}
			return nil
		}
	}
	for _, tc := range []struct {
		name    string
		jobs    func(release <-chan struct{}) []*frostui.Job
		refused bool
	}{
		{"no job", func(<-chan struct{}) []*frostui.Job { return nil }, false},
		{"a stoppable job runs", func(r <-chan struct{}) []*frostui.Job {
			return []*frostui.Job{{Title: "Cleaning", Run: held(r)}}
		}, false},
		{"an unstoppable job only waits", func(r <-chan struct{}) []*frostui.Job {
			return []*frostui.Job{{Title: "Cleaning", Run: held(r)}, {Title: "Staging", Unstoppable: true, Run: held(r)}}
		}, false},
		{"an unstoppable job runs", func(r <-chan struct{}) []*frostui.Job {
			return []*frostui.Job{{Title: "Staging", Unstoppable: true, Run: held(r)}}
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			loop := frostuitest.NewLoop()
			env := views.NewEnv(&config.Config{}, true, loop.Post)
			m := window.NewMain(env, views.NewStore(env))
			w := &closableWindow{}
			attachMain(w, &poster{}, m)
			if w.onClose == nil {
				t.Fatal("attachMain installed no close listener")
			}
			release := make(chan struct{})
			env.Jobs.Enqueue(tc.jobs(release)...)

			e := &mygo.CloseEvent{}
			w.onClose(e)
			if e.DefaultPrevented() != tc.refused {
				t.Errorf("close refused = %v, want %v", e.DefaultPrevented(), tc.refused)
			}
			close(release)
			loop.RunUntil(t, func() bool { return !env.Jobs.Busy() })

			after := &mygo.CloseEvent{}
			w.onClose(after)
			if after.DefaultPrevented() {
				t.Error("the close is still refused after the queue ended")
			}
		})
	}
}

// openMain needs a real window, so its source shows that it wires the guard.
func TestOpenMainGuardsTheClose(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "app.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "openMain" {
			continue
		}
		found := false
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "attachMain" {
					found = true
				}
			}
			return true
		})
		if !found {
			t.Error("openMain does not call attachMain: the main window has no close guard")
		}
		return
	}
	t.Fatal("app.go has no openMain")
}

func TestParseArgs(t *testing.T) {
	o := ParseArgs([]string{"-d", "--first-run"})
	if !o.DryRun || !o.FirstRun || o.ID() != FirstRunAppID {
		t.Errorf("ParseArgs = %+v, ID %s", o, o.ID())
	}
	if o := ParseArgs(nil); o.DryRun || o.FirstRun || o.ID() != AppID {
		t.Errorf("ParseArgs(nil) = %+v", o)
	}
	if !ParseArgs([]string{"--dry-run"}).DryRun {
		t.Error("--dry-run not parsed")
	}
}

func TestSourceOfNamesTheTool(t *testing.T) {
	cases := map[string][]string{
		"brew":                   {"brew", "upgrade", "gh"},
		"chairlift-updex-helper": {"/usr/bin/pkexec", "/usr/bin/chairlift-updex-helper", "update"},
		"fwupdmgr":               {"/usr/bin/fwupdmgr", "get-updates"},
		"":                       nil,
	}
	for want, argv := range cases {
		if got := sourceOf(argv); got != want {
			t.Errorf("sourceOf(%v) = %q, want %q", argv, got, want)
		}
	}
}

func TestStripTimestamp(t *testing.T) {
	var got []byte
	w := stripTimestamp{writerFunc(func(p []byte) (int, error) { got = append(got, p...); return len(p), nil })}
	_, _ = w.Write([]byte("2026/11/10 10:50:19 file.go:12: message\n"))
	if string(got) != "file.go:12: message\n" {
		t.Errorf("got %q", got)
	}
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }

func TestWireConsoleShowsCommandsAndTheLog(t *testing.T) {
	con := frostui.NewConsole(64)
	var mu sync.Mutex
	posts := 0
	wireConsole(con, func(func()) { mu.Lock(); posts++; mu.Unlock() })
	t.Cleanup(func() {
		cmdlog.SetSink(nil)
		log.SetOutput(os.Stderr)
	})

	run := cmdlog.Start("/usr/bin/pkexec", "/usr/bin/chairlift-updex-helper", "update")
	w := run.Stdout(nil)
	_, _ = w.Write([]byte("Merged docker into /usr\n"))
	run.End(nil)
	failed := cmdlog.Start("brew", "upgrade", "gh")
	failed.End(errors.New("exit status 1"))
	cmdlog.DryRun("flatpak", "update", "org.mozilla.firefox")
	log.Print("[DRY-RUN] would dismiss problem p1")
	log.Print("homebrew: list failed")
	log.Print("window: built")

	type want struct {
		level  frostui.Level
		source string
		text   string
	}
	for _, w := range []want{
		{frostui.LevelCommand, "chairlift-updex-helper", "/usr/bin/pkexec /usr/bin/chairlift-updex-helper update"},
		{frostui.LevelOutput, "chairlift-updex-helper", "Merged docker into /usr"},
		{frostui.LevelDebug, "chairlift-updex-helper", "exited after"},
		{frostui.LevelWarn, "brew", "exit status 1"},
		{frostui.LevelDryRun, "flatpak", "would run: flatpak update org.mozilla.firefox"},
		{frostui.LevelDryRun, "log", "[DRY-RUN] would dismiss problem p1"},
		{frostui.LevelWarn, "log", "homebrew: list failed"},
		{frostui.LevelDebug, "log", "window: built"},
	} {
		found := false
		for _, l := range con.Lines(nil, "") {
			if l.Level == w.level && l.Source == w.source && strings.Contains(l.Text, w.text) {
				found = true
			}
		}
		if !found {
			t.Errorf("no %s line from %q with %q in %+v", frostui.LevelNames[w.level], w.source, w.text, con.Lines(nil, ""))
		}
	}
	for _, l := range con.Lines(nil, "log") {
		if strings.HasPrefix(l.Text, "20") {
			t.Errorf("a log line kept its timestamp: %q", l.Text)
		}
	}
	time.Sleep(150 * time.Millisecond) // the waker coalesces
	mu.Lock()
	defer mu.Unlock()
	if posts == 0 || posts > 3 {
		t.Errorf("the console woke the window %d times for a burst, want a few", posts)
	}
}

func TestSetDryRunReachesEveryBackend(t *testing.T) {
	t.Cleanup(func() {
		for _, set := range []func(bool){flatpak.SetDryRun, fwupd.SetDryRun, homebrew.SetDryRun, bootc.SetDryRun, sysupdate.SetDryRun, updex.SetDryRun, firstrun.SetDryRun} {
			set(false)
		}
	})
	SetDryRun()
	for name, on := range map[string]bool{
		"flatpak": flatpak.IsDryRun(), "fwupd": fwupd.IsDryRun(), "homebrew": homebrew.IsDryRun(),
		"bootc": bootc.IsDryRun(), "sysupdate": sysupdate.IsDryRun(), "updex": updex.IsDryRun(), "firstrun": firstrun.IsDryRun(),
	} {
		if !on {
			t.Errorf("%s is not in dry-run mode", name)
		}
	}
}
