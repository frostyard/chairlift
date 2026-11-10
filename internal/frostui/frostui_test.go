package frostui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/chairlift/internal/frostui/frostuitest"
)

func TestMatchIsAnOrderedSubsequence(t *testing.T) {
	if _, ok := Match("upev", "Update everything"); !ok {
		t.Error(`"upev" does not match "Update everything"`)
	}
	if _, ok := Match("xyz", "Update everything"); ok {
		t.Error(`"xyz" matches "Update everything"`)
	}
	if _, ok := Match("", "anything"); !ok {
		t.Error("the empty query does not match")
	}
	start, _ := Match("upd", "Update everything")
	middle, _ := Match("upd", "Stage the system update")
	if start <= middle {
		t.Errorf("a match at the start (%d) does not beat one in the middle (%d)", start, middle)
	}
}

func TestFilterRanksAndGroups(t *testing.T) {
	cmds := []Command{
		{ID: "a", Title: "Go to Updates", Group: "Go to"},
		{ID: "b", Title: "Update everything", Group: "Actions"},
		{ID: "c", Title: "Clean up Homebrew", Group: "Actions", Hint: "brew cleanup"},
		{ID: "d", Title: "Go to Apps", Group: "Go to"},
	}
	all := Filter(cmds, "  ")
	var ids []string
	for _, c := range all {
		ids = append(ids, c.ID)
	}
	if strings.Join(ids, "") != "adbc" {
		t.Errorf("empty query order = %v, want grouped in first-seen group order", ids)
	}
	hits := Filter(cmds, "update")
	if len(hits) != 2 || hits[0].ID != "b" {
		t.Errorf("Filter(update) = %+v, want Update everything first", hits)
	}
	if got := Filter(cmds, "brew cl"); len(got) != 1 || got[0].ID != "c" {
		t.Errorf("a hint match is missing: %+v", got)
	}
}

func TestConsoleKeepsTheNewestLines(t *testing.T) {
	c := NewConsole(16)
	notified := 0
	c.SetNotify(func() { notified++ })
	for i := 0; i < 20; i++ {
		c.Addf(LevelOutput, "brew", "line %d", i)
	}
	c.Add(LevelError, "flatpak", "first\nsecond\n")
	lines := c.Lines(nil, "")
	if len(lines) != 16 {
		t.Fatalf("kept %d lines, want 16", len(lines))
	}
	if lines[0].Text != "line 6" || lines[15].Text != "second" {
		t.Errorf("oldest %q newest %q", lines[0].Text, lines[15].Text)
	}
	if notified != 21 {
		t.Errorf("notified %d times, want once per Add (21)", notified)
	}
	if _, errs := c.Counts(); errs != 2 {
		t.Errorf("errors = %d, want 2", errs)
	}
	only := LevelSet(1 << LevelError)
	if got := c.Lines(&only, ""); len(got) != 2 {
		t.Errorf("error filter kept %d lines", len(got))
	}
	if got := c.Lines(nil, "FLATPAK"); len(got) != 2 {
		t.Errorf("source filter kept %d lines", len(got))
	}
	// lines 6 to 19 are kept; "line 1" is in lines 10 to 19
	if got := c.Lines(nil, "line 1"); len(got) != 10 {
		t.Errorf("text filter kept %d lines, want 10", len(got))
	}
	text := Text(c.Lines(&only, ""))
	if !strings.Contains(text, "Error  [flatpak] first") {
		t.Errorf("Text = %q", text)
	}
	c.Clear()
	if len(c.Lines(nil, "")) != 0 || c.Seq() != 22 {
		t.Errorf("Clear kept lines or reset the sequence")
	}
	if w, e := c.Counts(); w != 0 || e != 0 {
		t.Error("Clear kept the counts")
	}
}

func TestLevelSet(t *testing.T) {
	s := DefaultLevels
	if s.Has(LevelDebug) || !s.Has(LevelError) {
		t.Errorf("DefaultLevels = %b", s)
	}
	s.Toggle(LevelDebug)
	if !s.Has(LevelDebug) {
		t.Error("Toggle did not add Debug")
	}
}

func TestConsoleWriterClassifies(t *testing.T) {
	c := NewConsole(16)
	w := c.Writer(LevelDebug, "log")
	w.Classify = func(s string) Level {
		if strings.Contains(s, "[DRY-RUN]") {
			return LevelDryRun
		}
		return LevelDebug
	}
	_, _ = w.Write([]byte("[DRY-RUN] would run brew upgrade\n"))
	_, _ = w.Write([]byte("window: built\n"))
	lines := c.Lines(nil, "")
	if len(lines) != 2 || lines[0].Level != LevelDryRun || lines[1].Level != LevelDebug {
		t.Errorf("lines = %+v", lines)
	}
}

