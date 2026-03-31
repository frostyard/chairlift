// Package views provides the page content for the ChairLift application
package views

import (
	"sync"

	"github.com/frostyard/chairlift/internal/config"

	sgtk "github.com/frostyard/snowkit/gtk"

	"codeberg.org/puregotk/puregotk/v4/adw"
	"codeberg.org/puregotk/puregotk/v4/gtk"
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

	// Pages (ToolbarViews)
	systemPage       *adw.ToolbarView
	updatesPage      *adw.ToolbarView
	applicationsPage *adw.ToolbarView
	maintenancePage  *adw.ToolbarView
	featuresPage     *adw.ToolbarView
	helpPage         *adw.ToolbarView

	// PreferencesPages inside each ToolbarView - keep references to prevent GC
	systemPrefsPage       *adw.PreferencesPage
	updatesPrefsPage      *adw.PreferencesPage
	applicationsPrefsPage *adw.PreferencesPage
	maintenancePrefsPage  *adw.PreferencesPage
	featuresPrefsPage     *adw.PreferencesPage
	helpPrefsPage         *adw.PreferencesPage

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
	searchResultRows       []*adw.ActionRow // Store references for cleanup

	// NBC update references
	nbcUpdateBtn      *gtk.Button
	nbcDownloadBtn    *gtk.Button
	nbcUpdateExpander *adw.ExpanderRow
	nbcCheckRow       *adw.ActionRow

	// Features page references
	featuresGroup            *adw.PreferencesGroup
	featuresUnavailableGroup *adw.PreferencesGroup
	featureRows              map[string]*adw.ActionRow

	// Groups with deferred visibility
	snapGroup               *adw.PreferencesGroup
	maintenanceBrewGroup    *adw.PreferencesGroup
	maintenanceFlatpakGroup *adw.PreferencesGroup

	// Update badge tracking
	nbcUpdateCount     int
	flatpakUpdateCount int
	brewUpdateCount    int
	updateCountMu      sync.Mutex
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
	uh.featuresPage, uh.featuresPrefsPage = uh.createPage()
	uh.helpPage, uh.helpPrefsPage = uh.createPage()

	// Build page content
	uh.buildSystemPage()
	uh.buildUpdatesPage()
	uh.buildApplicationsPage()
	uh.buildMaintenancePage()
	uh.buildFeaturesPage()
	uh.buildHelpPage()

	return uh
}

// updateBadgeCount updates the total update count and notifies the window
func (uh *UserHome) updateBadgeCount() {
	uh.updateCountMu.Lock()
	total := uh.nbcUpdateCount + uh.flatpakUpdateCount + uh.brewUpdateCount
	uh.updateCountMu.Unlock()

	sgtk.RunOnMainThread(func() {
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
		return uh.maintenancePage
	case "features":
		return uh.featuresPage
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
