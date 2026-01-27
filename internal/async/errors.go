// Package async provides thread-safe utilities for goroutine-to-UI communication.
//
// This file defines the UserError type for structured user-friendly error handling.

package async

import "fmt"

// UserError wraps an error with user-friendly messaging.
//
// It separates the user-facing summary from technical details, enabling:
//   - Clear, actionable error messages for users
//   - Preserved technical details for debugging and logging
//   - Expandable details view in the UI
//
// Tone guidelines (per CONTEXT.md):
//   - Use "Couldn't" not "Failed to" or "Error:"
//   - Keep summaries short and action-oriented
//   - Include hints only when actionable
//
// Example:
//
//	return &async.UserError{
//	    Summary:   "Couldn't install Firefox",
//	    Hint:      "Check your internet connection",
//	    Technical: err,
//	}
type UserError struct {
	// Summary is the user-facing message: "Couldn't install Firefox"
	Summary string

	// Hint is an optional action suggestion: "Check your internet connection"
	// Leave empty if no actionable advice is available.
	Hint string

	// Technical is the original error for logging and debugging.
	// This is never shown directly to users but is available via FormatWithDetails().
	Technical error
}

// Error returns the user-facing summary, satisfying the error interface.
func (e *UserError) Error() string {
	return e.Summary
}

// Unwrap returns the underlying technical error, enabling errors.Is/As.
func (e *UserError) Unwrap() error {
	return e.Technical
}

// FormatForUser returns the display message without technical details.
//
// If Hint is set, returns "Summary: Hint".
// Otherwise, returns just the Summary.
func (e *UserError) FormatForUser() string {
	if e.Hint != "" {
		return fmt.Sprintf("%s: %s", e.Summary, e.Hint)
	}
	return e.Summary
}

// FormatWithDetails returns the user message plus technical details
// for expandable error views or logging.
//
// Format:
//
//	{Summary}: {Hint}
//
//	Details: {Technical error}
func (e *UserError) FormatWithDetails() string {
	userMsg := e.FormatForUser()
	if e.Technical != nil {
		return fmt.Sprintf("%s\n\nDetails: %v", userMsg, e.Technical)
	}
	return userMsg
}

// NewUserError creates a UserError with just a summary and technical error.
// Use this when no actionable hint is available.
//
// Example:
//
//	return async.NewUserError("Couldn't load application list", err)
func NewUserError(summary string, technical error) *UserError {
	return &UserError{
		Summary:   summary,
		Technical: technical,
	}
}

// NewUserErrorWithHint creates a UserError with summary, hint, and technical error.
// Use this when you can suggest an action the user can take.
//
// Example:
//
//	return async.NewUserErrorWithHint(
//	    "Couldn't install Firefox",
//	    "Check your internet connection",
//	    err,
//	)
func NewUserErrorWithHint(summary, hint string, technical error) *UserError {
	return &UserError{
		Summary:   summary,
		Hint:      hint,
		Technical: technical,
	}
}
