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
	"github.com/frostyard/chairlift/internal/nbc"
	"github.com/frostyard/chairlift/internal/pages"
	"github.com/frostyard/chairlift/internal/pages/applications"
	"github.com/frostyard/chairlift/internal/pages/extensions"
	"github.com/frostyard/chairlift/internal/pages/help"
	"github.com/frostyard/chairlift/internal/pages/maintenance"
	"github.com/frostyard/chairlift/internal/pages/system"
	"github.com/frostyard/chairlift/internal/pages/updates"
	"github.com/frostyard/chairlift/internal/pm"

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
	systemPagePkg       *system.Page
	helpPagePkg         *help.Page
	maintenancePagePkg  *maintenance.Page
	extensionsPagePkg   *extensions.Page
	applicationsPagePkg *applications.Page
	updatesPagePkg      *updates.Page

	// Pages (ToolbarViews) - widgets returned by page packages or createPage()
	systemPage       *adw.ToolbarView
	updatesPage      *adw.ToolbarView
	applicationsPage *adw.ToolbarView
	helpPage         *adw.ToolbarView

	// References for dynamic updates (legacy - used by unreachable buildApplicationsPage)
	formulaeExpander       *adw.ExpanderRow
	casksExpander          *adw.ExpanderRow
	searchResultsExpander  *adw.ExpanderRow
	searchEntry            *gtk.SearchEntry
	flatpakUserExpander    *adw.ExpanderRow
	flatpakSystemExpander  *adw.ExpanderRow
	snapExpander           *adw.ExpanderRow
	snapStoreLinkRow       *adw.ActionRow
	snapStoreInstallRow    *adw.ActionRow
	snapRows               []*adw.ActionRow // Store references for cleanup

	// Progress tracking UI
	progressBottomSheet *adw.BottomSheet     // BottomSheet for displaying active operations
	progressPage        *adw.PreferencesPage // Preferences page inside the bottom sheet
	progressScrolled    *gtk.ScrolledWindow  // Scrolled window for progress content

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
// Extensions page's Destroy() cancels discovery goroutines
// Applications page's Destroy() cancels async operations
// Updates page's Destroy() cancels update check/install goroutines
// Example: func (uh *UserHome) Destroy() {
//     if uh.systemPagePkg != nil { uh.systemPagePkg.Destroy() }
//     if uh.helpPagePkg != nil { uh.helpPagePkg.Destroy() }
//     if uh.maintenancePagePkg != nil { uh.maintenancePagePkg.Destroy() }
//     if uh.extensionsPagePkg != nil { uh.extensionsPagePkg.Destroy() }
//     if uh.applicationsPagePkg != nil { uh.applicationsPagePkg.Destroy() }
//     if uh.updatesPagePkg != nil { uh.updatesPagePkg.Destroy() }
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

	// Create Extensions page using extracted package
	uh.extensionsPagePkg = extensions.New(deps)

	// Create Applications page using extracted package
	uh.applicationsPagePkg = applications.New(deps, uh.launchApp, uh.openURL)
	uh.applicationsPage = uh.applicationsPagePkg.Widget()

	// Create Updates page using extracted package
	uh.updatesPagePkg = updates.New(deps, uh.toastAdder.SetUpdateBadge)
	uh.updatesPage = uh.updatesPagePkg.Widget()

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
		// NOTE: All page packages now build their UI in their constructors, so no calls here
		uh.buildApplicationsPage()
	}()

	return uh
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
		return uh.extensionsPagePkg.Widget()
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

// buildApplicationsPage builds the Applications page content
// NOTE: This is now a no-op as the Applications page is built by the applications package.
// This method and its associated UI code will be removed in Plan 03.
func (uh *UserHome) buildApplicationsPage() {
	// Applications page is now managed by applications.Page package
	// Return early - the sidebar-based content will be added in Plan 03
	return

	// Legacy code below will be removed in Plan 03
	var page *adw.PreferencesPage // placeholder to avoid compile errors
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
