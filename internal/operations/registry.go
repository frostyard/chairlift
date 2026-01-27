package operations

import (
	"context"
	"sync"
	"time"

	"github.com/frostyard/chairlift/internal/async"
)

// Listener is a callback function that receives operation updates.
// Listeners are called via async.RunOnMain to ensure thread-safe UI updates.
type Listener func(op *Operation)

// Registry tracks all active and completed operations.
// It is thread-safe and uses async.RunOnMain for UI listener callbacks.
type Registry struct {
	mu         sync.RWMutex
	operations map[uint64]*Operation
	history    []*Operation // Completed ops, capped at maxHistory
	listeners  []Listener   // Called on every operation change
}

// maxHistory is the maximum number of completed operations to keep.
const maxHistory = 100

// defaultRegistry is the singleton registry instance.
var defaultRegistry = &Registry{
	operations: make(map[uint64]*Operation),
}

// Start creates a new operation and adds it to the registry.
// The operation is immediately in StateActive and listeners are notified.
func Start(name string, category Category, cancellable bool) *Operation {
	return defaultRegistry.start(name, category, cancellable, nil)
}

// StartWithContext creates a cancellable operation with a derived context.
// Returns the operation and a context that will be cancelled when Cancel() is called.
func StartWithContext(ctx context.Context, name string, category Category) (*Operation, context.Context) {
	derivedCtx, cancel := context.WithCancel(ctx)
	op := defaultRegistry.start(name, category, true, cancel)
	return op, derivedCtx
}

// Get returns the operation with the given ID, or nil if not found.
func Get(id uint64) *Operation {
	return defaultRegistry.get(id)
}

// ActiveCount returns the number of active operations.
func ActiveCount() int {
	return defaultRegistry.activeCount()
}

// Active returns a copy of all active operations.
func Active() []*Operation {
	return defaultRegistry.active()
}

// History returns a copy of the completed operations history.
func History() []*Operation {
	return defaultRegistry.getHistory()
}

// AddListener adds a listener that will be called for operation changes.
// The listener is called via async.RunOnMain for thread-safe UI updates.
func AddListener(fn Listener) {
	defaultRegistry.addListener(fn)
}

// start is the internal implementation of Start.
func (r *Registry) start(name string, category Category, cancellable bool, cancelFunc context.CancelFunc) *Operation {
	op := &Operation{
		ID:          nextID(),
		Name:        name,
		Category:    category,
		State:       StateActive,
		StartedAt:   time.Now(),
		Progress:    -1, // Indeterminate by default
		Cancellable: cancellable,
		CancelFunc:  cancelFunc,
		registry:    r,
	}

	// Add to registry under lock
	r.mu.Lock()
	r.operations[op.ID] = op
	// Copy operation and listeners for notification
	opCopy := *op
	listeners := make([]Listener, len(r.listeners))
	copy(listeners, r.listeners)
	r.mu.Unlock()

	// Notify listeners outside of lock
	r.notifyListeners(&opCopy, listeners)

	return op
}

// get is the internal implementation of Get.
func (r *Registry) get(id uint64) *Operation {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.operations[id]
}

// activeCount is the internal implementation of ActiveCount.
func (r *Registry) activeCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.operations)
}

// active is the internal implementation of Active.
func (r *Registry) active() []*Operation {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ops := make([]*Operation, 0, len(r.operations))
	for _, op := range r.operations {
		opCopy := *op
		ops = append(ops, &opCopy)
	}
	return ops
}

// getHistory is the internal implementation of History.
func (r *Registry) getHistory() []*Operation {
	r.mu.RLock()
	defer r.mu.RUnlock()

	history := make([]*Operation, len(r.history))
	for i, op := range r.history {
		opCopy := *op
		history[i] = &opCopy
	}
	return history
}

// addListener is the internal implementation of AddListener.
func (r *Registry) addListener(fn Listener) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.listeners = append(r.listeners, fn)
}

// updateProgress updates an operation's progress and message.
func (r *Registry) updateProgress(id uint64, progress float64, message string) {
	r.mu.Lock()
	op, ok := r.operations[id]
	if !ok || op.State != StateActive {
		r.mu.Unlock()
		return
	}

	op.Progress = progress
	op.Message = message

	// Copy for notification
	opCopy := *op
	listeners := make([]Listener, len(r.listeners))
	copy(listeners, r.listeners)
	r.mu.Unlock()

	r.notifyListeners(&opCopy, listeners)
}

// complete transitions an operation to completed or failed state.
func (r *Registry) complete(id uint64, err error) {
	r.mu.Lock()
	op, ok := r.operations[id]
	if !ok || op.State != StateActive {
		r.mu.Unlock()
		return
	}

	op.EndedAt = time.Now()
	op.Error = err

	if err != nil {
		// Failed - keep in active list for retry
		op.State = StateFailed
	} else {
		// Completed - move to history
		op.State = StateCompleted
		delete(r.operations, id)
		r.addToHistory(op)
	}

	// Copy for notification
	opCopy := *op
	listeners := make([]Listener, len(r.listeners))
	copy(listeners, r.listeners)
	r.mu.Unlock()

	r.notifyListeners(&opCopy, listeners)
}

// cancel cancels an operation.
func (r *Registry) cancel(id uint64) {
	r.mu.Lock()
	op, ok := r.operations[id]
	if !ok || op.State != StateActive {
		r.mu.Unlock()
		return
	}

	// Call cancel function if set
	if op.CancelFunc != nil {
		op.CancelFunc()
	}

	op.State = StateCancelled
	op.EndedAt = time.Now()

	// Move to history
	delete(r.operations, id)
	r.addToHistory(op)

	// Copy for notification
	opCopy := *op
	listeners := make([]Listener, len(r.listeners))
	copy(listeners, r.listeners)
	r.mu.Unlock()

	r.notifyListeners(&opCopy, listeners)
}

// addToHistory adds an operation to history, maintaining the cap.
// Must be called with lock held.
func (r *Registry) addToHistory(op *Operation) {
	r.history = append(r.history, op)
	// Cap at maxHistory, drop oldest
	if len(r.history) > maxHistory {
		r.history = r.history[len(r.history)-maxHistory:]
	}
}

// notifyListeners calls all listeners via async.RunOnMain.
// This pattern ensures we don't hold the lock during UI callbacks.
func (r *Registry) notifyListeners(op *Operation, listeners []Listener) {
	for _, listener := range listeners {
		// Capture listener for closure
		fn := listener
		async.RunOnMain(func() {
			fn(op)
		})
	}
}
