package reporting

import (
	"sync"
)

// WriteFunc writes 1 consent to the consent file. SaveConsent is the
// production implementation; a test injects a slow or failing writer.
type WriteFunc func(c Consent) error

// ConsentResult is the outcome of 1 consent write.
type ConsentResult struct {
	// Gen is the generation of the choice that was written.
	Gen uint64
	// Consent is the value the writer attempted.
	Consent Consent
	// Err is the write failure, or nil.
	Err error
	// Restored is true when the write failed and no newer choice existed, so
	// the writer put the last persisted value back as the wanted value.
	Restored bool
	// Persisted is the value that was on disk when the write ended.
	Persisted Consent
}

// ConsentWriter writes the consent one choice at a time, and the latest
// choice wins. The Problems page and the first run call Set on the GTK main
// thread for each switch or mode change; 1 worker goroutine writes, and a
// choice that arrives while a write runs replaces the pending value.
//
// A goroutine per choice could finish out of order and leave the wrong
// consent on disk, and a fast On then Off could end with On. Set therefore
// stores the wanted value at once: a send that calls Wanted sees a
// withdrawal before its next request, whatever the write goroutine is doing.
type ConsentWriter struct {
	write   WriteFunc
	onDone  func(ConsentResult)
	mu      sync.Mutex
	idle    *sync.Cond // signalled when the worker stops
	wanted  Consent
	gen     uint64
	onDisk  Consent
	pending bool
	running bool
	closed  bool
}

// NewConsentWriter returns a writer for initial (the consent read from the
// file) that writes with write, and reports each write's result to onDone,
// which runs on the worker goroutine, so a caller with widgets wraps it in
// sgtk.RunOnMainThread. write and onDone may be nil: nil write keeps the
// wanted value only (a dry run), a nil onDone discards results.
func NewConsentWriter(initial Consent, write WriteFunc, onDone func(ConsentResult)) *ConsentWriter {
	if write == nil {
		write = func(Consent) error { return nil }
	}
	w := &ConsentWriter{write: write, onDone: onDone, wanted: initial, onDisk: initial}
	w.idle = sync.NewCond(&w.mu)
	return w
}

// Set makes c the wanted consent at once and queues its write. It returns
// the generation of the choice; a caller compares it with Current to learn
// whether a result is still the newest choice. It does not block.
func (w *ConsentWriter) Set(c Consent) uint64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.gen++
	w.wanted = c
	w.pending = true
	gen := w.gen
	if w.running || w.closed {
		return gen
	}
	w.running = true
	go w.work()
	return gen
}

// Wanted returns the consent the user chose last, whether or not it reached
// the file. A send reads this before every request, so a withdrawal takes
// effect at once.
func (w *ConsentWriter) Wanted() Consent {
	if w == nil {
		return Consent{}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.wanted
}

// Current returns the generation of the latest wanted consent.
func (w *ConsentWriter) Current() uint64 {
	if w == nil {
		return 0
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.gen
}

// Persisted returns the consent that is known to be on disk.
func (w *ConsentWriter) Persisted() Consent {
	if w == nil {
		return Consent{}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.onDisk
}

// Wait blocks until the worker has no write left: the running write and the
// pending latest choice are both done. It returns at once when nothing is
// running. A test uses it; the page does not need to.
func (w *ConsentWriter) Wait() {
	w.mu.Lock()
	defer w.mu.Unlock()
	for w.running {
		w.idle.Wait()
	}
}

// Close stops the worker after the writes it already has. A later Set keeps
// the wanted value and writes nothing.
func (w *ConsentWriter) Close() {
	w.mu.Lock()
	w.closed = true
	w.mu.Unlock()
	w.Wait()
}

// work writes wanted values until none is left.
func (w *ConsentWriter) work() {
	for {
		w.mu.Lock()
		if !w.pending {
			w.running = false
			w.idle.Broadcast()
			w.mu.Unlock()
			return
		}
		value, gen := w.wanted, w.gen
		w.pending = false
		closed := w.closed
		w.mu.Unlock()

		err := w.write(value)
		w.finish(value, gen, err)

		if closed {
			w.mu.Lock()
			w.running = false
			w.idle.Broadcast()
			w.mu.Unlock()
			return
		}
	}
}

// finish records the write's outcome, restores the last persisted value when
// no newer choice exists and the write failed, and reports the result.
func (w *ConsentWriter) finish(value Consent, gen uint64, err error) {
	w.mu.Lock()
	result := ConsentResult{Gen: gen, Consent: value, Err: err}
	if err == nil {
		w.onDisk = value
	} else if gen == w.gen {
		// No newer choice: the file still holds the last value that reached
		// it, so that is the value the page must show again.
		w.wanted = w.onDisk
		result.Restored = true
	}
	result.Persisted = w.onDisk
	onDone := w.onDone
	w.mu.Unlock()

	if onDone != nil {
		onDone(result)
	}
}
