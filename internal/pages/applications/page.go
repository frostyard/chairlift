package applications

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/frostyard/chairlift/internal/async"
	"github.com/frostyard/chairlift/internal/config"
	"github.com/frostyard/chairlift/internal/pages"
	"github.com/frostyard/chairlift/internal/pm"
	"github.com/jwijenbergh/puregotk/v4/adw"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

// Page implements the Applications page.
type Page struct {
	toolbarView *adw.ToolbarView
	prefsPage   *adw.PreferencesPage

	// Flatpak UI elements
	flatpakUserExpander   *adw.ExpanderRow
	flatpakSystemExpander *adw.ExpanderRow

	// Snap UI elements
	snapExpander        *adw.ExpanderRow
	snapStoreLinkRow    *adw.ActionRow
	snapStoreInstallRow *adw.ActionRow
	snapRows            []*adw.ActionRow

	// Homebrew UI elements
	formulaeExpander      *adw.ExpanderRow
	casksExpander         *adw.ExpanderRow
	searchEntry           *gtk.SearchEntry
	searchResultsExpander *adw.ExpanderRow

	config    *config.Config
	toaster   pages.Toaster
	launchApp func(string)
	openURL   func(string)

	// Goroutine lifecycle management
	ctx    context.Context
	cancel context.CancelFunc
}

// New creates a new Applications page with the given dependencies.
func New(deps pages.Deps, launchApp, openURL func(string)) *Page {
	ctx, cancel := context.WithCancel(context.Background())

	p := &Page{
		config:    deps.Config,
		toaster:   deps.Toaster,
		launchApp: launchApp,
		openURL:   openURL,
		ctx:       ctx,
		cancel:    cancel,
	}

	p.buildUI()
	return p
}

// Widget returns the root widget for embedding.
func (p *Page) Widget() *adw.ToolbarView {
	return p.toolbarView
}

// Destroy cleans up resources and cancels running goroutines.
func (p *Page) Destroy() {
	if p.cancel != nil {
		p.cancel()
	}
}

func (p *Page) buildUI() {
	p.toolbarView = adw.NewToolbarView()

	headerBar := adw.NewHeaderBar()
	p.toolbarView.AddTopBar(&headerBar.Widget)

	scrolled := gtk.NewScrolledWindow()
	scrolled.SetPolicy(gtk.PolicyNeverValue, gtk.PolicyAutomaticValue)
	scrolled.SetVexpand(true)

	p.prefsPage = adw.NewPreferencesPage()
	scrolled.SetChild(&p.prefsPage.Widget)
	p.toolbarView.SetContent(&scrolled.Widget)

	// Build groups based on config and PM availability
	p.buildFlatpakGroups()
	p.buildSnapGroup()
	p.buildHomebrewGroup()
	p.buildSearchGroup()

	// Start async loads
	if pm.FlatpakIsInstalled() {
		go p.loadFlatpakApplications()
	}
	if pm.SnapIsInstalled() {
		go p.loadSnapApplications()
	}
	if pm.HomebrewIsInstalled() {
		go p.loadHomebrewPackages()
	}
}

func (p *Page) buildFlatpakGroups() {
	if !pm.FlatpakIsInstalled() {
		return
	}

	// User applications group
	if p.config.IsGroupEnabled("applications_page", "flatpak_user_group") {
		group := adw.NewPreferencesGroup()
		group.SetTitle("User Flatpak Applications")
		group.SetDescription("Flatpak applications installed for the current user")

		p.flatpakUserExpander = adw.NewExpanderRow()
		p.flatpakUserExpander.SetTitle("User Applications")
		p.flatpakUserExpander.SetSubtitle("Loading...")
		group.Add(&p.flatpakUserExpander.Widget)

		p.prefsPage.Add(group)
	}

	// System applications group
	if p.config.IsGroupEnabled("applications_page", "flatpak_system_group") {
		group := adw.NewPreferencesGroup()
		group.SetTitle("System Flatpak Applications")
		group.SetDescription("Flatpak applications installed system-wide")

		p.flatpakSystemExpander = adw.NewExpanderRow()
		p.flatpakSystemExpander.SetTitle("System Applications")
		p.flatpakSystemExpander.SetSubtitle("Loading...")
		group.Add(&p.flatpakSystemExpander.Widget)

		p.prefsPage.Add(group)
	}
}

