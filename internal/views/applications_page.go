package views

import (
	"fmt"
	"log"
	"os"
	"os/exec"

	"github.com/frostyard/chairlift/internal/flatpak"
	"github.com/frostyard/chairlift/internal/homebrew"
	"github.com/frostyard/chairlift/internal/snap"

	sgtk "github.com/frostyard/snowkit/gtk"

	"codeberg.org/puregotk/puregotk/v4/adw"
	"codeberg.org/puregotk/puregotk/v4/gtk"
)

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
	if uh.config.IsGroupEnabled("applications_page", "snap_group") {
		group := adw.NewPreferencesGroup()
		group.SetTitle("Snap Applications")
		group.SetDescription("Checking Snap availability...")
		uh.snapGroup = group

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

// loadHomebrewPackages loads installed Homebrew packages asynchronously
func (uh *UserHome) loadHomebrewPackages() {
	if !homebrew.IsInstalledCached() {
		sgtk.RunOnMainThread(func() {
			uh.formulaeExpander.SetSubtitle("Homebrew not installed")
			uh.casksExpander.SetSubtitle("Homebrew not installed")
		})
		return
	}

	// Load formulae
	formulae, err := homebrew.ListInstalledFormulae()
	if err != nil {
		sgtk.RunOnMainThread(func() {
			uh.formulaeExpander.SetSubtitle(fmt.Sprintf("Error: %v", err))
		})
	} else {
		sgtk.RunOnMainThread(func() {
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
		sgtk.RunOnMainThread(func() {
			uh.casksExpander.SetSubtitle(fmt.Sprintf("Error: %v", err))
		})
	} else {
		sgtk.RunOnMainThread(func() {
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

// loadFlatpakApplications loads installed Flatpak applications asynchronously
func (uh *UserHome) loadFlatpakApplications() {
	if !flatpak.IsInstalledCached() {
		sgtk.RunOnMainThread(func() {
			if uh.flatpakUserExpander != nil {
				uh.flatpakUserExpander.SetSubtitle("Flatpak not installed")
			}
			if uh.flatpakSystemExpander != nil {
				uh.flatpakSystemExpander.SetSubtitle("Flatpak not installed")
			}
		})
		return
	}

	// Load user applications
	if uh.flatpakUserExpander != nil {
		userApps, err := flatpak.ListUserApplications()
		if err != nil {
			sgtk.RunOnMainThread(func() {
				uh.flatpakUserExpander.SetSubtitle(fmt.Sprintf("Error: %v", err))
			})
		} else {
			sgtk.RunOnMainThread(func() {
				uh.flatpakUserExpander.SetSubtitle(fmt.Sprintf("%d installed", len(userApps)))
				for _, app := range userApps {
					row := adw.NewActionRow()
					row.SetTitle(app.Name)
					subtitle := app.ApplicationID
					if app.Version != "" {
						subtitle = fmt.Sprintf("%s (%s)", app.ApplicationID, app.Version)
					}
					row.SetSubtitle(subtitle)

					// Add uninstall button
					uninstallBtn := gtk.NewButtonFromIconName("user-trash-symbolic")
					uninstallBtn.SetValign(gtk.AlignCenterValue)
					uninstallBtn.AddCssClass("destructive-action")
					uninstallBtn.SetTooltipText("Uninstall")

					appID := app.ApplicationID
					clickedCb := func(btn gtk.Button) {
						btn.SetSensitive(false)
						go func() {
							if err := flatpak.Uninstall(appID, true); err != nil {
								sgtk.RunOnMainThread(func() {
									btn.SetSensitive(true)
									uh.toastAdder.ShowErrorToast(fmt.Sprintf("Uninstall failed: %v", err))
								})
								return
							}
							sgtk.RunOnMainThread(func() {
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
	}

	// Load system applications
	if uh.flatpakSystemExpander != nil {
		systemApps, err := flatpak.ListSystemApplications()
		if err != nil {
			sgtk.RunOnMainThread(func() {
				uh.flatpakSystemExpander.SetSubtitle(fmt.Sprintf("Error: %v", err))
			})
		} else {
			sgtk.RunOnMainThread(func() {
				uh.flatpakSystemExpander.SetSubtitle(fmt.Sprintf("%d installed", len(systemApps)))
				for _, app := range systemApps {
					row := adw.NewActionRow()
					row.SetTitle(app.Name)
					subtitle := app.ApplicationID
					if app.Version != "" {
						subtitle = fmt.Sprintf("%s (%s)", app.ApplicationID, app.Version)
					}
					row.SetSubtitle(subtitle)

					// Add uninstall button (requires elevated privileges for system apps)
					uninstallBtn := gtk.NewButtonFromIconName("user-trash-symbolic")
					uninstallBtn.SetValign(gtk.AlignCenterValue)
					uninstallBtn.AddCssClass("destructive-action")
					uninstallBtn.SetTooltipText("Uninstall (requires admin)")

					appID := app.ApplicationID
					clickedCb := func(btn gtk.Button) {
						btn.SetSensitive(false)
						go func() {
							if err := flatpak.Uninstall(appID, false); err != nil {
								sgtk.RunOnMainThread(func() {
									btn.SetSensitive(true)
									uh.toastAdder.ShowErrorToast(fmt.Sprintf("Uninstall failed: %v", err))
								})
								return
							}
							sgtk.RunOnMainThread(func() {
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
}

// loadSnapApplications loads installed snap packages asynchronously
func (uh *UserHome) loadSnapApplications() {
	if !snap.IsInstalledCached() {
		sgtk.RunOnMainThread(func() {
			if uh.snapGroup != nil {
				uh.snapGroup.SetVisible(false)
			}
		})
		return
	}

	sgtk.RunOnMainThread(func() {
		if uh.snapGroup != nil {
			uh.snapGroup.SetDescription("Manage Snap packages installed on your system")
		}
	})

	// Check if snap-store is installed
	snapStoreInstalled, err := snap.IsSnapInstalled("snap-store")
	if err != nil {
		log.Printf("Error checking snap-store: %v", err)
	}

	// Load installed snaps
	snaps, err := snap.ListInstalledSnaps()
	if err != nil {
		sgtk.RunOnMainThread(func() {
			if uh.snapExpander != nil {
				uh.snapExpander.SetSubtitle(fmt.Sprintf("Error: %v", err))
			}
		})
		return
	}

	sgtk.RunOnMainThread(func() {
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
		ctx, cancel := snap.DefaultContext()
		defer cancel()

		changeID, err := snap.Install(ctx, "snap-store")
		if err != nil {
			sgtk.RunOnMainThread(func() {
				button.SetSensitive(true)
				button.SetLabel("Install")
				uh.toastAdder.ShowErrorToast(fmt.Sprintf("Failed to install snap-store: %v", err))
			})
			return
		}

		// Wait for the installation to complete
		err = snap.WaitForChange(ctx, changeID)
		if err != nil {
			sgtk.RunOnMainThread(func() {
				button.SetSensitive(true)
				button.SetLabel("Install")
				uh.toastAdder.ShowErrorToast(fmt.Sprintf("Installation failed: %v", err))
			})
			return
		}

		sgtk.RunOnMainThread(func() {
			button.SetSensitive(true)
			button.SetLabel("Install")
			uh.toastAdder.ShowToast("Snap Store installed successfully!")
		})

		// Reload the snap list to update the UI
		uh.loadSnapApplications()
	}()
}

// onHomebrewSearch handles the Homebrew search action
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
			sgtk.RunOnMainThread(func() {
				uh.searchResultsExpander.SetSubtitle(fmt.Sprintf("Error: %v", err))
			})
			return
		}

		sgtk.RunOnMainThread(func() {
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
							sgtk.RunOnMainThread(func() {
								uh.toastAdder.ShowErrorToast(fmt.Sprintf("Install failed: %v", err))
							})
							return
						}
						sgtk.RunOnMainThread(func() {
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

// launchApp launches a desktop application by its application ID
func (uh *UserHome) launchApp(appID string) {
	log.Printf("Launching app: %s", appID)

	// Use gtk-launch to launch the application by its desktop file ID
	// gtk-launch handles looking up the desktop file and launching it correctly
	cmd := exec.Command("gtk-launch", appID)
	cmd.Env = os.Environ()

	if err := cmd.Start(); err != nil {
		log.Printf("Failed to launch app %s: %v", appID, err)
		uh.toastAdder.ShowErrorToast(fmt.Sprintf("Failed to launch %s", appID))
		return
	}

	// Don't wait for the command to finish - it's a GUI app
	go func() {
		_ = cmd.Wait()
	}()
}
