package frostui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/chairlift/internal/frostui/frostuitest"
)

// The tests of the pieces the page ports first wrote as helpers of their
// own (docs/design/frostui.md).

func TestDurationFormats(t *testing.T) {
	for d, want := range map[time.Duration]string{
		0: "0s", 8 * time.Second: "8s", 1400 * time.Millisecond: "1s", 63 * time.Second: "1m 03s",
		3723 * time.Second: "1h 02m",
	} {
		if got := Duration(d); got != want {
			t.Errorf("Duration(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestRunSummaryCountsEndedJobs(t *testing.T) {
	jobs := []*Job{{State: JobDone}, {State: JobFailed}, {State: JobDone}, {State: JobCanceled}, {State: JobRunning}}
	if got := RunSummary(jobs); got != "2 done · 1 failed · 1 canceled" {
		t.Errorf("RunSummary = %q", got)
	}
	if got := RunSummary([]*Job{{State: JobQueued}}); got != "" {
		t.Errorf("RunSummary of a batch that did not end = %q", got)
	}
}

func TestBatchJobsKeepsQueueOrderUntilIdle(t *testing.T) {
	loop := frostuitest.NewLoop()
	q := NewJobs(loop.Post, nil)
	gate := make(chan struct{})
	job := func(key string) *Job {
		return &Job{Title: key, Source: "s", Key: key, Run: func(context.Context, *Progress) error {
			if key == "a" {
				<-gate
			}
			return nil
		}}
	}
	q.Enqueue(job("a"), job("b"))
	q.Enqueue(job("c"))
	keys := func() string {
		var out []string
		for _, j := range q.BatchJobs() {
			out = append(out, j.Key)
		}
		return strings.Join(out, "")
	}
	if got := keys(); got != "abc" {
		t.Errorf("BatchJobs while a runs = %q", got)
	}
	close(gate)
	loop.RunUntil(t, func() bool { return !q.Busy() })
	if got := keys(); got != "" {
		t.Errorf("BatchJobs of an idle queue = %q", got)
	}
}

func TestSquareAvatarShowsInitials(t *testing.T) {
	for in, want := range map[string]string{"Mission Center": "MC", "fzf": "F", "JetBrains Mono Nerd Font": "JM", "": "?", "élan vital": "ÉV"} {
		if got := Initials(in); got != want {
			t.Errorf("Initials(%q) = %q, want %q", in, got, want)
		}
	}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Row(c).Children(func() { SquareAvatar(c, "Mission Center", 40) })
	}, 200, 100)
	if r, ok := tt.Find("MC"); !ok || r.W > 40 {
		t.Errorf("avatar text %v %v: %q", r, ok, tt.Texts())
	}
}

func TestPillsShowTheirText(t *testing.T) {
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Row(c).Gap(SpaceS).Children(func() {
			ColorPill(c, "Cask", ui.RGB(120, 60, 200))
			DotPill(c, "Update", c.Theme().Accent).Tooltip("Update to 2.0")
		})
	}, 300, 100)
	for _, want := range []string{"Cask", "Update"} {
		if !tt.HasText(want) {
			t.Errorf("missing %q in %q", want, tt.Texts())
		}
	}
}

func TestPillTabsChooseATab(t *testing.T) {
	sel := 0
	tabs := []Tab{{Title: "Installed", Icon: "package-x-generic"}, {Title: "Bundles", Icon: "view-list-bullet"}}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Row(c).Children(func() { PillTabs(c, &sel, tabs) })
	}, 400, 100)
	if _, ok := tt.AccessByRole(ui.RoleTab, "Bundles"); !ok {
		t.Fatalf("no Bundles tab: %q", tt.Texts())
	}
	if err := tt.Click("Bundles"); err != nil {
		t.Fatal(err)
	}
	if sel != 1 {
		t.Errorf("selected = %d after clicking Bundles", sel)
	}
}

func TestSoftSelectionTintsOnlyTheChoice(t *testing.T) {
	for _, base := range []*ui.Theme{ui.LightTheme(), ui.DarkTheme()} {
		accent, text := base.Accent, base.Text
		s := SoftSelection(base)
		if s.Accent == accent || s.InactiveAccent != s.Accent || s.AccentText != text {
			t.Errorf("dark=%v: accent %v inactive %v text %v", base.Dark, s.Accent, s.InactiveAccent, s.AccentText)
		}
		if base.Accent != accent || s.Background != base.Background {
			t.Error("SoftSelection changed the theme it was given, or more than the selection")
		}
	}
}

