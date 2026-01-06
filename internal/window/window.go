// Package window provides the main application window
package window

import (
	"fmt"
	"log"

	"github.com/frostyard/chairlift/internal/config"
	"github.com/frostyard/chairlift/internal/views"

	"github.com/jwijenbergh/puregotk/v4/adw"
	"github.com/jwijenbergh/puregotk/v4/gio"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

// Window represents the main application window
type Window struct {
	*adw.ApplicationWindow

	splitView    *adw.NavigationSplitView
	sidebarList  *gtk.ListBox
	contentStack *gtk.Stack
	toasts       *adw.ToastOverlay

	pages       map[string]*adw.ToolbarView
	navRows     map[string]*adw.ActionRow // Store references to nav rows for badges
	config      *config.Config
	views       *views.UserHome
	updateBadge *gtk.Button // Badge for updates count
}

// NavItem represents a navigation item in the sidebar
type NavItem struct {
	Name  string
	Title string
	Icon  string
}

// navItems defines the sidebar navigation structure
var navItems = []NavItem{
	{Name: "applications", Title: "Applications", Icon: "application-x-executable-symbolic"},
	{Name: "maintenance", Title: "Maintenance", Icon: "emblem-system-symbolic"},
	{Name: "updates", Title: "Updates", Icon: "software-update-available-symbolic"},
	{Name: "system", Title: "System", Icon: "computer-symbolic"},
	{Name: "help", Title: "Help", Icon: "help-browser-symbolic"},
}

// New creates a new main window
func New(app *adw.Application) *Window {
	w := &Window{
		ApplicationWindow: adw.NewApplicationWindow(&app.Application),
		pages:             make(map[string]*adw.ToolbarView),
		navRows:           make(map[string]*adw.ActionRow),
		config:            config.Load(),
	}

	w.SetDefaultSize(900, 700)
	w.SetTitle("ChairLift")

	w.buildUI()
	w.setupActions()

	return w
}

// buildUI constructs the window UI
func (w *Window) buildUI() {
	// Create views manager
	w.views = views.New(w.config, w)

	// Create the navigation split view
	w.splitView = adw.NewNavigationSplitView()

	// Create sidebar
	sidebarPage := w.buildSidebar()
	w.splitView.SetSidebar(sidebarPage)

	// Create content area
	contentPage := w.buildContentArea()
	w.splitView.SetContent(contentPage)

	// Create toast overlay for notifications
	w.toasts = adw.NewToastOverlay()
	w.toasts.SetChild(&w.splitView.Widget)

	// Set window content
	w.SetContent(&w.toasts.Widget)
}

// buildSidebar creates the sidebar navigation
func (w *Window) buildSidebar() *adw.NavigationPage {
	// Create toolbar view for sidebar
	toolbarView := adw.NewToolbarView()

	// Add header bar
	headerBar := adw.NewHeaderBar()
	headerBar.SetShowEndTitleButtons(false)
	toolbarView.AddTopBar(&headerBar.Widget)

	// Create scrolled window for the list
	scrolled := gtk.NewScrolledWindow()
	scrolled.SetPolicy(gtk.PolicyNeverValue, gtk.PolicyAutomaticValue)
	scrolled.SetVexpand(true)

	// Create list box for navigation
	w.sidebarList = gtk.NewListBox()
	w.sidebarList.SetSelectionMode(gtk.SelectionSingleValue)
	w.sidebarList.AddCssClass("navigation-sidebar")

	// Add navigation items
	for _, item := range navItems {
		row := w.createNavRow(item)
		w.sidebarList.Append(&row.Widget)
	}

	// Connect row activation
	rowActivatedCb := func(listbox gtk.ListBox, rowPtr uintptr) {
		// Convert uintptr to ListBoxRow
		row := gtk.ListBoxRowNewFromInternalPtr(rowPtr)
		w.onSidebarRowActivated(*row)
	}
	w.sidebarList.ConnectRowActivated(&rowActivatedCb)

	scrolled.SetChild(&w.sidebarList.Widget)
	toolbarView.SetContent(&scrolled.Widget)

	// Create navigation page
	navPage := adw.NewNavigationPage(&toolbarView.Widget, "ChairLift")

	return navPage
}

// createNavRow creates a navigation row for the sidebar
func (w *Window) createNavRow(item NavItem) *adw.ActionRow {
	row := adw.NewActionRow()
	row.SetTitle(item.Title)
	row.SetActivatable(true)

	// Add icon
	icon := gtk.NewImageFromIconName(item.Icon)
	row.AddPrefix(&icon.Widget)

	// Add badge for updates row (hidden by default)
	if item.Name == "updates" {
		w.updateBadge = gtk.NewButton()
		w.updateBadge.AddCssClass("circular")
		w.updateBadge.AddCssClass("warning")
		w.updateBadge.SetVisible(false)
		row.AddSuffix(&w.updateBadge.Widget)
	}

	// Store the page name in the row (using SetName for identification)
	row.SetName(item.Name)

	// Store reference to the row
	w.navRows[item.Name] = row

	return row
}

// buildContentArea creates the content stack
func (w *Window) buildContentArea() *adw.NavigationPage {
	// Create stack for content pages
	w.contentStack = gtk.NewStack()
	w.contentStack.SetTransitionType(gtk.StackTransitionTypeCrossfadeValue)

	// Add pages to the stack
	for _, item := range navItems {
		page := w.views.GetPage(item.Name)
		if page != nil {
			w.pages[item.Name] = page
			w.contentStack.AddNamed(&page.Widget, item.Name)
		}
	}

	// Create navigation page
	navPage := adw.NewNavigationPage(&w.contentStack.Widget, "Content")

	// Select first item by default
	if len(navItems) > 0 {
		firstRow := w.sidebarList.GetRowAtIndex(0)
		if firstRow != nil {
			w.sidebarList.SelectRow(firstRow)
			w.contentStack.SetVisibleChildName(navItems[0].Name)
		}
	}

	return navPage
}

// onSidebarRowActivated handles sidebar row activation
func (w *Window) onSidebarRowActivated(row gtk.ListBoxRow) {
	// Get the ActionRow from the ListBoxRow
	widget := row.GetChild()
	if widget == nil {
		return
	}

	// Get the name we stored
	name := row.GetName()
	if name == "" {
		return
	}

	// Switch to the corresponding page
	if _, ok := w.pages[name]; ok {
		w.contentStack.SetVisibleChildName(name)
		w.splitView.SetShowContent(true)
	}
}

// setupActions sets up window actions
func (w *Window) setupActions() {
	// Show shortcuts action
	shortcutsAction := gio.NewSimpleAction("show-shortcuts", nil)
	shortcutsActivateCb := func(action gio.SimpleAction, param uintptr) {
		w.onShowShortcuts()
	}
	shortcutsAction.ConnectActivate(&shortcutsActivateCb)
	w.AddAction(shortcutsAction)

	// About action
	aboutAction := gio.NewSimpleAction("show-about", nil)
	aboutActivateCb := func(action gio.SimpleAction, param uintptr) {
		w.onShowAbout()
	}
	aboutAction.ConnectActivate(&aboutActivateCb)
	w.AddAction(aboutAction)

	// Navigation actions
	for _, item := range navItems {
		itemName := item.Name // Capture for closure
		action := gio.NewSimpleAction("navigate-"+itemName, nil)
		navActivateCb := func(action gio.SimpleAction, param uintptr) {
			w.navigateToPage(itemName)
		}
		action.ConnectActivate(&navActivateCb)
		w.AddAction(action)
	}
}

// navigateToPage navigates to a specific page
func (w *Window) navigateToPage(pageName string) {
	if _, ok := w.pages[pageName]; ok {
		w.contentStack.SetVisibleChildName(pageName)

		// Select the corresponding row
		for i, item := range navItems {
			if item.Name == pageName {
				row := w.sidebarList.GetRowAtIndex(i)
				if row != nil {
					w.sidebarList.SelectRow(row)
				}
				break
			}
		}
	}
}

// onShowShortcuts shows the keyboard shortcuts window
func (w *Window) onShowShortcuts() {
	log.Println("Show shortcuts requested")
	// TODO: Implement shortcuts window
}

// onShowAbout shows the about dialog
func (w *Window) onShowAbout() {
	about := adw.NewAboutWindow()
	about.SetTransientFor(&w.Window)
	about.SetApplicationName("ChairLift")
	about.SetApplicationIcon("org.frostyard.ChairLift")
	about.SetVersion("1.0.0")
	about.SetDeveloperName("Frostyard")
	about.SetWebsite("https://github.com/frostyard/chairlift")
	about.SetIssueUrl("https://github.com/frostyard/chairlift/issues")
	about.SetLicenseType(gtk.LicenseGpl30Value)
	about.SetCopyright("© 2024-2026 Frostyard")
	about.SetDevelopers([]string{"mirkobrombin", "Frostyard Contributors"})
	about.Present()
}

// AddToast adds a toast notification
func (w *Window) AddToast(toast *adw.Toast) {
	w.toasts.AddToast(toast)
}

// ShowToast shows a simple toast message
func (w *Window) ShowToast(message string) {
	toast := adw.NewToast(message)
	toast.SetTimeout(3)
	w.AddToast(toast)
}

// ShowErrorToast shows an error toast
func (w *Window) ShowErrorToast(message string) {
	toast := adw.NewToast(message)
	toast.SetTimeout(0) // Persist until dismissed
	w.AddToast(toast)
}

// SetUpdateBadge updates the badge on the Updates navigation row
func (w *Window) SetUpdateBadge(count int) {
	if w.updateBadge == nil {
		return
	}

	if count > 0 {
		w.updateBadge.SetLabel(fmt.Sprintf("%d", count))
		w.updateBadge.SetVisible(true)
	} else {
		w.updateBadge.SetVisible(false)
	}
}
