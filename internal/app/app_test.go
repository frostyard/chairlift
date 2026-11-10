package app

import (
	"errors"
	"log"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/frostyard/chairlift/internal/bootc"
	"github.com/frostyard/chairlift/internal/cmdlog"
	"github.com/frostyard/chairlift/internal/firstrun"
	"github.com/frostyard/chairlift/internal/flatpak"
	"github.com/frostyard/chairlift/internal/frostui"
	"github.com/frostyard/chairlift/internal/fwupd"
	"github.com/frostyard/chairlift/internal/homebrew"
	"github.com/frostyard/chairlift/internal/sysupdate"
	"github.com/frostyard/chairlift/internal/updex"
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
