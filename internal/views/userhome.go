// Package views provides the page content for the ChairLift application
package views

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/frostyard/chairlift/internal/async"
	"github.com/frostyard/chairlift/internal/config"
	"github.com/frostyard/chairlift/internal/instex"
	"github.com/frostyard/chairlift/internal/nbc"
	"github.com/frostyard/chairlift/internal/operations"
	"github.com/frostyard/chairlift/internal/pages"
	"github.com/frostyard/chairlift/internal/pages/help"
	"github.com/frostyard/chairlift/internal/pages/maintenance"
	"github.com/frostyard/chairlift/internal/pages/system"
	"github.com/frostyard/chairlift/internal/pm"
	"github.com/frostyard/chairlift/internal/updex"

	pmlib "github.com/frostyard/pm"
	"github.com/jwijenbergh/puregotk/v4/adw"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

// ToastAdder is an interface for adding toasts and notifying about updates
type ToastAdder interface {
	ShowToast(message string)
	ShowErrorToast(message string)
	SetUpdateBadge(count int)
}

// UserHome manages all content pages
type UserHome struct {
	config     *config.Config
	toastAdder ToastAdder

	// Page packages (extracted pages with lifecycle management)
	systemPagePkg      *system.Page
	helpPagePkg        *help.Page
	maintenancePagePkg *maintenance.Page

	// Pages (ToolbarViews) - widgets returned by page packages or createPage()
	systemPage       *adw.ToolbarView
	updatesPage      *adw.ToolbarView
	applicationsPage *adw.ToolbarView
	extensionsPage   *adw.ToolbarView
	helpPage         *adw.ToolbarView

	// PreferencesPages inside each ToolbarView - keep references to prevent GC
	// (System, Help, and Maintenance no longer need these - managed by their page packages)
	updatesPrefsPage      *adw.PreferencesPage
	applicationsPrefsPage *adw.PreferencesPage
	extensionsPrefsPage   *adw.PreferencesPage

	// References for dynamic updates
	formulaeExpander       *adw.ExpanderRow
	casksExpander          *adw.ExpanderRow
	outdatedExpander       *adw.ExpanderRow
	searchResultsExpander  *adw.ExpanderRow
	searchEntry            *gtk.SearchEntry
	flatpakUserExpander    *adw.ExpanderRow
	flatpakSystemExpander  *adw.ExpanderRow
	flatpakUpdatesExpander *adw.ExpanderRow
	flatpakUpdateRows      []*adw.ActionRow // Store references for cleanup
	snapExpander           *adw.ExpanderRow
	snapStoreLinkRow       *adw.ActionRow
	snapStoreInstallRow    *adw.ActionRow
	snapRows               []*adw.ActionRow // Store references for cleanup

	// NBC update references
	nbcUpdateBtn      *gtk.Button
	nbcDownloadBtn    *gtk.Button
	nbcUpdateExpander *adw.ExpanderRow
	nbcCheckRow       *adw.ActionRow

	// Extensions page references
	extensionsGroup        *adw.PreferencesGroup
	discoverEntry          *gtk.Entry
	discoverResultsGroup   *adw.PreferencesGroup
	discoverResultRows     []*adw.ActionRow // Track rows to clear on new discovery
	installedComponentsMap map[string]bool  // Cache of installed component names

	// Progress tracking UI
	progressBottomSheet *adw.BottomSheet     // BottomSheet for displaying active operations
	progressPage        *adw.PreferencesPage // Preferences page inside the bottom sheet
	progressScrolled    *gtk.ScrolledWindow  // Scrolled window for progress content

	// Update badge tracking
	nbcUpdateCount     int
	flatpakUpdateCount int
	brewUpdateCount    int
	updateCountMu      sync.Mutex

	// Progress UI tracking
	progressExpanders map[string]*adw.ExpanderRow      // Map of action names to expander rows
	progressGroups    map[string]*adw.PreferencesGroup // Map of action names to preference groups
	progressRows      map[string]*adw.ActionRow        // Map of action:task keys to progress rows
	progressSpinners  map[string]*gtk.Spinner          // Map of action:task keys to spinner widgets
	progressActions   map[string]string                // Map of action IDs to action names
	progressTasks     map[string]string                // Map of task IDs to "actionName:taskName" keys
	currentProgressMu sync.Mutex
}

// TODO: Call page Destroy() methods when view lifecycle is added
// System page's Destroy() cancels NBC status fetch goroutine
// Help page's Destroy() is a no-op (no goroutines)
// Maintenance page's Destroy() cancels action goroutines
// Example: func (uh *UserHome) Destroy() {
//     if uh.systemPagePkg != nil { uh.systemPagePkg.Destroy() }
//     if uh.helpPagePkg != nil { uh.helpPagePkg.Destroy() }
//     if uh.maintenancePagePkg != nil { uh.maintenancePagePkg.Destroy() }
// }

// New creates a new UserHome views manager
func New(cfg *config.Config, toastAdder ToastAdder) *UserHome {
	uh := &UserHome{
		config:            cfg,
		toastAdder:        toastAdder,
		progressExpanders: make(map[string]*adw.ExpanderRow),
		progressGroups:    make(map[string]*adw.PreferencesGroup),
		progressRows:      make(map[string]*adw.ActionRow),
		progressSpinners:  make(map[string]*gtk.Spinner),
		progressActions:   make(map[string]string),
		progressTasks:     make(map[string]string),
	}

	// Create page packages (System and Help) with dependency injection
	deps := pages.Deps{
		Config:  cfg,
		Toaster: toastAdder,
	}

	// Create System page using extracted package
	uh.systemPagePkg = system.New(deps, uh.launchApp, uh.openURL)
	uh.systemPage = uh.systemPagePkg.Widget()

	// Create Help page using extracted package
	uh.helpPagePkg = help.New(deps, uh.openURL)
	uh.helpPage = uh.helpPagePkg.Widget()

	// Create Maintenance page using extracted package
	uh.maintenancePagePkg = maintenance.New(deps)

	// Create remaining pages using createPage (not yet extracted)
	uh.updatesPage, uh.updatesPrefsPage = uh.createPage()
	uh.applicationsPage, uh.applicationsPrefsPage = uh.createPage()
	uh.extensionsPage, uh.extensionsPrefsPage = uh.createPage()

	// Re-initialize Flatpak and Homebrew managers with progress callback
	// This allows us to receive progress updates from long-running operations
	// Do this asynchronously after page creation to avoid blocking the UI
	go func() {
		// Initialize PM with progress callback
		if err := pm.InitializeFlatpak(uh.onPMProgressUpdate); err != nil {
			log.Printf("Warning: Failed to re-initialize Flatpak with progress callback: %v", err)
		}

		if err := pm.InitializeHomebrew(uh.onPMProgressUpdate); err != nil {
			log.Printf("Warning: Failed to re-initialize Homebrew with progress callback: %v", err)
		}

		// Give async availability checks time to complete after re-initialization
		time.Sleep(200 * time.Millisecond)

		// Build page content now that PM managers are initialized with progress callbacks
		// NOTE: System, Help, and Maintenance pages build their UI in their constructors, so no calls here
		uh.buildUpdatesPage()
		uh.buildApplicationsPage()
		uh.buildExtensionsPage()
	}()

	return uh
}

// updateBadgeCount updates the total update count and notifies the window
func (uh *UserHome) updateBadgeCount() {
	uh.updateCountMu.Lock()
	total := uh.nbcUpdateCount + uh.flatpakUpdateCount + uh.brewUpdateCount
	uh.updateCountMu.Unlock()

	async.RunOnMain(func() {
		uh.toastAdder.SetUpdateBadge(total)
	})
}

// GetPage returns a page by name
func (uh *UserHome) GetPage(name string) *adw.ToolbarView {
	switch name {
	case "system":
		return uh.systemPage
	case "updates":
		return uh.updatesPage
	case "applications":
		return uh.applicationsPage
	case "maintenance":
		return uh.maintenancePagePkg.Widget()
	case "extensions":
		return uh.extensionsPage
	case "help":
		return uh.helpPage
	default:
		return nil
	}
}

// BuildProgressBottomSheet creates and returns the progress BottomSheet
func (uh *UserHome) BuildProgressBottomSheet() *adw.BottomSheet {
	// Create the bottom sheet
	uh.progressBottomSheet = adw.NewBottomSheet()
	uh.progressBottomSheet.SetModal(true)
	uh.progressBottomSheet.SetShowDragHandle(true)
	uh.progressBottomSheet.SetFullWidth(true)

	// Create scrolled window for the sheet content
	uh.progressScrolled = gtk.NewScrolledWindow()
	uh.progressScrolled.SetPolicy(gtk.PolicyNeverValue, gtk.PolicyAutomaticValue)
	uh.progressScrolled.SetVexpand(true)
	uh.progressScrolled.SetMinContentHeight(400)
	uh.progressScrolled.SetMaxContentHeight(600)

	// Create preferences page for progress items
	uh.progressPage = adw.NewPreferencesPage()
	uh.progressPage.SetTitle("Activity Monitor")
	uh.progressPage.SetDescription("Active operations and progress")
	uh.progressScrolled.SetChild(&uh.progressPage.Widget)

	// Set the sheet widget
	uh.progressBottomSheet.SetSheet(&uh.progressScrolled.Widget)

	return uh.progressBottomSheet
}

