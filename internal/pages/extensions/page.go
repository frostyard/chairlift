package extensions

import (
	"context"
	"fmt"
	"log"

	"github.com/frostyard/chairlift/internal/async"
	"github.com/frostyard/chairlift/internal/config"
	"github.com/frostyard/chairlift/internal/operations"
	"github.com/frostyard/chairlift/internal/pages"
	"github.com/jwijenbergh/puregotk/v4/adw"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

// Page implements the Extensions page.
type Page struct {
	toolbarView *adw.ToolbarView
	prefsPage   *adw.PreferencesPage

	// UI state
	extensionsGroup      *adw.PreferencesGroup
	discoverEntry        *gtk.Entry
	discoverResultsGroup *adw.PreferencesGroup
	discoverResultRows   []*adw.ActionRow

	// Data
	installedComponentsMap map[string]bool

	// Dependencies
	config  *config.Config
	toaster pages.Toaster
	client  *Client

	// Goroutine lifecycle management
	ctx    context.Context
	cancel context.CancelFunc
}

// New creates a new Extensions page with the given dependencies.
func New(deps pages.Deps) *Page {
	ctx, cancel := context.WithCancel(context.Background())

	p := &Page{
		config:                 deps.Config,
		toaster:                deps.Toaster,
		client:                 NewClient(),
		installedComponentsMap: make(map[string]bool),
		ctx:                    ctx,
		cancel:                 cancel,
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

	// Build groups based on config and availability
	p.buildInstalledGroup()
	p.buildDiscoverGroup()
}

func (p *Page) buildInstalledGroup() {
	// Check if extensions system is available using library
	if !IsAvailable() {
		// Show a message that extensions are not available
		group := adw.NewPreferencesGroup()
		group.SetTitle("Installed")
		group.SetDescription("Manage systemd-sysext extensions")

		row := adw.NewActionRow()
		row.SetTitle("Extension Manager Not Available")
		row.SetSubtitle("systemd-sysext is not installed on this system")
		group.Add(&row.Widget)
		p.prefsPage.Add(group)
		return
	}

	if !p.config.IsGroupEnabled("extensions_page", "installed_group") {
		return
	}

	p.extensionsGroup = adw.NewPreferencesGroup()
	p.extensionsGroup.SetTitle("Installed")
	p.extensionsGroup.SetDescription("Loading extensions...")

	p.prefsPage.Add(p.extensionsGroup)

	// Load extensions asynchronously
	go p.loadExtensions()
}

func (p *Page) loadExtensions() {
	extensions, err := p.client.ListInstalled(p.ctx)

	// Check if page was destroyed while fetching
	select {
	case <-p.ctx.Done():
		return
	default:
	}

	async.RunOnMain(func() {
		// Double-check before UI update
		if p.ctx.Err() != nil {
			return
		}

		if p.extensionsGroup == nil {
			return
		}

		if err != nil {
			p.extensionsGroup.SetDescription(fmt.Sprintf("Error: %v", err))
			return
		}

		if len(extensions) == 0 {
			p.extensionsGroup.SetDescription("No extensions installed")
			return
		}

		// Group extensions by component and populate installed cache
		componentMap := make(map[string][]ExtensionInfo)
		for _, ext := range extensions {
			componentMap[ext.Component] = append(componentMap[ext.Component], ext)
			p.installedComponentsMap[ext.Component] = true
		}

		p.extensionsGroup.SetDescription(fmt.Sprintf("%d components installed", len(componentMap)))

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

			p.extensionsGroup.Add(&expander.Widget)
		}
	})
}

func (p *Page) buildDiscoverGroup() {
	// Only show if discover is available (via updex SDK which needs systemd-sysext)
	if !p.client.IsDiscoverAvailable() {
		return
	}

	if !p.config.IsGroupEnabled("extensions_page", "discover_group") {
		return
	}

	group := adw.NewPreferencesGroup()
	group.SetTitle("Discover")
	group.SetDescription("Find and install extensions from remote repositories")

	// URL entry row
	entryRow := adw.NewActionRow()
	entryRow.SetTitle("Repository URL")

	p.discoverEntry = gtk.NewEntry()
	p.discoverEntry.SetText("https://repository.frostyard.org")
	p.discoverEntry.SetHexpand(true)
	p.discoverEntry.SetValign(gtk.AlignCenterValue)
	entryRow.AddSuffix(&p.discoverEntry.Widget)

	discoverBtn := gtk.NewButtonWithLabel("Discover")
	discoverBtn.SetValign(gtk.AlignCenterValue)
	discoverBtn.AddCssClass("suggested-action")

	clickedCb := func(btn gtk.Button) {
		p.onDiscoverClicked(discoverBtn)
	}
	discoverBtn.ConnectClicked(&clickedCb)
	entryRow.AddSuffix(&discoverBtn.Widget)

	group.Add(&entryRow.Widget)
	p.prefsPage.Add(group)

	// Results group (initially hidden, will be populated after discovery)
	p.discoverResultsGroup = adw.NewPreferencesGroup()
	p.discoverResultsGroup.SetTitle("Available Extensions")
	p.discoverResultsGroup.SetVisible(false)
	p.prefsPage.Add(p.discoverResultsGroup)
}

func (p *Page) onDiscoverClicked(button *gtk.Button) {
	if p.discoverEntry == nil {
		return
	}

	url := p.discoverEntry.GetText()
	if url == "" {
		p.toaster.ShowErrorToast("Please enter a repository URL")
		return
	}

	button.SetSensitive(false)
	button.SetLabel("Discovering...")

	go func() {
		result, err := p.client.Discover(p.ctx, url)

		// Check if page was destroyed
		select {
		case <-p.ctx.Done():
			return
		default:
		}

		async.RunOnMain(func() {
			if p.ctx.Err() != nil {
				return
			}

			button.SetSensitive(true)
			button.SetLabel("Discover")

			if err != nil {
				p.toaster.ShowErrorToast(fmt.Sprintf("Discovery failed: %v", err))
				return
			}

			p.displayDiscoveryResults(url, result)
		})
	}()
}

func (p *Page) displayDiscoveryResults(repoURL string, result []DiscoveredExtension) {
	if p.discoverResultsGroup == nil {
		return
	}

	// Clear existing result rows
	for _, row := range p.discoverResultRows {
		p.discoverResultsGroup.Remove(&row.Widget)
	}
	p.discoverResultRows = nil

	p.discoverResultsGroup.SetVisible(true)

	if len(result) == 0 {
		p.discoverResultsGroup.SetDescription("No extensions found in repository")
		return
	}

	p.discoverResultsGroup.SetDescription(fmt.Sprintf("%d extensions available", len(result)))

	for _, ext := range result {
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
		if p.installedComponentsMap[ext.Name] {
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
				p.onInstallExtensionClicked(installBtn, url, extName)
			}
			installBtn.ConnectClicked(&installClickedCb)
			row.AddSuffix(&installBtn.Widget)
		}

		p.discoverResultsGroup.Add(&row.Widget)
		p.discoverResultRows = append(p.discoverResultRows, row)
	}
}

