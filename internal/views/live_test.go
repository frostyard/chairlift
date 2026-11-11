package views

import (
	"errors"
	"sync"
	"testing"

	"github.com/frostyard/chairlift/internal/sysinfo"
	"github.com/frostyard/frostui/frostuitest"
)

// liveFake is a sampler's sources: every reading raises the processor use
// by one point.
type liveFake struct {
	mu    sync.Mutex
	n     int
	disk  sysinfo.Disk
	err   error
	fulls int
}

func (f *liveFake) next() sysinfo.Sample {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.n++
	f.fulls++
	return sysinfo.Sample{
		CPU:    float64(f.n),
		Memory: sysinfo.Memory{Total: 16_000_000_000, Available: 12_000_000_000},
		Disk:   f.disk,
	}
}

func (f *liveFake) readDisk() (sysinfo.Disk, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.disk, f.err
}

func (f *liveFake) setDisk(d sysinfo.Disk) {
	f.mu.Lock()
	f.disk = d
	f.mu.Unlock()
}

func newLiveWith(env *Env, f *liveFake) *Live {
	l := NewLive(env)
	l.next, l.disk = f.next, f.readDisk
	return l
}

var liveDisk = sysinfo.Disk{Path: "/var/home/frosty", Free: 288_000_000_000, Total: 512_000_000_000}

func TestLiveKeepsAMinuteOfHistory(t *testing.T) {
	env, _ := sysEnv(t, sysConfig("system_page/health_group"), false)
	f := &liveFake{disk: liveDisk}
	l := newLiveWith(env, f)
	l.history = true
	var last sysinfo.Disk
	for i := 0; i < 75; i++ {
		s, ok := l.sampler().collect(true, &last)
		if !ok {
			t.Fatalf("reading %d was not posted", i)
		}
		l.apply(s)
	}
	cpu, mem := l.CPU(), l.Memory()
	if len(cpu) != liveHistory || len(mem) != liveHistory {
		t.Fatalf("kept %d CPU and %d memory values, want %d", len(cpu), len(mem), liveHistory)
	}
	if cpu[0] != 16 || cpu[liveHistory-1] != 75 {
		t.Errorf("CPU oldest %v newest %v, want 16 and 75", cpu[0], cpu[liveHistory-1])
	}
	if mem[0] != 25 {
		t.Errorf("memory = %v%%, want 25%%", mem[0])
	}
	if got := l.Latest(); got.CPU != 75 || got.Disk != liveDisk {
		t.Errorf("Latest() = %+v", got)
	}
	// The slices are copies: a caller cannot change the history.
	cpu[0] = -1
	if l.CPU()[0] == -1 {
		t.Error("CPU() shares its backing array")
	}
}

func TestLiveWithoutHealthReadsOnlyTheDisk(t *testing.T) {
	env, _ := sysEnv(t, sysConfig(), false)
	f := &liveFake{disk: liveDisk}
	l := newLiveWith(env, f)
	var last sysinfo.Disk

	s, ok := l.sampler().collect(false, &last)
	if !ok || s.Disk != liveDisk {
		t.Fatalf("first reading = %+v, %v", s, ok)
	}
	l.apply(s)
	if _, ok := l.sampler().collect(false, &last); ok {
		t.Error("an unchanged disk was posted again")
	}
	f.setDisk(sysinfo.Disk{Path: liveDisk.Path, Free: liveDisk.Free + 500, Total: liveDisk.Total})
	if _, ok := l.sampler().collect(false, &last); ok {
		t.Error("a change the status bar cannot show was posted")
	}
	f.setDisk(sysinfo.Disk{Path: liveDisk.Path, Free: 250_000_000_000, Total: liveDisk.Total})
	s, ok = l.sampler().collect(false, &last)
	if !ok {
		t.Fatal("a visible change was not posted")
	}
	l.apply(s)
	if l.Latest().Disk.Free != 250_000_000_000 {
		t.Errorf("Latest().Disk = %+v", l.Latest().Disk)
	}
	if len(l.CPU()) != 0 || len(l.Memory()) != 0 || l.History() {
		t.Error("history kept without the health group")
	}
	if f.fulls != 0 {
		t.Errorf("read /proc %d times without the health group", f.fulls)
	}
	f.err = errors.New("statfs failed")
	if _, ok := l.sampler().collect(false, &last); ok {
		t.Error("a failed read was posted")
	}
}

func TestLiveStartSamplesOnItsOwnGoroutine(t *testing.T) {
	env, q := sysEnv(t, sysConfig("system_page/health_group"), false)
	f := &liveFake{disk: liveDisk}
	l := newLiveWith(env, f)
	l.Start()
	l.Start() // a second Start starts no second sampler
	q.until(t, "a sample", func() bool { return len(l.CPU()) > 0 })
	if !l.History() || l.Latest().Disk != liveDisk {
		t.Errorf("Latest() = %+v, history %v", l.Latest(), l.History())
	}
}

