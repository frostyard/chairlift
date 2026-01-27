package operations

import (
	"fmt"
	"sort"
	"time"

	"github.com/jwijenbergh/puregotk/v4/adw"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

// OperationsButton is a MenuButton that shows the operations popover.
// It displays a badge when operations are active.
type OperationsButton struct {
	Button      *gtk.MenuButton
	badge       *gtk.Label // Badge showing active count (circular overlay)
	popover     *gtk.Popover
	activeList  *gtk.ListBox
	historyList *gtk.ListBox
	emptyState  *adw.StatusPage // "No active operations" empty state
	viewStack   *adw.ViewStack
}

// BuildOperationsButton creates a MenuButton that shows operations in a popover.
//
// The button displays:
//   - An icon indicating operations status
//   - A badge showing the count of active operations (hidden when 0)
//
// The popover contains:
//   - ViewSwitcher to toggle between Active and History tabs
//   - Active tab: Shows all active operations grouped by category
//   - History tab: Shows completed operations with timestamps
//
// The button automatically registers as a listener for operation updates
// and refreshes its content in real-time.
//
// Must be called from the GTK main thread.
func BuildOperationsButton() *OperationsButton {
	ob := &OperationsButton{}

	// Create the menu button
	ob.Button = gtk.NewMenuButton()
	ob.Button.SetIconName("emblem-synchronizing-symbolic")
	ob.Button.SetTooltipText("Operations")
	ob.Button.SetHasFrame(false)

	// Create badge overlay
	overlay := gtk.NewOverlay()
	icon := gtk.NewImageFromIconName("emblem-synchronizing-symbolic")
	overlay.SetChild(&icon.Widget)

	// Badge for operation count (circular label)
	ob.badge = gtk.NewLabel("")
	ob.badge.AddCssClass("circular")
	ob.badge.AddCssClass("badge")
	ob.badge.AddCssClass("accent")
	ob.badge.SetHalign(gtk.AlignEndValue)
	ob.badge.SetValign(gtk.AlignStartValue)
	ob.badge.SetVisible(false)
	overlay.AddOverlay(&ob.badge.Widget)

	ob.Button.SetChild(&overlay.Widget)

	// Create popover content
	ob.popover = gtk.NewPopover()
	ob.popover.SetSizeRequest(320, 400)

	content := ob.buildPopoverContent()
	ob.popover.SetChild(content)
	ob.Button.SetPopover(ob.popover)

	// Register as listener for updates
	AddListener(ob.onOperationChanged)

	// Initial refresh
	ob.refreshContent()

	return ob
}

// buildPopoverContent creates the popover's internal UI structure.
func (ob *OperationsButton) buildPopoverContent() *gtk.Widget {
	mainBox := gtk.NewBox(gtk.OrientationVerticalValue, 0)

	// Create ViewStack for tabs
	ob.viewStack = adw.NewViewStack()

	// Active operations page
	activeScrolled := gtk.NewScrolledWindow()
	activeScrolled.SetPolicy(gtk.PolicyNeverValue, gtk.PolicyAutomaticValue)
	activeScrolled.SetVexpand(true)

	activeContent := gtk.NewBox(gtk.OrientationVerticalValue, 0)

	// Empty state with StatusPage for GNOME HIG compliance
	ob.emptyState = adw.NewStatusPage()
	ob.emptyState.SetTitle("No Active Operations")
	ob.emptyState.SetDescription("Operations will appear here when running")
	ob.emptyState.SetIconName("emblem-synchronizing-symbolic")
	ob.emptyState.AddCssClass("compact")
	activeContent.Append(&ob.emptyState.Widget)

	// Active operations list
	ob.activeList = gtk.NewListBox()
	ob.activeList.SetSelectionMode(gtk.SelectionNoneValue)
	ob.activeList.AddCssClass("boxed-list")
	ob.activeList.SetMarginTop(12)
	ob.activeList.SetMarginBottom(12)
	ob.activeList.SetMarginStart(12)
	ob.activeList.SetMarginEnd(12)
	activeContent.Append(&ob.activeList.Widget)

	activeScrolled.SetChild(&activeContent.Widget)
	ob.viewStack.AddTitledWithIcon(&activeScrolled.Widget, "active", "Active", "emblem-synchronizing-symbolic")

	// History page
	historyScrolled := gtk.NewScrolledWindow()
	historyScrolled.SetPolicy(gtk.PolicyNeverValue, gtk.PolicyAutomaticValue)
	historyScrolled.SetVexpand(true)

	ob.historyList = gtk.NewListBox()
	ob.historyList.SetSelectionMode(gtk.SelectionNoneValue)
	ob.historyList.AddCssClass("boxed-list")
	ob.historyList.SetMarginTop(12)
	ob.historyList.SetMarginBottom(12)
	ob.historyList.SetMarginStart(12)
	ob.historyList.SetMarginEnd(12)
	historyScrolled.SetChild(&ob.historyList.Widget)
	ob.viewStack.AddTitledWithIcon(&historyScrolled.Widget, "history", "History", "document-open-recent-symbolic")

	// ViewSwitcher at top
	viewSwitcher := adw.NewViewSwitcher()
	viewSwitcher.SetStack(ob.viewStack)
	viewSwitcher.SetPolicy(adw.ViewSwitcherPolicyWideValue)

	// Header with switcher
	header := gtk.NewBox(gtk.OrientationHorizontalValue, 0)
	header.SetHalign(gtk.AlignCenterValue)
	header.SetMarginTop(12)
	header.SetMarginBottom(6)
	header.Append(&viewSwitcher.Widget)
	mainBox.Append(&header.Widget)

	// Add separator
	separator := gtk.NewSeparator(gtk.OrientationHorizontalValue)
	mainBox.Append(&separator.Widget)

	// Add view stack
	mainBox.Append(&ob.viewStack.Widget)

	return &mainBox.Widget
}

// onOperationChanged is called when any operation changes state.
// This callback is invoked via async.RunOnMain, so it's safe to update UI.
func (ob *OperationsButton) onOperationChanged(_ *Operation) {
	ob.refreshContent()
}

// refreshContent updates the popover content and badge.
func (ob *OperationsButton) refreshContent() {
	// Update badge
	count := ActiveCount()
	if count > 0 {
		ob.badge.SetLabel(fmt.Sprintf("%d", count))
		ob.badge.SetVisible(true)
	} else {
		ob.badge.SetVisible(false)
	}

	// Update active list
	ob.refreshActiveList()

	// Update history list
	ob.refreshHistoryList()
}

// refreshActiveList rebuilds the active operations list.
func (ob *OperationsButton) refreshActiveList() {
	// Clear existing content
	clearListBox(ob.activeList)

	ops := Active()

	// Show/hide empty state
	if len(ops) == 0 {
		ob.emptyState.SetVisible(true)
		ob.activeList.SetVisible(false)
		return
	}

	ob.emptyState.SetVisible(false)
	ob.activeList.SetVisible(true)

	// Group operations by category
	groups := make(map[Category][]*Operation)
	for _, op := range ops {
		groups[op.Category] = append(groups[op.Category], op)
	}

	// Sort categories for consistent ordering
	categories := []Category{CategoryLoading, CategoryInstall, CategoryUpdate}

	for _, cat := range categories {
		ops := groups[cat]
		if len(ops) == 0 {
			continue
		}

		// Add category header
		header := gtk.NewLabel(categoryTitle(cat))
		header.AddCssClass("heading")
		header.SetHalign(gtk.AlignStartValue)
		header.SetMarginTop(6)
		header.SetMarginBottom(6)
		header.SetMarginStart(6)
		ob.activeList.Append(&header.Widget)

		// Sort operations by start time
		sort.Slice(ops, func(i, j int) bool {
			return ops[i].StartedAt.Before(ops[j].StartedAt)
		})

		// Add operation rows
		for _, op := range ops {
			row := ob.buildActiveRow(op)
			ob.activeList.Append(&row.Widget)
		}
	}
}

// buildActiveRow creates a row for an active operation.
func (ob *OperationsButton) buildActiveRow(op *Operation) *adw.ActionRow {
	row := adw.NewActionRow()
	row.SetTitle(op.Name)

	// Build subtitle with message and state
	subtitle := op.Message
	if op.State == StateFailed && op.Error != nil {
		subtitle = fmt.Sprintf("Error: %s", op.Error.Error())
		row.AddCssClass("error")
	}
	row.SetSubtitle(subtitle)

	// Add progress indicator or spinner
	if op.State == StateActive {
		if op.Progress >= 0 && op.Progress <= 1 {
			// Determinate progress bar
			progressBar := gtk.NewProgressBar()
			progressBar.SetFraction(op.Progress)
			progressBar.SetValign(gtk.AlignCenterValue)
			progressBar.SetSizeRequest(60, -1)
			row.AddSuffix(&progressBar.Widget)
		} else {
			// Indeterminate spinner
			spinner := gtk.NewSpinner()
			spinner.Start()
			spinner.SetValign(gtk.AlignCenterValue)
			row.AddSuffix(&spinner.Widget)
		}
	}

	// Add action buttons based on state
	if op.State == StateFailed && op.RetryFunc != nil {
		retryBtn := gtk.NewButton()
		retryBtn.SetLabel("Retry")
		retryBtn.AddCssClass("suggested-action")
		retryBtn.SetValign(gtk.AlignCenterValue)

		opID := op.ID
		retryFn := op.RetryFunc
		clickedCb := func(_ gtk.Button) {
			// Clear the failed operation and retry
			if foundOp := Get(opID); foundOp != nil && foundOp.RetryFunc != nil {
				retryFn()
			}
		}
		retryBtn.ConnectClicked(&clickedCb)
		row.AddSuffix(&retryBtn.Widget)
	}

	if op.IsCancellable() {
		cancelBtn := gtk.NewButton()
		cancelBtn.SetIconName("process-stop-symbolic")
		cancelBtn.SetTooltipText("Cancel")
		cancelBtn.AddCssClass("flat")
		cancelBtn.SetValign(gtk.AlignCenterValue)

		opID := op.ID
		opName := op.Name
		clickedCb := func(_ gtk.Button) {
			// Show confirmation dialog
			// Note: We need parent window context - caller should provide this
			// For now, directly cancel (dialog shown from caller context)
			if foundOp := Get(opID); foundOp != nil {
				foundOp.Cancel()
			}
		}
		// Store operation info for potential dialog use
		_ = opName
		cancelBtn.ConnectClicked(&clickedCb)
		row.AddSuffix(&cancelBtn.Widget)
	}

	return row
}

// refreshHistoryList rebuilds the history list.
func (ob *OperationsButton) refreshHistoryList() {
	// Clear existing content
	clearListBox(ob.historyList)

	history := History()
	if len(history) == 0 {
		emptyState := adw.NewStatusPage()
		emptyState.SetTitle("No Completed Operations")
		emptyState.SetDescription("Completed operations will appear here")
		emptyState.SetIconName("document-open-recent-symbolic")
		emptyState.AddCssClass("compact")
		ob.historyList.Append(&emptyState.Widget)
		return
	}

	// Sort by completion time (most recent first)
	sort.Slice(history, func(i, j int) bool {
		return history[i].EndedAt.After(history[j].EndedAt)
	})

	for _, op := range history {
		row := ob.buildHistoryRow(op)
		ob.historyList.Append(&row.Widget)
	}
}

// buildHistoryRow creates a row for a completed operation.
func (ob *OperationsButton) buildHistoryRow(op *Operation) *adw.ActionRow {
	row := adw.NewActionRow()
	row.SetTitle(op.Name)

	// Build subtitle with outcome and timing
	outcome := stateLabel(op.State)
	duration := formatDuration(op.Duration())
	timeAgo := formatTimeAgo(op.EndedAt)

	subtitle := fmt.Sprintf("%s • %s • %s", outcome, duration, timeAgo)
	row.SetSubtitle(subtitle)

	// Add state icon
	icon := gtk.NewImageFromIconName(stateIcon(op.State))
	icon.SetValign(gtk.AlignCenterValue)
	row.AddPrefix(&icon.Widget)

	return row
}

// categoryTitle returns a human-readable title for a category.
func categoryTitle(cat Category) string {
	switch cat {
	case CategoryInstall:
		return "Installing"
	case CategoryUpdate:
		return "Updating"
	case CategoryLoading:
		return "Loading"
	default:
		return string(cat)
	}
}

// stateLabel returns a human-readable label for an operation state.
func stateLabel(state OperationState) string {
	switch state {
	case StateCompleted:
		return "Completed"
	case StateFailed:
		return "Failed"
	case StateCancelled:
		return "Cancelled"
	default:
		return "Unknown"
	}
}

// stateIcon returns an appropriate icon name for an operation state.
func stateIcon(state OperationState) string {
	switch state {
	case StateCompleted:
		return "emblem-ok-symbolic"
	case StateFailed:
		return "dialog-error-symbolic"
	case StateCancelled:
		return "process-stop-symbolic"
	default:
		return "dialog-question-symbolic"
	}
}

// formatDuration formats a duration for display.
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return "<1s"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
}

// formatTimeAgo formats a time as relative to now.
func formatTimeAgo(t time.Time) string {
	d := time.Since(t)

	if d < time.Minute {
		return "Just now"
	}
	if d < time.Hour {
		mins := int(d.Minutes())
		if mins == 1 {
			return "1 min ago"
		}
		return fmt.Sprintf("%d mins ago", mins)
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	}

	return t.Format("Jan 2, 3:04 PM")
}

// clearListBox removes all children from a ListBox.
func clearListBox(list *gtk.ListBox) {
	// Remove all children by iterating through rows
	for {
		row := list.GetRowAtIndex(0)
		if row == nil {
			break
		}
		list.Remove(&row.Widget)
	}
}
