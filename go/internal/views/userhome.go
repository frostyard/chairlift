// Package views provides the page content for the ChairLift application
package views

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/frostyard/chairlift/internal/config"
	"github.com/frostyard/chairlift/internal/homebrew"

	"github.com/jwijenbergh/puregotk/v4/adw"
	"github.com/jwijenbergh/puregotk/v4/glib"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

// idleCallbackRegistry stores callbacks to prevent GC collection
var (
	idleCallbackMu sync.Mutex
	idleCallbacks  = make(map[uintptr]func())
	idleCallbackID uintptr
)

// runOnMainThread schedules a function to run on the GTK main thread
func runOnMainThread(fn func()) {
	idleCallbackMu.Lock()
	idleCallbackID++
	id := idleCallbackID
	idleCallbacks[id] = fn
	idleCallbackMu.Unlock()

	cb := glib.SourceFunc(func(data uintptr) bool {
		idleCallbackMu.Lock()
		callback, ok := idleCallbacks[data]
		delete(idleCallbacks, data)
		idleCallbackMu.Unlock()

		if ok {
			callback()
		}
		return false // Remove source after execution
	})
	glib.IdleAdd(&cb, id)
}

// ToastAdder is an interface for adding toasts
type ToastAdder interface {
	ShowToast(message string)
	ShowErrorToast(message string)
}

// UserHome manages all content pages
type UserHome struct {
	config     *config.Config
	toastAdder ToastAdder

	// Pages (ToolbarViews)
	systemPage       *adw.ToolbarView
	updatesPage      *adw.ToolbarView
	applicationsPage *adw.ToolbarView
	maintenancePage  *adw.ToolbarView
	helpPage         *adw.ToolbarView

	// PreferencesPages inside each ToolbarView - keep references to prevent GC
	systemPrefsPage       *adw.PreferencesPage
	updatesPrefsPage      *adw.PreferencesPage
	applicationsPrefsPage *adw.PreferencesPage
	maintenancePrefsPage  *adw.PreferencesPage
	helpPrefsPage         *adw.PreferencesPage

	// References for dynamic updates
	formulaeExpander      *adw.ExpanderRow
	casksExpander         *adw.ExpanderRow
	outdatedExpander      *adw.ExpanderRow
	searchResultsExpander *adw.ExpanderRow
	searchEntry           *gtk.SearchEntry
}

// New creates a new UserHome views manager
func New(cfg *config.Config, toastAdder ToastAdder) *UserHome {
	uh := &UserHome{
		config:     cfg,
		toastAdder: toastAdder,
	}

	// Create pages - createPage returns both ToolbarView and PreferencesPage
	uh.systemPage, uh.systemPrefsPage = uh.createPage()
	uh.updatesPage, uh.updatesPrefsPage = uh.createPage()
	uh.applicationsPage, uh.applicationsPrefsPage = uh.createPage()
	uh.maintenancePage, uh.maintenancePrefsPage = uh.createPage()
	uh.helpPage, uh.helpPrefsPage = uh.createPage()

	// Build page content
	uh.buildSystemPage()
	uh.buildUpdatesPage()
	uh.buildApplicationsPage()
	uh.buildMaintenancePage()
	uh.buildHelpPage()

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
		return uh.maintenancePage
	case "help":
		return uh.helpPage
	default:
		return nil
	}
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