func (p *Page) buildSnapGroup() {
	if !pm.SnapIsInstalled() {
		return
	}
	if !p.config.IsGroupEnabled("applications_page", "snap_group") {
		return
	}

	group := adw.NewPreferencesGroup()
	group.SetTitle("Snap Applications")
	group.SetDescription("Manage Snap packages installed on your system")

	// Snap Store link row - shown when snap-store is installed
	p.snapStoreLinkRow = adw.NewActionRow()
	p.snapStoreLinkRow.SetTitle("Manage Snaps")
	p.snapStoreLinkRow.SetSubtitle("Open the Snap Store to install and manage applications")
	p.snapStoreLinkRow.SetActivatable(true)
	p.snapStoreLinkRow.SetVisible(false)

	linkIcon := gtk.NewImageFromIconName("adw-external-link-symbolic")
	p.snapStoreLinkRow.AddSuffix(&linkIcon.Widget)

	linkActivatedCb := func(row adw.ActionRow) {
		p.launchApp("snap-store_snap-store")
	}
	p.snapStoreLinkRow.ConnectActivated(&linkActivatedCb)
	group.Add(&p.snapStoreLinkRow.Widget)

	// Snap Store install row - shown when snap-store is NOT installed
	p.snapStoreInstallRow = adw.NewActionRow()
	p.snapStoreInstallRow.SetTitle("Snap Store")
	p.snapStoreInstallRow.SetSubtitle("Install the Snap Store for a graphical package manager")
	p.snapStoreInstallRow.SetVisible(false)

	storeIcon := gtk.NewImageFromIconName("system-software-install-symbolic")
	p.snapStoreInstallRow.AddPrefix(&storeIcon.Widget)

	installBtn := gtk.NewButtonWithLabel("Install")
	installBtn.SetValign(gtk.AlignCenterValue)
	installBtn.AddCssClass("suggested-action")
	installClickedCb := func(btn gtk.Button) {
		p.onInstallSnapStoreClicked(installBtn)
	}
	installBtn.ConnectClicked(&installClickedCb)
	p.snapStoreInstallRow.AddSuffix(&installBtn.Widget)
	group.Add(&p.snapStoreInstallRow.Widget)

	p.snapExpander = adw.NewExpanderRow()
	p.snapExpander.SetTitle("Installed Snaps")
	p.snapExpander.SetSubtitle("Loading...")
	group.Add(&p.snapExpander.Widget)

	p.prefsPage.Add(group)
}

func (p *Page) buildHomebrewGroup() {
	if !pm.HomebrewIsInstalled() {
		return
	}
	if !p.config.IsGroupEnabled("applications_page", "brew_group") {
		return
	}

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
		p.onBrewBundleDumpClicked()
	}
	dumpBtn.ConnectClicked(&dumpClickedCb)

	dumpRow.AddSuffix(&dumpBtn.Widget)
	group.Add(&dumpRow.Widget)

	// Formulae expander
	p.formulaeExpander = adw.NewExpanderRow()
	p.formulaeExpander.SetTitle("Formulae")
	p.formulaeExpander.SetSubtitle("Loading...")
	group.Add(&p.formulaeExpander.Widget)

	// Casks expander
	p.casksExpander = adw.NewExpanderRow()
	p.casksExpander.SetTitle("Casks")
	p.casksExpander.SetSubtitle("Loading...")
	group.Add(&p.casksExpander.Widget)

	p.prefsPage.Add(group)
}

