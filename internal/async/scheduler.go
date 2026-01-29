// Package async provides thread-safe utilities for goroutine-to-UI communication
// in GTK4 applications using puregotk.
//
// This package re-exports functionality from pkg/adwutil for backward compatibility.
// New code should import pkg/adwutil directly.
package async

import "github.com/frostyard/chairlift/pkg/adwutil"

// RunOnMain schedules a function to run on the GTK main thread.
//
// See [adwutil.RunOnMain] for full documentation.
func RunOnMain(fn func()) {
	adwutil.RunOnMain(fn)
}