// createPage creates a page with toolbar view and scrolled content
func (uh *UserHome) createPage() (*adw.ToolbarView, *adw.PreferencesPage) {
	toolbarView := adw.NewToolbarView()

	// Add header bar
	headerBar := adw.NewHeaderBar()
	toolbarView.AddTopBar(&headerBar.Widget)

	// Create scrolled window with preferences page
	scrolled := gtk.NewScrolledWindow()
	scrolled.SetPolicy(gtk.PolicyNeverValue, gtk.PolicyAutomaticValue)
	scrolled.SetVexpand(true)

	prefsPage := adw.NewPreferencesPage()
	scrolled.SetChild(&prefsPage.Widget)

	toolbarView.SetContent(&scrolled.Widget)

	return toolbarView, prefsPage
}

// onSystemUpdateClicked handles the system update button click using the nbc package
func (uh *UserHome) onSystemUpdateClicked(button *gtk.Button) {
	// Disable the button while updating
	if button != nil {
		button.SetSensitive(false)
		button.SetLabel("Updating...")
	}
	uh.toastAdder.ShowToast("Starting system update...")

	go func() {
		ctx, cancel := nbc.DefaultContext()
		defer cancel()

		progressCh := make(chan nbc.ProgressEvent)

		var updateErr error
		var wg sync.WaitGroup
		wg.Add(1)

		go func() {
			defer wg.Done()
			updateErr = nbc.Update(ctx, nbc.UpdateOptions{Auto: true}, progressCh)
		}()

		// Process progress events and show key updates via toasts
		var lastStep string
		for event := range progressCh {
			evt := event
			if evt.Type == nbc.EventTypeStep && evt.StepName != lastStep {
				lastStep = evt.StepName
				async.RunOnMain(func() {
					uh.toastAdder.ShowToast(fmt.Sprintf("[%d/%d] %s", evt.Step, evt.TotalSteps, evt.StepName))
				})
			} else if evt.Type == nbc.EventTypeError {
				async.RunOnMain(func() {
					uh.toastAdder.ShowErrorToast(evt.Message)
				})
			}
		}

		wg.Wait()

		async.RunOnMain(func() {
			// Re-enable the button
			if button != nil {
				button.SetSensitive(true)
				button.SetLabel("Update")
			}

			if updateErr != nil {
				userErr := async.NewUserErrorWithHint(
					"Couldn't update the system",
					"Try again later or check your internet connection",
					updateErr,
				)
				uh.toastAdder.ShowErrorToast(userErr.FormatForUser())
				log.Printf("System update error details: %v", updateErr)
			} else {
				uh.toastAdder.ShowToast("Update complete! Reboot now to apply changes.")
			}
		})
	}()
}

// buildUpdatesPage builds the Updates page content
func (uh *UserHome) buildUpdatesPage() {
	page := uh.updatesPrefsPage
	if page == nil {
		return
	}

	// NBC System Updates group - only show if NBC is available
	if _, err := os.Stat("/run/nbc-booted"); err == nil {
		if uh.config.IsGroupEnabled("updates_page", "nbc_updates_group") {
			group := adw.NewPreferencesGroup()
			group.SetTitle("System Updates")
			group.SetDescription("Check for and install NBC system updates")

			// Check for updates row
			uh.nbcCheckRow = adw.NewActionRow()
			uh.nbcCheckRow.SetTitle("Check for Updates")
			uh.nbcCheckRow.SetSubtitle("Checking...")

			checkBtn := gtk.NewButtonWithLabel("Check")
			checkBtn.SetValign(gtk.AlignCenterValue)
			checkClickedCb := func(btn gtk.Button) {
				uh.onNBCCheckUpdateClicked()
			}
			checkBtn.ConnectClicked(&checkClickedCb)
			uh.nbcCheckRow.AddSuffix(&checkBtn.Widget)
			group.Add(&uh.nbcCheckRow.Widget)

			// Update now row with progress expander
			uh.nbcUpdateExpander = adw.NewExpanderRow()
			uh.nbcUpdateExpander.SetTitle("Install System Update")
			uh.nbcUpdateExpander.SetSubtitle("Checking for updates...")

			uh.nbcDownloadBtn = gtk.NewButtonWithLabel("Download")
			uh.nbcDownloadBtn.SetValign(gtk.AlignCenterValue)
			uh.nbcDownloadBtn.SetSensitive(false) // Disabled until we check for updates
			downloadClickedCb := func(btn gtk.Button) {
				uh.onNBCDownloadClicked(uh.nbcUpdateExpander, uh.nbcDownloadBtn)
			}
			uh.nbcDownloadBtn.ConnectClicked(&downloadClickedCb)
			uh.nbcUpdateExpander.AddSuffix(&uh.nbcDownloadBtn.Widget)

			uh.nbcUpdateBtn = gtk.NewButtonWithLabel("Update")
			uh.nbcUpdateBtn.SetValign(gtk.AlignCenterValue)
			uh.nbcUpdateBtn.AddCssClass("suggested-action")
			uh.nbcUpdateBtn.SetSensitive(false) // Disabled until we check for updates
			updateClickedCb := func(btn gtk.Button) {
				uh.onNBCUpdateClicked(uh.nbcUpdateExpander, uh.nbcUpdateBtn)
			}
			uh.nbcUpdateBtn.ConnectClicked(&updateClickedCb)
			uh.nbcUpdateExpander.AddSuffix(&uh.nbcUpdateBtn.Widget)
			group.Add(&uh.nbcUpdateExpander.Widget)

			page.Add(group)

			// Check for updates on startup
			go uh.checkNBCUpdateAvailability()
		}
	}

	// Flatpak Updates group
	if uh.config.IsGroupEnabled("updates_page", "flatpak_updates_group") {
		group := adw.NewPreferencesGroup()
		group.SetTitle("Flatpak Updates")
		group.SetDescription("Available updates for Flatpak applications")

		uh.flatpakUpdatesExpander = adw.NewExpanderRow()
		uh.flatpakUpdatesExpander.SetTitle("Available Updates")
		uh.flatpakUpdatesExpander.SetSubtitle("Loading...")
		group.Add(&uh.flatpakUpdatesExpander.Widget)

		page.Add(group)

		// Load flatpak updates asynchronously
		go uh.loadFlatpakUpdates()
	}

	// Homebrew Updates group
	if uh.config.IsGroupEnabled("updates_page", "brew_updates_group") {
		group := adw.NewPreferencesGroup()
		group.SetTitle("Homebrew Updates")
		group.SetDescription("Check for and install Homebrew package updates")

		// Update button row
		updateRow := adw.NewActionRow()
		updateRow.SetTitle("Update Homebrew")
		updateRow.SetSubtitle("Update Homebrew itself and all formulae definitions")

		updateBtn := gtk.NewButtonWithLabel("Update")
		updateBtn.SetValign(gtk.AlignCenterValue)
		updateBtn.AddCssClass("suggested-action")
		updateClickedCb := func(btn gtk.Button) {
			uh.onUpdateHomebrewClicked(updateBtn)
		}
		updateBtn.ConnectClicked(&updateClickedCb)

		updateRow.AddSuffix(&updateBtn.Widget)
		group.Add(&updateRow.Widget)

		// Outdated packages expander
		uh.outdatedExpander = adw.NewExpanderRow()
		uh.outdatedExpander.SetTitle("Outdated Packages")
		uh.outdatedExpander.SetSubtitle("Loading...")
		group.Add(&uh.outdatedExpander.Widget)

		page.Add(group)

		// Load outdated packages asynchronously
		go uh.loadOutdatedPackages()
	}
}