func (p *Page) buildSearchGroup() {
	if !HasSearchCapability() {
		return
	}
	if !p.config.IsGroupEnabled("applications_page", "brew_search_group") {
		return
	}

	group := adw.NewPreferencesGroup()
	group.SetTitle("Search Homebrew")
	group.SetDescription("Search for and install Homebrew formulae")

	// Search entry row
	searchRow := adw.NewActionRow()
	searchRow.SetTitle("Search for packages")

	p.searchEntry = gtk.NewSearchEntry()
	p.searchEntry.SetHexpand(true)

	searchActivateCb := func(entry gtk.SearchEntry) {
		p.onHomebrewSearch()
	}
	p.searchEntry.ConnectActivate(&searchActivateCb)

	searchRow.AddSuffix(&p.searchEntry.Widget)
	group.Add(&searchRow.Widget)

	// Search results expander
	p.searchResultsExpander = adw.NewExpanderRow()
	p.searchResultsExpander.SetTitle("Search Results")
	p.searchResultsExpander.SetSubtitle("No search performed")
	p.searchResultsExpander.SetEnableExpansion(false)
	group.Add(&p.searchResultsExpander.Widget)

	p.prefsPage.Add(group)
}

// loadFlatpakApplications loads installed Flatpak applications asynchronously.
func (p *Page) loadFlatpakApplications() {
	if !pm.FlatpakIsInstalled() {
		async.RunOnMain(func() {
			select {
			case <-p.ctx.Done():
				return
			default:
			}
			if p.flatpakUserExpander != nil {
				p.flatpakUserExpander.SetSubtitle("Flatpak not installed")
			}
			if p.flatpakSystemExpander != nil {
				p.flatpakSystemExpander.SetSubtitle("Flatpak not installed")
			}
		})
		return
	}

	// Load all applications (user and system combined via pm library)
	apps, err := pm.ListFlatpakApplications()
	if err != nil {
		async.RunOnMain(func() {
			select {
			case <-p.ctx.Done():
				return
			default:
			}
			if p.flatpakUserExpander != nil {
				p.flatpakUserExpander.SetSubtitle(fmt.Sprintf("Error: %v", err))
			}
			if p.flatpakSystemExpander != nil {
				p.flatpakSystemExpander.SetSubtitle(fmt.Sprintf("Error: %v", err))
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
	if p.flatpakUserExpander != nil {
		async.RunOnMain(func() {
			select {
			case <-p.ctx.Done():
				return
			default:
			}
			p.flatpakUserExpander.SetSubtitle(fmt.Sprintf("%d installed", len(userApps)))
			for _, app := range userApps {
				app := app // Capture loop variable
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
								select {
								case <-p.ctx.Done():
									return
								default:
								}
								btn.SetSensitive(true)
								p.toaster.ShowErrorToast(fmt.Sprintf("Uninstall failed: %v", err))
							})
							return
						}
						async.RunOnMain(func() {
							select {
							case <-p.ctx.Done():
								return
							default:
							}
							p.toaster.ShowToast(fmt.Sprintf("%s uninstalled", appID))
							// Refresh the list
							go p.loadFlatpakApplications()
						})
					}()
				}
				uninstallBtn.ConnectClicked(&clickedCb)

				row.AddSuffix(&uninstallBtn.Widget)
				p.flatpakUserExpander.AddRow(&row.Widget)
			}
		})
	}

	// Load system applications
	if p.flatpakSystemExpander != nil {
		async.RunOnMain(func() {
			select {
			case <-p.ctx.Done():
				return
			default:
			}
			p.flatpakSystemExpander.SetSubtitle(fmt.Sprintf("%d installed", len(systemApps)))
			for _, app := range systemApps {
				app := app // Capture loop variable
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
								select {
								case <-p.ctx.Done():
									return
								default:
								}
								btn.SetSensitive(true)
								p.toaster.ShowErrorToast(fmt.Sprintf("Uninstall failed: %v", err))
							})
							return
						}
						async.RunOnMain(func() {
							select {
							case <-p.ctx.Done():
								return
							default:
							}
							p.toaster.ShowToast(fmt.Sprintf("%s uninstalled", appID))
							// Refresh the list
							go p.loadFlatpakApplications()
						})
					}()
				}
				uninstallBtn.ConnectClicked(&clickedCb)

				row.AddSuffix(&uninstallBtn.Widget)
				p.flatpakSystemExpander.AddRow(&row.Widget)
			}
		})
	}
}

