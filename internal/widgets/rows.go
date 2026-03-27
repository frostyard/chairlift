// Package widgets provides reusable GTK4/Libadwaita widget components.
//
// This package re-exports functionality from pkg/adwutil for backward compatibility.
package widgets

import (
	"github.com/frostyard/chairlift/pkg/adwutil"
	"github.com/jwijenbergh/puregotk/v4/adw"
)

// NewLinkRow creates an ActionRow that triggers an action when activated.
// See [adwutil.NewLinkRow] for full documentation.
func NewLinkRow(title, subtitle string, onClick func()) *adw.ActionRow {
	return adwutil.NewLinkRow(title, subtitle, onClick)
}

// NewInfoRow creates a simple ActionRow for displaying information.
// See [adwutil.NewInfoRow] for full documentation.
func NewInfoRow(title, subtitle string) *adw.ActionRow {
	return adwutil.NewInfoRow(title, subtitle)
}

// NewButtonRow creates an ActionRow with an action button suffix.
// See [adwutil.NewButtonRow] for full documentation.
func NewButtonRow(title, subtitle, buttonLabel string, onClick func()) *adw.ActionRow {
	return adwutil.NewButtonRow(title, subtitle, buttonLabel, onClick)
}

// NewButtonRowWithClass creates an ActionRow with a styled button suffix.
// See [adwutil.NewButtonRowWithClass] for full documentation.
func NewButtonRowWithClass(title, subtitle, buttonLabel, cssClass string, onClick func()) *adw.ActionRow {
	return adwutil.NewButtonRowWithClass(title, subtitle, buttonLabel, cssClass, onClick)
}

// NewIconRow creates an ActionRow with a prefix icon.
// See [adwutil.NewIconRow] for full documentation.
func NewIconRow(title, subtitle, iconName string) *adw.ActionRow {
	return adwutil.NewIconRow(title, subtitle, iconName)
}
