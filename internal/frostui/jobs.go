package frostui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

func sprintf(format string, args ...any) string {
	if len(args) == 0 {
		return format
	}
	return fmt.Sprintf(format, args...)
}

// JobState is where a job is.
type JobState int

const (
	// JobQueued waits for the jobs before it.
	JobQueued JobState = iota
	// JobRunning runs now.
	JobRunning
	// JobDone ended without an error.
	JobDone
	// JobFailed ended with an error.
	JobFailed
	// JobCanceled was canceled before or while it ran.
	JobCanceled
)

// Ended reports whether the job is over.
func (s JobState) Ended() bool { return s >= JobDone }

// Job is one unit of work the queue runs: one command, one package, one
// device. Run runs on a goroutine of its own; Done runs on the UI thread
// after it, with Run's error.
type Job struct {
	// Title says what the job does, as "Updating Firefox".
	Title string
	// Source groups jobs, as the update source they belong to.
	Source string
	// Key identifies what the job works on within its source, so a view
	// can show the job's state on that item's row.
	Key string

	Run  func(ctx context.Context, p *Progress) error
	Done func(err error)

	// Unstoppable marks a job whose work a cancel cannot stop: one that
	// runs as root through pkexec (only the unprivileged parent can be
	// signaled) or flashes firmware. CancelAll drops the jobs queued after
	// it but lets it run to its end and report what really happened, and
	// the queue stays busy until then, so no other change overlaps it.
	Unstoppable bool

	// Set by the queue; read on the UI thread.
	ID       int
	State    JobState
	Err      error
	Fraction float64 // 0..1, or -1 while it cannot tell
	Status   string
	Started  time.Time
	Ended    time.Time
}

// Progress is how a running job reports, from its goroutine.
type Progress struct {
	job     *Job
	post    func(func())
	log     func(Level, string)
	mu      sync.Mutex
	value   float64
	text    string
	pending bool // a post that applies the latest value waits to run
}

// Set reports the fraction done (0..1, or -1 when unknown) and a status
// line. Calls are coalesced: a post rebuilds the whole view, and a job may
// report every output line, so at most one post waits at a time and it
// applies the latest value.
func (p *Progress) Set(fraction float64, status string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.value, p.text = fraction, status
	if p.pending {
		p.mu.Unlock()
		return
	}
	p.pending = true
	p.mu.Unlock()
	job := p.job
	p.post(func() {
		p.mu.Lock()
		job.Fraction, job.Status = p.value, p.text
		p.pending = false
		p.mu.Unlock()
	})
}

// Log adds a line to the console under the job's source.
func (p *Progress) Log(level Level, format string, args ...any) {
	if p == nil || p.log == nil {
		return
	}
	p.log(level, sprintf(format, args...))
}

// Jobs runs jobs one at a time, in the order they were queued, so that two
// changes never race (one privileged prompt at a time, one firmware flash at
// a time). Every method is for the UI thread.
type Jobs struct {
	post    func(func())
	console *Console

	queue   []*Job
	current *Job
	cancel  context.CancelFunc
	history []*Job
	nextID  int

	// batchTotal and batchDone count the jobs since the queue was last
	// empty, for "Job 3 of 10".
	batchTotal, batchDone int
	// stopping is set when CancelAll found an Unstoppable job running.
	stopping bool
	// OnIdle, when set, runs on the UI thread when the last queued job ends.
	OnIdle func(batch []*Job)
	batch  []*Job
}

// NewJobs returns a queue that hands work back to the UI thread with post
// (MyGo's Window.Update) and logs each job's start and end to console,
// which may be nil.
func NewJobs(post func(func()), console *Console) *Jobs {
	return &Jobs{post: post, console: console}
}

// Enqueue adds jobs at the end of the queue and starts the first when
// nothing runs.
func (q *Jobs) Enqueue(jobs ...*Job) {
	for _, j := range jobs {
		q.nextID++
		j.ID, j.State, j.Fraction = q.nextID, JobQueued, -1
		q.queue = append(q.queue, j)
		q.batchTotal++
		q.batch = append(q.batch, j)
	}
	q.next()
}

// Busy reports whether a job runs or waits.
func (q *Jobs) Busy() bool { return q.current != nil || len(q.queue) > 0 }

// Current is the running job, or nil.
func (q *Jobs) Current() *Job { return q.current }

// Queued are the jobs waiting, in order.
func (q *Jobs) Queued() []*Job { return append([]*Job(nil), q.queue...) }

// History are the ended jobs, newest last, at most 200.
func (q *Jobs) History() []*Job { return append([]*Job(nil), q.history...) }

// Batch returns how many jobs of the current batch ended and how many it
// has: a batch is every job queued since the queue was last idle.
func (q *Jobs) Batch() (done, total int) { return q.batchDone, q.batchTotal }

// BatchJobs returns the jobs of the current batch in the order they were
// queued: those that ended, the running one and those that wait. It is
// empty while the queue is idle, so a view that shows a run after it ended
// keeps its own copy.
func (q *Jobs) BatchJobs() []*Job { return append([]*Job(nil), q.batch...) }

