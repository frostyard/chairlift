package views

import (
	"errors"
	"sync"
	"testing"

	"github.com/frostyard/chairlift/internal/sysinfo"
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
		s, ok := l.collect(true, &last)
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

	s, ok := l.collect(false, &last)
	if !ok || s.Disk != liveDisk {
		t.Fatalf("first reading = %+v, %v", s, ok)
	}
	l.apply(s)
	if _, ok := l.collect(false, &last); ok {
		t.Error("an unchanged disk was posted again")
	}
	f.setDisk(sysinfo.Disk{Path: liveDisk.Path, Free: liveDisk.Free + 500, Total: liveDisk.Total})
	if _, ok := l.collect(false, &last); ok {
		t.Error("a change the status bar cannot show was posted")
	}
	f.setDisk(sysinfo.Disk{Path: liveDisk.Path, Free: 250_000_000_000, Total: liveDisk.Total})
	s, ok = l.collect(false, &last)
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
	if _, ok := l.collect(false, &last); ok {
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
