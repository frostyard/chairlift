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

// Page implements the Applications page with sidebar navigation.
type Page struct {
	toolbarView  *adw.ToolbarView
	splitView    *adw.NavigationSplitView
	sidebarList  *gtk.ListBox
	contentStack *gtk.Stack

	currentCategory PMCategory

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

	// Create NavigationSplitView for sidebar layout
	p.splitView = adw.NewNavigationSplitView()
	p.splitView.SetSidebarWidthFraction(0.25)
	p.splitView.SetMinSidebarWidth(180)
	p.splitView.SetMaxSidebarWidth(280)

	// Build sidebar
	sidebarPage := p.buildSidebar()
	p.splitView.SetSidebar(sidebarPage)

	// Build content area with real content
	contentPage := p.buildContent()
	p.splitView.SetContent(contentPage)

	p.toolbarView.SetContent(&p.splitView.Widget)

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

func (p *Page) buildSidebar() *adw.NavigationPage {
	toolbarView := adw.NewToolbarView()

	headerBar := adw.NewHeaderBar()
	headerBar.SetShowEndTitleButtons(false)
	toolbarView.AddTopBar(&headerBar.Widget)

	scrolled := gtk.NewScrolledWindow()
	scrolled.SetPolicy(gtk.PolicyNeverValue, gtk.PolicyAutomaticValue)
	scrolled.SetVexpand(true)

	p.sidebarList = gtk.NewListBox()
	p.sidebarList.SetSelectionMode(gtk.SelectionSingleValue)
	p.sidebarList.AddCssClass("navigation-sidebar")

	// Add sidebar items
	items := GetSidebarItems()
	for _, item := range items {
		p.addSidebarRow(item)
	}

	// Connect row-activated signal
	rowActivatedCb := func(listbox gtk.ListBox, rowPtr uintptr) {
		row := gtk.ListBoxRowNewFromInternalPtr(rowPtr)
		p.onSidebarRowActivated(*row)
	}
	p.sidebarList.ConnectRowActivated(&rowActivatedCb)

	// Select first row by default
	if first := p.sidebarList.GetRowAtIndex(0); first != nil {
		p.sidebarList.SelectRow(first)
	}

	scrolled.SetChild(&p.sidebarList.Widget)
	toolbarView.SetContent(&scrolled.Widget)

	return adw.NewNavigationPage(&toolbarView.Widget, "Categories")
}

func (p *Page) addSidebarRow(item SidebarItem) {
	row := gtk.NewListBoxRow()

	box := gtk.NewBox(gtk.OrientationHorizontalValue, 12)
	box.SetMarginTop(8)
	box.SetMarginBottom(8)
	box.SetMarginStart(12)
	box.SetMarginEnd(12)

	icon := gtk.NewImageFromIconName(item.IconName)
	box.Append(&icon.Widget)

	label := gtk.NewLabel(item.Label)
	label.SetHexpand(true)
	label.SetXalign(0)
	box.Append(&label.Widget)

	// Show "Not installed" indicator if PM unavailable
	if !item.IsInstalled && item.Category != CategoryAll {
		dimLabel := gtk.NewLabel("Not installed")
		dimLabel.AddCssClass("dim-label")
		box.Append(&dimLabel.Widget)
		row.SetSensitive(false)
	}

	row.SetChild(&box.Widget)
	// Store category in row name for lookup
	row.SetName(string(item.Category))

	p.sidebarList.Append(&row.Widget)
}

func (p *Page) buildContent() *adw.NavigationPage {
	toolbarView := adw.NewToolbarView()

	headerBar := adw.NewHeaderBar()
	toolbarView.AddTopBar(&headerBar.Widget)

	scrolled := gtk.NewScrolledWindow()
	scrolled.SetPolicy(gtk.PolicyNeverValue, gtk.PolicyAutomaticValue)
	scrolled.SetVexpand(true)

	// Stack to hold different category views
	p.contentStack = gtk.NewStack()
	p.contentStack.SetTransitionType(gtk.StackTransitionTypeCrossfadeValue)

	// Build content pages for each category
	p.contentStack.AddNamed(&p.buildAllContent().Widget, string(CategoryAll))
	p.contentStack.AddNamed(&p.buildFlatpakContent().Widget, string(CategoryFlatpak))
	p.contentStack.AddNamed(&p.buildHomebrewContent().Widget, string(CategoryHomebrew))
	p.contentStack.AddNamed(&p.buildSnapContent().Widget, string(CategorySnap))

	// Show "all" by default
	p.contentStack.SetVisibleChildName(string(CategoryAll))
	p.currentCategory = CategoryAll

	scrolled.SetChild(&p.contentStack.Widget)
	toolbarView.SetContent(&scrolled.Widget)

	return adw.NewNavigationPage(&toolbarView.Widget, "Applications")
}

func (p *Page) buildAllContent() *adw.PreferencesPage {
	page := adw.NewPreferencesPage()

	// Add flatpak groups if available
	if pm.FlatpakIsInstalled() && p.config.IsGroupEnabled("applications_page", "flatpak_user_group") {
		p.addFlatpakGroupsToPage(page)
	}

	// Add snap group if available
	if pm.SnapIsInstalled() && p.config.IsGroupEnabled("applications_page", "snap_group") {
		p.addSnapGroupToPage(page)
	}

	// Add homebrew group if available
	if pm.HomebrewIsInstalled() && p.config.IsGroupEnabled("applications_page", "brew_group") {
		p.addHomebrewGroupToPage(page)
	}

	// Add search at the bottom
	if HasSearchCapability() && p.config.IsGroupEnabled("applications_page", "brew_search_group") {
		p.addSearchGroupToPage(page)
	}

	return page
}

func (p *Page) buildFlatpakContent() *adw.PreferencesPage {
	page := adw.NewPreferencesPage()

	if !pm.FlatpakIsInstalled() {
		group := adw.NewPreferencesGroup()
		group.SetTitle("Flatpak")
		group.SetDescription("Flatpak is not installed on this system")
		page.Add(group)
		return page
	}

	p.addFlatpakGroupsToPage(page)
	return page
}

func (p *Page) buildSnapContent() *adw.PreferencesPage {
	page := adw.NewPreferencesPage()

	if !pm.SnapIsInstalled() {
		group := adw.NewPreferencesGroup()
		group.SetTitle("Snap")
		group.SetDescription("Snap is not installed on this system")
		page.Add(group)
		return page
	}

	p.addSnapGroupToPage(page)
	return page
}

func (p *Page) buildHomebrewContent() *adw.PreferencesPage {
	page := adw.NewPreferencesPage()

	if !pm.HomebrewIsInstalled() {
		group := adw.NewPreferencesGroup()
		group.SetTitle("Homebrew")
		group.SetDescription("Homebrew is not installed on this system")
		page.Add(group)
		return page
	}

	p.addHomebrewGroupToPage(page)
	p.addSearchGroupToPage(page)
	return page
}

func (p *Page) addFlatpakGroupsToPage(page *adw.PreferencesPage) {
	// User applications group
	if p.config.IsGroupEnabled("applications_page", "flatpak_user_group") {
		group := adw.NewPreferencesGroup()
		group.SetTitle("User Flatpak Applications")
		group.SetDescription("Flatpak applications installed for the current user")

		p.flatpakUserExpander = adw.NewExpanderRow()
		p.flatpakUserExpander.SetTitle("User Applications")
		p.flatpakUserExpander.SetSubtitle("Loading...")
		group.Add(&p.flatpakUserExpander.Widget)

		page.Add(group)
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

		page.Add(group)
	}
}

func (p *Page) addSnapGroupToPage(page *adw.PreferencesPage) {
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

	page.Add(group)
}

func (p *Page) addHomebrewGroupToPage(page *adw.PreferencesPage) {
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

	page.Add(group)
}

func (p *Page) addSearchGroupToPage(page *adw.PreferencesPage) {
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

	page.Add(group)
}

func (p *Page) onSidebarRowActivated(row gtk.ListBoxRow) {
	category := PMCategory(row.GetName())
	if category == p.currentCategory {
		return
	}
	p.currentCategory = category
	p.contentStack.SetVisibleChildName(string(category))
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
