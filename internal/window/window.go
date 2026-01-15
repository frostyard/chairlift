// Package window provides the main application window
package window

import (
	"fmt"

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
	contentPage  *adw.NavigationPage // Content navigation page for dynamic title
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
	{Name: "extensions", Title: "Extensions", Icon: "application-x-addon-symbolic"},
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

	// Add header bar with menu button
	headerBar := adw.NewHeaderBar()
	headerBar.SetShowEndTitleButtons(false)

	// Create hamburger menu button
	menuButton := w.buildMenuButton()
	headerBar.PackEnd(&menuButton.Widget)

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

	// Create navigation page with initial title from first nav item
	initialTitle := "Content"
	if len(navItems) > 0 {
		initialTitle = navItems[0].Title
	}
	w.contentPage = adw.NewNavigationPage(&w.contentStack.Widget, initialTitle)

	// Select first item by default
	if len(navItems) > 0 {
		firstRow := w.sidebarList.GetRowAtIndex(0)
		if firstRow != nil {
			w.sidebarList.SelectRow(firstRow)
			w.contentStack.SetVisibleChildName(navItems[0].Name)
		}
	}

	return w.contentPage
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

		// Update the content page title
		for _, item := range navItems {
			if item.Name == name {
				w.contentPage.SetTitle(item.Title)
				break
			}
		}
	}
}

// buildMenuButton creates the hamburger menu button
func (w *Window) buildMenuButton() *gtk.MenuButton {
	// Create menu model
	menu := gio.NewMenu()

	// Add menu items
	menu.Append("Keyboard Shortcuts", "win.show-shortcuts")
	menu.Append("About ChairLift", "win.show-about")

	// Create menu button
	menuButton := gtk.NewMenuButton()
	menuButton.SetIconName("open-menu-symbolic")
	menuButton.SetMenuModel(&menu.MenuModel)
	menuButton.SetTooltipText("Main Menu")

	return menuButton
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

		// Select the corresponding row and update title
		for i, item := range navItems {
			if item.Name == pageName {
				row := w.sidebarList.GetRowAtIndex(i)
				if row != nil {
					w.sidebarList.SelectRow(row)
				}
				w.contentPage.SetTitle(item.Title)
				break
			}
		}
	}
}

// onShowShortcuts shows the keyboard shortcuts window
func (w *Window) onShowShortcuts() {
	// Create a dialog to show shortcuts since GtkShortcutsWindow isn't available in puregotk
	dialog := adw.NewWindow()
	dialog.SetTransientFor(&w.Window)
	dialog.SetModal(true)
	dialog.SetTitle("Keyboard Shortcuts")
	dialog.SetDefaultSize(400, 450)

	// Create toolbar view
	toolbarView := adw.NewToolbarView()

	// Add header bar
	headerBar := adw.NewHeaderBar()
	toolbarView.AddTopBar(&headerBar.Widget)

	// Create scrolled window
	scrolled := gtk.NewScrolledWindow()
	scrolled.SetPolicy(gtk.PolicyNeverValue, gtk.PolicyAutomaticValue)
	scrolled.SetVexpand(true)

	// Create main box
	mainBox := gtk.NewBox(gtk.OrientationVerticalValue, 0)
	mainBox.SetMarginTop(12)
	mainBox.SetMarginBottom(12)
	mainBox.SetMarginStart(12)
	mainBox.SetMarginEnd(12)

	// Create clamp for content width
	clamp := adw.NewClamp()
	clamp.SetMaximumSize(400)

	// Navigation shortcuts group
	navGroup := adw.NewPreferencesGroup()
	navGroup.SetTitle("Navigation")

	navShortcuts := []struct {
		accel string
		title string
	}{
		{"Alt+1", "Go to Applications"},
		{"Alt+2", "Go to Maintenance"},
		{"Alt+3", "Go to Updates"},
		{"Alt+4", "Go to System"},
		{"Alt+5", "Go to Help"},
	}

	for _, s := range navShortcuts {
		row := adw.NewActionRow()
		row.SetTitle(s.title)

		label := gtk.NewLabel(s.accel)
		label.AddCssClass("dim-label")
		row.AddSuffix(&label.Widget)

		navGroup.Add(&row.Widget)
	}

	mainBox.Append(&navGroup.Widget)

	// General shortcuts group
	generalGroup := adw.NewPreferencesGroup()
	generalGroup.SetTitle("General")

	generalShortcuts := []struct {
		accel string
		title string
	}{
		{"Ctrl+?", "Keyboard Shortcuts"},
		{"Ctrl+Q", "Quit"},
		{"F1", "Help"},
	}

	for _, s := range generalShortcuts {
		row := adw.NewActionRow()
		row.SetTitle(s.title)

		label := gtk.NewLabel(s.accel)
		label.AddCssClass("dim-label")
		row.AddSuffix(&label.Widget)

		generalGroup.Add(&row.Widget)
	}

	mainBox.Append(&generalGroup.Widget)

	clamp.SetChild(&mainBox.Widget)
	scrolled.SetChild(&clamp.Widget)
	toolbarView.SetContent(&scrolled.Widget)

	dialog.SetContent(&toolbarView.Widget)
	dialog.Present()
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
	about.SetDevelopers([]string{"Brian Ketelsen", "ChairLift Contributors"})
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
