package views

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/frostyard/chairlift/internal/flatpak"
	"github.com/frostyard/chairlift/internal/homebrew"
	"github.com/frostyard/chairlift/internal/views/actionmsg"
	"github.com/frostyard/chairlift/internal/views/actionstate"
	"github.com/frostyard/chairlift/internal/views/bundleview"

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
		group.SetDescription("Search for and install Homebrew formulae and casks")

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

	// Curated Homebrew bundles group
	if uh.config.IsGroupEnabled("applications_page", "brew_bundles_group") {
		group := adw.NewPreferencesGroup()
		group.SetTitle("Brew Bundles")
		group.SetDescription("Loading configured Brewfile bundles...")
		page.Add(group)
		uh.brewBundlesGroup = group

		var bundlePaths []string
		groupCfg := uh.config.GetGroupConfig("applications_page", "brew_bundles_group")
		if groupCfg != nil {
			bundlePaths = append(bundlePaths, groupCfg.BundlesPaths...)
		}
		go uh.loadBrewBundles(bundlePaths)
	}
}

// loadBrewBundles discovers configured Brewfiles on a worker goroutine and
// builds all bundle rows on GTK's main thread.
func (uh *UserHome) loadBrewBundles(paths []string) {
	bundles, discoveryErr := homebrew.AvailableBundles(paths)
	homebrewAvailable := homebrew.IsInstalledCached()
	warning := ""
	if discoveryErr != nil {
		log.Printf("Error discovering Brew bundles: %v", discoveryErr)
		warning = discoveryErr.Error()
	}
	presentation := bundleview.Present(len(bundles), warning, homebrewAvailable)

	sgtk.RunOnMainThread(func() {
		if uh.brewBundlesGroup == nil {
			return
		}
		uh.brewBundlesGroup.SetDescription(presentation.Description)

		if len(bundles) == 0 {
			row := adw.NewActionRow()
			row.SetTitle(presentation.PlaceholderTitle)
			row.SetSubtitle(presentation.PlaceholderSubtitle)
			uh.brewBundlesGroup.Add(&row.Widget)
			return
		}

		for _, bundle := range bundles {
			row := adw.NewActionRow()
			row.SetTitle(bundle.Name)
			subtitle := bundle.Path
			if bundle.Description != "" {
				subtitle = fmt.Sprintf("%s — %s", bundle.Description, bundle.Path)
			}
			row.SetSubtitle(subtitle)

			installBtn := gtk.NewButtonWithLabel("Install")
			installBtn.SetValign(gtk.AlignCenterValue)
			installBtn.AddCssClass("suggested-action")
			installBtn.SetSensitive(homebrewAvailable)
			if !homebrewAvailable {
				installBtn.SetTooltipText("Homebrew is not installed")
			}

			gate := &bundleview.InstallGate{}
			bundle := bundle
			clickedCb := func(btn gtk.Button) {
				if !gate.TryStart() {
					return
				}
				btn.SetSensitive(false)
				btn.SetLabel("Installing...")

				go func() {
					if err := homebrew.BundleInstall(bundle.Path); err != nil {
						sgtk.RunOnMainThread(func() {
							gate.Reset()
							btn.SetLabel("Install")
							btn.SetSensitive(homebrewAvailable)
							uh.toastAdder.ShowErrorToast(fmt.Sprintf(
								"Could not install Brew bundle %s: %v",
								bundle.Name,
								err,
							))
						})
						return
					}

					decision := actionmsg.BundleInstall(homebrew.IsDryRun(), bundle.Name)
					sgtk.RunOnMainThread(func() {
						if decision.Complete {
							gate.Complete()
							btn.SetLabel("Installed")
							btn.SetSensitive(false)
						} else {
							gate.Reset()
							btn.SetLabel("Install")
							btn.SetSensitive(homebrewAvailable)
						}
						uh.toastAdder.ShowToast(decision.Toast)
					})
				}()
			}
			installBtn.ConnectClicked(&clickedCb)

			row.AddSuffix(&installBtn.Widget)
			uh.brewBundlesGroup.Add(&row.Widget)
		}
	})
}

