package window

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/chairlift/internal/config"
	"github.com/frostyard/chairlift/internal/frostui"
	"github.com/frostyard/chairlift/internal/views"
)

func newTestMain(t *testing.T) (*Main, *ui.Tester) {
	t.Helper()
	frostui.SetIcons(frostui.NewIcons(map[string][]byte{"org.frostyard.ChairLift-symbolic": LogoSVG()}))
	var tt *ui.Tester
	env := views.NewEnv(&config.Config{}, true, func(fn func()) { fn() })
	m := NewMain(env, views.NewStore(env))
	tt = ui.NewTester(m.View, 1360, 880)
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