// buildApplicationsPage builds the Applications page content
func (uh *UserHome) buildApplicationsPage() {
	page := uh.applicationsPrefsPage
	if page == nil {
		return
	}

	// Installed Applications group
	if uh.config.IsGroupEnabled("applications_page", "applications_installed_group") {
		group := adw.NewPreferencesGroup()
		group.SetTitle("Installed Applications")
		group.SetDescription("Manage your installed applications")

		row := adw.NewActionRow()
		row.SetTitle("Manage Flatpaks")
		row.SetSubtitle("Open the application manager to install and manage applications")
		row.SetActivatable(true)

		icon := gtk.NewImageFromIconName("adw-external-link-symbolic")
		row.AddSuffix(&icon.Widget)

		groupCfg := uh.config.GetGroupConfig("applications_page", "applications_installed_group")
		appID := "io.github.kolunmi.Bazaar"
		if groupCfg != nil && groupCfg.AppID != "" {
			appID = groupCfg.AppID
		}

		activatedCb := func(row adw.ActionRow) {
			uh.launchApp(appID)
		}
		row.ConnectActivated(&activatedCb)

		group.Add(&row.Widget)
		page.Add(group)
	}

	// Flatpak User Applications group
	if uh.config.IsGroupEnabled("applications_page", "flatpak_user_group") {
		group := adw.NewPreferencesGroup()
		group.SetTitle("User Flatpak Applications")
		group.SetDescription("Flatpak applications installed for the current user")

		uh.flatpakUserExpander = adw.NewExpanderRow()
		uh.flatpakUserExpander.SetTitle("User Applications")
		uh.flatpakUserExpander.SetSubtitle("Loading...")
		group.Add(&uh.flatpakUserExpander.Widget)

		page.Add(group)
	}

	// Flatpak System Applications group
	if uh.config.IsGroupEnabled("applications_page", "flatpak_system_group") {
		group := adw.NewPreferencesGroup()
		group.SetTitle("System Flatpak Applications")
		group.SetDescription("Flatpak applications installed system-wide")

		uh.flatpakSystemExpander = adw.NewExpanderRow()
		uh.flatpakSystemExpander.SetTitle("System Applications")
		uh.flatpakSystemExpander.SetSubtitle("Loading...")
		group.Add(&uh.flatpakSystemExpander.Widget)

		page.Add(group)
	}

	// Load flatpak applications if either group is enabled
	if uh.config.IsGroupEnabled("applications_page", "flatpak_user_group") ||
		uh.config.IsGroupEnabled("applications_page", "flatpak_system_group") {
		go uh.loadFlatpakApplications()
	}

	// Snap Applications group
	if uh.config.IsGroupEnabled("applications_page", "snap_group") && pm.SnapIsInstalled() {
		group := adw.NewPreferencesGroup()
		group.SetTitle("Snap Applications")
		group.SetDescription("Manage Snap packages installed on your system")

		// Snap Store link row - shown when snap-store is installed
		uh.snapStoreLinkRow = adw.NewActionRow()
		uh.snapStoreLinkRow.SetTitle("Manage Snaps")
		uh.snapStoreLinkRow.SetSubtitle("Open the Snap Store to install and manage applications")
		uh.snapStoreLinkRow.SetActivatable(true)
		uh.snapStoreLinkRow.SetVisible(false) // Hidden by default, shown if snap-store is installed

		linkIcon := gtk.NewImageFromIconName("adw-external-link-symbolic")
		uh.snapStoreLinkRow.AddSuffix(&linkIcon.Widget)

		linkActivatedCb := func(row adw.ActionRow) {
			uh.launchApp("snap-store_snap-store")
		}
		uh.snapStoreLinkRow.ConnectActivated(&linkActivatedCb)
		group.Add(&uh.snapStoreLinkRow.Widget)

		// Snap Store install row - shown when snap-store is NOT installed
		uh.snapStoreInstallRow = adw.NewActionRow()
		uh.snapStoreInstallRow.SetTitle("Snap Store")
		uh.snapStoreInstallRow.SetSubtitle("Install the Snap Store for a graphical package manager")
		uh.snapStoreInstallRow.SetVisible(false) // Hidden by default, shown if snap-store not installed

		storeIcon := gtk.NewImageFromIconName("system-software-install-symbolic")
		uh.snapStoreInstallRow.AddPrefix(&storeIcon.Widget)

		installBtn := gtk.NewButtonWithLabel("Install")
		installBtn.SetValign(gtk.AlignCenterValue)
		installBtn.AddCssClass("suggested-action")
		installClickedCb := func(btn gtk.Button) {
			uh.onInstallSnapStoreClicked(installBtn)
		}
		installBtn.ConnectClicked(&installClickedCb)
		uh.snapStoreInstallRow.AddSuffix(&installBtn.Widget)
		group.Add(&uh.snapStoreInstallRow.Widget)

		uh.snapExpander = adw.NewExpanderRow()
		uh.snapExpander.SetTitle("Installed Snaps")
		uh.snapExpander.SetSubtitle("Loading...")
		group.Add(&uh.snapExpander.Widget)

		page.Add(group)

		// Load snap applications asynchronously
		go uh.loadSnapApplications()
	}

	// Homebrew group
	if uh.config.IsGroupEnabled("applications_page", "brew_group") {
		group := adw.NewPreferencesGroup()
		group.SetTitle("Homebrew")
		group.SetDescription("Manage Homebrew packages installed on your system")

		// Bundle dump row
		// NOTE: Brewfile operations (dump/install-from) are not supported by the pm library.
		// These remain implemented using internal/homebrew package for now.
		dumpRow := adw.NewActionRow()
		dumpRow.SetTitle("Brew Bundle Dump")
		dumpRow.SetSubtitle("Export currently installed packages to ~/Brewfile")

		dumpBtn := gtk.NewButtonWithLabel("Dump")
		dumpBtn.SetValign(gtk.AlignCenterValue)
		dumpBtn.AddCssClass("suggested-action")
		dumpClickedCb := func(btn gtk.Button) {
			uh.onBrewBundleDumpClicked()
		}
		dumpBtn.ConnectClicked(&dumpClickedCb)

		dumpRow.AddSuffix(&dumpBtn.Widget)
		group.Add(&dumpRow.Widget)

		// Formulae expander
		uh.formulaeExpander = adw.NewExpanderRow()
		uh.formulaeExpander.SetTitle("Formulae")
		uh.formulaeExpander.SetSubtitle("Loading...")
		group.Add(&uh.formulaeExpander.Widget)

		// Casks expander
		uh.casksExpander = adw.NewExpanderRow()
		uh.casksExpander.SetTitle("Casks")
		uh.casksExpander.SetSubtitle("Loading...")
		group.Add(&uh.casksExpander.Widget)

		page.Add(group)

		// Load packages asynchronously
		go uh.loadHomebrewPackages()
	}

	// Homebrew Search group
	if uh.config.IsGroupEnabled("applications_page", "brew_search_group") {
		group := adw.NewPreferencesGroup()
		group.SetTitle("Search Homebrew")
		group.SetDescription("Search for and install Homebrew formulae")

		// Search entry row
		searchRow := adw.NewActionRow()
		searchRow.SetTitle("Search for packages")

		uh.searchEntry = gtk.NewSearchEntry()
		uh.searchEntry.SetHexpand(true)

		searchActivateCb := func(entry gtk.SearchEntry) {
			uh.onHomebrewSearch()
		}
		uh.searchEntry.ConnectActivate(&searchActivateCb)

		searchRow.AddSuffix(&uh.searchEntry.Widget)
		group.Add(&searchRow.Widget)

		// Search results expander
		uh.searchResultsExpander = adw.NewExpanderRow()
		uh.searchResultsExpander.SetTitle("Search Results")
		uh.searchResultsExpander.SetSubtitle("No search performed")
		uh.searchResultsExpander.SetEnableExpansion(false)
		group.Add(&uh.searchResultsExpander.Widget)

		page.Add(group)
	}
}

// buildExtensionsPage builds the Extensions page content
func (uh *UserHome) buildExtensionsPage() {
	page := uh.extensionsPrefsPage
	if page == nil {
		return
	}

	// Initialize the installed components cache
	uh.installedComponentsMap = make(map[string]bool)

	// Installed extensions group - only show if updex is available
	if updex.IsInstalled() && uh.config.IsGroupEnabled("extensions_page", "installed_group") {
		uh.extensionsGroup = adw.NewPreferencesGroup()
		uh.extensionsGroup.SetTitle("Installed")
		uh.extensionsGroup.SetDescription("Loading extensions...")

		page.Add(uh.extensionsGroup)

		// Load extensions asynchronously
		go uh.loadExtensions()
	} else if !updex.IsInstalled() {
		// Show a message that updex is not installed
		group := adw.NewPreferencesGroup()
		group.SetTitle("Installed")
		group.SetDescription("Manage systemd-sysext extensions")

		row := adw.NewActionRow()
		row.SetTitle("Extension Manager Not Available")
		row.SetSubtitle("The updex command is not installed on this system")
		group.Add(&row.Widget)
		page.Add(group)
	}

	// Discover extensions group - only show if instex is available
	if instex.IsInstalled() && uh.config.IsGroupEnabled("extensions_page", "discover_group") {
		group := adw.NewPreferencesGroup()
		group.SetTitle("Discover")
		group.SetDescription("Find and install extensions from remote repositories")

		// URL entry row
		entryRow := adw.NewActionRow()
		entryRow.SetTitle("Repository URL")

		uh.discoverEntry = gtk.NewEntry()
		//uh.discoverEntry.SetPlaceholderText("https://repository.example.org")
		uh.discoverEntry.SetText("https://repository.frostyard.org")
		uh.discoverEntry.SetHexpand(true)
		uh.discoverEntry.SetValign(gtk.AlignCenterValue)
		entryRow.AddSuffix(&uh.discoverEntry.Widget)

		discoverBtn := gtk.NewButtonWithLabel("Discover")
		discoverBtn.SetValign(gtk.AlignCenterValue)
		discoverBtn.AddCssClass("suggested-action")
		discoverClickedCb := func(btn gtk.Button) {
			uh.onDiscoverClicked(discoverBtn)
		}
		discoverBtn.ConnectClicked(&discoverClickedCb)
		entryRow.AddSuffix(&discoverBtn.Widget)

		group.Add(&entryRow.Widget)
		page.Add(group)

		// Results group (initially hidden, will be populated after discovery)
		uh.discoverResultsGroup = adw.NewPreferencesGroup()
		uh.discoverResultsGroup.SetTitle("Available Extensions")
		uh.discoverResultsGroup.SetVisible(false)
		page.Add(uh.discoverResultsGroup)
	}
}

