// Package widgets provides reusable GTK4/Libadwaita widget components.
//
// This file provides the NewEmptyState helper for creating GNOME HIG-compliant
// empty state placeholders using AdwStatusPage.
package widgets

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
// StatusPage is the GNOME HIG-recommended widget for showing empty states with
// an icon, title, and descriptive text. Use it instead of plain labels for
// empty lists, no-results states, and placeholder content.
//
// Parameters:
//   - cfg.Title: The main heading (e.g., "No Active Operations")
//   - cfg.Description: Secondary explanatory text (e.g., "Operations will appear here when running")
//   - cfg.IconName: Icon to display (see common names below)
//   - cfg.Compact: If true, adds "compact" CSS class for inline use in expanders/popovers
//
// Common icon names for empty states:
//   - "folder-symbolic" for empty file/folder lists
//   - "emblem-synchronizing-symbolic" for empty operations list
//   - "document-open-recent-symbolic" for empty history
//   - "package-x-generic-symbolic" for empty package lists
//   - "application-x-addon-symbolic" for empty extensions list
//
// Must be called from the GTK main thread.
//
// Example:
//
//	emptyState := widgets.NewEmptyState(widgets.EmptyStateConfig{
//	    Title:       "No Active Operations",
//	    Description: "Operations will appear here when running",
//	    IconName:    "emblem-synchronizing-symbolic",
//	    Compact:     true,
//	})
//	listBox.Append(&emptyState.Widget)
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
