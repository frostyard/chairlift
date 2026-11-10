package window

import (
	"context"
	"errors"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/chairlift/internal/cmdlog"
	"github.com/frostyard/chairlift/internal/config"
	"github.com/frostyard/chairlift/internal/navigation"
	"github.com/frostyard/chairlift/internal/views"
	"github.com/frostyard/frostui"
	"github.com/frostyard/frostui/frostuitest"
)

// newTestMain builds the main window over a Store whose update sources
// are empty, posting through a frostuitest.Loop the test drains, as the UI
// thread would. It fails the test if anything starts an external command:
// window tests never reach brew, flatpak, bootc, fwupdmgr or updex.
func newTestMain(t *testing.T) (*Main, *ui.Tester) {
	t.Helper()
	m, loop := newTestMainLoop(t)
	tt := ui.NewTester(func(c *ui.Context) {
		loop.Flush()
		m.View(c)
	}, 1360, 880)
	return m, tt
}

// newTestMainLoop is newTestMain for a test that draws the window itself.
func newTestMainLoop(t *testing.T) (*Main, *frostuitest.Loop) {
	t.Helper()
	frostui.SetIcons(frostui.NewIcons(map[string][]byte{"org.frostyard.ChairLift-symbolic": LogoSVG()}))
	var mu sync.Mutex
	var started []string
	cmdlog.SetSink(func(e cmdlog.Event) {
		if e.Kind == cmdlog.KindStart {
			mu.Lock()
			started = append(started, strings.Join(e.Argv, " "))
			mu.Unlock()
		}
	})
	t.Cleanup(func() {
		cmdlog.SetSink(nil)
		mu.Lock()
		defer mu.Unlock()
		if len(started) > 0 {
			t.Errorf("the window test started commands: %q", started)
		}
	})
	loop := frostuitest.NewLoop()
	env := views.NewEnv(&config.Config{}, true, loop.Post)
	store := views.NewStore(env)
	store.SourcesOverride = func() []views.UpdateSource { return nil }
	return NewMain(env, store), loop
}

func TestMainWindowShowsTheShell(t *testing.T) {
	m, tt := newTestMain(t)
	for _, want := range []string{"ChairLift", "Overview", "Search or run a command", "Dry run: nothing changes"} {
		if !tt.HasText(want) {
			t.Errorf("missing %q in %q", want, tt.Texts())
		}
	}
	if m.Shell.Selected != "overview" {
		t.Errorf("first page = %q", m.Shell.Selected)
	}
	if dir := os.Getenv("CHAIRLIFT_SNAPSHOT_DIR"); dir != "" {
		f, err := os.Create(filepath.Join(dir, "window-shell.png"))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = f.Close() }()
		if err := png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
}

func TestEveryAdvertisedShortcutWorks(t *testing.T) {
	m, tt := newTestMain(t)
	quit := false
	m.Quit = func() { quit = true }
	for _, s := range navigation.Shortcuts(m.Items) {
		mods, key, ok := keyOf(s.Keys)
		if !ok {
			t.Errorf("shortcut %s (%s) has no key binding", s.Display(), s.Action)
			continue
		}
		tt.Key(mods, key)
		tt.Advance(50 * time.Millisecond)
		switch s.Action {
		case navigation.ActionPalette:
			if !m.Shell.Palette.Open {
				t.Errorf("%s did not open the palette", s.Display())
			}
			tt.Key(0, ui.KeyEscape)
		case navigation.ActionConsole:
			if !m.Shell.Console.Open {
				t.Errorf("%s did not open the console", s.Display())
			}
			tt.Key(ui.Cmd, ui.KeyJ)
			if m.Shell.Console.Open {
				t.Errorf("%s did not close the console", s.Display())
			}
		case navigation.ActionShortcuts, navigation.ActionHelp:
			if !m.Shell.Info.Open {
				t.Errorf("%s did not open its dialog", s.Display())
			}
			tt.Key(0, ui.KeyEscape)
		case navigation.ActionQuit:
			if !quit {
				t.Errorf("%s did not quit", s.Display())
			}
		case navigation.ActionCheck:
			// checks every source; nothing to see with no sources
		default:
			if m.Shell.Selected != s.Action {
				t.Errorf("%s selected %q, want %q", s.Display(), m.Shell.Selected, s.Action)
			}
		}
		tt.Frame()
	}
}

