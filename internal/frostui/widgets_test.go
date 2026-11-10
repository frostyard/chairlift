package frostui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// hasAll fails t for each text tt does not show.
func hasAll(t *testing.T, tt *ui.Tester, texts ...string) {
	t.Helper()
	for _, s := range texts {
		if !tt.HasText(s) {
			t.Errorf("missing %q in %q", s, tt.Texts())
		}
	}
}

func TestCardsHeadersAndStates(t *testing.T) {
	acted, closed, figure := 0, 0, 0
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Gap(SpaceS).AlignItems(ui.Stretch).Children(func() {
			PageHeader(c, "Updates", "3 ready", func() { ui.Button(c, "Check now") })
			Card(c).Children(func() {
				CardHeader(c, "System image", "Staged with bootc", func() { Pill(c, "staged", Accent) })
				Loading(c, "Checking for updates")
				Problem(c, "brew outdated failed: exit 1")
				Empty(c, "object-select", "Up to date", "Nothing waits.")
			})
			a, x := Banner(c, Warning, "changes-prevent", "A tap is not trusted.", "Trust tap", true)
			if a {
				acted++
			}
			if x {
				closed++
			}
			Figures(c, []Figure{
				{Label: "Updates", Value: "12", Note: "across 5 sources", Tone: Accent, OnClick: func() { figure++ }},
				{Label: "Free space", Value: "288 GB"},
			})
		})
	}, 1000, 900)
	hasAll(t, tt, "Updates", "3 ready", "Check now", "System image", "Staged with bootc", "staged",
		"Checking for updates", "brew outdated failed: exit 1", "Up to date", "Nothing waits.",
		"A tap is not trusted.", "12", "across 5 sources", "288 GB")
	if err := tt.Click("Trust tap"); err != nil || acted != 1 {
		t.Errorf("banner action: %v, acted %d", err, acted)
	}
	if err := tt.Click("Dismiss"); err != nil || closed != 1 {
		t.Errorf("banner close: %v, closed %d", err, closed)
	}
	if err := tt.Click("Updates: 12"); err != nil || figure != 1 {
		t.Errorf("figure click: %v, clicks %d", err, figure)
	}
}

func TestChartsAndGauges(t *testing.T) {
	clicks := 0
	tt := ui.NewTesterWith(func(c *ui.Context) {
		ui.Column(c).Fill().Gap(SpaceS).AlignItems(ui.Stretch).Children(func() {
			LiveChart(c, "Processor and memory", 120,
				Series{Name: "CPU", Values: []float64{5, 40, 120, -3}, Tone: Accent},
				Series{Name: "Memory", Values: []float64{50}, Tone: Warning}) // too short: skipped
			Legend(c, Accent, "CPU 12%")
			Gauge(c, 288, 512, "288 GB", "free", Success, 160)
			Gauge(c, 1, 0, "?", "no total", Neutral, 80)
			SegmentBar(c, "Disk", 100, []Segment{{Label: "Home", Value: 60, Tone: Accent}, {Label: "Empty", Value: 0}}, func(s Segment) string {
				return s.Label + " part"
			})
			SegmentBar(c, "Nothing", 0, nil, nil)
			if ToggleTile(c, "application-x-addon", "Docker", "On · 28.5", true, false) {
				clicks++
			}
			if ToggleTile(c, "application-x-addon", "Incus", "Working…", false, true) {
				clicks += 10
			}
		})
	}, 900, 1000, ui.TesterOptions{Dark: true})
	hasAll(t, tt, "CPU 12%", "288 GB", "free", "Home part", "Docker", "Incus")
	for _, label := range []string{"Processor and memory", "free: 288 GB", "Disk"} {
		if _, ok := tt.Find(label); !ok {
			t.Errorf("no element named %q", label)
		}
	}
	if err := tt.Click("Docker"); err != nil || clicks != 1 {
		t.Errorf("tile click: %v, clicks %d", err, clicks)
	}
	if err := tt.Click("Incus"); err != nil || clicks != 1 {
		t.Errorf("a busy tile took a click: %v, clicks %d", err, clicks)
	}
}