// loadHomebrewPackages loads installed Homebrew packages asynchronously
func (uh *UserHome) loadHomebrewPackages() {
	generation := uh.brewPackagesRefresh.Begin()
	if !homebrew.IsInstalledCached() {
		sgtk.RunOnMainThread(func() {
			if !uh.brewPackagesRefresh.IsCurrent(generation) {
				return
			}
			if uh.formulaeExpander != nil {
				uh.formulaeExpander.SetSubtitle("Homebrew not installed")
			}
			if uh.casksExpander != nil {
				uh.casksExpander.SetSubtitle("Homebrew not installed")
			}
		})
		return
	}

	// Load formulae
	if uh.formulaeExpander != nil {
		formulae, err := homebrew.ListInstalledFormulae()
		if err != nil {
			sgtk.RunOnMainThread(func() {
				if !uh.brewPackagesRefresh.IsCurrent(generation) {
					return
				}
				uh.formulaeExpander.SetSubtitle(fmt.Sprintf("Error: %v", err))
			})
		} else {
			sgtk.RunOnMainThread(func() {
				if !uh.brewPackagesRefresh.IsCurrent(generation) {
					return
				}
				uh.formulaeRows.Clear(func(row *adw.ActionRow) {
					uh.formulaeExpander.Remove(&row.Widget)
				})
				uh.formulaeExpander.SetSubtitle(fmt.Sprintf("%d installed", len(formulae)))
				uh.formulaeExpander.SetEnableExpansion(len(formulae) > 0)
				for _, pkg := range formulae {
					row := adw.NewActionRow()
					row.SetTitle(pkg.Name)
					row.SetSubtitle(pkg.Version)
					uh.formulaeExpander.AddRow(&row.Widget)
					uh.formulaeRows.Add(row)
				}
			})
		}
	}

	// Load casks
	if uh.casksExpander != nil {
		casks, err := homebrew.ListInstalledCasks()
		if err != nil {
			sgtk.RunOnMainThread(func() {
				if !uh.brewPackagesRefresh.IsCurrent(generation) {
					return
				}
				uh.casksExpander.SetSubtitle(fmt.Sprintf("Error: %v", err))
			})
		} else {
			sgtk.RunOnMainThread(func() {
				if !uh.brewPackagesRefresh.IsCurrent(generation) {
					return
				}
				uh.caskRows.Clear(func(row *adw.ActionRow) {
					uh.casksExpander.Remove(&row.Widget)
				})
				uh.casksExpander.SetSubtitle(fmt.Sprintf("%d installed", len(casks)))
				uh.casksExpander.SetEnableExpansion(len(casks) > 0)
				for _, pkg := range casks {
					row := adw.NewActionRow()
					row.SetTitle(pkg.Name)
					row.SetSubtitle(pkg.Version)
					uh.casksExpander.AddRow(&row.Widget)
					uh.caskRows.Add(row)
				}
			})
		}
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
				// Clear rows added by a previous load before repopulating
				uh.flatpakUserRows.Clear(func(r *adw.ActionRow) { uh.flatpakUserExpander.Remove(&r.Widget) })

				uh.flatpakUserExpander.SetSubtitle(fmt.Sprintf("%d installed", len(userApps)))
				uh.flatpakUserExpander.SetEnableExpansion(len(userApps) > 0)
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
								uh.toastAdder.ShowToast(actionmsg.Uninstall(flatpak.IsDryRun(), appID))
								// Refresh the list
								go uh.loadFlatpakApplications()
							})
						}()
					}
					uninstallBtn.ConnectClicked(&clickedCb)

					row.AddSuffix(&uninstallBtn.Widget)
					uh.flatpakUserExpander.AddRow(&row.Widget)
					uh.flatpakUserRows.Add(row)
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
				// Clear rows added by a previous load before repopulating
				uh.flatpakSystemRows.Clear(func(r *adw.ActionRow) { uh.flatpakSystemExpander.Remove(&r.Widget) })

				uh.flatpakSystemExpander.SetSubtitle(fmt.Sprintf("%d installed", len(systemApps)))
				uh.flatpakSystemExpander.SetEnableExpansion(len(systemApps) > 0)
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
								uh.toastAdder.ShowToast(actionmsg.Uninstall(flatpak.IsDryRun(), appID))
								// Refresh the list
								go uh.loadFlatpakApplications()
							})
						}()
					}
					uninstallBtn.ConnectClicked(&clickedCb)

					row.AddSuffix(&uninstallBtn.Widget)
					uh.flatpakSystemExpander.AddRow(&row.Widget)
					uh.flatpakSystemRows.Add(row)
				}
			})
		}
	}
}