func TestJobsRunOneAtATimeInOrder(t *testing.T) {
	sp := frostuitest.NewLoop()
	con := NewConsole(64)
	q := NewJobs(sp.Post, con)
	var order []string
	var running, maxRunning int
	var mu sync.Mutex
	work := func(name string, err error) *Job {
		return &Job{
			Title: name, Source: "brew", Key: name,
			Run: func(ctx context.Context, p *Progress) error {
				mu.Lock()
				running++
				maxRunning = max(maxRunning, running)
				mu.Unlock()
				p.Set(0.5, "half")
				p.Log(LevelOutput, "working on %s", name)
				time.Sleep(5 * time.Millisecond)
				mu.Lock()
				running--
				mu.Unlock()
				return err
			},
			Done: func(error) { order = append(order, name) },
		}
	}
	var idle []*Job
	q.OnIdle = func(b []*Job) { idle = b }
	failure := errors.New("boom")
	q.Enqueue(work("a", nil), work("b", failure))
	q.Enqueue(work("c", nil))
	if !q.Busy() || !q.Pending("brew", "c") {
		t.Fatal("queue is not busy after Enqueue")
	}
	if done, total := q.Batch(); done != 0 || total != 3 {
		t.Errorf("Batch = %d of %d", done, total)
	}
	sp.RunUntil(t, func() bool { return idle != nil })
	if strings.Join(order, "") != "abc" {
		t.Errorf("order = %v", order)
	}
	if maxRunning != 1 {
		t.Errorf("%d jobs ran at once", maxRunning)
	}
	if idle[1].State != JobFailed || !errors.Is(idle[1].Err, failure) || idle[0].State != JobDone {
		t.Errorf("states = %v %v", idle[0].State, idle[1].State)
	}
	if q.Busy() || q.Find("brew", "a") != nil {
		t.Error("an idle queue still reports jobs")
	}
	if len(q.History()) != 3 {
		t.Errorf("history has %d jobs", len(q.History()))
	}
	if got := con.Lines(nil, "working on b"); len(got) != 1 {
		t.Errorf("job log line missing: %v", con.Lines(nil, ""))
	}
	if got := con.Lines(nil, "b failed: boom"); len(got) != 1 {
		t.Errorf("failure line missing")
	}
}

func TestJobsCancelAll(t *testing.T) {
	sp := frostuitest.NewLoop()
	q := NewJobs(sp.Post, nil)
	started := make(chan struct{})
	var results []error
	q.Enqueue(&Job{Title: "long", Run: func(ctx context.Context, _ *Progress) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}, Done: func(err error) { results = append(results, err) }},
		&Job{Title: "next", Run: func(context.Context, *Progress) error { t.Error("a dropped job ran"); return nil },
			Done: func(err error) { results = append(results, err) }})
	<-started
	q.CancelAll()
	sp.RunUntil(t, func() bool { return !q.Busy() })
	if len(results) != 2 || !errors.Is(results[0], context.Canceled) || !errors.Is(results[1], context.Canceled) {
		t.Errorf("results = %v", results)
	}
}

func TestJobsRecoverAPanic(t *testing.T) {
	sp := frostuitest.NewLoop()
	q := NewJobs(sp.Post, nil)
	var got error
	q.Enqueue(&Job{Title: "bad", Run: func(context.Context, *Progress) error { panic("oops") }, Done: func(err error) { got = err }})
	sp.RunUntil(t, func() bool { return got != nil })
	if !strings.Contains(got.Error(), "panic: oops") {
		t.Errorf("err = %v", got)
	}
}

func TestStateOf(t *testing.T) {
	cases := map[JobState]StepState{JobQueued: StepQueued, JobRunning: StepRunning, JobDone: StepDone, JobFailed: StepFailed, JobCanceled: StepSkipped}
	for js, want := range cases {
		if got := StateOf(&Job{State: js}); got != want {
			t.Errorf("StateOf(%v) = %v, want %v", js, got, want)
		}
	}
	if StateOf(nil) != StepReady {
		t.Error("StateOf(nil) is not ready")
	}
}

