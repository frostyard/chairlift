// Package widgets provides reusable GTK4/Libadwaita widget components.
//
// This package re-exports functionality from pkg/adwutil for backward compatibility.
package widgets

import (
	"codeberg.org/puregotk/puregotk/v4/adw"
	"github.com/frostyard/chairlift/pkg/adwutil"
)

// EmptyStateConfig is an alias for [adwutil.EmptyStateConfig].
type EmptyStateConfig = adwutil.EmptyStateConfig

// NewEmptyState creates a StatusPage configured for empty state display.
// See [adwutil.NewEmptyState] for full documentation.
func NewEmptyState(cfg EmptyStateConfig) *adw.StatusPage {
	return adwutil.NewEmptyState(cfg)
}
