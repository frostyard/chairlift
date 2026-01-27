package operations

import (
	"context"
	"sync/atomic"
	"time"
)

// OperationState represents the current state of an operation.
type OperationState int

const (
	// StateActive indicates the operation is in progress.
	StateActive OperationState = iota
	// StateCompleted indicates the operation finished successfully.
	StateCompleted
	// StateFailed indicates the operation finished with an error.
	StateFailed
	// StateCancelled indicates the operation was cancelled by the user.
	StateCancelled
)

// String returns a human-readable representation of the state.
func (s OperationState) String() string {
	switch s {
	case StateActive:
		return "Active"
	case StateCompleted:
		return "Completed"
	case StateFailed:
		return "Failed"
	case StateCancelled:
		return "Cancelled"
	default:
		return "Unknown"
	}
}

// Category represents the type of operation being performed.
type Category string

const (
	// CategoryInstall represents package installation operations.
	CategoryInstall Category = "install"
	// CategoryUpdate represents system update operations.
	CategoryUpdate Category = "update"
	// CategoryLoading represents data loading operations.
	CategoryLoading Category = "loading"
)

// Operation represents an async operation being tracked by the registry.
type Operation struct {
	// ID is the unique identifier for this operation.
	ID uint64
	// Name is a human-readable description of the operation.
	Name string
	// Category indicates the type of operation.
	Category Category
	// State is the current state of the operation.
	State OperationState
	// StartedAt is when the operation was started.
	StartedAt time.Time
	// EndedAt is when the operation completed (zero if still active).
	EndedAt time.Time
	// Progress is the completion progress from 0.0 to 1.0, or -1 for indeterminate.
	Progress float64
	// Message is the current status message.
	Message string
	// Cancellable indicates whether this operation can be cancelled.
	Cancellable bool
	// CancelFunc is called when the user requests cancellation.
	CancelFunc context.CancelFunc
	// Error holds the error if the operation failed.
	Error error
	// RetryFunc is called when retry is clicked for failed operations.
	RetryFunc func()

	// registry holds a reference to the registry for notifications.
	registry *Registry
}

// idCounter provides atomic ID generation for operations.
var idCounter uint64

// nextID returns the next unique operation ID.
func nextID() uint64 {
	return atomic.AddUint64(&idCounter, 1)
}

// UpdateProgress updates the operation's progress and message, then notifies listeners.
// Progress should be between 0.0 and 1.0, or -1 for indeterminate.
func (o *Operation) UpdateProgress(progress float64, message string) {
	if o.registry == nil {
		return
	}
	o.registry.updateProgress(o.ID, progress, message)
}

// Complete transitions the operation to completed or failed state based on err.
// If err is nil, the operation is marked as completed and moved to history.
// If err is not nil, the operation is marked as failed and stays in active list.
func (o *Operation) Complete(err error) {
	if o.registry == nil {
		return
	}
	o.registry.complete(o.ID, err)
}

// Cancel calls the CancelFunc if set and transitions to cancelled state.
func (o *Operation) Cancel() {
	if o.registry == nil {
		return
	}
	o.registry.cancel(o.ID)
}

// Duration returns the elapsed time since the operation started.
// If the operation has ended, it returns the total duration.
func (o *Operation) Duration() time.Duration {
	if o.EndedAt.IsZero() {
		return time.Since(o.StartedAt)
	}
	return o.EndedAt.Sub(o.StartedAt)
}

// IsCancellable returns true if the operation can be cancelled.
// An operation is cancellable if it was marked as cancellable, is still active,
// and has been running for more than 5 seconds.
func (o *Operation) IsCancellable() bool {
	if !o.Cancellable || o.State != StateActive {
		return false
	}
	return o.Duration() > 5*time.Second
}
