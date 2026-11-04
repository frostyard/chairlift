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
	pending    ColorScheme
	hasPending bool
	running    bool
	idle       sync.WaitGroup
}

// NewSchemeWriter returns a writer that applies schemes with apply
// (ApplyColorScheme in production; tests inject a runner).
func NewSchemeWriter(apply func(ColorScheme) error) *SchemeWriter {
	return &SchemeWriter{apply: apply}
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
	w.idle.Add(1)
	go w.work()
}

// work writes pending values until none is left.
func (w *SchemeWriter) work() {
	defer w.idle.Done()
	for {
		w.mu.Lock()
		if !w.hasPending {
			w.running = false
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

// Wait blocks until the worker has no write left. Tests use it.
func (w *SchemeWriter) Wait() {
	w.idle.Wait()
}
