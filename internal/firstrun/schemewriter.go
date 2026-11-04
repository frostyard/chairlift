package firstrun

import (
	"log"
	"sync"
)

// SchemeWriter applies color-scheme choices one at a time, in order, and the
// latest choice wins. The Style step calls Request from the GTK main thread
// on each toggle. One worker goroutine writes; a request that arrives while
// a write runs replaces the pending value, so a quick Dark, Light, Dark,
// Light ends with Light and runs at most two writes. Separate goroutines per
// toggle could finish out of order and leave the wrong scheme.
type SchemeWriter struct {
	apply func(ColorScheme) error

	mu         sync.Mutex
	idle       *sync.Cond // signalled when the worker stops
	pending    ColorScheme
	hasPending bool
	running    bool
}

// NewSchemeWriter returns a writer that applies schemes with apply
// (ApplyColorScheme in production; tests inject a runner).
func NewSchemeWriter(apply func(ColorScheme) error) *SchemeWriter {
	w := &SchemeWriter{apply: apply}
	w.idle = sync.NewCond(&w.mu)
	return w
}

// Request records scheme as the latest choice and starts the worker when it
// is not running. It does not block.
func (w *SchemeWriter) Request(scheme ColorScheme) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pending, w.hasPending = scheme, true
	if w.running {
		return
	}
	w.running = true
	go w.work()
}

// work writes pending values until none is left.
func (w *SchemeWriter) work() {
	for {
		w.mu.Lock()
		if !w.hasPending {
			w.running = false
			w.idle.Broadcast()
			w.mu.Unlock()
			return
		}
		scheme := w.pending
		w.hasPending = false
		w.mu.Unlock()

		if err := w.apply(scheme); err != nil {
			log.Printf("first-run: color scheme: %v", err)
		}
	}
}

// Wait blocks until the worker has no write left: the running write and the
// pending latest choice are both done. It returns at once when nothing is
// running. The Progress sequence (RunProgress) calls it from the worker
// goroutine before it records completion, so the last choice is written
// before Exit can end the process. Safe for concurrent use with Request.
func (w *SchemeWriter) Wait() {
	w.mu.Lock()
	defer w.mu.Unlock()
	for w.running {
		w.idle.Wait()
	}
}
