package widgets

import (
	"github.com/jwijenbergh/puregotk/v4/adw"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

// NewLinkRow creates an ActionRow that triggers an action when activated.
//
// The row is styled with an external link icon suffix and is activatable,
// making it suitable for "open URL" or "launch app" patterns.
//
// Parameters:
//   - title: The main row text (e.g., "System Performance", "Website")
//   - subtitle: Secondary text (e.g., "Monitor CPU and memory", "https://example.com")
//   - onClick: Callback invoked when the row is activated
//
// Must be called from the GTK main thread.
//
// Example:
//
//	row := widgets.NewLinkRow(
//	    "System Performance",
//	    "Monitor CPU, memory, and system resources",
//	    func() { launchApp("io.missioncenter.MissionCenter") },
//	)
//	group.Add(&row.Widget)
func NewLinkRow(title, subtitle string, onClick func()) *adw.ActionRow {
	row := adw.NewActionRow()
	row.SetTitle(title)
	row.SetSubtitle(subtitle)
	row.SetActivatable(true)

	icon := gtk.NewImageFromIconName("adw-external-link-symbolic")
	row.AddSuffix(&icon.Widget)

	cb := func(_ adw.ActionRow) {
		onClick()
	}
	row.ConnectActivated(&cb)

	return row
}

// NewInfoRow creates a simple ActionRow for displaying information.
//
// The row is not activatable and has no icons or buttons - just title and subtitle.
// Suitable for displaying read-only information in preference groups.
//
// Parameters:
//   - title: The main row text (e.g., "Image", "Version", "Status")
//   - subtitle: The value text (e.g., "fedora:latest", "1.2.3", "Active")
//
// Must be called from the GTK main thread.
//
// Example:
//
//	row := widgets.NewInfoRow("Filesystem", "btrfs")
//	expander.AddRow(&row.Widget)
func NewInfoRow(title, subtitle string) *adw.ActionRow {
	row := adw.NewActionRow()
	row.SetTitle(title)
	row.SetSubtitle(subtitle)
	return row
}

// NewButtonRow creates an ActionRow with an action button suffix.
//
// The button is styled with "suggested-action" CSS class (blue/accent color).
// Use [NewButtonRowWithClass] for different button styles.
//
// Parameters:
//   - title: The main row text (e.g., "Staged Update", "Extension")
//   - subtitle: Secondary text (e.g., "Ready to apply", "v1.2.3 available")
//   - buttonLabel: Text for the button (e.g., "Apply", "Install", "Update")
//   - onClick: Callback invoked when the button is clicked
//
// Must be called from the GTK main thread.
//
// Example:
//
//	row := widgets.NewButtonRow(
//	    "Staged Update",
//	    "Ready: sha256:abc123...",
//	    "Apply",
//	    func() { applyUpdate() },
//	)
//	group.Add(&row.Widget)
func NewButtonRow(title, subtitle, buttonLabel string, onClick func()) *adw.ActionRow {
	return NewButtonRowWithClass(title, subtitle, buttonLabel, "suggested-action", onClick)
}

// NewButtonRowWithClass creates an ActionRow with a styled button suffix.
//
// Parameters:
//   - title: The main row text
//   - subtitle: Secondary text
//   - buttonLabel: Text for the button
//   - cssClass: CSS class for the button (e.g., "suggested-action", "destructive-action")
//   - onClick: Callback invoked when the button is clicked
//
// Common CSS classes:
//   - "suggested-action": Blue/accent colored button for primary actions
//   - "destructive-action": Red colored button for dangerous actions
//
// Must be called from the GTK main thread.
//
// Example:
//
//	row := widgets.NewButtonRowWithClass(
//	    "Remove Extension",
//	    "This will delete all extension data",
//	    "Remove",
//	    "destructive-action",
//	    func() { removeExtension() },
//	)
//	group.Add(&row.Widget)
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
	btn.ConnectClicked(&cb)

	row.AddSuffix(&btn.Widget)
	return row
}

// NewIconRow creates an ActionRow with a prefix icon.
//
// Suitable for rows that need visual categorization or status indication.
//
// Parameters:
//   - title: The main row text (e.g., "Error", "Warning", "Success")
//   - subtitle: Secondary text (e.g., error message, status details)
//   - iconName: Icon name (e.g., "dialog-error-symbolic", "dialog-warning-symbolic")
//
// Common icon names:
//   - "dialog-error-symbolic": Error indicator (red X)
//   - "dialog-warning-symbolic": Warning indicator (yellow triangle)
//   - "object-select-symbolic": Checkmark for selected/active items
//   - "user-trash-symbolic": Trash can for cleanup actions
//   - "system-software-install-symbolic": Software/package icon
//
// Must be called from the GTK main thread.
//
// Example:
//
//	row := widgets.NewIconRow(
//	    "Error",
//	    "Failed to connect to server",
//	    "dialog-error-symbolic",
//	)
//	expander.AddRow(&row.Widget)
func NewIconRow(title, subtitle, iconName string) *adw.ActionRow {
	row := adw.NewActionRow()
	row.SetTitle(title)
	row.SetSubtitle(subtitle)

	icon := gtk.NewImageFromIconName(iconName)
	row.AddPrefix(&icon.Widget)

	return row
}