// loadExtensions loads extension information asynchronously
func (uh *UserHome) loadExtensions() {
	ctx, cancel := updex.DefaultContext()
	defer cancel()

	extensions, err := updex.ListInstalled(ctx)

	async.RunOnMain(func() {
		if uh.extensionsGroup == nil {
			return
		}

		if err != nil {
			uh.extensionsGroup.SetDescription(fmt.Sprintf("Error: %v", err))
			return
		}

		if len(extensions) == 0 {
			uh.extensionsGroup.SetDescription("No extensions installed")
			return
		}

		// Group extensions by component and populate installed cache
		componentMap := make(map[string][]updex.Extension)
		for _, ext := range extensions {
			componentMap[ext.Component] = append(componentMap[ext.Component], ext)
			uh.installedComponentsMap[ext.Component] = true
		}

		uh.extensionsGroup.SetDescription(fmt.Sprintf("%d components installed", len(componentMap)))

		// Create an expander row for each component
		for component, versions := range componentMap {
			expander := adw.NewExpanderRow()
			expander.SetTitle(component)

			// Count current version and set subtitle
			var currentVersion string
			for _, v := range versions {
				if v.Current {
					currentVersion = v.Version
					break
				}
			}
			if currentVersion != "" {
				expander.SetSubtitle(fmt.Sprintf("%d versions (current: %s)", len(versions), currentVersion))
			} else {
				expander.SetSubtitle(fmt.Sprintf("%d versions", len(versions)))
			}

			// Add version rows
			for _, ext := range versions {
				row := adw.NewActionRow()
				row.SetTitle(ext.Version)

				// Add checkmark icon if this is the current (active) version
				if ext.Current {
					icon := gtk.NewImageFromIconName("object-select-symbolic")
					row.AddSuffix(&icon.Widget)
				}

				expander.AddRow(&row.Widget)
			}

			uh.extensionsGroup.Add(&expander.Widget)
		}
	})
}

// onDiscoverClicked handles the discover button click
func (uh *UserHome) onDiscoverClicked(button *gtk.Button) {
	if uh.discoverEntry == nil {
		return
	}

	url := uh.discoverEntry.GetText()
	if url == "" {
		uh.toastAdder.ShowErrorToast("Please enter a repository URL")
		return
	}

	button.SetSensitive(false)
	button.SetLabel("Discovering...")

	go func() {
		ctx, cancel := instex.DefaultContext()
		defer cancel()

		result, err := instex.Discover(ctx, url)

		async.RunOnMain(func() {
			button.SetSensitive(true)
			button.SetLabel("Discover")

			if err != nil {
				uh.toastAdder.ShowErrorToast(fmt.Sprintf("Discovery failed: %v", err))
				return
			}

			uh.displayDiscoveryResults(url, result)
		})
	}()
}

// displayDiscoveryResults shows discovered extensions in the results group
func (uh *UserHome) displayDiscoveryResults(repoURL string, result *instex.DiscoverOutput) {
	if uh.discoverResultsGroup == nil {
		return
	}

	// Clear existing result rows
	for _, row := range uh.discoverResultRows {
		uh.discoverResultsGroup.Remove(&row.Widget)
	}
	uh.discoverResultRows = nil

	uh.discoverResultsGroup.SetVisible(true)

	if len(result.Extensions) == 0 {
		uh.discoverResultsGroup.SetDescription("No extensions found in repository")
		return
	}

	uh.discoverResultsGroup.SetDescription(fmt.Sprintf("%d extensions available", len(result.Extensions)))

	for _, ext := range result.Extensions {
		row := adw.NewActionRow()
		row.SetTitle(ext.Name)

		// Show version count
		if len(ext.Versions) > 0 {
			row.SetSubtitle(fmt.Sprintf("%d versions available (latest: %s)", len(ext.Versions), ext.Versions[0]))
		}

		// Add extension icon
		icon := gtk.NewImageFromIconName("application-x-addon-symbolic")
		row.AddPrefix(&icon.Widget)

		// Check if already installed
		if uh.installedComponentsMap[ext.Name] {
			// Show installed badge
			installedLabel := gtk.NewLabel("Installed")
			installedLabel.AddCssClass("dim-label")
			installedLabel.SetValign(gtk.AlignCenterValue)
			row.AddSuffix(&installedLabel.Widget)
		} else {
			// Add install button
			installBtn := gtk.NewButtonWithLabel("Install")
			installBtn.SetValign(gtk.AlignCenterValue)
			installBtn.AddCssClass("suggested-action")

			// Capture values for callback
			extName := ext.Name
			url := repoURL
			installClickedCb := func(btn gtk.Button) {
				uh.onInstallExtensionClicked(installBtn, url, extName)
			}
			installBtn.ConnectClicked(&installClickedCb)
			row.AddSuffix(&installBtn.Widget)
		}

		uh.discoverResultsGroup.Add(&row.Widget)
		uh.discoverResultRows = append(uh.discoverResultRows, row)
	}
}

// onInstallExtensionClicked handles installing an extension
func (uh *UserHome) onInstallExtensionClicked(button *gtk.Button, repoURL, component string) {
	button.SetSensitive(false)
	button.SetLabel("Installing...")

	go func() {
		ctx, cancel := instex.DefaultContext()
		defer cancel()

		err := instex.Install(ctx, repoURL, component)

		async.RunOnMain(func() {
			if err != nil {
				button.SetSensitive(true)
				button.SetLabel("Install")
				userErr := async.NewUserErrorWithHint(
					fmt.Sprintf("Couldn't install %s", component),
					"Check the repository URL and try again",
					err,
				)
				uh.toastAdder.ShowErrorToast(userErr.FormatForUser())
				log.Printf("Extension install error details: %v", err)
				return
			}

			// Update button to show installed
			button.SetLabel("Installed")
			button.RemoveCssClass("suggested-action")
			button.AddCssClass("dim-label")

			// Update installed components cache
			uh.installedComponentsMap[component] = true

			uh.toastAdder.ShowToast(fmt.Sprintf("Installed %s successfully", component))

			// Reload the installed extensions list
			go uh.loadExtensions()
		})
	}()
}

// Helper methods

func (uh *UserHome) launchApp(appID string) {
	log.Printf("Launching app: %s", appID)

	var cmd *exec.Cmd

	// Check if this looks like a flatpak app ID (reverse DNS with 3+ parts)
	// e.g., "io.missioncenter.MissionCenter" or "org.gnome.Calculator"
	parts := strings.Split(appID, ".")
	isFlatpakStyle := len(parts) >= 3

	if isFlatpakStyle {
		// Check if flatpak knows about this app
		checkCmd := exec.Command("flatpak", "info", appID)
		if err := checkCmd.Run(); err == nil {
			// It's a flatpak app - use flatpak run
			log.Printf("Detected flatpak app, using 'flatpak run': %s", appID)
			cmd = exec.Command("flatpak", "run", appID)
		}
	}

	// Fall back to gtk-launch for non-flatpak apps or if flatpak check failed
	if cmd == nil {
		cmd = exec.Command("gtk-launch", appID)
	}

	cmd.Env = os.Environ()

	if err := cmd.Start(); err != nil {
		log.Printf("Failed to launch app %s: %v", appID, err)
		uh.toastAdder.ShowErrorToast(fmt.Sprintf("Failed to launch %s", appID))
		return
	}

	log.Printf("App launch started successfully: %s (pid: %d)", appID, cmd.Process.Pid)

	// Don't wait for the command to finish - it's a GUI app
	go func() {
		if err := cmd.Wait(); err != nil {
			log.Printf("App %s exited with error: %v", appID, err)
		}
	}()
}

func (uh *UserHome) openURL(url string) {
	log.Printf("Opening URL: %s", url)

	cmd := exec.Command("xdg-open", url)
	cmd.Env = os.Environ()

	if err := cmd.Start(); err != nil {
		log.Printf("Failed to open URL %s: %v", url, err)
		uh.toastAdder.ShowErrorToast(fmt.Sprintf("Failed to open %s", url))
		return
	}

	// Don't wait for xdg-open to finish
	go func() {
		_ = cmd.Wait()
	}()
}

// onNBCCheckUpdateClicked checks if an NBC system update is available
func (uh *UserHome) onNBCCheckUpdateClicked() {
	if uh.nbcCheckRow != nil {
		uh.nbcCheckRow.SetSubtitle("Checking for updates...")
	}
	if uh.nbcUpdateBtn != nil {
		uh.nbcUpdateBtn.SetSensitive(false)
	}
	if uh.nbcDownloadBtn != nil {
		uh.nbcDownloadBtn.SetSensitive(false)
	}

	go uh.checkNBCUpdateAvailability()
}