// BatchFraction is the share of the batch done, counting the running job's
// own fraction.
func (q *Jobs) BatchFraction() float64 {
	if q.batchTotal == 0 {
		return 0
	}
	f := float64(q.batchDone)
	if q.current != nil && q.current.Fraction > 0 {
		f += q.current.Fraction
	}
	return f / float64(q.batchTotal)
}

// Find returns the newest job of source and key that is queued, running,
// or ended in the current batch; nil when there is none.
func (q *Jobs) Find(source, key string) *Job {
	if q.current != nil && q.current.Source == source && q.current.Key == key {
		return q.current
	}
	for _, j := range q.queue {
		if j.Source == source && j.Key == key {
			return j
		}
	}
	for i := len(q.batch) - 1; i >= 0; i-- {
		if j := q.batch[i]; j.Source == source && j.Key == key {
			return j
		}
	}
	return nil
}

// Pending reports whether a job of source and key waits or runs.
func (q *Jobs) Pending(source, key string) bool {
	j := q.Find(source, key)
	return j != nil && !j.State.Ended()
}

// CancelAll cancels the running job and drops the queued ones; each
// dropped job's Done runs with context.Canceled.
//
// An Unstoppable running job is not canceled: it finishes and reports its
// own result, and the queue stays busy until it does.
func (q *Jobs) CancelAll() {
	dropped := q.queue
	q.queue = nil
	for _, j := range dropped {
		q.finish(j, context.Canceled)
	}
	if q.cancel != nil && (q.current == nil || !q.current.Unstoppable) {
		q.cancel()
	}
	if q.current != nil && q.current.Unstoppable {
		q.stopping = true
	}
}

// Stopping reports whether a stop was asked while an Unstoppable job runs:
// it finishes first.
func (q *Jobs) Stopping() bool { return q.stopping && q.current != nil }

// CanStop reports whether CancelAll would stop the running job, false when
// nothing runs or the running job is Unstoppable.
func (q *Jobs) CanStop() bool { return q.current != nil && !q.current.Unstoppable }

func (q *Jobs) next() {
	if q.current != nil {
		return
	}
	if len(q.queue) == 0 {
		if q.batchTotal > 0 {
			batch := q.batch
			q.batch, q.batchTotal, q.batchDone = nil, 0, 0
			if q.OnIdle != nil {
				q.OnIdle(batch)
			}
		}
		return
	}
	j := q.queue[0]
	q.queue = q.queue[1:]
	q.current = j
	j.State, j.Started = JobRunning, time.Now()
	q.logf(LevelInfo, j.Source, "» %s", j.Title)
	ctx, cancel := context.WithCancel(context.Background())
	q.cancel = cancel
	p := &Progress{job: j, post: q.post}
	if q.console != nil {
		con, source := q.console, j.Source
		p.log = func(l Level, s string) { con.Add(l, source, s) }
	}
	run := j.Run
	go func() {
		var err error
		func() {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("%s: panic: %v", j.Title, r)
				}
			}()
			if run != nil {
				err = run(ctx, p)
			}
		}()
		// A job that finished despite a cancel (its command took no
		// context) did its work: its own error, nil included, stands.
		cancel()
		q.post(func() {
			q.current, q.cancel, q.stopping = nil, nil, false
			q.finish(j, err)
			q.next()
		})
	}()
}

func (q *Jobs) finish(j *Job, err error) {
	j.Err, j.Ended = err, time.Now()
	switch {
	case err == nil:
		j.State, j.Fraction = JobDone, 1
		q.logf(LevelInfo, j.Source, "✓ %s", j.Title)
	case errors.Is(err, context.Canceled):
		j.State = JobCanceled
		q.logf(LevelWarn, j.Source, "Canceled: %s", j.Title)
	default:
		j.State = JobFailed
		q.logf(LevelError, j.Source, "%s failed: %v", j.Title, err)
	}
	q.batchDone++
	q.history = append(q.history, j)
	if len(q.history) > 200 {
		q.history = q.history[len(q.history)-200:]
	}
	if j.Done != nil {
		j.Done(err)
	}
}

func (q *Jobs) logf(level Level, source, format string, args ...any) {
	if q.console != nil {
		q.console.Addf(level, source, format, args...)
	}
}

// Duration formats how long a job ran as a step's note: "8s", "1m 03s",
// "2h 05m".
func Duration(d time.Duration) string {
	s := int(d.Round(time.Second) / time.Second)
	if s < 60 {
		return fmt.Sprintf("%ds", s)
	}
	if s < 3600 {
		return fmt.Sprintf("%dm %02ds", s/60, s%60)
	}
	return fmt.Sprintf("%dh %02dm", s/3600, s%3600/60)
}

// RunSummary says how a batch of jobs ended, as "9 done · 1 failed · 2
// canceled"; jobs that did not end count for nothing, and it is "" when
// none ended.
func RunSummary(jobs []*Job) string {
	done, failed, canceled := 0, 0, 0
	for _, j := range jobs {
		switch j.State {
		case JobDone:
			done++
		case JobFailed:
			failed++
		case JobCanceled:
			canceled++
		}
	}
	var parts []string
	if done > 0 {
		parts = append(parts, fmt.Sprintf("%d done", done))
	}
	if failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", failed))
	}
	if canceled > 0 {
		parts = append(parts, fmt.Sprintf("%d canceled", canceled))
	}
	return strings.Join(parts, " · ")
}