func TestLivePostsOnlyForVisibleChanges(t *testing.T) {
	q := frostuitest.NewLoop()
	env := NewEnv(sysConfig("system_page/health_group"), false, q.Post)
	f := &liveFake{disk: liveDisk}
	l := newLiveWith(env, f)
	l.history = true
	l.SetVisible(false)
	sampler := l.sampler()
	var last sysinfo.Disk
	// Run sampling on a worker; only the test's UI loop applies readings.
	sample := func(n int) {
		t.Helper()
		done := make(chan struct{})
		go func() {
			defer close(done)
			for range n {
				sampler.sample(true, &last)
			}
		}()
		<-done
	}
	if n := len(l.CPU()); n != 0 {
		t.Fatalf("history before sampling = %d", n)
	}
	sample(1)
	if n := q.Flush(); n != 1 {
		t.Fatalf("first disk reading posted %d times, want 1", n)
	}
	sample(75)
	if n := q.Flush(); n != 0 {
		t.Fatalf("hidden Overview posted %d times with unchanged disk", n)
	}
	if l.Latest().CPU != 1 || len(l.CPU()) != 1 {
		t.Fatal("sampling changed UI-owned history without a post")
	}
	l.SetVisible(true)
	if cpu := l.CPU(); len(cpu) != liveHistory || cpu[0] != 17 || cpu[liveHistory-1] != 76 {
		t.Fatalf("history on returning to Overview = %v", cpu)
	}
	if mem := l.Memory(); len(mem) != liveHistory || mem[0] != 25 {
		t.Fatalf("memory history = %v", mem)
	}
	if n := q.Flush(); n != 0 {
		t.Fatalf("navigation requested %d extra posts", n)
	}
	for range 3 {
		sample(1)
		if n := q.Flush(); n != 1 {
			t.Fatalf("visible Overview posted %d times for one sample", n)
		}
	}
	l.SetVisible(false)
	f.setDisk(sysinfo.Disk{Path: liveDisk.Path, Free: liveDisk.Free + 500, Total: liveDisk.Total})
	sample(1)
	if n := q.Flush(); n != 0 {
		t.Errorf("invisible disk change posted %d times", n)
	}
	f.setDisk(sysinfo.Disk{Path: liveDisk.Path, Free: 250_000_000_000, Total: liveDisk.Total})
	sample(1)
	if n := q.Flush(); n != 1 || l.Latest().Disk.Free != 250_000_000_000 {
		t.Fatalf("changed disk: posts %d, latest %+v", n, l.Latest())
	}
}

func TestLiveCoalescesSamplesWhileAPostWaits(t *testing.T) {
	q := frostuitest.NewLoop()
	env := NewEnv(sysConfig("system_page/health_group"), false, q.Post)
	l := newLiveWith(env, &liveFake{disk: liveDisk})
	l.history = true
	sampler := l.sampler()
	var last sysinfo.Disk
	for range 75 {
		sampler.sample(true, &last)
	}
	if n := q.Pending(); n != 1 {
		t.Fatalf("pending posts = %d, want 1", n)
	}
	// Navigate away and back before the posted callback runs. Navigation
	// takes the latest history and the callback must not duplicate it.
	l.SetVisible(false)
	l.SetVisible(true)
	q.Flush()
	if cpu := l.CPU(); len(cpu) != liveHistory || cpu[0] != 16 || cpu[liveHistory-1] != 75 {
		t.Fatalf("history after delayed post = %v", cpu)
	}
	sampler.sample(true, &last)
	if n := q.Flush(); n != 1 || l.Latest().CPU != 76 {
		t.Fatalf("next sample: posts %d, latest %+v", n, l.Latest())
	}
}

func TestLiveDisabledHealthNeverPostsForHistory(t *testing.T) {
	q := frostuitest.NewLoop()
	env := NewEnv(sysConfig(), false, q.Post)
	f := &liveFake{disk: liveDisk}
	l := newLiveWith(env, f)
	sampler := l.sampler()
	var last sysinfo.Disk
	for _, visible := range []bool{true, false, true} {
		l.SetVisible(visible)
		sampler.sample(false, &last)
		q.Flush()
	}
	if q.Ran() != 1 || f.fulls != 0 || len(l.CPU()) != 0 {
		t.Fatalf("posts %d, full reads %d, history %v", q.Ran(), f.fulls, l.CPU())
	}
}

func TestLiveNavigationDuringSampling(t *testing.T) {
	q := frostuitest.NewLoop()
	env := NewEnv(sysConfig("system_page/health_group"), false, q.Post)
	l := newLiveWith(env, &liveFake{disk: liveDisk})
	l.history = true
	sampler := l.sampler()
	done := make(chan struct{})
	go func() {
		defer close(done)
		var last sysinfo.Disk
		for range 1000 {
			sampler.sample(true, &last)
		}
	}()
	for range 1000 {
		l.SetVisible(false)
		q.Flush()
		l.SetVisible(true)
	}
	<-done
	l.SetVisible(true)
	q.Flush()
	cpu := l.CPU()
	if len(cpu) != liveHistory {
		t.Fatalf("kept %d readings, want %d", len(cpu), liveHistory)
	}
	for i, value := range cpu {
		if value != float64(941+i) {
			t.Fatalf("history out of order or duplicated: %v", cpu)
		}
	}
}
