// Package adwutil provides reusable GTK4/Libadwaita patterns.
//
// This file provides empty state display helpers.
package adwutil

import (
	"github.com/jwijenbergh/puregotk/v4/adw"
)

// EmptyStateConfig configures an empty state display.
type EmptyStateConfig struct {
	Title       string
	Description string
	IconName    string
	Compact     bool // For inline use in expanders/popovers
}

// NewEmptyState creates a StatusPage configured for empty state display.
//
// StatusPage is the GNOME HIG-recommended widget for showing empty states.
//
// Common icon names:
//   - "folder-symbolic" for empty file/folder lists
//   - "system-run-symbolic" for empty operations
//   - "document-open-recent-symbolic" for empty history
//   - "package-x-generic-symbolic" for empty package lists
//
// Must be called from the GTK main thread.
func NewEmptyState(cfg EmptyStateConfig) *adw.StatusPage {
	status := adw.NewStatusPage()
	status.SetTitle(cfg.Title)
	status.SetDescription(cfg.Description)
	if cfg.IconName != "" {
		status.SetIconName(cfg.IconName)
	}
	if cfg.Compact {
		status.AddCssClass("compact")
	}
	return status
}
