// Package frostuitest helps test frostui views and the models behind them
// without a window. Loop stands in for the UI thread: workers post to it
// from any goroutine, as they would through MyGo's Window.Update, and the
// test runs what they posted on its own goroutine, as the UI thread would.
//
// It imports only the standard library, so it leaves ChairLift with
// frostui.
package frostuitest

import (
	"sync"
	"testing"
	"time"
)

// Timeout is how long RunUntil waits for its condition before it fails the
// test.
const Timeout = 5 * time.Second

// Loop queues functions posted from any goroutine and runs them on the
// test's goroutine. The zero value is ready to use.
type Loop struct {
	mu   sync.Mutex
	fns  []func()
	wake chan struct{}
	ran  int
}

// NewLoop returns an empty loop.
func NewLoop() *Loop { return &Loop{} }

// waker is the channel a post signals, made on first use.
func (l *Loop) waker() chan struct{} {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.wake == nil {
		l.wake = make(chan struct{}, 1)
	}
	return l.wake
}

// Post queues fn; any goroutine may call it. Pass it where the app passes
// its window's Update.
func (l *Loop) Post(fn func()) {
	w := l.waker()
	l.mu.Lock()
	l.fns = append(l.fns, fn)
	l.mu.Unlock()
	select {
	case w <- struct{}{}:
	default:
	}
}

// Flush runs what was posted, and what that posts, until nothing is left,
// and returns how many functions ran.
func (l *Loop) Flush() int {
	n := 0
	for {
		l.mu.Lock()
		fns := l.fns
		l.fns = nil
		l.mu.Unlock()
		if len(fns) == 0 {
			return n
		}
		for _, fn := range fns {
			fn()
			n++
			l.mu.Lock()
			l.ran++
			l.mu.Unlock()
		}
	}
}

// RunUntil runs what is posted until cond holds, checking it after each
// flush, and fails the test after Timeout. cond runs on the test's
// goroutine, so it may read what the posted functions write. It also
// checks cond every millisecond, for a condition a worker meets without
// posting.
func (l *Loop) RunUntil(t testing.TB, cond func() bool) {
	t.Helper()
	w := l.waker()
	deadline := time.After(Timeout)
	for {
		l.Flush()
		if cond() {
			return
		}
		select {
		case <-w:
		case <-time.After(time.Millisecond):
		case <-deadline:
			t.Fatal("frostuitest: timed out waiting for the UI thread's work")
		}
	}
}

// Settle runs what is posted until nothing comes for quiet: the workers
// ended and their results are applied.
func (l *Loop) Settle(quiet time.Duration) {
	w := l.waker()
	for {
		l.Flush()
		select {
		case <-w:
		case <-time.After(quiet):
			if l.Pending() == 0 {
				return
			}
		}
	}
}

// Pending is how many posted functions wait.
func (l *Loop) Pending() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.fns)
}

// Ran is how many posted functions ran so far.
func (l *Loop) Ran() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.ran
}