func TestDialogsAskInOrderAndRunTheChoice(t *testing.T) {
	var d Dialogs
	var got []string
	d.Ask(Confirm{Title: "Turn on Docker?", Action: "Turn on", OnConfirm: func() { got = append(got, "docker") }})
	d.Ask(Confirm{Title: "Trust the tap?", Action: "Trust", OnCancel: func() { got = append(got, "cancel") }})
	tt := ui.NewTester(func(c *ui.Context) { d.View(c) }, 800, 600)
	if !tt.HasText("Turn on Docker?") || tt.HasText("Trust the tap?") {
		t.Fatalf("first dialog not alone: %q", tt.Texts())
	}
	if err := tt.Click("Turn on"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Trust the tap?") {
		t.Fatalf("second dialog missing: %q", tt.Texts())
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "docker,cancel" || d.Showing() {
		t.Errorf("got %v, showing %v", got, d.Showing())
	}
}

func TestPaletteRunsTheChosenCommand(t *testing.T) {
	var s Shell
	s.Brand = Brand{Name: "Test"}
	ran := ""
	cmds := []Command{
		{ID: "u", Title: "Update everything", Group: "Actions", Run: func() { ran = "u" }},
		{ID: "x", Title: "Disabled thing", Group: "Actions", Disabled: true, Reason: "nothing to do", Run: func() { ran = "x" }},
	}
	view := func(c *ui.Context) {
		s.View(c, Frame{
			Nav:      []NavSection{{Title: "System", Items: []NavItem{{ID: "overview", Title: "Overview", Icon: "view-grid"}}}},
			Content:  func() { Title(c, "Page") },
			Commands: cmds,
		})
	}
	tt := ui.NewTester(view, 1280, 800)
	if err := tt.Click("Search or run a command"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Update everything") {
		t.Fatalf("palette did not open: %q", tt.Texts())
	}
	if err := tt.Click("Disabled thing"); err != nil {
		t.Fatal(err)
	}
	if ran != "" || !s.Palette.Open {
		t.Fatalf("a disabled command ran (%q) or closed the palette", ran)
	}
	if err := tt.Click("Update everything"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(100 * time.Millisecond)
	if ran != "u" {
		t.Errorf("ran = %q, want u", ran)
	}
}

func TestShellShowsRailStatusAndConsole(t *testing.T) {
	var s Shell
	s.Brand = Brand{Name: "ChairLift", Tagline: "Snow Linux command center"}
	s.Selected = "overview"
	con := NewConsole(32)
	con.Add(LevelCommand, "brew", "brew outdated --json=v2")
	clicked := false
	view := func(c *ui.Context) {
		s.View(c, Frame{
			Nav: []NavSection{{Title: "System", Items: []NavItem{
				{ID: "overview", Title: "Overview", Icon: "view-grid"},
				{ID: "updates", Title: "Updates", Icon: "software-update-available", Badge: "12", BadgeTone: Accent},
			}}},
			Content:     func() { Title(c, s.Selected) },
			Status:      []StatusItem{{Text: "Snow 2026.11.05", Progress: -1}},
			StatusEnd:   []StatusItem{{Text: "12 updates", Progress: -1, OnClick: func() { clicked = true }}},
			ConsoleData: con,
		})
	}
	tt := ui.NewTester(view, 1280, 800)
	for _, want := range []string{"ChairLift", "Overview", "Updates", "12", "Snow 2026.11.05", "12 updates"} {
		if !tt.HasText(want) {
			t.Errorf("missing %q in %q", want, tt.Texts())
		}
	}
	if err := tt.Click("Updates"); err != nil {
		t.Fatal(err)
	}
	if s.Selected != "updates" {
		t.Errorf("Selected = %q after clicking Updates", s.Selected)
	}
	if err := tt.Click("12 updates"); err != nil {
		t.Fatal(err)
	}
	if !clicked {
		t.Error("status item click did not run")
	}
	if err := tt.Click("Show or hide the console (Ctrl J)"); err != nil {
		t.Fatal(err)
	}
	if !s.Console.Open || !tt.HasText("$ brew outdated --json=v2") {
		t.Errorf("console did not open with its line: %q", tt.Texts())
	}
}

func TestLabeledButtonBuildsOneButton(t *testing.T) {
	clicks := 0
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Row(c).Children(func() {
			if LabeledButton(c, "view-refresh", "Check now", true).Clicked() {
				clicks++
			}
		})
	}, 400, 200)
	if _, ok := tt.AccessByRole(ui.RoleButton, "Check now"); !ok {
		t.Fatal("no Check now button")
	}
	if err := tt.Click("Check now"); err != nil || clicks != 1 {
		t.Errorf("click: %v, clicks %d", err, clicks)
	}
}

