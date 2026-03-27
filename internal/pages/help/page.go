package help

import (
	"github.com/frostyard/chairlift/internal/config"
	"github.com/frostyard/chairlift/internal/pages"
	"github.com/frostyard/chairlift/internal/widgets"
	"github.com/jwijenbergh/puregotk/v4/adw"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

// Page implements the Help page.
type Page struct {
	toolbarView *adw.ToolbarView
	prefsPage   *adw.PreferencesPage

	config  *config.Config
	toaster pages.Toaster
	openURL func(string)
}

// New creates a new Help page with the given dependencies.
//
// Parameters:
//   - deps: Common page dependencies (config, toaster)
//   - openURL: Callback function to open URLs (e.g., via xdg-open)
//
// Must be called from the GTK main thread.
func New(deps pages.Deps, openURL func(string)) *Page {
	p := &Page{
		config:  deps.Config,
		toaster: deps.Toaster,
		openURL: openURL,
	}

	p.buildUI()
	return p
}

// Widget returns the root widget for embedding in navigation.
func (p *Page) Widget() *adw.ToolbarView {
	return p.toolbarView
}

// Destroy cleans up resources. No-op for Help page (no goroutines to cancel).
func (p *Page) Destroy() {}

// buildUI creates the page UI structure.
func (p *Page) buildUI() {
	p.toolbarView = adw.NewToolbarView()

	// Add header bar
	headerBar := adw.NewHeaderBar()
	p.toolbarView.AddTopBar(&headerBar.Widget)

	// Create scrolled window with preferences page
	scrolled := gtk.NewScrolledWindow()
	scrolled.SetPolicy(gtk.PolicyNeverValue, gtk.PolicyAutomaticValue)
	scrolled.SetVexpand(true)

	p.prefsPage = adw.NewPreferencesPage()
	scrolled.SetChild(&p.prefsPage.Widget)

	p.toolbarView.SetContent(&scrolled.Widget)

	// Build page content
	p.buildResourcesGroup()
}

// buildResourcesGroup creates the Help & Resources group with link rows.
func (p *Page) buildResourcesGroup() {
	if !p.config.IsGroupEnabled("help_page", "help_resources_group") {
		return
	}

	groupCfg := p.config.GetGroupConfig("help_page", "help_resources_group")
	if groupCfg == nil {
		return
	}

	group := adw.NewPreferencesGroup()
	group.SetTitle("Help &amp; Resources")
	group.SetDescription("Get help and learn more about ChairLift")

	// Build resource links using business logic layer
	links := BuildResourceLinks(groupCfg)
	for _, link := range links {
		// IMPORTANT: Capture link.URL in local variable to avoid closure bug
		url := link.URL
		row := widgets.NewLinkRow(link.Title, link.Subtitle, func() {
			p.openURL(url)
		})
		group.Add(&row.Widget)
	}

	p.prefsPage.Add(group)
}
