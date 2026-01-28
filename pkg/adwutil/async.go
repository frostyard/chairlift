// Package adwutil provides reusable GTK4/Libadwaita patterns.
//
// This file provides thread-safe utilities for goroutine-to-UI communication.
package adwutil

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
//	    adwutil.RunOnMain(func() {
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
