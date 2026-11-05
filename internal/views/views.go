// Package views provides the page content for the ChairLift application
package views

import (
	"log"
	"sync"
	"time"

	"github.com/frostyard/chairlift/internal/config"
	"github.com/frostyard/chairlift/internal/problems"
	"github.com/frostyard/chairlift/internal/reporting"
	"github.com/frostyard/chairlift/internal/views/actionstate"
	"github.com/frostyard/chairlift/internal/views/badgestate"
	"github.com/frostyard/chairlift/internal/views/rowset"

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
	problemsPage     *adw.ToolbarView
	helpPage         *adw.ToolbarView

	// PreferencesPages inside each ToolbarView - keep references to prevent GC
	systemPrefsPage       *adw.PreferencesPage
	updatesPrefsPage      *adw.PreferencesPage
	applicationsPrefsPage *adw.PreferencesPage
	maintenancePrefsPage  *adw.PreferencesPage
	featuresPrefsPage     *adw.PreferencesPage
	problemsPrefsPage     *adw.PreferencesPage
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
	flatpakUpdateRows      rowset.Tracker[*adw.ActionRow] // Store references for cleanup
	flatpakUserRows        rowset.Tracker[*adw.ActionRow] // Store references for cleanup
	flatpakSystemRows      rowset.Tracker[*adw.ActionRow] // Store references for cleanup
	formulaeRows           rowset.Tracker[*adw.ActionRow]
	caskRows               rowset.Tracker[*adw.ActionRow]
	searchResultRows       rowset.Tracker[*adw.ActionRow]
	brewBundlesGroup       *adw.PreferencesGroup
	brewTrustGroup         *adw.PreferencesGroup
	brewTrustRows          map[string]*adw.ActionRow
	outdatedRows           rowset.Tracker[*adw.ActionRow]

	// bootc update references
	bootcStageExpander *adw.ExpanderRow
	bootcStageBtn      *gtk.Button
	bootcActivityRow   *adw.ActionRow
	bootcLogExpander   *adw.ExpanderRow

	// native A/B (sysupdate) update references
	sysupdateStageExpander *adw.ExpanderRow
	sysupdateStageBtn      *gtk.Button
	sysupdateActivityRow   *adw.ActionRow
	sysupdateLogExpander   *adw.ExpanderRow
	sysupdateRollbackRow   *adw.ActionRow

	// firmware (fwupd) update references
	firmwareGroup    *adw.PreferencesGroup
	firmwareExpander *adw.ExpanderRow
	firmwareRows     rowset.Tracker[*adw.ActionRow]

	// Features page references
	featuresGroup            *adw.PreferencesGroup
	featuresUnavailableGroup *adw.PreferencesGroup
	featureRows              map[string]*adw.ActionRow
	// featureBaseSubtitles holds each row's subtitle before any update check
	// ran (pageview.Feature's presentation). The re-check gives it back to every
	// feature the check did not answer for, so a stale "update available" from
	// an earlier check cannot outlive the update that removed it.
	featureBaseSubtitles map[string]string
	// featureCheck is the generation gate of the feature update check, so an
	// older check that finishes late cannot replace a newer one's rows.
	featureCheck actionstate.RefreshGate

	// Problems page references (nil when the group is disabled)
	problemsSystemGroup *adw.PreferencesGroup
	problemsAppsGroup   *adw.PreferencesGroup

	// Problems reporting (ADR-0015, phase 2). The widgets live on the GTK
	// main thread; consent and reportingState are guarded by reportingMu,
	// because the send goroutine reads the consent before every request.
	reportingGroup    *adw.PreferencesGroup
	reportingSwitch   *adw.SwitchRow
	reportingMode     *adw.ComboRow
	reportingSent     *adw.ExpanderRow
	reportingCountRow *adw.ActionRow
	reportingSentRows []*gtk.Widget
	reportingMu       sync.Mutex
	reportingConsent  reporting.Consent
	// reportingEndpointErr is set when the configured report endpoint is not
	// a valid URL: every send path is off then.
	reportingEndpointErr error
	// reportingApplying is true while the page itself redraws the consent
	// widgets, so a redraw cannot start a second save. Main thread only.
	reportingApplying  bool
	reportingState     *reporting.State
	reportingRows      map[string]*reportRow
	reportingConsentAt string
	reportingStateAt   string
	reportingEndpoint  string
	reportingClient    string
	reportingItems     []problems.Item
	reportingEnvelope  problems.Envelope
	reportingRunning   bool

	// Groups with deferred visibility
	maintenanceBrewGroup    *adw.PreferencesGroup
	maintenanceFlatpakGroup *adw.PreferencesGroup

	// Update badge tracking
	updateCounts badgestate.Counts

	brewRefresh         actionstate.RefreshGate
	searchRefresh       actionstate.RefreshGate
	brewPackagesRefresh actionstate.RefreshGate
	firmwareRefresh     actionstate.RefreshGate
	// flatpakUpdatesRefresh bounds overlapping Flatpak update inventory
	// reloads. Every completed update kicks a reload, so two finishing close
	// together race; only the newest generation may publish rows and count.
	flatpakUpdatesRefresh actionstate.RefreshGate
	// firmwareInstallGate allows one firmware install at a time for the whole
	// page: two flashes at once would overwrite each other's device state and
	// make one confirmation dialog lie about what is happening.
	firmwareInstallGate actionstate.Gate
}

// New creates a new UserHome views manager
func New(cfg *config.Config, toastAdder ToastAdder) *UserHome {
	start := time.Now()

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
	uh.problemsPage, uh.problemsPrefsPage = uh.createPage()
	uh.helpPage, uh.helpPrefsPage = uh.createPage()

	// Build page content
	uh.buildSystemPage()
	uh.buildUpdatesPage()
	uh.buildApplicationsPage()
	uh.buildMaintenancePage()
	uh.buildFeaturesPage()
	uh.buildProblemsPage()
	uh.buildHelpPage()

	log.Printf("views: all pages built in %s", time.Since(start))

	return uh
}

// updateBadgeCount updates the total update count and notifies the window
func (uh *UserHome) updateBadgeCount() {
	total := uh.updateCounts.Total()

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
	case "problems":
		return uh.problemsPage
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
