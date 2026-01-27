// Package async provides thread-safe utilities for goroutine-to-UI communication
// in GTK4 applications using puregotk.
//
// This package consolidates the runOnMainThread pattern to ensure consistent,
// safe updates to GTK widgets from background goroutines.
//
// # Thread Safety
//
// GTK is not thread-safe. All widget updates MUST happen on the GTK main thread.
// Calling widget methods from goroutines will cause segfaults or silent corruption.
//
// # Usage
//
// Use RunOnMain() to schedule any UI update from a goroutine:
//
//	go func() {
//	    result := doExpensiveWork()
//	    async.RunOnMain(func() {
//	        label.SetText(result)
//	        spinner.Stop()
//	    })
//	}()
//
// # Callback Registry
//
// This package uses a callback registry to prevent garbage collection of scheduled
// callbacks. Without this registry, the Go GC may collect callback functions before
// GTK executes them, causing crashes or no-ops.
//
// The registry stores callbacks keyed by a unique ID. When glib.IdleAdd executes
// the callback, it retrieves and deletes the function from the registry.
package async

import (
	"sync"

	"github.com/jwijenbergh/puregotk/v4/glib"
)

// callbackRegistry stores callbacks to prevent GC collection before GTK executes them.
// This is essential because puregotk passes function pointers to C, and if Go
// doesn't hold a reference, GC may collect the function.
var (
	callbackMu sync.Mutex
	callbacks  = make(map[uintptr]func())
	callbackID uintptr
)

// RunOnMain schedules a function to run on the GTK main thread.
//
// This is the ONLY safe way to update UI from goroutines. GTK is not thread-safe;
// calling widget methods from goroutines will cause segfaults or silent corruption.
//
// The function is scheduled via glib.IdleAdd and will execute when the main loop
// is idle. The callback is stored in a registry to prevent garbage collection.
//
// Example:
//
//	go func() {
//	    data, err := fetchData()
//	    async.RunOnMain(func() {
//	        if err != nil {
//	            showError(err)
//	            return
//	        }
//	        updateUI(data)
//	    })
//	}()
func RunOnMain(fn func()) {
	// Lock, increment ID, store callback, unlock - in that order.
	// We must NOT hold the lock when calling glib.IdleAdd to avoid potential
	// deadlocks if the callback is executed synchronously in some edge cases.
	callbackMu.Lock()
	callbackID++
	id := callbackID
	callbacks[id] = fn
	callbackMu.Unlock()

	// Create the SourceFunc that glib.IdleAdd will execute.
	// The callback retrieves the stored function by ID, deletes it from the registry,
	// and executes it.
	cb := glib.SourceFunc(func(data uintptr) bool {
		callbackMu.Lock()
		callback, ok := callbacks[data]
		delete(callbacks, data)
		callbackMu.Unlock()

		if ok {
			callback()
		}
		return false // Return false to remove the source after execution
	})

	// Schedule the callback to run when the main loop is idle.
	// The id is passed as user data and used to retrieve the callback.
	glib.IdleAdd(&cb, id)
}
