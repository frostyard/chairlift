package window

import (
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
	m := NewMain(env, store)
	tt := ui.NewTester(func(c *ui.Context) {
		loop.Flush()
		m.View(c)
	}, 1360, 880)
	return m, tt
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