// onHomebrewSearch handles the Homebrew search action
func (uh *UserHome) onHomebrewSearch() {
	query := strings.TrimSpace(uh.searchEntry.GetText())
	if query == "" {
		return
	}

	generation := uh.searchRefresh.Begin()
	uh.searchResultsExpander.SetSubtitle("Searching...")
	uh.searchResultsExpander.SetEnableExpansion(false)

	go func() {
		results, err := homebrew.Search(query)
		if err != nil {
			sgtk.RunOnMainThread(func() {
				if !uh.searchRefresh.IsCurrent(generation) {
					return
				}
				uh.searchResultsExpander.SetSubtitle(fmt.Sprintf("Error: %v", err))
			})
			return
		}

		sgtk.RunOnMainThread(func() {
			if !uh.searchRefresh.IsCurrent(generation) {
				return
			}
			// Clear previous search results
			uh.searchResultRows.Clear(func(row *adw.ActionRow) {
				uh.searchResultsExpander.Remove(&row.Widget)
			})

			uh.searchResultsExpander.SetSubtitle(fmt.Sprintf("%d results", len(results)))
			uh.searchResultsExpander.SetEnableExpansion(len(results) > 0)

			// Add result rows
			for _, result := range results {
				row := adw.NewActionRow()
				row.SetTitle(result.Name)
				row.SetSubtitle(result.Kind.DisplayName())

				installBtn := gtk.NewButtonWithLabel("Install")
				installBtn.SetValign(gtk.AlignCenterValue)
				installBtn.AddCssClass("suggested-action")

				result := result
				gate := &actionstate.Gate{}
				button := installBtn
				clickedCb := func(_ gtk.Button) {
					if !gate.TryStart() {
						return
					}
					uh.confirmHomebrewInstall(result, button, gate)
				}
				installBtn.ConnectClicked(&clickedCb)

				row.AddSuffix(&installBtn.Widget)
				uh.searchResultsExpander.AddRow(&row.Widget)
				uh.searchResultRows.Add(row)
			}
		})
	}()
}

func (uh *UserHome) confirmHomebrewInstall(result homebrew.SearchResult, button *gtk.Button, gate *actionstate.Gate) {
	kind := strings.ToLower(result.Kind.DisplayName())
	dialog := adw.NewAlertDialog(
		fmt.Sprintf("Install %s?", result.Name),
		fmt.Sprintf("Homebrew will install the %s %s and run its package-defined installation steps.", kind, result.Name),
	)
	dialog.AddResponse("cancel", "Cancel")
	dialog.AddResponse("install", "Install")
	dialog.SetResponseAppearance("install", adw.ResponseSuggestedValue)

	responseCb := func(_ adw.AlertDialog, response string) {
		if response != "install" {
			gate.Reset()
			return
		}
		button.SetSensitive(false)
		button.SetLabel("Installing...")
		go uh.installHomebrewSearchResult(result, button, gate)
	}
	dialog.ConnectResponse(&responseCb)
	dialog.Present(&uh.applicationsPrefsPage.Widget)
}

func (uh *UserHome) installHomebrewSearchResult(result homebrew.SearchResult, button *gtk.Button, gate *actionstate.Gate) {
	err := homebrew.Install(result.Name, result.Kind == homebrew.Cask)
	dryRun := homebrew.IsDryRun()
	decision := actionstate.PackageInstall(err == nil, dryRun)

	sgtk.RunOnMainThread(func() {
		if decision.RestoreControl {
			gate.Reset()
			button.SetSensitive(true)
			button.SetLabel("Install")
		}
		if err != nil {
			uh.toastAdder.ShowErrorToast(fmt.Sprintf("Install failed: %v", err))
			return
		}
		if decision.CompleteControl {
			gate.Complete()
			button.SetLabel("Installed")
			button.SetSensitive(false)
		}
		uh.toastAdder.ShowToast(actionmsg.Install(dryRun, result.Name))
		if decision.Refresh {
			go uh.loadHomebrewPackages()
		}
	})
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
