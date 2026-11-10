package frostuitest

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestLoopRunsPostsOnTheTestGoroutine(t *testing.T) {
	var l Loop
	done := 0
	for range 3 {
		go l.Post(func() { done++ })
	}
	l.RunUntil(t, func() bool { return done == 3 })
	if l.Ran() != 3 || l.Pending() != 0 {
		t.Errorf("ran %d, pending %d", l.Ran(), l.Pending())
	}
}

func TestFlushRunsWhatPostsPost(t *testing.T) {
	l := NewLoop()
	var order []int
	l.Post(func() {
		order = append(order, 1)
		l.Post(func() { order = append(order, 2) })
	})
	if n := l.Flush(); n != 2 || len(order) != 2 || order[1] != 2 {
		t.Errorf("Flush ran %d: %v", n, order)
	}
	if n := l.Flush(); n != 0 {
		t.Errorf("an empty Flush ran %d", n)
	}
}

func TestRunUntilSeesAWorkerThatDoesNotPost(t *testing.T) {
	var l Loop
	var flag atomic.Bool
	go func() {
		time.Sleep(5 * time.Millisecond)
		flag.Store(true)
	}()
	l.RunUntil(t, flag.Load)
}

func TestSettleWaitsForAQuietSpell(t *testing.T) {
	var l Loop
	var got atomic.Int32
	go func() {
		for range 3 {
			time.Sleep(2 * time.Millisecond)
			l.Post(func() { got.Add(1) })
		}
	}()
	l.Settle(50 * time.Millisecond)
	if got.Load() != 3 {
		t.Errorf("settled after %d posts, want 3", got.Load())
	}
}