func (p *Page) onInstallExtensionClicked(button *gtk.Button, repoURL, component string) {
	button.SetSensitive(false)
	button.SetLabel("Installing...")

	// Track via operations with retry support
	op := operations.Start(fmt.Sprintf("Install %s", component), operations.CategoryInstall, false)
	op.RetryFunc = func() {
		p.onInstallExtensionClicked(button, repoURL, component)
	}

	go func() {
		err := p.client.Install(p.ctx, repoURL, component)

		// Check if page was destroyed
		select {
		case <-p.ctx.Done():
			op.Complete(nil) // Mark as complete to avoid stuck operations
			return
		default:
		}

		async.RunOnMain(func() {
			if p.ctx.Err() != nil {
				op.Complete(nil)
				return
			}

			op.Complete(err)

			if err != nil {
				button.SetSensitive(true)
				button.SetLabel("Install")
				userErr := async.NewUserErrorWithHint(
					fmt.Sprintf("Couldn't install %s", component),
					"Check the repository URL and try again",
					err,
				)
				p.toaster.ShowErrorToast(userErr.FormatForUser())
				log.Printf("Extension install error details: %v", err)
				return
			}

			// Update button to show installed
			button.SetLabel("Installed")
			button.RemoveCssClass("suggested-action")
			button.AddCssClass("dim-label")

			// Update installed components cache
			p.installedComponentsMap[component] = true

			p.toaster.ShowToast(fmt.Sprintf("Installed %s successfully", component))

			// Reload the installed extensions list
			go p.loadExtensions()
		})
	}()
}
