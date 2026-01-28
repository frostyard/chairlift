// Package operations provides async operation tracking for the UI.
//
// This package re-exports types from pkg/adwutil for backward compatibility.
package operations

import "github.com/frostyard/chairlift/pkg/adwutil"

// OperationState is an alias for [adwutil.State].
type OperationState = adwutil.State

// State constants.
const (
	StateActive    = adwutil.StateActive
	StateCompleted = adwutil.StateCompleted
	StateFailed    = adwutil.StateFailed
	StateCancelled = adwutil.StateCancelled
)

// Category is an alias for [adwutil.Category].
type Category = adwutil.Category

// Category constants.
const (
	CategoryInstall     = adwutil.CategoryInstall
	CategoryUpdate      = adwutil.CategoryUpdate
	CategoryLoading     = adwutil.CategoryLoading
	CategoryMaintenance = adwutil.CategoryMaintenance
)

// Operation is an alias for [adwutil.Operation].
type Operation = adwutil.Operation