func TestPipelineShowsEveryStepState(t *testing.T) {
	stages := []Stage{
		{Title: "System image", Icon: "security-high", Note: "1m 03s", Steps: []Step{{Title: "Snow Linux", State: StepDone}}},
		{Title: "Flatpak", Icon: "package-x-generic", Steps: []Step{
			{Title: "Firefox", State: StepRunning, Note: "9s"}, {Title: "Bazaar", State: StepQueued}, {Title: "Inkscape", State: StepReady},
		}},
		{Title: "Homebrew", Icon: "utilities-terminal", Steps: []Step{
			{Title: "gh", State: StepFailed}, {Title: "bluefin-cli", State: StepHeld}, {Title: "go", State: StepSkipped},
		}},
		{Title: "Features", Icon: "application-x-addon"},
		{Title: "Firmware", Icon: "drive-harddisk", Empty: "Waits for you"},
	}
	tt := ui.NewTester(func(c *ui.Context) { Pipeline(c, stages) }, 1300, 500)
	hasAll(t, tt, "System image", "1m 03s", "Firefox", "9s", "Bazaar", "Inkscape", "gh", "bluefin-cli", "go",
		"Nothing to do", "Waits for you")
	for _, label := range []string{"Firefox, running", "Bazaar, queued", "Inkscape, ready", "gh, failed", "bluefin-cli, held", "go, skipped", "Snow Linux, done"} {
		if _, ok := tt.Find(label); !ok {
			t.Errorf("no step named %q", label)
		}
	}
}

func TestJobCardShowsTheBatch(t *testing.T) {
	q := NewJobs(func(func()) {}, nil)
	tt := ui.NewTester(func(c *ui.Context) { JobCard(c, q) }, 400, 200)
	if len(tt.Texts()) != 0 {
		t.Errorf("an idle queue drew %q", tt.Texts())
	}
	block := make(chan struct{})
	t.Cleanup(func() { close(block) })
	q.Enqueue(&Job{Title: "Updating Firefox", Run: func(_ context.Context, _ *Progress) error { <-block; return nil }},
		&Job{Title: "Updating gh"})
	tt.Frame()
	hasAll(t, tt, "Updating Firefox", "Job 1 of 2")
	q.Current().Status = "Downloading 3 of 5"
	tt.Frame()
	hasAll(t, tt, "Downloading 3 of 5")
}

func TestDialogsShowShortcutsAndAbout(t *testing.T) {
	var info Info
	tt := ui.NewTester(func(c *ui.Context) { info.View(c) }, 900, 700)
	if len(tt.Texts()) != 0 {
		t.Errorf("a closed dialog drew %q", tt.Texts())
	}
	info.Show("Keyboard shortcuts", ShortcutsBody([]ShortcutEntry{
		{Group: "Navigation", Title: "Go to Updates", Keys: []string{"Alt", "2"}},
		{Group: "General", Title: "Quit", Keys: []string{"Ctrl", "Q"}},
	}))
	tt.Frame()
	hasAll(t, tt, "Keyboard shortcuts", "Navigation", "Go to Updates", "Alt", "2", "General", "Quit")
	if err := tt.Click("Close"); err != nil || info.Open {
		t.Errorf("close: %v, open %v", err, info.Open)
	}
	info.Show("About ChairLift", AboutBody(About{
		Name: "ChairLift", Version: "v0.12.0", Summary: "The system manager of Snow Linux.", Logo: "computer",
		Website: "https://example.org/site", Issues: "https://example.org/issues", License: "GNU GPL 3.0",
		Copyright: "© Frostyard", Developers: []string{"Brian", "Contributors"},
	}))
	tt.Frame()
	hasAll(t, tt, "ChairLift", "v0.12.0", "The system manager of Snow Linux.", "Developers", "Brian", "Contributors",
		"GNU GPL 3.0", "© Frostyard", "Website", "Report an issue")
}

