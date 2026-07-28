// Package window provides the main application window
package window

import (
	"fmt"
	"log"
	"time"
	"unsafe"

	"github.com/frostyard/chairlift/internal/config"
	"github.com/frostyard/chairlift/internal/navigation"
	"github.com/frostyard/chairlift/internal/version"
	"github.com/frostyard/chairlift/internal/views"

	"github.com/frostyard/snowkit/gobj"

	"codeberg.org/puregotk/puregotk/v4/adw"
	"codeberg.org/puregotk/puregotk/v4/gio"
	"codeberg.org/puregotk/puregotk/v4/gobject"
	"codeberg.org/puregotk/puregotk/v4/gtk"
)

var (
	gTypeWindow    gobject.Type
	windowRegistry *gobj.InstanceRegistry
)

// Window represents the main application window
type Window struct {
	adw.ApplicationWindow

	splitView    *adw.NavigationSplitView
	sidebarList  *gtk.ListBox
	contentStack *gtk.Stack
	contentPage  *adw.NavigationPage // Content navigation page for dynamic title
	toasts       *adw.ToastOverlay

	pages       map[string]*adw.ToolbarView
	navRows     map[string]*adw.ActionRow // Store references to nav rows for badges
	config      *config.Config
	configError *config.LoadError
	views       *views.UserHome
	updateBadge *gtk.Button // Badge for updates count
	navItems    []navigation.Item
}

func init() {
	gTypeWindow, windowRegistry = gobj.RegisterType(gobj.TypeDef{
		ParentGLibType: adw.ApplicationWindowGLibType,
		ClassName:      "ChairLiftWindow",
		ClassInit: func(tc *gobject.TypeClass, reg *gobj.InstanceRegistry) {
			objClass := (*gobject.ObjectClass)(unsafe.Pointer(tc))
			objClass.OverrideConstructed(func(o *gobject.Object) {
				windowStart := time.Now()

				parentObjClass := (*gobject.ObjectClass)(unsafe.Pointer(tc.PeekParent()))
				parentObjClass.GetConstructed()(o)

				var parent adw.ApplicationWindow
				o.Cast(&parent)

				cfgStart := time.Now()
				cfg, configErr := config.Load()
				log.Printf("window: config loaded in %s", time.Since(cfgStart))

				w := &Window{
					ApplicationWindow: parent,
					pages:             make(map[string]*adw.ToolbarView),
					navRows:           make(map[string]*adw.ActionRow),
					config:            cfg,
					configError:       configErr,
				}

				reg.Pin(o, unsafe.Pointer(w))

				w.SetDefaultSize(900, 700)
				w.SetTitle("ChairLift")
				w.buildUI()
				if w.configError != nil {
					// OverrideConstructed and buildUI both run on GTK's main
					// thread; the toast overlay now exists and timeout 0 makes
					// this diagnostic persist until dismissed.
					w.ShowErrorToast(w.configError.ToastMessage())
				}
				w.setupActions()

				log.Printf("window: constructed in %s", time.Since(windowStart))
			})
		},
	})
}

// New creates a new main window
func New(app adw.Application) *Window {
	obj := gobject.NewObject(gTypeWindow, "application", &app.Application)
	if obj == nil {
		log.Fatal("Failed to create window")
	}
	return (*Window)(windowRegistry.Get(obj.GoPointer()))
}

// buildUI constructs the window UI
func (w *Window) buildUI() {
	start := time.Now()

	w.navItems = navigation.VisibleItems(w.config.IsGroupEnabled)

	// Create views manager
	w.views = views.New(w.config, w)
	log.Printf("window: views built in %s", time.Since(start))

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
	for _, item := range w.navItems {
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
func (w *Window) createNavRow(item navigation.Item) *adw.ActionRow {
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
	items := w.navItems
	for _, item := range items {
		page := w.views.GetPage(item.Name)
		if page != nil {
			w.pages[item.Name] = page
			w.contentStack.AddNamed(&page.Widget, item.Name)
		}
	}

	// Create navigation page with initial title from first nav item
	initialTitle := "Content"
	if len(items) > 0 {
		initialTitle = items[0].Title
	}
	w.contentPage = adw.NewNavigationPage(&w.contentStack.Widget, initialTitle)

	// Select first item by default
	if len(items) > 0 {
		firstRow := w.sidebarList.GetRowAtIndex(0)
		if firstRow != nil {
			w.sidebarList.SelectRow(firstRow)
			w.contentStack.SetVisibleChildName(items[0].Name)
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

	w.navigateToPage(name)
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
	for _, item := range w.navItems {
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
	transition, ok := navigation.Resolve(pageName, w.navItems, func(name string) bool {
		_, exists := w.pages[name]
		return exists
	})
	if !ok {
		return
	}

	row := w.sidebarList.GetRowAtIndex(int32(transition.SelectedIndex))
	if row != nil {
		w.sidebarList.SelectRow(row)
	}
	w.contentStack.SetVisibleChildName(transition.VisibleChild)
	w.contentPage.SetTitle(transition.Title)
	w.splitView.SetShowContent(transition.ShowContent)
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

	for _, shortcut := range navigation.Shortcuts(w.navItems) {
		if shortcut.Group != navigation.GroupNavigation {
			continue
		}
		row := adw.NewActionRow()
		row.SetTitle(shortcut.Title)

		label := gtk.NewLabel(shortcut.Display)
		label.AddCssClass("dim-label")
		row.AddSuffix(&label.Widget)

		navGroup.Add(&row.Widget)
	}

	mainBox.Append(&navGroup.Widget)

	// General shortcuts group
	generalGroup := adw.NewPreferencesGroup()
	generalGroup.SetTitle("General")

	for _, shortcut := range navigation.Shortcuts(w.navItems) {
		if shortcut.Group != navigation.GroupGeneral {
			continue
		}
		row := adw.NewActionRow()
		row.SetTitle(shortcut.Title)

		label := gtk.NewLabel(shortcut.Display)
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

// NavigationItems returns the visible, compacted page inventory used by this
// window. The application uses the same inventory to register accelerators.
func (w *Window) NavigationItems() []navigation.Item {
	return append([]navigation.Item(nil), w.navItems...)
}

// onShowAbout shows the about dialog
func (w *Window) onShowAbout() {
	about := adw.NewAboutWindow()
	about.SetTransientFor(&w.Window)
	about.SetApplicationName("ChairLift")
	about.SetApplicationIcon("org.frostyard.ChairLift")
	about.SetVersion(version.Version)
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
