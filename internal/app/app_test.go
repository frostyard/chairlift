package app

import (
	"sync"
	"testing"
)

type fakeWindow struct {
	mu  sync.Mutex
	ran []string
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