func TestConfigurationErrorToastPersists(t *testing.T) {
	m, tt := newTestMain(t)
	m.Env.ConfigErr = errors.New("config parse/type error: /etc/chairlift/config.yml: yaml: line 1")
	tt.Frame()
	tt.Advance(time.Minute)
	if !tt.HasText("Configuration error") {
		t.Fatalf("no configuration error toast after a minute: %q", tt.Texts())
	}
}

// waitIdle draws frames, which run what the jobs post, until the queue ends.
func waitIdle(t *testing.T, m *Main, tt *ui.Tester) {
	t.Helper()
	for end := time.Now().Add(frostuitest.Timeout); m.Env.Jobs.Busy(); time.Sleep(time.Millisecond) {
		if time.Now().After(end) {
			t.Fatal("the job queue did not end")
		}
		tt.Frame()
	}
}

func TestCloseRefusedWhileAnUnstoppableJobRuns(t *testing.T) {
	m, tt := newTestMain(t)
	release := make(chan struct{})
	m.Env.Jobs.Enqueue(&frostui.Job{Title: "Installing firmware for Dock", Unstoppable: true,
		Run: func(context.Context, *frostui.Progress) error { <-release; return nil }})
	want := closeRefusedText("Installing firmware for Dock")
	if m.CanClose() {
		t.Error("the window can close while an unstoppable job runs")
	}
	tt.Frame()
	if !tt.HasText(want) {
		t.Errorf("no toast says why the window stayed: %q", tt.Texts())
	}
	close(release)
	waitIdle(t, m, tt)
	if !m.CanClose() {
		t.Error("the close is still refused after the job ended")
	}
}

func TestCloseAllowedWithoutARunningUnstoppableJob(t *testing.T) {
	held := func(ctx context.Context, _ *frostui.Progress) error { <-ctx.Done(); return nil }
	for _, tc := range []struct {
		name string
		jobs []*frostui.Job
	}{
		{"no job", nil},
		{"a stoppable job", []*frostui.Job{{Title: "Cleaning", Run: held}}},
		{"an unstoppable job in the queue", []*frostui.Job{{Title: "Cleaning", Run: held}, {Title: "Staging", Unstoppable: true, Run: held}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, tt := newTestMain(t)
			m.Env.Jobs.Enqueue(tc.jobs...)
			if !m.CanClose() {
				t.Error("the close is refused")
			}
			tt.Frame()
			if tt.HasText("ChairLift cannot close yet") {
				t.Errorf("a toast refuses a close that went through: %q", tt.Texts())
			}
			m.Env.Jobs.CancelAll()
			waitIdle(t, m, tt)
		})
	}
}

// paletteChoose opens the palette and chooses title. The Tester draws on
// until the view settles, so the command has run when it returns: a test
// of which frame did what notes it from the view.
func paletteChoose(t *testing.T, tt *ui.Tester, title string) {
	t.Helper()
	if err := tt.Click("Search or run a command"); err != nil {
		t.Fatal(err)
	}
	tt.Type(title)
	if err := tt.Click(title); err != nil {
		t.Fatalf("%v in %q", err, tt.Texts())
	}
}

// pageProbe is an update source that notes the frames the Updates page
// draws it: only that page calls Extra.
type pageProbe struct {
	views.UpdateSource
	frame int
	drawn []int
}

func (p *pageProbe) ID() views.SourceID         { return views.SourceFeatures }
func (p *pageProbe) Title() string              { return "Probe" }
func (p *pageProbe) Icon() string               { return "application-x-addon" }
func (p *pageProbe) Blurb() string              { return "" }
func (p *pageProbe) Status() views.SourceStatus { return views.SourceStatus{Available: true} }
func (p *pageProbe) Items() []views.UpdateItem  { return nil }
func (p *pageProbe) Extra(*ui.Context)          { p.drawn = append(p.drawn, p.frame) }