// checkNBCUpdateAvailability checks for NBC updates and updates the UI accordingly
func (uh *UserHome) checkNBCUpdateAvailability() {
	ctx, cancel := nbc.DefaultContext()
	defer cancel()

	result, err := nbc.CheckUpdate(ctx)
	if err != nil {
		uh.updateCountMu.Lock()
		uh.nbcUpdateCount = 0
		uh.updateCountMu.Unlock()
		uh.updateBadgeCount()

		async.RunOnMain(func() {
			if uh.nbcCheckRow != nil {
				uh.nbcCheckRow.SetSubtitle(fmt.Sprintf("Error: %v", err))
			}
			if uh.nbcUpdateExpander != nil {
				uh.nbcUpdateExpander.SetSubtitle("Failed to check for updates")
			}
			if uh.nbcUpdateBtn != nil {
				uh.nbcUpdateBtn.SetSensitive(false)
			}
			if uh.nbcDownloadBtn != nil {
				uh.nbcDownloadBtn.SetSensitive(false)
			}
		})
		return
	}

	// Update the badge count
	if result.UpdateNeeded {
		uh.updateCountMu.Lock()
		uh.nbcUpdateCount = 1
		uh.updateCountMu.Unlock()
	} else {
		uh.updateCountMu.Lock()
		uh.nbcUpdateCount = 0
		uh.updateCountMu.Unlock()
	}
	uh.updateBadgeCount()

	async.RunOnMain(func() {
		if result.UpdateNeeded {
			if uh.nbcCheckRow != nil {
				digest := result.NewDigest
				if len(digest) > 19 {
					digest = digest[:19] + "..."
				}
				uh.nbcCheckRow.SetSubtitle(fmt.Sprintf("Update available: %s", digest))
			}
			if uh.nbcUpdateExpander != nil {
				uh.nbcUpdateExpander.SetSubtitle("Update available - click to install")
			}
			if uh.nbcUpdateBtn != nil {
				uh.nbcUpdateBtn.SetSensitive(true)
			}
			if uh.nbcDownloadBtn != nil {
				uh.nbcDownloadBtn.SetSensitive(true)
			}
		} else {
			if uh.nbcCheckRow != nil {
				uh.nbcCheckRow.SetSubtitle("System is up to date")
			}
			if uh.nbcUpdateExpander != nil {
				uh.nbcUpdateExpander.SetSubtitle("No updates available")
			}
			if uh.nbcUpdateBtn != nil {
				uh.nbcUpdateBtn.SetSensitive(false)
			}
			if uh.nbcDownloadBtn != nil {
				uh.nbcDownloadBtn.SetSensitive(false)
			}
		}
	})
}

// onNBCUpdateClicked initiates an NBC system update with progress display
func (uh *UserHome) onNBCUpdateClicked(expander *adw.ExpanderRow, button *gtk.Button) {
	// Disable button and expand to show progress
	button.SetSensitive(false)
	button.SetLabel("Updating...")
	expander.SetExpanded(true)
	expander.SetSubtitle("Starting update...")

	// Clear any existing progress rows
	// Note: GTK doesn't have a direct "remove all children" for ExpanderRow,
	// so we'll just add new rows as progress updates come in

	// Create a progress bar row
	progressRow := adw.NewActionRow()
	progressRow.SetTitle("Progress")
	progressRow.SetSubtitle("Initializing...")

	progressBar := gtk.NewProgressBar()
	progressBar.SetHexpand(true)
	progressBar.SetValign(gtk.AlignCenterValue)
	progressBar.SetFraction(0)
	progressRow.AddSuffix(&progressBar.Widget)
	expander.AddRow(&progressRow.Widget)

	// Create a log expander for detailed messages
	logExpander := adw.NewExpanderRow()
	logExpander.SetTitle("Details")
	logExpander.SetSubtitle("View detailed progress messages")
	expander.AddRow(&logExpander.Widget)

	go func() {
		ctx, cancel := nbc.DefaultContext()
		defer cancel()

		progressCh := make(chan nbc.ProgressEvent)

		// Start processing progress events in a separate goroutine
		var updateErr error
		var wg sync.WaitGroup
		wg.Add(1)

		go func() {
			defer wg.Done()
			updateErr = nbc.Update(ctx, nbc.UpdateOptions{Auto: true}, progressCh)
		}()

		// Process progress events
		for event := range progressCh {
			evt := event // capture for closure
			async.RunOnMain(func() {
				switch evt.Type {
				case nbc.EventTypeStep:
					// Update main progress
					progress := float64(evt.Step) / float64(evt.TotalSteps)
					progressBar.SetFraction(progress)
					progressRow.SetSubtitle(fmt.Sprintf("Step %d/%d: %s", evt.Step, evt.TotalSteps, evt.StepName))
					expander.SetSubtitle(fmt.Sprintf("[%d/%d] %s", evt.Step, evt.TotalSteps, evt.StepName))

				case nbc.EventTypeProgress:
					// Update progress bar with percentage
					progressBar.SetFraction(float64(evt.Percent) / 100.0)
					if evt.Message != "" {
						progressRow.SetSubtitle(fmt.Sprintf("%d%% - %s", evt.Percent, evt.Message))
					}

				case nbc.EventTypeMessage:
					// Add message to log
					msgRow := adw.NewActionRow()
					msgRow.SetTitle(evt.Message)
					msgRow.SetSubtitle(time.Now().Format("15:04:05"))
					logExpander.AddRow(&msgRow.Widget)

				case nbc.EventTypeWarning:
					// Add warning to log with icon
					warnRow := adw.NewActionRow()
					warnRow.SetTitle(evt.Message)
					warnRow.SetSubtitle("Warning")
					warnIcon := gtk.NewImageFromIconName("dialog-warning-symbolic")
					warnRow.AddPrefix(&warnIcon.Widget)
					logExpander.AddRow(&warnRow.Widget)

				case nbc.EventTypeError:
					// Add error to log with icon
					errRow := adw.NewActionRow()
					errRow.SetTitle(evt.Message)
					errRow.SetSubtitle("Error")
					errIcon := gtk.NewImageFromIconName("dialog-error-symbolic")
					errRow.AddPrefix(&errIcon.Widget)
					logExpander.AddRow(&errRow.Widget)
					logExpander.SetExpanded(true)

				case nbc.EventTypeComplete:
					// Update with success
					progressBar.SetFraction(1.0)
					progressRow.SetSubtitle("Complete")
					expander.SetSubtitle("Update complete - please reboot")

					// Add completion message
					completeRow := adw.NewActionRow()
					completeRow.SetTitle(evt.Message)
					completeRow.SetSubtitle("Complete")
					completeIcon := gtk.NewImageFromIconName("object-select-symbolic")
					completeRow.AddPrefix(&completeIcon.Widget)
					logExpander.AddRow(&completeRow.Widget)
				}
			})
		}

		// Wait for update to finish
		wg.Wait()

		// Handle final result
		async.RunOnMain(func() {
			button.SetSensitive(true)
			button.SetLabel("Update")

			if updateErr != nil {
				expander.SetSubtitle(fmt.Sprintf("Update failed: %v", updateErr))
				uh.toastAdder.ShowErrorToast(fmt.Sprintf("Update failed: %v", updateErr))
			} else {
				uh.toastAdder.ShowToast("System update complete! Please reboot to apply changes.")
			}
		})
	}()
}

// onNBCDownloadClicked initiates an NBC system update download with progress display
func (uh *UserHome) onNBCDownloadClicked(expander *adw.ExpanderRow, button *gtk.Button) {
	// Disable buttons and expand to show progress
	button.SetSensitive(false)
	button.SetLabel("Downloading...")
	if uh.nbcUpdateBtn != nil {
		uh.nbcUpdateBtn.SetSensitive(false)
	}
	expander.SetExpanded(true)
	expander.SetSubtitle("Starting download...")

	// Create a progress bar row
	progressRow := adw.NewActionRow()
	progressRow.SetTitle("Progress")
	progressRow.SetSubtitle("Initializing...")

	progressBar := gtk.NewProgressBar()
	progressBar.SetHexpand(true)
	progressBar.SetValign(gtk.AlignCenterValue)
	progressBar.SetFraction(0)
	progressRow.AddSuffix(&progressBar.Widget)
	expander.AddRow(&progressRow.Widget)

	// Create a log expander for detailed messages
	logExpander := adw.NewExpanderRow()
	logExpander.SetTitle("Details")
	logExpander.SetSubtitle("View detailed progress messages")
	expander.AddRow(&logExpander.Widget)

	go func() {
		ctx, cancel := nbc.DefaultContext()
		defer cancel()

		progressCh := make(chan nbc.ProgressEvent)

		// Start processing progress events in a separate goroutine
		var downloadErr error
		var wg sync.WaitGroup
		wg.Add(1)

		go func() {
			defer wg.Done()
			downloadErr = nbc.Download(ctx, nbc.DownloadOptions{ForUpdate: true}, progressCh)
		}()

		// Process progress events
		for event := range progressCh {
			evt := event // capture for closure
			async.RunOnMain(func() {
				switch evt.Type {
				case nbc.EventTypeStep:
					// Update main progress
					progress := float64(evt.Step) / float64(evt.TotalSteps)
					progressBar.SetFraction(progress)
					progressRow.SetSubtitle(fmt.Sprintf("Step %d/%d: %s", evt.Step, evt.TotalSteps, evt.StepName))
					expander.SetSubtitle(fmt.Sprintf("[%d/%d] %s", evt.Step, evt.TotalSteps, evt.StepName))

				case nbc.EventTypeProgress:
					// Update progress bar with percentage
					progressBar.SetFraction(float64(evt.Percent) / 100.0)
					if evt.Message != "" {
						progressRow.SetSubtitle(fmt.Sprintf("%d%% - %s", evt.Percent, evt.Message))
					}

				case nbc.EventTypeMessage:
					// Add message to log
					msgRow := adw.NewActionRow()
					msgRow.SetTitle(evt.Message)
					msgRow.SetSubtitle(time.Now().Format("15:04:05"))
					logExpander.AddRow(&msgRow.Widget)

				case nbc.EventTypeWarning:
					// Add warning to log with icon
					warnRow := adw.NewActionRow()
					warnRow.SetTitle(evt.Message)
					warnRow.SetSubtitle("Warning")
					warnIcon := gtk.NewImageFromIconName("dialog-warning-symbolic")
					warnRow.AddPrefix(&warnIcon.Widget)
					logExpander.AddRow(&warnRow.Widget)

				case nbc.EventTypeError:
					// Add error to log with icon
					errRow := adw.NewActionRow()
					errRow.SetTitle(evt.Message)
					errRow.SetSubtitle("Error")
					errIcon := gtk.NewImageFromIconName("dialog-error-symbolic")
					errRow.AddPrefix(&errIcon.Widget)
					logExpander.AddRow(&errRow.Widget)
					logExpander.SetExpanded(true)

				case nbc.EventTypeComplete:
					// Update with success
					progressBar.SetFraction(1.0)
					progressRow.SetSubtitle("Complete")
					expander.SetSubtitle("Download complete - ready to install")

					// Add completion message
					completeRow := adw.NewActionRow()
					completeRow.SetTitle(evt.Message)
					completeRow.SetSubtitle("Complete")
					completeIcon := gtk.NewImageFromIconName("object-select-symbolic")
					completeRow.AddPrefix(&completeIcon.Widget)
					logExpander.AddRow(&completeRow.Widget)
				}
			})
		}

		// Wait for download to finish
		wg.Wait()

		// Handle final result
		async.RunOnMain(func() {
			button.SetSensitive(true)
			button.SetLabel("Download")

			if downloadErr != nil {
				expander.SetSubtitle(fmt.Sprintf("Download failed: %v", downloadErr))
				uh.toastAdder.ShowErrorToast(fmt.Sprintf("Download failed: %v", downloadErr))
				if uh.nbcUpdateBtn != nil {
					uh.nbcUpdateBtn.SetSensitive(true)
				}
			} else {
				uh.toastAdder.ShowToast("Update downloaded! Click Update to install.")
				// Keep update button enabled so user can install the downloaded update
				if uh.nbcUpdateBtn != nil {
					uh.nbcUpdateBtn.SetSensitive(true)
				}
			}
		})
	}()
}

