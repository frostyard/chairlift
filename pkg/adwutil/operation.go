// Package adwutil provides reusable GTK4/Libadwaita patterns.
//
// This file defines the Operation type for tracking async operations.
package adwutil

import (
	"context"
	"sync/atomic"
	"time"
)

// State represents the current state of an operation.
type State int

const (
	// StateActive indicates the operation is in progress.
	StateActive State = iota
	// StateCompleted indicates the operation finished successfully.
	StateCompleted
	// StateFailed indicates the operation finished with an error.
	StateFailed
	// StateCancelled indicates the operation was cancelled by the user.
	StateCancelled
)

// String returns a human-readable representation of the state.
func (s State) String() string {
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
	// CategoryMaintenance represents cleanup and maintenance operations.
	CategoryMaintenance Category = "maintenance"
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
	State State
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

// operationID is the atomic counter for unique operation IDs.
var operationID uint64

// nextOperationID returns the next unique operation ID.
func nextOperationID() uint64 {
	return atomic.AddUint64(&operationID, 1)
}

// Duration returns how long the operation has been running or ran.
func (o *Operation) Duration() time.Duration {
	if o.EndedAt.IsZero() {
		return time.Since(o.StartedAt)
	}
	return o.EndedAt.Sub(o.StartedAt)
}

// IsCancellable returns whether the operation can be cancelled.
// Requires Cancellable flag AND >5s runtime.
func (o *Operation) IsCancellable() bool {
	if !o.Cancellable || o.State != StateActive {
		return false
	}
	return o.Duration() > 5*time.Second
}

// UpdateProgress updates the operation's progress and message.
func (o *Operation) UpdateProgress(progress float64, message string) {
	if o.registry != nil {
		o.registry.updateProgress(o.ID, progress, message)
	}
}

// Complete marks the operation as completed or failed.
func (o *Operation) Complete(err error) {
	if o.registry != nil {
		o.registry.complete(o.ID, err)
	}
}

// Cancel cancels the operation.
func (o *Operation) Cancel() {
	if o.registry != nil {
		o.registry.cancel(o.ID)
	}
}
