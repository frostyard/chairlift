package views

import (
	"os"
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
// its sampler runs on a goroutine of its own for the app's life.
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
}

// NewLive returns the sampler; it starts with Start.
func NewLive(env *Env) *Live {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "/"
	}
	sampler := sysinfo.NewSampler(home)
	return &Live{
		env:  env,
		next: sampler.Next,
		disk: func() (sysinfo.Disk, error) { return sampler.Statfs(sampler.DiskPath) },
	}
}

// Start begins sampling. It runs once; the sampler lives as long as the app.
func (l *Live) Start() {
	if l.started {
		return
	}
	l.started = true
	l.history = l.env.Enabled("system_page", "health_group")
	go l.run(l.history)
}

// run samples until the process ends. It reads no state: each reading goes
// to the UI thread through Env.Post.
func (l *Live) run(history bool) {
	every := 5 * time.Second
	if history {
		every = time.Second
	}
	tick := time.NewTicker(every)
	defer tick.Stop()
	var last sysinfo.Disk
	for {
		if sample, ok := l.collect(history, &last); ok {
			l.env.Post(func() { l.apply(sample) })
		}
		<-tick.C
	}
}

// collect takes one reading on the sampling goroutine. Without history it
// reads only the disk and reports ok only when the free space shown would
// change, so an idle window is not redrawn for nothing.
func (l *Live) collect(history bool, last *sysinfo.Disk) (sysinfo.Sample, bool) {
	if history {
		s := l.next()
		*last = s.Disk
		return s, true
	}
	d, err := l.disk()
	if err != nil {
		return sysinfo.Sample{}, false
	}
	changed := d.Total != last.Total || sysinfo.FormatBytes(d.Free) != sysinfo.FormatBytes(last.Free)
	*last = d
	return sysinfo.Sample{Disk: d}, changed
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
