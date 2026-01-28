// Package adwutil provides reusable GTK4/Libadwaita patterns.
//
// This file provides the operations registry for tracking async operations.
package adwutil

import (
	"context"
	"sync"
	"time"
)

// Listener is a callback function that receives operation updates.
type Listener func(op *Operation)

// Registry tracks all active and completed operations.
type Registry struct {
	mu         sync.RWMutex
	operations map[uint64]*Operation
	history    []*Operation
	listeners  []Listener
}

// MaxHistory is the maximum number of completed operations to keep.
const MaxHistory = 100

// DefaultRegistry is the singleton registry instance.
var DefaultRegistry = NewRegistry()

// NewRegistry creates a new operations registry.
func NewRegistry() *Registry {
	return &Registry{
		operations: make(map[uint64]*Operation),
	}
}

// Start creates a new operation and adds it to the default registry.
func Start(name string, category Category, cancellable bool) *Operation {
	return DefaultRegistry.Start(name, category, cancellable)
}

// StartWithContext creates a cancellable operation with a derived context.
func StartWithContext(ctx context.Context, name string, category Category) (*Operation, context.Context) {
	return DefaultRegistry.StartWithContext(ctx, name, category)
}

// Get returns the operation with the given ID from the default registry.
func Get(id uint64) *Operation {
	return DefaultRegistry.Get(id)
}

// ActiveCount returns the number of active operations in the default registry.
func ActiveCount() int {
	return DefaultRegistry.ActiveCount()
}

// Active returns a copy of all active operations from the default registry.
func Active() []*Operation {
	return DefaultRegistry.Active()
}

// History returns a copy of the completed operations history.
func History() []*Operation {
	return DefaultRegistry.History()
}

// AddListener adds a listener to the default registry.
func AddListener(fn Listener) {
	DefaultRegistry.AddListener(fn)
}

// Start creates a new operation and adds it to the registry.
func (r *Registry) Start(name string, category Category, cancellable bool) *Operation {
	return r.start(name, category, cancellable, nil)
}

// StartWithContext creates a cancellable operation with a derived context.
func (r *Registry) StartWithContext(ctx context.Context, name string, category Category) (*Operation, context.Context) {
	derivedCtx, cancel := context.WithCancel(ctx)
	op := r.start(name, category, true, cancel)
	return op, derivedCtx
}

// Get returns the operation with the given ID.
func (r *Registry) Get(id uint64) *Operation {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.operations[id]
}

// ActiveCount returns the number of active operations.
func (r *Registry) ActiveCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.operations)
}

// Active returns a copy of all active operations.
func (r *Registry) Active() []*Operation {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ops := make([]*Operation, 0, len(r.operations))
	for _, op := range r.operations {
		opCopy := *op
		ops = append(ops, &opCopy)
	}
	return ops
}

// History returns a copy of the completed operations.
func (r *Registry) History() []*Operation {
	r.mu.RLock()
	defer r.mu.RUnlock()

	history := make([]*Operation, len(r.history))
	for i, op := range r.history {
		opCopy := *op
		history[i] = &opCopy
	}
	return history
}

// AddListener adds a listener that will be called for operation changes.
func (r *Registry) AddListener(fn Listener) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.listeners = append(r.listeners, fn)
}

// start is the internal implementation.
func (r *Registry) start(name string, category Category, cancellable bool, cancelFunc context.CancelFunc) *Operation {
	op := &Operation{
		ID:          nextOperationID(),
		Name:        name,
		Category:    category,
		State:       StateActive,
		StartedAt:   time.Now(),
		Progress:    -1,
		Cancellable: cancellable,
		CancelFunc:  cancelFunc,
		registry:    r,
	}

	r.mu.Lock()
	r.operations[op.ID] = op
	opCopy := *op
	listeners := make([]Listener, len(r.listeners))
	copy(listeners, r.listeners)
	r.mu.Unlock()

	r.notifyListeners(&opCopy, listeners)
	return op
}

func (r *Registry) updateProgress(id uint64, progress float64, message string) {
	r.mu.Lock()
	op, ok := r.operations[id]
	if !ok || op.State != StateActive {
		r.mu.Unlock()
		return
	}

	op.Progress = progress
	op.Message = message
	opCopy := *op
	listeners := make([]Listener, len(r.listeners))
	copy(listeners, r.listeners)
	r.mu.Unlock()

	r.notifyListeners(&opCopy, listeners)
}

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
		op.State = StateFailed
	} else {
		op.State = StateCompleted
		delete(r.operations, id)
		r.addToHistory(op)
	}

	opCopy := *op
	listeners := make([]Listener, len(r.listeners))
	copy(listeners, r.listeners)
	r.mu.Unlock()

	r.notifyListeners(&opCopy, listeners)
}

func (r *Registry) cancel(id uint64) {
	r.mu.Lock()
	op, ok := r.operations[id]
	if !ok || op.State != StateActive {
		r.mu.Unlock()
		return
	}

	if op.CancelFunc != nil {
		op.CancelFunc()
	}

	op.State = StateCancelled
	op.EndedAt = time.Now()
	delete(r.operations, id)
	r.addToHistory(op)

	opCopy := *op
	listeners := make([]Listener, len(r.listeners))
	copy(listeners, r.listeners)
	r.mu.Unlock()

	r.notifyListeners(&opCopy, listeners)
}

func (r *Registry) addToHistory(op *Operation) {
	r.history = append(r.history, op)
	if len(r.history) > MaxHistory {
		r.history = r.history[len(r.history)-MaxHistory:]
	}
}

func (r *Registry) notifyListeners(op *Operation, listeners []Listener) {
	for _, listener := range listeners {
		fn := listener
		RunOnMain(func() {
			fn(op)
		})
	}
}
