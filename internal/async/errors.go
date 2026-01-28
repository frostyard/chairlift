// Package async provides thread-safe utilities for goroutine-to-UI communication.
//
// This file re-exports UserError from pkg/adwutil for backward compatibility.
package async

import "github.com/frostyard/chairlift/pkg/adwutil"

// UserError is an alias for [adwutil.UserError].
// New code should use adwutil.UserError directly.
type UserError = adwutil.UserError

// NewUserError creates a UserError with just a summary and technical error.
// See [adwutil.NewUserError] for full documentation.
func NewUserError(summary string, technical error) *UserError {
	return adwutil.NewUserError(summary, technical)
}

// NewUserErrorWithHint creates a UserError with summary, hint, and technical error.
// See [adwutil.NewUserErrorWithHint] for full documentation.
func NewUserErrorWithHint(summary, hint string, technical error) *UserError {
	return adwutil.NewUserErrorWithHint(summary, hint, technical)
}