func (uh *UserHome) onUpdateHomebrewClicked(button *gtk.Button) {
	// Disable button and show working state
	button.SetSensitive(false)
	button.SetLabel("Updating...")

	// Start tracked operation (visible in operations popover)
	op := operations.Start("Update Homebrew", operations.CategoryUpdate, false)

	// Wire retry capability - enables Retry button in operations popover
	op.RetryFunc = func() {
		uh.onUpdateHomebrewClicked(button)
	}

	go func() {
		err := pm.HomebrewUpdate()

		async.RunOnMain(func() {
			// Restore button state
			button.SetSensitive(true)
			button.SetLabel("Update")

			// Complete the tracked operation
			op.Complete(err)

			if err != nil {
				userErr := async.NewUserError("Couldn't update Homebrew", err)
				uh.toastAdder.ShowErrorToast(userErr.FormatForUser())
				log.Printf("Homebrew update error details: %v", err)
				return
			}
			uh.toastAdder.ShowToast("Homebrew updated successfully")
			// Refresh outdated packages list
			go uh.loadOutdatedPackages()
		})
	}()
}

func (uh *UserHome) onBrewBundleDumpClicked() {
	go func() {
		homeDir, _ := os.UserHomeDir()
		path := homeDir + "/Brewfile"
		if err := pm.HomebrewBundleDump(path, true); err != nil {
			async.RunOnMain(func() {
				uh.toastAdder.ShowErrorToast(fmt.Sprintf("Bundle dump failed: %v", err))
			})
			return
		}
		async.RunOnMain(func() {
			uh.toastAdder.ShowToast(fmt.Sprintf("Brewfile saved to %s", path))
		})
	}()
}

func (uh *UserHome) loadOutdatedPackages() {
	if !pm.HomebrewIsInstalled() {
		uh.updateCountMu.Lock()
		uh.brewUpdateCount = 0
		uh.updateCountMu.Unlock()
		uh.updateBadgeCount()

		async.RunOnMain(func() {
			uh.outdatedExpander.SetSubtitle("Homebrew not installed")
		})
		return
	}

	packages, err := pm.ListHomebrewOutdated()
	if err != nil {
		uh.updateCountMu.Lock()
		uh.brewUpdateCount = 0
		uh.updateCountMu.Unlock()
		uh.updateBadgeCount()

		async.RunOnMain(func() {
			uh.outdatedExpander.SetSubtitle(fmt.Sprintf("Error: %v", err))
		})
		return
	}

	// Update the badge count
	uh.updateCountMu.Lock()
	uh.brewUpdateCount = len(packages)
	uh.updateCountMu.Unlock()
	uh.updateBadgeCount()

	async.RunOnMain(func() {
		uh.outdatedExpander.SetSubtitle(fmt.Sprintf("%d packages available", len(packages)))
		for _, pkg := range packages {
			row := adw.NewActionRow()
			row.SetTitle(pkg.Name)
			row.SetSubtitle(pkg.Version)

			upgradeBtn := gtk.NewButtonWithLabel("Upgrade")
			upgradeBtn.SetValign(gtk.AlignCenterValue)
			pkgName := pkg.Name
			clickedCb := func(btn gtk.Button) {
				go func() {
					if err := pm.HomebrewUpgrade(pkgName); err != nil {
						async.RunOnMain(func() {
							uh.toastAdder.ShowErrorToast(fmt.Sprintf("Upgrade failed: %v", err))
						})
						return
					}
					async.RunOnMain(func() {
						uh.toastAdder.ShowToast(fmt.Sprintf("%s upgraded", pkgName))
					})
				}()
			}
			upgradeBtn.ConnectClicked(&clickedCb)

			row.AddSuffix(&upgradeBtn.Widget)
			uh.outdatedExpander.AddRow(&row.Widget)
		}
	})
}

func (uh *UserHome) loadHomebrewPackages() {
	if !pm.HomebrewIsInstalled() {
		async.RunOnMain(func() {
			uh.formulaeExpander.SetSubtitle("Homebrew not installed")
			uh.casksExpander.SetSubtitle("Homebrew not installed")
		})
		return
	}

	// Load formulae
	formulae, err := pm.ListHomebrewFormulae()
	if err != nil {
		async.RunOnMain(func() {
			uh.formulaeExpander.SetSubtitle(fmt.Sprintf("Error: %v", err))
		})
	} else {
		async.RunOnMain(func() {
			uh.formulaeExpander.SetSubtitle(fmt.Sprintf("%d installed", len(formulae)))
			for _, pkg := range formulae {
				row := adw.NewActionRow()
				row.SetTitle(pkg.Name)
				row.SetSubtitle(pkg.Version)
				uh.formulaeExpander.AddRow(&row.Widget)
			}
		})
	}

	// Load casks
	casks, err := pm.ListHomebrewCasks()
	if err != nil {
		async.RunOnMain(func() {
			uh.casksExpander.SetSubtitle(fmt.Sprintf("Error: %v", err))
		})
	} else {
		async.RunOnMain(func() {
			uh.casksExpander.SetSubtitle(fmt.Sprintf("%d installed", len(casks)))
			for _, pkg := range casks {
				row := adw.NewActionRow()
				row.SetTitle(pkg.Name)
				row.SetSubtitle(pkg.Version)
				uh.casksExpander.AddRow(&row.Widget)
			}
		})
	}
}