// buildSystemPage builds the System page content
func (uh *UserHome) buildSystemPage() {
	page := uh.systemPrefsPage
	if page == nil {
		return
	}

	// System Information group
	if uh.config.IsGroupEnabled("system_page", "system_info_group") {
		group := adw.NewPreferencesGroup()
		group.SetTitle("System Information")
		group.SetDescription("View system details and hardware information")

		// OS Release expander
		osExpander := adw.NewExpanderRow()
		osExpander.SetTitle("Operating System Details")

		uh.loadOSRelease(osExpander)
		group.Add(&osExpander.Widget)
		page.Add(group)
	}

	// System Health group
	if uh.config.IsGroupEnabled("system_page", "health_group") {
		group := adw.NewPreferencesGroup()
		group.SetTitle("System Health")
		group.SetDescription("Overview of system health and diagnostics")

		perfRow := adw.NewActionRow()
		perfRow.SetTitle("System Performance")
		perfRow.SetSubtitle("Monitor CPU, memory, and system resources")
		perfRow.SetActivatable(true)

		icon := gtk.NewImageFromIconName("adw-external-link-symbolic")
		perfRow.AddSuffix(&icon.Widget)

		groupCfg := uh.config.GetGroupConfig("system_page", "health_group")
		appID := "io.missioncenter.MissionCenter"
		if groupCfg != nil && groupCfg.AppID != "" {
			appID = groupCfg.AppID
		}

		activatedCb := func(row adw.ActionRow) {
			uh.launchApp(appID)
		}
		perfRow.ConnectActivated(&activatedCb)

		group.Add(&perfRow.Widget)
		page.Add(group)
	}
}

// loadOSRelease loads /etc/os-release into the expander
func (uh *UserHome) loadOSRelease(expander *adw.ExpanderRow) {
	file, err := os.Open("/etc/os-release")
	if err != nil {
		row := adw.NewActionRow()
		row.SetTitle("OS Information")
		row.SetSubtitle("Not available")
		expander.AddRow(&row.Widget)
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		key := parts[0]
		value := strings.Trim(parts[1], "\"'")

		// Convert key to readable format
		readableKey := strings.ReplaceAll(key, "_", " ")
		readableKey = strings.Title(strings.ToLower(readableKey))

		row := adw.NewActionRow()
		row.SetTitle(readableKey)
		row.SetSubtitle(value)

		// Make URL rows clickable
		if strings.HasSuffix(key, "URL") {
			row.SetActivatable(true)
			icon := gtk.NewImageFromIconName("adw-external-link-symbolic")
			row.AddSuffix(&icon.Widget)

			url := value
			activatedCb := func(row adw.ActionRow) {
				uh.openURL(url)
			}
			row.ConnectActivated(&activatedCb)
		}

		expander.AddRow(&row.Widget)
	}
}

