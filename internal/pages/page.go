// Package pages provides page interfaces and common dependencies for all pages.
package pages

import (
	"github.com/frostyard/chairlift/internal/config"
	"github.com/jwijenbergh/puregotk/v4/adw"
)

// Toaster provides toast notification methods.
type Toaster interface {
	ShowToast(message string)
	ShowErrorToast(message string)
}

// Deps holds dependencies shared by all pages.
type Deps struct {
	Config  *config.Config
	Toaster Toaster
}

// Page is the interface all page packages implement.
type Page interface {
	Widget() *adw.ToolbarView
	Destroy()
}
