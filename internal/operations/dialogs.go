package operations

import (
	"fmt"

	"github.com/jwijenbergh/puregotk/v4/adw"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

// ShowCancelConfirmation shows a confirmation dialog before cancelling an operation.
//
// Per CONTEXT.md: cancellation always requires a confirmation dialog to prevent
// accidental cancellation of long-running operations.
//
// The dialog uses AdwAlertDialog per GNOME HIG with:
//   - "Continue" as the default response (safe option)
//   - "Cancel Operation" with destructive appearance (dangerous option)
//
// Parameters:
//   - window: Parent window for the dialog
//   - opName: Name of the operation being cancelled (shown in dialog body)
//   - onConfirm: Called if user confirms cancellation
//
// The onConfirm callback should:
//  1. Call operation.Cancel() to cancel the operation
//  2. Show a toast notification to confirm the cancellation
//
// Must be called from the GTK main thread.
//
// Example:
//
//	ShowCancelConfirmation(window, "Downloading Firefox", func() {
//	    op.Cancel()
//	    window.ShowToast("Cancelled")
//	})
func ShowCancelConfirmation(window *gtk.Window, opName string, onConfirm func()) {
	dialog := adw.NewAlertDialog("Cancel Operation?", "")
	dialog.SetBody(fmt.Sprintf("This will cancel \"%s\". This action cannot be undone.", opName))

	dialog.AddResponse("continue", "_Continue")
	dialog.AddResponse("cancel", "_Cancel Operation")

	dialog.SetResponseAppearance("cancel", adw.ResponseDestructiveValue)
	dialog.SetDefaultResponse("continue")
	dialog.SetCloseResponse("continue")

	responseCb := func(_ adw.AlertDialog, response string) {
		if response == "cancel" {
			onConfirm()
		}
	}
	dialog.ConnectResponse(&responseCb)

	dialog.Present(&window.Widget)
}