// buildUpdatesPage builds the Updates page content
func (uh *UserHome) buildUpdatesPage() {
	page := uh.updatesPrefsPage
	if page == nil {
		return
	}

	// System Updates group
	if uh.config.IsGroupEnabled("updates_page", "updates_status_group") {
		group := adw.NewPreferencesGroup()
		group.SetTitle("System Updates")
		group.SetDescription("Check for and install system updates")

		groupCfg := uh.config.GetGroupConfig("updates_page", "updates_status_group")
		if groupCfg != nil {
			for _, action := range groupCfg.Actions {
				row := adw.NewActionRow()
				row.SetTitle(action.Title)
				row.SetSubtitle(action.Script)

				if action.Sudo {
					sudoIcon := gtk.NewImageFromIconName("dialog-password-symbolic")
					row.AddPrefix(&sudoIcon.Widget)
				}

				button := gtk.NewButtonWithLabel("Run")
				button.SetValign(gtk.AlignCenterValue)
				button.AddCssClass("suggested-action")

				script := action.Script
				sudo := action.Sudo
				title := action.Title
				clickedCb := func(btn gtk.Button) {
					uh.runMaintenanceAction(title, script, sudo)
				}
				button.ConnectClicked(&clickedCb)

				row.AddSuffix(&button.Widget)
				group.Add(&row.Widget)
			}
		}

		page.Add(group)
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
			uh.onUpdateHomebrewClicked()
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

	// Homebrew group
	if uh.config.IsGroupEnabled("applications_page", "brew_group") {
		group := adw.NewPreferencesGroup()
		group.SetTitle("Homebrew")
		group.SetDescription("Manage Homebrew packages installed on your system")

		// Bundle dump row
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

// buildMaintenancePage builds the Maintenance page content
func (uh *UserHome) buildMaintenancePage() {
	page := uh.maintenancePrefsPage
	if page == nil {
		return
	}

	// Cleanup group
	if uh.config.IsGroupEnabled("maintenance_page", "maintenance_cleanup_group") {
		group := adw.NewPreferencesGroup()
		group.SetTitle("System Cleanup")
		group.SetDescription("Clean up system files and free disk space")

		groupCfg := uh.config.GetGroupConfig("maintenance_page", "maintenance_cleanup_group")
		if groupCfg != nil {
			for _, action := range groupCfg.Actions {
				row := adw.NewActionRow()
				row.SetTitle(action.Title)
				row.SetSubtitle(action.Script)

				if action.Sudo {
					sudoIcon := gtk.NewImageFromIconName("dialog-password-symbolic")
					row.AddPrefix(&sudoIcon.Widget)
				}

				button := gtk.NewButtonWithLabel("Run")
				button.SetValign(gtk.AlignCenterValue)
				button.AddCssClass("suggested-action")

				script := action.Script
				sudo := action.Sudo
				title := action.Title
				clickedCb := func(btn gtk.Button) {
					uh.runMaintenanceAction(title, script, sudo)
				}
				button.ConnectClicked(&clickedCb)

				row.AddSuffix(&button.Widget)
				group.Add(&row.Widget)
			}
		}

		page.Add(group)
	}

	// Optimization group
	if uh.config.IsGroupEnabled("maintenance_page", "maintenance_optimization_group") {
		group := adw.NewPreferencesGroup()
		group.SetTitle("System Optimization")
		group.SetDescription("Optimize system performance")

		// Placeholder for optimization features
		row := adw.NewActionRow()
		row.SetTitle("Optimization tools")
		row.SetSubtitle("Coming soon")
		group.Add(&row.Widget)

		page.Add(group)
	}
}

// buildHelpPage builds the Help page content
func (uh *UserHome) buildHelpPage() {
	page := uh.helpPrefsPage
	if page == nil {
		return
	}

	// Help Resources group
	if uh.config.IsGroupEnabled("help_page", "help_resources_group") {
		group := adw.NewPreferencesGroup()
		group.SetTitle("Help &amp; Resources")
		group.SetDescription("Get help and learn more about ChairLift")

		groupCfg := uh.config.GetGroupConfig("help_page", "help_resources_group")

		// Website row
		if groupCfg != nil && groupCfg.Website != "" {
			row := adw.NewActionRow()
			row.SetTitle("Website")
			row.SetSubtitle(groupCfg.Website)
			row.SetActivatable(true)

			icon := gtk.NewImageFromIconName("adw-external-link-symbolic")
			row.AddSuffix(&icon.Widget)

			url := groupCfg.Website
			activatedCb := func(row adw.ActionRow) {
				uh.openURL(url)
			}
			row.ConnectActivated(&activatedCb)

			group.Add(&row.Widget)
		}

		// Issues row
		if groupCfg != nil && groupCfg.Issues != "" {
			row := adw.NewActionRow()
			row.SetTitle("Report Issues")
			row.SetSubtitle(groupCfg.Issues)
			row.SetActivatable(true)

			icon := gtk.NewImageFromIconName("adw-external-link-symbolic")
			row.AddSuffix(&icon.Widget)

			url := groupCfg.Issues
			activatedCb := func(row adw.ActionRow) {
				uh.openURL(url)
			}
			row.ConnectActivated(&activatedCb)

			group.Add(&row.Widget)
		}

		// Chat row
		if groupCfg != nil && groupCfg.Chat != "" {
			row := adw.NewActionRow()
			row.SetTitle("Community Discussions")
			row.SetSubtitle(groupCfg.Chat)
			row.SetActivatable(true)

			icon := gtk.NewImageFromIconName("adw-external-link-symbolic")
			row.AddSuffix(&icon.Widget)

			url := groupCfg.Chat
			activatedCb := func(row adw.ActionRow) {
				uh.openURL(url)
			}
			row.ConnectActivated(&activatedCb)

			group.Add(&row.Widget)
		}

		page.Add(group)
	}
}

// Helper methods

func (uh *UserHome) launchApp(appID string) {
	log.Printf("Launching app: %s", appID)
	// TODO: Use D-Bus to launch the application
}

func (uh *UserHome) openURL(url string) {
	log.Printf("Opening URL: %s", url)
	// Use gtk_show_uri or xdg-open
}

func (uh *UserHome) runMaintenanceAction(title, script string, sudo bool) {
	log.Printf("Running action: %s (script: %s, sudo: %v)", title, script, sudo)
	// TODO: Execute the script
}

func (uh *UserHome) onUpdateHomebrewClicked() {
	go func() {
		if err := homebrew.Update(); err != nil {
			runOnMainThread(func() {
				uh.toastAdder.ShowErrorToast(fmt.Sprintf("Update failed: %v", err))
			})
			return
		}
		runOnMainThread(func() {
			uh.toastAdder.ShowToast("Homebrew updated successfully")
		})
	}()
}

func (uh *UserHome) onBrewBundleDumpClicked() {
	go func() {
		homeDir, _ := os.UserHomeDir()
		path := homeDir + "/Brewfile"
		if err := homebrew.BundleDump(path, true); err != nil {
			runOnMainThread(func() {
				uh.toastAdder.ShowErrorToast(fmt.Sprintf("Bundle dump failed: %v", err))
			})
			return
		}
		runOnMainThread(func() {
			uh.toastAdder.ShowToast(fmt.Sprintf("Brewfile saved to %s", path))
		})
	}()
}

func (uh *UserHome) loadOutdatedPackages() {
	if !homebrew.IsInstalled() {
		runOnMainThread(func() {
			uh.outdatedExpander.SetSubtitle("Homebrew not installed")
		})
		return
	}

	packages, err := homebrew.ListOutdated()
	if err != nil {
		runOnMainThread(func() {
			uh.outdatedExpander.SetSubtitle(fmt.Sprintf("Error: %v", err))
		})
		return
	}

	runOnMainThread(func() {
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
					if err := homebrew.Upgrade(pkgName); err != nil {
						runOnMainThread(func() {
							uh.toastAdder.ShowErrorToast(fmt.Sprintf("Upgrade failed: %v", err))
						})
						return
					}
					runOnMainThread(func() {
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
	if !homebrew.IsInstalled() {
		runOnMainThread(func() {
			uh.formulaeExpander.SetSubtitle("Homebrew not installed")
			uh.casksExpander.SetSubtitle("Homebrew not installed")
		})
		return
	}

	// Load formulae
	formulae, err := homebrew.ListInstalledFormulae()
	if err != nil {
		runOnMainThread(func() {
			uh.formulaeExpander.SetSubtitle(fmt.Sprintf("Error: %v", err))
		})
	} else {
		runOnMainThread(func() {
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
	casks, err := homebrew.ListInstalledCasks()
	if err != nil {
		runOnMainThread(func() {
			uh.casksExpander.SetSubtitle(fmt.Sprintf("Error: %v", err))
		})
	} else {
		runOnMainThread(func() {
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

func (uh *UserHome) onHomebrewSearch() {
	query := uh.searchEntry.GetText()
	if query == "" {
		return
	}

	uh.searchResultsExpander.SetSubtitle("Searching...")
	uh.searchResultsExpander.SetEnableExpansion(false)

	go func() {
		results, err := homebrew.Search(query)
		if err != nil {
			runOnMainThread(func() {
				uh.searchResultsExpander.SetSubtitle(fmt.Sprintf("Error: %v", err))
			})
			return
		}

		runOnMainThread(func() {
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
					go func() {
						if err := homebrew.Install(pkgName, false); err != nil {
							runOnMainThread(func() {
								uh.toastAdder.ShowErrorToast(fmt.Sprintf("Install failed: %v", err))
							})
							return
						}
						runOnMainThread(func() {
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