func TestKeyValuesStyleSetsTheKeyColumn(t *testing.T) {
	valueX := func(style ...KeyValuesStyle) float32 {
		tt := ui.NewTester(func(c *ui.Context) {
			KeyValues(c, []KeyValue{{Key: "ID", Value: "org.example.App", Mono: true}, {Key: "Origin", Value: "flathub"}}, style...)
		}, 400, 200)
		r, ok := tt.Find("flathub")
		if !ok {
			t.Fatalf("no value: %q", tt.Texts())
		}
		return r.X
	}
	wide, narrow := valueX(), valueX(KeyValuesStyle{KeyWidth: 72, Gap: 10, FontSize: 12.5})
	if wide-narrow < 40 || wide-narrow > 56 {
		t.Errorf("value at %v by default and %v with a 72 DIP key column", wide, narrow)
	}
}

func TestSectionHeaderLinesUp(t *testing.T) {
	tt := ui.NewTester(func(c *ui.Context) {
		SectionHeader(c, SectionHead{Icon: "computer", Title: "System", Note: "From snosi-health", Aside: "2 hidden", Quiet: true})
		Section(c, "", "Apps", "", func() { Body(c, "content") })
	}, 600, 200)
	for _, want := range []string{"System", "From snosi-health", "2 hidden", "Apps", "content"} {
		if !tt.HasText(want) {
			t.Errorf("missing %q in %q", want, tt.Texts())
		}
	}
	note, _ := tt.Find("From snosi-health")
	aside, _ := tt.Find("2 hidden")
	if aside.X < 500 || aside.X < note.X+note.W {
		t.Errorf("the aside (%v) does not sit at the end, after the note (%v)", aside, note)
	}
}

func TestJobStatusShowsEachState(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status RowStatus
		texts  []string
		access string
		none   bool
	}{
		{name: "queued", status: RowStatus{State: StepQueued, Text: "Queued", Label: "Queued: Firefox", Compact: true}, texts: []string{"Queued"}, access: "Queued: Firefox"},
		{name: "running", status: RowStatus{State: StepRunning, Text: "Cleaning…"}, texts: []string{"Cleaning…"}, access: "Cleaning…"},
		{name: "done pill", status: RowStatus{State: StepDone, Text: "Updated", Compact: true}, texts: []string{"Updated"}},
		{name: "done", status: RowStatus{State: StepDone, Text: "Cleaned", Tooltip: "Cleaned at 09:30"}, texts: []string{"Cleaned"}},
		{name: "failed icon", status: RowStatus{State: StepFailed, Text: "Failed", Tooltip: "signature mismatch", Compact: true}, access: "Failed"},
		{name: "failed", status: RowStatus{State: StepFailed, Text: "Failed", Tooltip: "exit 1"}, texts: []string{"Failed"}},
		{name: "note", status: RowStatus{State: StepSkipped, Text: "Previewed"}, texts: []string{"Previewed"}},
		{name: "nothing", status: RowStatus{}, none: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tt := ui.NewTester(func(c *ui.Context) {
				ui.Row(c).Gap(6).Children(func() { JobStatus(c, tc.status) })
			}, 300, 80)
			for _, want := range tc.texts {
				if !tt.HasText(want) {
					t.Errorf("missing %q in %q", want, tt.Texts())
				}
			}
			if tc.access != "" {
				if _, ok := tt.Access(tc.access); !ok {
					t.Errorf("nothing is named %q: %q", tc.access, tt.Texts())
				}
			}
			if tc.none && len(tt.Texts()) != 0 {
				t.Errorf("an empty status drew %q", tt.Texts())
			}
		})
	}
}

func TestVersionChangeShowsTheArrowBetweenTwo(t *testing.T) {
	draw := func(from, to string) *ui.Tester {
		return ui.NewTester(func(c *ui.Context) {
			ui.Row(c).Children(func() { VersionChange(c, from, to) })
		}, 300, 60)
	}
	if tt := draw("1.0", "2.0"); !tt.HasText("1.0") || !tt.HasText("→") || !tt.HasText("2.0") {
		t.Errorf("both: %q", tt.Texts())
	}
	if tt := draw("", "2.0"); tt.HasText("→") || !tt.HasText("2.0") {
		t.Errorf("new only: %q", tt.Texts())
	}
}