func (uh *UserHome) loadFlatpakApplications() {
	if !pm.FlatpakIsInstalled() {
		async.RunOnMain(func() {
			if uh.flatpakUserExpander != nil {
				uh.flatpakUserExpander.SetSubtitle("Flatpak not installed")
			}
			if uh.flatpakSystemExpander != nil {
				uh.flatpakSystemExpander.SetSubtitle("Flatpak not installed")
			}
		})
		return
	}

	// Load all applications (user and system combined via pm library)
	apps, err := pm.ListFlatpakApplications()
	if err != nil {
		async.RunOnMain(func() {
			if uh.flatpakUserExpander != nil {
				uh.flatpakUserExpander.SetSubtitle(fmt.Sprintf("Error: %v", err))
			}
			if uh.flatpakSystemExpander != nil {
				uh.flatpakSystemExpander.SetSubtitle(fmt.Sprintf("Error: %v", err))
			}
		})
		return
	}

	// Separate into user and system apps
	var userApps []pm.FlatpakApplication
	var systemApps []pm.FlatpakApplication
	for _, app := range apps {
		if app.IsUser {
			userApps = append(userApps, app)
		} else {
			systemApps = append(systemApps, app)
		}
	}

	// Load user applications
	if uh.flatpakUserExpander != nil {
		async.RunOnMain(func() {
			uh.flatpakUserExpander.SetSubtitle(fmt.Sprintf("%d installed", len(userApps)))
			for _, app := range userApps {
				row := adw.NewActionRow()
				row.SetTitle(app.Name)
				subtitle := app.ID
				if app.Version != "" {
					subtitle = fmt.Sprintf("%s (%s)", app.ID, app.Version)
				}
				row.SetSubtitle(subtitle)

				// Add uninstall button
				uninstallBtn := gtk.NewButtonFromIconName("user-trash-symbolic")
				uninstallBtn.SetValign(gtk.AlignCenterValue)
				uninstallBtn.AddCssClass("destructive-action")
				uninstallBtn.SetTooltipText("Uninstall")

				appID := app.ID
				clickedCb := func(btn gtk.Button) {
					btn.SetSensitive(false)
					go func() {
						if err := pm.FlatpakUninstall(appID, true); err != nil {
							async.RunOnMain(func() {
								btn.SetSensitive(true)
								uh.toastAdder.ShowErrorToast(fmt.Sprintf("Uninstall failed: %v", err))
							})
							return
						}
						async.RunOnMain(func() {
							uh.toastAdder.ShowToast(fmt.Sprintf("%s uninstalled", appID))
							// Refresh the list
							go uh.loadFlatpakApplications()
						})
					}()
				}
				uninstallBtn.ConnectClicked(&clickedCb)

				row.AddSuffix(&uninstallBtn.Widget)
				uh.flatpakUserExpander.AddRow(&row.Widget)
			}
		})
	}

	// Load system applications
	if uh.flatpakSystemExpander != nil {
		async.RunOnMain(func() {
			uh.flatpakSystemExpander.SetSubtitle(fmt.Sprintf("%d installed", len(systemApps)))
			for _, app := range systemApps {
				row := adw.NewActionRow()
				row.SetTitle(app.Name)
				subtitle := app.ID
				if app.Version != "" {
					subtitle = fmt.Sprintf("%s (%s)", app.ID, app.Version)
				}
				row.SetSubtitle(subtitle)

				// Add uninstall button (requires elevated privileges for system apps)
				uninstallBtn := gtk.NewButtonFromIconName("user-trash-symbolic")
				uninstallBtn.SetValign(gtk.AlignCenterValue)
				uninstallBtn.AddCssClass("destructive-action")
				uninstallBtn.SetTooltipText("Uninstall (requires admin)")

				appID := app.ID
				clickedCb := func(btn gtk.Button) {
					btn.SetSensitive(false)
					go func() {
						if err := pm.FlatpakUninstall(appID, false); err != nil {
							async.RunOnMain(func() {
								btn.SetSensitive(true)
								uh.toastAdder.ShowErrorToast(fmt.Sprintf("Uninstall failed: %v", err))
							})
							return
						}
						async.RunOnMain(func() {
							uh.toastAdder.ShowToast(fmt.Sprintf("%s uninstalled", appID))
							// Refresh the list
							go uh.loadFlatpakApplications()
						})
					}()
				}
				uninstallBtn.ConnectClicked(&clickedCb)

				row.AddSuffix(&uninstallBtn.Widget)
				uh.flatpakSystemExpander.AddRow(&row.Widget)
			}
		})
	}
}

// loadSnapApplications loads installed snap packages asynchronously
func (uh *UserHome) loadSnapApplications() {
	if !pm.SnapIsInstalled() {
		async.RunOnMain(func() {
			if uh.snapExpander != nil {
				uh.snapExpander.SetSubtitle("Snap not installed")
			}
		})
		return
	}

	// Check if snap-store is installed
	snapStoreInstalled, err := pm.IsSnapInstalled("snap-store")
	if err != nil {
		log.Printf("Error checking snap-store: %v", err)
	}

	// Load installed snaps
	snaps, err := pm.ListInstalledSnaps()
	if err != nil {
		async.RunOnMain(func() {
			if uh.snapExpander != nil {
				uh.snapExpander.SetSubtitle(fmt.Sprintf("Error: %v", err))
			}
		})
		return
	}

	async.RunOnMain(func() {
		if uh.snapExpander != nil {
			// Clear existing rows
			for _, row := range uh.snapRows {
				uh.snapExpander.Remove(&row.Widget)
			}
			uh.snapRows = nil

			uh.snapExpander.SetSubtitle(fmt.Sprintf("%d installed", len(snaps)))

			for _, s := range snaps {
				row := adw.NewActionRow()
				row.SetTitle(s.Name)

				subtitle := s.Version
				if s.Channel != "" {
					subtitle = fmt.Sprintf("%s (%s)", s.Version, s.Channel)
				}
				row.SetSubtitle(subtitle)

				// Add confinement indicator
				if s.Confinement == "classic" {
					classicLabel := gtk.NewLabel("classic")
					classicLabel.AddCssClass("dim-label")
					classicLabel.SetValign(gtk.AlignCenterValue)
					row.AddSuffix(&classicLabel.Widget)
				}

				uh.snapExpander.AddRow(&row.Widget)
				uh.snapRows = append(uh.snapRows, row)
			}
		}

		// Show snap-store link row if installed, otherwise show install row
		if uh.snapStoreLinkRow != nil {
			uh.snapStoreLinkRow.SetVisible(snapStoreInstalled)
		}
		if uh.snapStoreInstallRow != nil {
			uh.snapStoreInstallRow.SetVisible(!snapStoreInstalled)
		}
	})
}

// onInstallSnapStoreClicked handles installing the snap-store snap
func (uh *UserHome) onInstallSnapStoreClicked(button *gtk.Button) {
	button.SetSensitive(false)
	button.SetLabel("Installing...")

	go func() {
		ctx, cancel := pm.SnapDefaultContext()
		defer cancel()

		changeID, err := pm.SnapInstall(ctx, "snap-store")
		if err != nil {
			async.RunOnMain(func() {
				button.SetSensitive(true)
				button.SetLabel("Install")
				uh.toastAdder.ShowErrorToast(fmt.Sprintf("Failed to install snap-store: %v", err))
			})
			return
		}

		// Wait for the installation to complete
		err = pm.SnapWaitForChange(ctx, changeID)
		if err != nil {
			async.RunOnMain(func() {
				button.SetSensitive(true)
				button.SetLabel("Install")
				uh.toastAdder.ShowErrorToast(fmt.Sprintf("Installation failed: %v", err))
			})
			return
		}

		async.RunOnMain(func() {
			button.SetSensitive(true)
			button.SetLabel("Install")
			uh.toastAdder.ShowToast("Snap Store installed successfully!")
		})

		// Reload the snap list to update the UI
		uh.loadSnapApplications()
	}()
}

func (uh *UserHome) loadFlatpakUpdates() {
	if !pm.FlatpakIsInstalled() {
		uh.updateCountMu.Lock()
		uh.flatpakUpdateCount = 0
		uh.updateCountMu.Unlock()
		uh.updateBadgeCount()

		async.RunOnMain(func() {
			if uh.flatpakUpdatesExpander != nil {
				uh.flatpakUpdatesExpander.SetSubtitle("Flatpak not installed")
			}
		})
		return
	}

	// Get updates from pm library (automatically handles user/system distinction)
	allUpdates, err := pm.ListFlatpakUpdates()
	if err != nil {
		log.Printf("Error loading flatpak updates: %v", err)
		uh.updateCountMu.Lock()
		uh.flatpakUpdateCount = 0
		uh.updateCountMu.Unlock()
		uh.updateBadgeCount()

		async.RunOnMain(func() {
			if uh.flatpakUpdatesExpander != nil {
				uh.flatpakUpdatesExpander.SetSubtitle(fmt.Sprintf("Error: %v", err))
			}
		})
		return
	}

	// Update the badge count
	uh.updateCountMu.Lock()
	uh.flatpakUpdateCount = len(allUpdates)
	uh.updateCountMu.Unlock()
	uh.updateBadgeCount()

	async.RunOnMain(func() {
		if uh.flatpakUpdatesExpander == nil {
			return
		}

		// Clear existing rows
		for _, row := range uh.flatpakUpdateRows {
			uh.flatpakUpdatesExpander.Remove(&row.Widget)
		}
		uh.flatpakUpdateRows = nil

		if len(allUpdates) == 0 {
			uh.flatpakUpdatesExpander.SetSubtitle("All applications are up to date")
			uh.flatpakUpdatesExpander.SetEnableExpansion(false)
			return
		}

		uh.flatpakUpdatesExpander.SetSubtitle(fmt.Sprintf("%d updates available", len(allUpdates)))
		uh.flatpakUpdatesExpander.SetEnableExpansion(true)

		for _, update := range allUpdates {
			row := adw.NewActionRow()
			row.SetTitle(update.ID)
			subtitle := update.ID
			if update.AvailableVer != "" {
				subtitle = fmt.Sprintf("%s → %s", update.CurrentVer, update.AvailableVer)
			}
			if !update.IsUser {
				subtitle += " (system)"
			}
			row.SetSubtitle(subtitle)

			// Add update button
			updateBtn := gtk.NewButtonWithLabel("Update")
			updateBtn.SetValign(gtk.AlignCenterValue)
			updateBtn.AddCssClass("suggested-action")

			appID := update.ID
			isUser := update.IsUser
			clickedCb := func(btn gtk.Button) {
				btn.SetSensitive(false)
				btn.SetLabel("Updating...")
				go func() {
					if err := pm.FlatpakUpdate(appID, isUser); err != nil {
						async.RunOnMain(func() {
							btn.SetSensitive(true)
							btn.SetLabel("Update")
							uh.toastAdder.ShowErrorToast(fmt.Sprintf("Update failed: %v", err))
						})
						return
					}
					async.RunOnMain(func() {
						uh.toastAdder.ShowToast(fmt.Sprintf("%s updated", appID))
						// Refresh the updates list
						go uh.loadFlatpakUpdates()
					})
				}()
			}
			updateBtn.ConnectClicked(&clickedCb)

			row.AddSuffix(&updateBtn.Widget)
			uh.flatpakUpdatesExpander.AddRow(&row.Widget)
			uh.flatpakUpdateRows = append(uh.flatpakUpdateRows, row)
		}
	})
}