func TestJobsKeepAFinishedJobsResultAfterACancel(t *testing.T) {
	sp := frostuitest.NewLoop()
	q := NewJobs(sp.Post, nil)
	started, release := make(chan struct{}), make(chan struct{})
	var got []error
	q.Enqueue(&Job{Title: "no context", Run: func(context.Context, *Progress) error {
		close(started)
		<-release // a command that takes no context runs to its end
		return nil
	}, Done: func(err error) { got = append(got, err) }})
	<-started
	q.CancelAll()
	close(release)
	sp.RunUntil(t, func() bool { return !q.Busy() })
	if len(got) != 1 || got[0] != nil {
		t.Errorf("results = %v, want the job's own nil: it did its work", got)
	}
}

func TestJobCardStopCancelsTheQueue(t *testing.T) {
	sp := frostuitest.NewLoop()
	q := NewJobs(sp.Post, nil)
	started := make(chan struct{})
	var got []error
	done := func(err error) { got = append(got, err) }
	q.Enqueue(&Job{Title: "Updating Firefox", Run: func(ctx context.Context, _ *Progress) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}, Done: done}, &Job{Title: "Updating gh", Run: func(context.Context, *Progress) error { return nil }, Done: done})
	<-started
	tt := ui.NewTester(func(c *ui.Context) { JobCard(c, q) }, 400, 200)
	if err := tt.Click("Stop: drop the queued jobs and cancel this one"); err != nil {
		t.Fatal(err)
	}
	sp.RunUntil(t, func() bool { return !q.Busy() })
	if len(got) != 2 || !errors.Is(got[0], context.Canceled) || !errors.Is(got[1], context.Canceled) {
		t.Errorf("results = %v, want both canceled", got)
	}
}

func TestStopLetsAnUnstoppableJobFinishFirst(t *testing.T) {
	sp := frostuitest.NewLoop()
	q := NewJobs(sp.Post, nil)
	started, release := make(chan struct{}), make(chan struct{})
	var mu sync.Mutex
	ran := []string{}
	var results []error
	q.Enqueue(&Job{Title: "Staging the system update", Unstoppable: true, Run: func(ctx context.Context, _ *Progress) error {
		close(started)
		<-release // root work goes on whatever the context says
		if ctx.Err() != nil {
			t.Error("an Unstoppable job's context was canceled")
		}
		mu.Lock()
		ran = append(ran, "stage")
		mu.Unlock()
		return nil
	}, Done: func(err error) { results = append(results, err) }},
		&Job{Title: "Updating Firefox", Run: func(context.Context, *Progress) error {
			mu.Lock()
			ran = append(ran, "firefox")
			mu.Unlock()
			return nil
		}, Done: func(err error) { results = append(results, err) }})
	<-started
	if q.CanStop() {
		t.Error("CanStop is true for an Unstoppable job")
	}
	tt := ui.NewTester(func(c *ui.Context) { JobCard(c, q) }, 500, 200)
	if err := tt.Click("Stop: drop the queued jobs; this one cannot be stopped and finishes first"); err != nil {
		t.Fatal(err)
	}
	sp.Flush()
	if !q.Busy() || !q.Stopping() {
		t.Errorf("busy %v stopping %v: the queue freed while root work runs", q.Busy(), q.Stopping())
	}
	tt.Frame()
	if !tt.HasText("Finishing; the queued jobs were dropped") {
		t.Errorf("the card does not say the job finishes first: %q", tt.Texts())
	}
	close(release)
	sp.RunUntil(t, func() bool { return !q.Busy() })
	mu.Lock()
	defer mu.Unlock()
	if strings.Join(ran, ",") != "stage" {
		t.Errorf("ran %v, want only the stage: the queued job was dropped", ran)
	}
	if len(results) != 2 || !errors.Is(results[0], context.Canceled) || results[1] != nil {
		t.Errorf("results = %v, want the dropped job canceled and the stage's own nil", results)
	}
	if q.Stopping() {
		t.Error("Stopping outlived the job")
	}
}

func TestProgressSetIsCoalesced(t *testing.T) {
	sp := frostuitest.NewLoop()
	q := NewJobs(sp.Post, nil)
	done := make(chan struct{})
	q.Enqueue(&Job{Title: "chatty", Run: func(_ context.Context, p *Progress) error {
		for i := range 500 {
			p.Set(float64(i)/500, fmt.Sprintf("line %d", i))
		}
		close(done)
		return nil
	}})
	<-done
	if n := sp.Pending(); n > 2 {
		t.Errorf("500 Set calls queued %d posts, want at most one pending", n)
	}
	sp.Flush()
	if cur := q.Current(); cur != nil && cur.Status != "line 499" {
		t.Errorf("status = %q, want the latest", cur.Status)
	}
	sp.RunUntil(t, func() bool { return !q.Busy() })
}