func TestCommandRowRunsItsAction(t *testing.T) {
	clicks := 0
	tt := ui.NewTester(func(c *ui.Context) {
		CommandRow(c, CommandItem{
			Icon: "user-trash", Title: "Clean up Homebrew", Command: "brew cleanup --prune=all", Admin: true,
			Status: func() { JobStatus(c, RowStatus{State: StepDone, Text: "Cleaned"}) },
			Action: func() {
				if ui.Button(c, "Clean").Label("Clean: Clean up Homebrew").Clicked() {
					clicks++
				}
			},
		})
	}, 700, 80)
	for _, want := range []string{"Clean up Homebrew", "brew cleanup --prune=all", "admin", "Cleaned"} {
		if !tt.HasText(want) {
			t.Errorf("missing %q in %q", want, tt.Texts())
		}
	}
	if err := tt.Click("Clean: Clean up Homebrew"); err != nil || clicks != 1 {
		t.Errorf("click: %v, clicks %d", err, clicks)
	}
}

func TestStepTileShowsItsValue(t *testing.T) {
	tt := ui.NewTester(func(c *ui.Context) {
		StepTile(c, Tile{Icon: "object-select", Caption: "Booted · running now", Value: "2026.11.05", Detail: "sha256:0a1b2c3d",
			DetailTooltip: "sha256:0a1b2c3d4e5f", Pill: "pinned", PillIcon: "changes-prevent", Current: true})
		StepTile(c, Tile{Icon: "document-revert", Caption: "Rollback", Value: "none", Dim: true})
	}, 500, 200)
	for _, want := range []string{"Booted · running now", "2026.11.05", "sha256:0a1b2c3d", "pinned", "Rollback", "none"} {
		if !tt.HasText(want) {
			t.Errorf("missing %q in %q", want, tt.Texts())
		}
	}
}

func TestSwitchRowReportsAChange(t *testing.T) {
	model, changes := false, 0
	tt := ui.NewTester(func(c *ui.Context) {
		on := model
		if SwitchRow(c, "Send problem reports", "Off: nothing is sent", &on).Changed() {
			model = on
			changes++
		}
		SwitchRow(c, "Plain", "", new(bool))
	}, 400, 120)
	sw, ok := tt.AccessByRole(ui.RoleSwitch, "Send problem reports")
	if !ok || sw.Checked {
		t.Fatalf("switch %+v %v", sw, ok)
	}
	if !tt.HasText("Off: nothing is sent") || !tt.HasText("Plain") {
		t.Errorf("texts %q", tt.Texts())
	}
	tt.ClickAt(sw.Bounds.X+sw.Bounds.W/2, sw.Bounds.Y+sw.Bounds.H/2)
	if !model || changes != 1 {
		t.Errorf("model %v after %d changes", model, changes)
	}
}

func TestDisclosureCardOpensInTheFrameOfTheClick(t *testing.T) {
	open, toggles := false, 0
	tt := ui.NewTester(func(c *ui.Context) {
		if DisclosureCard(c.Key("disk"), &open, Disclosure{
			Icon: "dialog-warning", Title: "Disk almost full", Subtitle: "sda1 · today",
			Trailing: func() { Pill(c, "3×", Warning) },
			Body:     func() { Body(c, "Free some space") },
		}) {
			toggles++
		}
	}, 600, 300)
	if tt.HasText("Free some space") || !tt.HasText("3×") {
		t.Fatalf("closed card: %q", tt.Texts())
	}
	if err := tt.Click("Disk almost full"); err != nil {
		t.Fatal(err)
	}
	if !open || toggles != 1 || !tt.HasText("Free some space") {
		t.Errorf("open %v, toggles %d, texts %q", open, toggles, tt.Texts())
	}
	if head, ok := tt.Access("Disk almost full"); !ok || !head.Expanded {
		t.Errorf("head %+v", head)
	}
}

