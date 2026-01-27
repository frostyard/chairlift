// Package operations provides centralized tracking of all async operations
// in the Chairlift application.
//
// This package coordinates between async operations (running in goroutines)
// and UI updates (running on the GTK main thread), providing consistent visual
// feedback and cancellation support across the application.
//
// # Threading Model
//
// The registry is thread-safe. All operation state modifications are protected
// by a mutex. However, UI updates must still happen on the GTK main thread.
//
// When notifying listeners of operation changes, the registry:
//  1. Copies the operation data while holding the lock
//  2. Releases the lock before notifying listeners
//  3. Notifies each listener via async.RunOnMain
//
// This pattern prevents deadlocks that could occur if a listener tried to
// access the registry while it was locked during notification.
//
// # Operation Lifecycle
//
// Operations transition through the following states:
//
//	StateActive     -> StateCompleted (on success)
//	StateActive     -> StateFailed (on error)
//	StateActive     -> StateCancelled (on user cancel)
//
// Operations start in StateActive when registered via Start() or StartWithContext().
// They transition to a terminal state via Complete() or Cancel().
//
// # History Management
//
// Successfully completed operations move to history (capped at 100 items).
// Failed operations stay in the active list with retry option.
// Cancelled operations move to history.
//
// # Usage
//
//	// Start a simple operation
//	op := operations.Start("Installing Firefox", operations.CategoryInstall, false)
//	go func() {
//	    err := doInstall()
//	    op.Complete(err)
//	}()
//
//	// Start a cancellable operation with context
//	op, ctx := operations.StartWithContext(context.Background(), "Updating system", operations.CategoryUpdate)
//	go func() {
//	    if err := doUpdateWithContext(ctx); err != nil {
//	        op.Complete(err)
//	        return
//	    }
//	    op.Complete(nil)
//	}()
//
//	// Listen for operation changes
//	operations.AddListener(func(op *operations.Operation) {
//	    // Update UI based on operation state
//	})
package operations