// loadSnapApplications loads installed snap packages asynchronously.
func (p *Page) loadSnapApplications() {
	if !pm.SnapIsInstalled() {
		async.RunOnMain(func() {
			select {
			case <-p.ctx.Done():
				return
			default:
			}
			if p.snapExpander != nil {
				p.snapExpander.SetSubtitle("Snap not installed")
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
			select {
			case <-p.ctx.Done():
				return
			default:
			}
			if p.snapExpander != nil {
				p.snapExpander.SetSubtitle(fmt.Sprintf("Error: %v", err))
			}
		})
		return
	}

	async.RunOnMain(func() {
		select {
		case <-p.ctx.Done():
			return
		default:
		}
		if p.snapExpander != nil {
			// Clear existing rows
			for _, row := range p.snapRows {
				p.snapExpander.Remove(&row.Widget)
			}
			p.snapRows = nil

			p.snapExpander.SetSubtitle(fmt.Sprintf("%d installed", len(snaps)))

			for _, s := range snaps {
				s := s // Capture loop variable
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

				p.snapExpander.AddRow(&row.Widget)
				p.snapRows = append(p.snapRows, row)
			}
		}

		// Show snap-store link row if installed, otherwise show install row
		if p.snapStoreLinkRow != nil {
			p.snapStoreLinkRow.SetVisible(snapStoreInstalled)
		}
		if p.snapStoreInstallRow != nil {
			p.snapStoreInstallRow.SetVisible(!snapStoreInstalled)
		}
	})
}

// onInstallSnapStoreClicked handles installing the snap-store snap.
func (p *Page) onInstallSnapStoreClicked(button *gtk.Button) {
	button.SetSensitive(false)
	button.SetLabel("Installing...")

	go func() {
		ctx, cancel := pm.SnapDefaultContext()
		defer cancel()

		changeID, err := pm.SnapInstall(ctx, "snap-store")
		if err != nil {
			async.RunOnMain(func() {
				select {
				case <-p.ctx.Done():
					return
				default:
				}
				button.SetSensitive(true)
				button.SetLabel("Install")
				p.toaster.ShowErrorToast(fmt.Sprintf("Failed to install snap-store: %v", err))
			})
			return
		}

		// Wait for the installation to complete
		err = pm.SnapWaitForChange(ctx, changeID)
		if err != nil {
			async.RunOnMain(func() {
				select {
				case <-p.ctx.Done():
					return
				default:
				}
				button.SetSensitive(true)
				button.SetLabel("Install")
				p.toaster.ShowErrorToast(fmt.Sprintf("Installation failed: %v", err))
			})
			return
		}

		async.RunOnMain(func() {
			select {
			case <-p.ctx.Done():
				return
			default:
			}
			button.SetSensitive(true)
			button.SetLabel("Install")
			p.toaster.ShowToast("Snap Store installed successfully!")
		})

		// Reload the snap list to update the UI
		p.loadSnapApplications()
	}()
}

// loadHomebrewPackages loads installed Homebrew packages asynchronously.
func (p *Page) loadHomebrewPackages() {
	if !pm.HomebrewIsInstalled() {
		async.RunOnMain(func() {
			select {
			case <-p.ctx.Done():
				return
			default:
			}
			if p.formulaeExpander != nil {
				p.formulaeExpander.SetSubtitle("Homebrew not installed")
			}
			if p.casksExpander != nil {
				p.casksExpander.SetSubtitle("Homebrew not installed")
			}
		})
		return
	}

	// Load formulae
	formulae, err := pm.ListHomebrewFormulae()
	if err != nil {
		async.RunOnMain(func() {
			select {
			case <-p.ctx.Done():
				return
			default:
			}
			if p.formulaeExpander != nil {
				p.formulaeExpander.SetSubtitle(fmt.Sprintf("Error: %v", err))
			}
		})
	} else {
		async.RunOnMain(func() {
			select {
			case <-p.ctx.Done():
				return
			default:
			}
			if p.formulaeExpander != nil {
				p.formulaeExpander.SetSubtitle(fmt.Sprintf("%d installed", len(formulae)))
				for _, pkg := range formulae {
					row := adw.NewActionRow()
					row.SetTitle(pkg.Name)
					row.SetSubtitle(pkg.Version)
					p.formulaeExpander.AddRow(&row.Widget)
				}
			}
		})
	}

	// Load casks
	casks, err := pm.ListHomebrewCasks()
	if err != nil {
		async.RunOnMain(func() {
			select {
			case <-p.ctx.Done():
				return
			default:
			}
			if p.casksExpander != nil {
				p.casksExpander.SetSubtitle(fmt.Sprintf("Error: %v", err))
			}
		})
	} else {
		async.RunOnMain(func() {
			select {
			case <-p.ctx.Done():
				return
			default:
			}
			if p.casksExpander != nil {
				p.casksExpander.SetSubtitle(fmt.Sprintf("%d installed", len(casks)))
				for _, pkg := range casks {
					row := adw.NewActionRow()
					row.SetTitle(pkg.Name)
					row.SetSubtitle(pkg.Version)
					p.casksExpander.AddRow(&row.Widget)
				}
			}
		})
	}
}

// onBrewBundleDumpClicked handles the Brew Bundle Dump button.
func (p *Page) onBrewBundleDumpClicked() {
	go func() {
		homeDir, _ := os.UserHomeDir()
		path := homeDir + "/Brewfile"
		if err := pm.HomebrewBundleDump(path, true); err != nil {
			async.RunOnMain(func() {
				select {
				case <-p.ctx.Done():
					return
				default:
				}
				p.toaster.ShowErrorToast(fmt.Sprintf("Bundle dump failed: %v", err))
			})
			return
		}
		async.RunOnMain(func() {
			select {
			case <-p.ctx.Done():
				return
			default:
			}
			p.toaster.ShowToast(fmt.Sprintf("Brewfile saved to %s", path))
		})
	}()
}

// onHomebrewSearch handles the Homebrew search entry activation.
func (p *Page) onHomebrewSearch() {
	query := p.searchEntry.GetText()
	if query == "" {
		return
	}

	p.searchResultsExpander.SetSubtitle("Searching...")
	p.searchResultsExpander.SetEnableExpansion(false)

	go func() {
		results, err := pm.HomebrewSearch(query)
		if err != nil {
			async.RunOnMain(func() {
				select {
				case <-p.ctx.Done():
					return
				default:
				}
				p.searchResultsExpander.SetSubtitle(fmt.Sprintf("Error: %v", err))
			})
			return
		}

		async.RunOnMain(func() {
			select {
			case <-p.ctx.Done():
				return
			default:
			}
			p.searchResultsExpander.SetSubtitle(fmt.Sprintf("%d results", len(results)))
			p.searchResultsExpander.SetEnableExpansion(len(results) > 0)

			// Add result rows
			for _, result := range results {
				result := result // Capture loop variable
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
								select {
								case <-p.ctx.Done():
									return
								default:
								}
								btn.SetSensitive(true)
								btn.SetLabel("Install")
								userErr := async.NewUserErrorWithHint(
									fmt.Sprintf("Couldn't install %s", pkgName),
									"Check your internet connection and try again",
									err,
								)
								p.toaster.ShowErrorToast(userErr.FormatForUser())
							})
							return
						}
						log.Printf("Successfully installed %s", pkgName)
						async.RunOnMain(func() {
							select {
							case <-p.ctx.Done():
								return
							default:
							}
							btn.SetLabel("Installed")
							p.toaster.ShowToast(fmt.Sprintf("%s installed", pkgName))
						})
					}()
				}
				installBtn.ConnectClicked(&clickedCb)

				row.AddSuffix(&installBtn.Widget)
				p.searchResultsExpander.AddRow(&row.Widget)
			}
		})
	}()
}