func TestTileGridWraps(t *testing.T) {
	names := []string{"first", "second", "third"}
	tt := ui.NewTester(func(c *ui.Context) {
		TileGrid(c).Children(func() {
			for i, name := range names {
				ui.Box(c.Key(i)).Size(150, 40).Label(name).Role(ui.RoleImage)
			}
		})
	}, 400, 200)
	var at []ui.Rect
	for _, name := range names {
		info, ok := tt.Access(name)
		if !ok {
			t.Fatalf("no %s tile", name)
		}
		at = append(at, info.Bounds)
	}
	if at[1].Y != at[0].Y || at[1].X <= at[0].X || at[2].Y <= at[0].Y {
		t.Errorf("tiles at %v: want two on the first line and the third wrapped", at)
	}
}

func TestCodeDialogReportsActionAndCancel(t *testing.T) {
	open, acted, canceled := true, 0, 0
	tt := ui.NewTester(func(c *ui.Context) {
		if !open {
			return
		}
		a, x := CodeDialog(c, "Report this problem?", "This is the exact report.", "kind: crash\n  code: 1", "Send")
		if a {
			acted++
			open = false
		}
		if x {
			canceled++
			open = false
		}
	}, 900, 700)
	if !tt.HasText("Report this problem?") || !tt.HasText("This is the exact report.") {
		t.Fatalf("dialog: %q", tt.Texts())
	}
	if err := tt.Click("Send"); err != nil {
		t.Fatal(err)
	}
	if acted != 1 || canceled != 0 || open {
		t.Errorf("acted %d canceled %d open %v", acted, canceled, open)
	}
	open = true
	tt.Frame()
	tt.Key(0, ui.KeyEscape)
	if canceled != 1 || open {
		t.Errorf("Escape: canceled %d, open %v", canceled, open)
	}
}

func TestHeroAndStepDots(t *testing.T) {
	tt := ui.NewTester(func(c *ui.Context) {
		StepDots(c, 5, 1)
		Hero(c, HeroIcon{Name: "network-offline", Tone: Warning}, "No internet", "Connect later.")
		Hero(c, HeroIcon{Busy: true}, "Checking", "")
	}, 600, 500)
	if _, ok := tt.AccessByRole(ui.RoleProgress, "2 / 5"); !ok {
		t.Errorf("no step indicator at 2 / 5: %q", tt.Texts())
	}
	if _, ok := tt.AccessByRole(ui.RoleHeading, "No internet"); !ok {
		t.Errorf("no heading: %q", tt.Texts())
	}
	for _, want := range []string{"Connect later.", "Checking"} {
		if !tt.HasText(want) {
			t.Errorf("missing %q in %q", want, tt.Texts())
		}
	}
}

func TestTopToastsCloseWithTheirButton(t *testing.T) {
	added := false
	tt := ui.NewTester(func(c *ui.Context) {
		if !added {
			c.AddToast(ui.Toast{Title: "The configuration has an error", Type: "error", Timeout: -1})
			added = true
		}
		TopToasts(c, "Close")
		Body(c, "page")
	}, 800, 400)
	r, ok := tt.Find("The configuration has an error")
	if !ok || r.Y > 100 {
		t.Fatalf("toast at %v (%v): %q", r, ok, tt.Texts())
	}
	if err := tt.Click("Close"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if tt.HasText("The configuration has an error") {
		t.Error("the toast stayed after Close")
	}
}

func TestLaunchDesktopAppStartsGtkLaunch(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "args")
	// Only shell builtins: PATH holds nothing but this script.
	script := "#!/bin/sh\necho \"$@\" > " + out + "\n"
	if err := os.WriteFile(filepath.Join(dir, "gtk-launch"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	if err := LaunchDesktopApp("org.example.App"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		data, err := os.ReadFile(out)
		if err == nil && strings.HasSuffix(string(data), "\n") {
			if got := strings.TrimSpace(string(data)); got != "org.example.App" {
				t.Errorf("gtk-launch got %q", got)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("gtk-launch did not run")
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Setenv("PATH", t.TempDir())
	if err := LaunchDesktopApp("org.example.App"); err == nil || !errors.Is(err, os.ErrNotExist) && !strings.Contains(err.Error(), "not found") {
		t.Errorf("without gtk-launch: %v", err)
	}
}