func TestPaletteCommandDrawsItsPageInTheFrameThatRunsIt(t *testing.T) {
	m, loop := newTestMainLoop(t)
	probe := &pageProbe{}
	m.Store.SourcesOverride = func() []views.UpdateSource { return []views.UpdateSource{probe} }
	frame, selectedAt := 0, 0
	tt := ui.NewTester(func(c *ui.Context) {
		loop.Flush()
		frame++
		probe.frame = frame
		before := m.Shell.Selected
		m.View(c)
		if m.Shell.Selected != before {
			selectedAt = frame
		}
	}, 1360, 880)
	paletteChoose(t, tt, "Go to Updates")
	tt.Frame()
	if m.Shell.Selected != navigation.Updates || selectedAt == 0 {
		t.Fatalf("selected = %q at frame %d, want the Updates page", m.Shell.Selected, selectedAt)
	}
	if len(probe.drawn) == 0 || probe.drawn[0] != selectedAt {
		t.Errorf("the command ran in frame %d, the Updates page was drawn in frames %v", selectedAt, probe.drawn)
	}
}

func TestPaletteCommandToastShowsInTheFrameThatRunsIt(t *testing.T) {
	m, loop := newTestMainLoop(t)
	late := 0
	tt := ui.NewTester(func(c *ui.Context) {
		loop.Flush()
		m.View(c)
		// What the frame left behind waits for the next one.
		for _, toast := range m.Env.TakeToasts() {
			late++
			c.AddToast(toast)
		}
	}, 1360, 880)
	m.Env.Jobs.Enqueue(
		&frostui.Job{Title: "Cleaning", Run: func(ctx context.Context, _ *frostui.Progress) error { <-ctx.Done(); return nil }},
		&frostui.Job{Title: "Queued", Done: func(error) { m.Env.Toast("The queued job was dropped") }},
	)
	tt.Frame()
	paletteChoose(t, tt, "Stop the running jobs")
	tt.Frame()
	if !tt.HasText("The queued job was dropped") {
		t.Errorf("the command's toast does not show: %q", tt.Texts())
	}
	if late != 0 {
		t.Errorf("%d toasts waited a frame after the frame that ran the command", late)
	}
	waitIdle(t, m, tt)
}

func TestPaletteListsEveryVisiblePage(t *testing.T) {
	m, _ := newTestMain(t)
	titles := map[string]bool{}
	ids := map[string]bool{}
	for _, c := range m.Commands() {
		if ids[c.ID] {
			t.Errorf("command ID %q is used twice", c.ID)
		}
		ids[c.ID] = true
		titles[c.Title] = true
	}
	for _, item := range m.Items {
		if !titles["Go to "+item.Title] {
			t.Errorf("palette has no Go to %s", item.Title)
		}
	}
}

func TestLogoMatchesTheInstalledIcon(t *testing.T) {
	installed, err := os.ReadFile(filepath.Join("..", "..", "data", "icons", "hicolor", "symbolic", "apps", "org.frostyard.ChairLift-symbolic.svg"))
	if err != nil {
		t.Fatal(err)
	}
	if string(installed) != string(LogoSVG()) {
		t.Error("internal/window/assets/chairlift-symbolic.svg differs from the installed symbolic icon; copy it again")
	}
}

func TestPageConfirmationsAreDrawn(t *testing.T) {
	m, tt := newTestMain(t)
	confirmed := false
	m.Env.Dialogs.Ask(frostui.Confirm{Title: "Clean up Homebrew?", Action: "Clean", OnConfirm: func() { confirmed = true }})
	tt.Frame()
	if !tt.HasText("Clean up Homebrew?") {
		t.Fatalf("a page's confirmation is not drawn: %q", tt.Texts())
	}
	if err := tt.Click("Clean"); err != nil {
		t.Fatal(err)
	}
	if !confirmed {
		t.Error("confirming did not run the action")
	}
}

// The About dialog's copyright is LICENSE's line, with the sign for "(c)".
func TestAboutCopyrightMatchesTheLicense(t *testing.T) {
	license, err := os.ReadFile(filepath.Join("..", "..", "LICENSE"))
	if err != nil {
		t.Fatal(err)
	}
	want := ""
	for line := range strings.Lines(string(license)) {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "Copyright (c) "); ok {
			want = "© " + rest
			break
		}
	}
	if want == "" {
		t.Fatal("LICENSE has no copyright line")
	}
	m, tt := newTestMain(t)
	m.showAbout()
	tt.Frame()
	if !tt.HasText(want) {
		t.Errorf("the About dialog does not show %q: %q", want, tt.Texts())
	}
}