func (uh *UserHome) onHomebrewSearch() {
	query := uh.searchEntry.GetText()
	if query == "" {
		return
	}

	uh.searchResultsExpander.SetSubtitle("Searching...")
	uh.searchResultsExpander.SetEnableExpansion(false)

	go func() {
		results, err := pm.HomebrewSearch(query)
		if err != nil {
			async.RunOnMain(func() {
				uh.searchResultsExpander.SetSubtitle(fmt.Sprintf("Error: %v", err))
			})
			return
		}

		async.RunOnMain(func() {
			uh.searchResultsExpander.SetSubtitle(fmt.Sprintf("%d results", len(results)))
			uh.searchResultsExpander.SetEnableExpansion(len(results) > 0)

			// Add result rows
			for _, result := range results {
				row := adw.NewActionRow()
				row.SetTitle(result.Name)

				installBtn := gtk.NewButtonWithLabel("Install")
				installBtn.SetValign(gtk.AlignCenterValue)
				installBtn.AddCssClass("suggested-action")

				pkgName := result.Name
				clickedCb := func(btn gtk.Button) {
					log.Printf("Install button clicked for package: %s", pkgName)
					btn.SetSensitive(false)
					btn.SetLabel("Installing...")
					go func() {
						log.Printf("Starting installation of %s", pkgName)
						if err := pm.HomebrewInstall(pkgName, false); err != nil {
							log.Printf("Installation failed for %s: %v", pkgName, err)
							async.RunOnMain(func() {
								btn.SetSensitive(true)
								btn.SetLabel("Install")
								userErr := async.NewUserErrorWithHint(
									fmt.Sprintf("Couldn't install %s", pkgName),
									"Check your internet connection and try again",
									err,
								)
								uh.toastAdder.ShowErrorToast(userErr.FormatForUser())
							})
							return
						}
						log.Printf("Successfully installed %s", pkgName)
						async.RunOnMain(func() {
							btn.SetLabel("Installed")
							uh.toastAdder.ShowToast(fmt.Sprintf("%s installed", pkgName))
						})
					}()
				}
				installBtn.ConnectClicked(&clickedCb)

				row.AddSuffix(&installBtn.Widget)
				uh.searchResultsExpander.AddRow(&row.Widget)
			}
		})
	}()
}

// onPMProgressUpdate handles progress updates from pm library operations
// This creates a nested UI hierarchy: Action → Task → Step progress with messages
func (uh *UserHome) onPMProgressUpdate(action *pmlib.ProgressAction, task *pmlib.ProgressTask, step *pmlib.ProgressStep, message *pmlib.ProgressMessage) {
	uh.currentProgressMu.Lock()
	defer uh.currentProgressMu.Unlock()

	// Handle action-level progress
	if action != nil {
		log.Printf("[Progress] Action: %s (ID: %s, Started: %v, Ended: %v)", action.Name, action.ID, action.StartedAt, action.EndedAt)

		// Store the action ID -> Name mapping
		uh.progressActions[action.ID] = action.Name

		async.RunOnMain(func() {
			if expander, exists := uh.progressExpanders[action.Name]; exists {
				// Update existing action
				if !action.EndedAt.IsZero() {
					// Action completed
					expander.SetSubtitle("Completed")

					// Remove the action after a delay
					go func() {
						time.Sleep(2 * time.Second)
						async.RunOnMain(func() {
							uh.currentProgressMu.Lock()
							defer uh.currentProgressMu.Unlock()

							// Remove the group from the page
							if group, ok := uh.progressGroups[action.Name]; ok {
								if uh.progressPage != nil {
									uh.progressPage.Remove(group)
								}
								delete(uh.progressGroups, action.Name)
							}
							delete(uh.progressExpanders, action.Name)

							// Close bottom sheet if no more active operations
							if len(uh.progressExpanders) == 0 && uh.progressBottomSheet != nil {
								uh.progressBottomSheet.SetOpen(false)
							}
						})
					}()
				} else {
					expander.SetSubtitle("In progress...")
				}
			} else if !action.StartedAt.IsZero() && action.EndedAt.IsZero() {
				// Create new action expander when it starts
				expander := adw.NewExpanderRow()
				expander.SetTitle(action.Name)
				expander.SetSubtitle("Starting...")
				uh.progressExpanders[action.Name] = expander

				// Add to progress page and open bottom sheet
				if uh.progressPage != nil {
					group := adw.NewPreferencesGroup()
					group.Add(&expander.Widget)
					uh.progressPage.Add(group)
					uh.progressGroups[action.Name] = group
				}
				if uh.progressBottomSheet != nil {
					uh.progressBottomSheet.SetOpen(true)
				}
			}
		})
	}

	// Handle task-level progress
	if task != nil {
		actionName := uh.progressActions[task.ActionID]
		log.Printf("[Progress] Task: %s (Action: %s, ID: %s, ActionID: %s, Started: %v, Ended: %v)",
			task.Name, actionName, task.ID, task.ActionID, task.StartedAt, task.EndedAt)

		// Store task ID mapping
		key := actionName + ":" + task.Name
		uh.progressTasks[task.ID] = key

		async.RunOnMain(func() {
			if row, exists := uh.progressRows[key]; exists {
				// Update existing task
				if !task.EndedAt.IsZero() {
					// Task completed - stop spinner and replace with checkmark
					row.SetSubtitle("Completed")
					if spinner, ok := uh.progressSpinners[key]; ok {
						spinner.Stop()
						row.Remove(&spinner.Widget)
						delete(uh.progressSpinners, key)
						// Add checkmark icon
						checkmark := gtk.NewImageFromIconName("object-select-symbolic")
						row.AddPrefix(&checkmark.Widget)
					}
				} else {
					row.SetSubtitle("In progress...")
				}
			} else if !task.StartedAt.IsZero() && task.EndedAt.IsZero() {
				// Create new task row when it starts
				if expander, exists := uh.progressExpanders[actionName]; exists {
					row := adw.NewActionRow()
					row.SetTitle(task.Name)
					row.SetSubtitle("Starting...")
					spinner := gtk.NewSpinner()
					spinner.Start()
					row.AddPrefix(&spinner.Widget)
					uh.progressRows[key] = row
					uh.progressSpinners[key] = spinner
					expander.AddRow(&row.Widget)
				}
			}
		})
	}

	// Handle step-level progress
	if step != nil {
		log.Printf("[Progress] Step: %s (TaskID: %s, Started: %v, Ended: %v)",
			step.Name, step.TaskID, step.StartedAt, step.EndedAt)

		async.RunOnMain(func() {
			// Find the task row using the task ID
			if key, ok := uh.progressTasks[step.TaskID]; ok {
				if row, exists := uh.progressRows[key]; exists {
					if !step.EndedAt.IsZero() {
						// Step completed
						row.SetSubtitle(step.Name + " - Completed")
					} else if !step.StartedAt.IsZero() {
						// Step in progress
						row.SetSubtitle(step.Name)
					}
				}
			}
		})
	}

	// Handle messages
	if message != nil {
		actionName := uh.progressActions[message.ActionID]
		severity := "info"
		switch message.Severity {
		case pmlib.SeverityWarning:
			severity = "warning"
		case pmlib.SeverityError:
			severity = "error"
		}

		log.Printf("[Progress] Message: %s (Action: %s, TaskID: %s, StepID: %s, ActionID: %s, Severity: %s)",
			message.Text, actionName, message.TaskID, message.StepID, message.ActionID, severity)

		async.RunOnMain(func() {
			// Update the action or task row based on what IDs are present
			if message.TaskID != "" {
				// Find the task row by action name (we don't have task name from ID)
				for key, row := range uh.progressRows {
					if strings.HasPrefix(key, actionName+":") {
						row.SetSubtitle(message.Text)
						break
					}
				}
			} else if actionName != "" {
				// Update the action expander
				if expander, exists := uh.progressExpanders[actionName]; exists {
					expander.SetSubtitle(message.Text)
				}
			}

			// Show toast notification based on severity
			switch severity {
			case "warning":
				uh.toastAdder.ShowToast("⚠️ " + message.Text)
			case "error":
				uh.toastAdder.ShowErrorToast("❌ " + message.Text)
			default:
				// info level - log only, don't spam with toasts
			}
		})
	}
}

// cleanupProgressUI clears all progress UI elements after operations complete
// Can be called manually to clear progress indicators
//
//nolint:unused // Reserved for manual cleanup or batch operations
func (uh *UserHome) cleanupProgressUI() {
	uh.currentProgressMu.Lock()
	defer uh.currentProgressMu.Unlock()

	log.Printf("[Progress] Cleanup")

	async.RunOnMain(func() {
		// Clear all progress rows
		for key := range uh.progressRows {
			// Note: Spinners will be garbage collected when rows are removed
			delete(uh.progressRows, key)
		}

		// Clear all progress expanders
		for key := range uh.progressExpanders {
			delete(uh.progressExpanders, key)
		}
	})
}
