package views

import (
	"os"
	"sync"
	"time"

	"github.com/frostyard/chairlift/internal/sysinfo"
)

// liveHistory is how many samples the live chart keeps: one minute.
const liveHistory = 60

// Live samples processor, memory and disk. With the Overview's health
// group on, it samples everything each second and keeps a minute of
// processor and memory history for the live chart; otherwise it reads only
// the free space of the home directory's file system (which the status bar
// and Maintenance show) every few seconds. It belongs to the UI thread;
// its sampler runs on a goroutine of its own for the app's life. Only the
// synchronized sample buffer is shared with that goroutine.
type Live struct {
	env *Env

	// next takes a full reading and disk reads only the file system; both
	// run on the sampling goroutine. NewLive sets them from a
	// sysinfo.Sampler of the home directory; tests replace them.
	next func() sysinfo.Sample
	disk func() (sysinfo.Disk, error)

	started bool
	history bool
	latest  sysinfo.Sample
	cpu     []float64
	mem     []float64
	buffer  liveBuffer
}

// NewLive returns the sampler; it starts with Start.
func NewLive(env *Env) *Live {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "/"
	}
	sampler := sysinfo.NewSampler(home)
	return &Live{
		buffer: liveBuffer{visible: true},
		env:    env,
		next:   sampler.Next,
		disk:   func() (sysinfo.Disk, error) { return sampler.Statfs(sampler.DiskPath) },
	}
}

// Start begins sampling. It runs once; the sampler lives as long as the app.
func (l *Live) Start() {
	if l.started {
		return
	}
	l.started = true
	l.history = l.env.Enabled("system_page", "health_group")
	sampler := l.sampler()
	go sampler.run(l.history)
}

// liveSampler holds only worker inputs copied before starting the goroutine.
type liveSampler struct {
	next    func() sysinfo.Sample
	disk    func() (sysinfo.Disk, error)
	publish func(sysinfo.Sample)
}

func (l *Live) sampler() liveSampler {
	buffer, post, apply := &l.buffer, l.env.Post, l.flush
	history := l.history
	return liveSampler{next: l.next, disk: l.disk, publish: func(s sysinfo.Sample) {
		if buffer.add(s, history) {
			post(apply)
		}
	}}
}

// run samples until the process ends, without reading UI-owned state.
func (s liveSampler) run(history bool) {
	every := 5 * time.Second
	if history {
		every = time.Second
	}
	tick := time.NewTicker(every)
	defer tick.Stop()
	var last sysinfo.Disk
	for {
		s.sample(history, &last)
		<-tick.C
	}
}

func (s liveSampler) sample(history bool, last *sysinfo.Disk) {
	if reading, ok := s.collect(history, last); ok {
		s.publish(reading)
	}
}

// collect reads every full sample, or just visible disk changes when the
// health group is disabled.
func (s liveSampler) collect(history bool, last *sysinfo.Disk) (sysinfo.Sample, bool) {
	if history {
		return s.next(), true
	}
	d, err := s.disk()
	if err != nil {
		return sysinfo.Sample{}, false
	}
	changed := diskChanged(d, *last)
	*last = d
	return sysinfo.Sample{Disk: d}, changed
}

func diskChanged(a, b sysinfo.Disk) bool {
	return a.Total != b.Total || sysinfo.FormatBytes(a.Free) != sysinfo.FormatBytes(b.Free)
}

// liveBuffer transfers at most a minute of samples to the UI thread. The
// mutex protects every field; the worker never touches Live's UI history.
type liveBuffer struct {
	mu      sync.Mutex
	visible bool
	posted  bool
	last    sysinfo.Disk
	samples []sysinfo.Sample
}

func (b *liveBuffer) add(s sysinfo.Sample, history bool) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	changed := diskChanged(s.Disk, b.last)
	b.last = s.Disk
	if len(b.samples) == liveHistory {
		copy(b.samples, b.samples[1:])
		b.samples = b.samples[:liveHistory-1]
	}
	b.samples = append(b.samples, s)
	visibleHistory := history && b.visible
	if b.posted || (!changed && !visibleHistory) {
		return false
	}
	b.posted = true
	return true
}

// SetVisible tells the sampler whether the Overview is selected. It runs
// on the UI thread, taking buffered history immediately when returning to
// Overview, without requesting an extra frame for navigation itself.
func (l *Live) SetVisible(visible bool) {
	l.buffer.mu.Lock()
	l.buffer.visible = visible
	var samples []sysinfo.Sample
	if visible {
		samples, l.buffer.samples = l.buffer.samples, nil
	}
	l.buffer.mu.Unlock()
	for _, s := range samples {
		l.apply(s)
	}
}

// flush is posted at most once at a time; it drains the newest samples on
// the UI thread even when several readings arrived before the post ran.
func (l *Live) flush() {
	l.buffer.mu.Lock()
	samples := l.buffer.samples
	l.buffer.samples = nil
	l.buffer.posted = false
	l.buffer.mu.Unlock()
	for _, s := range samples {
		l.apply(s)
	}
}

// apply keeps a reading on the UI thread.
func (l *Live) apply(s sysinfo.Sample) {
	l.latest = s
	if !l.history {
		return
	}
	l.cpu = appendWindow(l.cpu, s.CPU)
	l.mem = appendWindow(l.mem, s.Memory.UsedPercent())
}

func appendWindow(values []float64, v float64) []float64 {
	values = append(values, v)
	if len(values) > liveHistory {
		values = append(values[:0], values[len(values)-liveHistory:]...)
	}
	return values
}

// Latest is the newest sample. Without the health group only its Disk is
// filled.
func (l *Live) Latest() sysinfo.Sample { return l.latest }

// History reports whether the processor and memory history is kept: the
// health group is on.
func (l *Live) History() bool { return l.history }

// CPU and Memory are the last 60 seconds, oldest first, 0 to 100; empty
// without the health group.
func (l *Live) CPU() []float64    { return append([]float64(nil), l.cpu...) }
func (l *Live) Memory() []float64 { return append([]float64(nil), l.mem...) }
