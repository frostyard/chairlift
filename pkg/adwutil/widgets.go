// Package adwutil provides reusable GTK4/Libadwaita patterns.
//
// This file provides helper functions for common widget patterns.
package adwutil

import (
	"sync"

	"github.com/jwijenbergh/puregotk/v4/adw"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

// signalCallbackRegistry stores signal callbacks to prevent GC collection.
var (
	signalCallbackMu sync.Mutex
	signalCallbacks  = make(map[uintptr]any)
	signalCallbackID uintptr
)

// storeSignalCallback saves a callback to prevent GC and returns its ID.
func storeSignalCallback(cb any) uintptr {
	signalCallbackMu.Lock()
	defer signalCallbackMu.Unlock()
	signalCallbackID++
	signalCallbacks[signalCallbackID] = cb
	return signalCallbackID
}

// NewLinkRow creates an ActionRow that triggers an action when activated.
//
// The row has an external link icon suffix, suitable for "open URL" patterns.
//
// Must be called from the GTK main thread.
func NewLinkRow(title, subtitle string, onClick func()) *adw.ActionRow {
	row := adw.NewActionRow()
	row.SetTitle(title)
	row.SetSubtitle(subtitle)

	btn := gtk.NewButtonFromIconName("adw-external-link-symbolic")
	btn.SetValign(gtk.AlignCenterValue)
	btn.AddCssClass("flat")
	btn.SetTooltipText("Open link")

	cb := func(_ gtk.Button) {
		onClick()
	}
	storeSignalCallback(cb)
	btn.ConnectClicked(&cb)

	row.AddSuffix(&btn.Widget)
	return row
}

// NewInfoRow creates a simple ActionRow for displaying information.
//
// The row is not activatable - just title and subtitle.
// Suitable for read-only information in preference groups.
//
// Must be called from the GTK main thread.
func NewInfoRow(title, subtitle string) *adw.ActionRow {
	row := adw.NewActionRow()
	row.SetTitle(title)
	row.SetSubtitle(subtitle)
	return row
}

// NewButtonRow creates an ActionRow with an action button suffix.
//
// The button uses "suggested-action" CSS class (blue/accent color).
// Use [NewButtonRowWithClass] for different button styles.
//
// Must be called from the GTK main thread.
func NewButtonRow(title, subtitle, buttonLabel string, onClick func()) *adw.ActionRow {
	return NewButtonRowWithClass(title, subtitle, buttonLabel, "suggested-action", onClick)
}

// NewButtonRowWithClass creates an ActionRow with a styled button suffix.
//
// Common CSS classes:
//   - "suggested-action": Blue/accent colored for primary actions
//   - "destructive-action": Red colored for dangerous actions
//
// Must be called from the GTK main thread.
func NewButtonRowWithClass(title, subtitle, buttonLabel, cssClass string, onClick func()) *adw.ActionRow {
	row := adw.NewActionRow()
	row.SetTitle(title)
	row.SetSubtitle(subtitle)

	btn := gtk.NewButtonWithLabel(buttonLabel)
	btn.SetValign(gtk.AlignCenterValue)
	btn.AddCssClass(cssClass)

	cb := func(_ gtk.Button) {
		onClick()
	}
	storeSignalCallback(cb)
	btn.ConnectClicked(&cb)

	row.AddSuffix(&btn.Widget)
	return row
}

// NewIconRow creates an ActionRow with a prefix icon.
//
// Common icon names:
//   - "dialog-error-symbolic": Error indicator
//   - "dialog-warning-symbolic": Warning indicator
//   - "object-select-symbolic": Checkmark for selected items
//
// Must be called from the GTK main thread.
func NewIconRow(title, subtitle, iconName string) *adw.ActionRow {
	row := adw.NewActionRow()
	row.SetTitle(title)
	row.SetSubtitle(subtitle)

	icon := gtk.NewImageFromIconName(iconName)
	row.AddPrefix(&icon.Widget)

	return row
}