// writeIcon writes an SVG icon at root/rel.
func writeIcon(t *testing.T, root, rel string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	svg := `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16"><rect width="16" height="16"/></svg>`
	if err := os.WriteFile(path, []byte(svg), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestThemeIconsFindFallbackAndEmbedded(t *testing.T) {
	data := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_DIRS", data+":"+data) // the second copy is deduped
	t.Setenv("FROSTUI_ICON_THEME", "Frost")
	writeIcon(t, data, "icons/Frost/symbolic/actions/edit-copy-symbolic.svg") // the theme's own
	writeIcon(t, data, "icons/breeze/actions/16/go-next.svg")                 // a Breeze layout
	writeIcon(t, data, "icons/hicolor/scalable/apps/app.svg")                 // the last fallback

	ic := NewIcons(map[string][]byte{"brand-symbolic": []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16"><circle cx="8" cy="8" r="8"/></svg>`)})
	if ic.Theme != "Frost" {
		t.Errorf("Theme = %q, want the FROSTUI_ICON_THEME override", ic.Theme)
	}
	for _, name := range []string{"edit-copy", "edit-copy-symbolic", "go-next", "app", "brand", "brand-symbolic"} {
		if ic.Get(name) == nil {
			t.Errorf("Get(%q) = nil", name)
		}
	}
	if ic.Get("no-such-icon") != nil || ic.Get("") != nil {
		t.Error("a missing icon is not nil")
	}
	if _, cached := ic.cache["no-such-icon"]; !cached {
		t.Error("a missing icon is not cached")
	}
	var none *Icons
	if none.Get("edit-copy") != nil {
		t.Error("a nil Icons returned an icon")
	}

	SetIcons(ic)
	t.Cleanup(func() { SetIcons(nil) })
	if IconSVG("go-next") == nil {
		t.Error("IconSVG does not read the kit's icons")
	}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Row(c).Children(func() {
			Icon(c, "go-next").Label("next")
			Icon(c, "no-such-icon").Label("placeholder")
		})
	}, 200, 100)
	icon, ok1 := tt.Find("next")
	hole, ok2 := tt.Find("placeholder")
	if !ok1 || !ok2 || icon.W != hole.W || icon.H != hole.H || icon.W == 0 {
		t.Errorf("icon %+v and missing icon %+v differ in size: layouts would move", icon, hole)
	}
}

func TestDesktopIconThemeFromKDE(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FROSTUI_ICON_THEME", "")
	t.Setenv("XDG_CURRENT_DESKTOP", "KDE")
	if got := DesktopIconTheme(); got != "breeze" {
		t.Errorf("KDE without kdeglobals = %q, want breeze", got)
	}
	cfg := filepath.Join(home, ".config")
	if err := os.MkdirAll(cfg, 0o755); err != nil {
		t.Fatal(err)
	}
	globals := "[General]\nTheme=wrong\n[Icons]\nTheme=breeze-dark\n"
	if err := os.WriteFile(filepath.Join(cfg, "kdeglobals"), []byte(globals), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := DesktopIconTheme(); got != "breeze-dark" {
		t.Errorf("KDE theme = %q, want breeze-dark", got)
	}
	if got := strings.Join(dedupe([]string{"a", "b", "a", "c", "b"}), ","); got != "a,b,c" {
		t.Errorf("dedupe = %q", got)
	}
}

func TestSetAppIDNamesTheProgram(t *testing.T) {
	err := SetAppID("org.frostyard.Test", "Test")
	if err != nil && strings.Contains(err.Error(), "libglib") {
		t.Skipf("no GLib here: %v", err)
	}
	if err != nil {
		t.Fatalf("SetAppID: %v", err)
	}
	// Only the first call counts, as GLib allows.
	if err := SetAppID("", ""); err != nil {
		t.Errorf("a second call = %v, want the first call's nil", err)
	}
	if b := cString("abc"); b == nil || *b != 'a' {
		t.Error("cString does not copy the text")
	}
}
