package system

import (
	"context"
	"fmt"

	"github.com/frostyard/chairlift/internal/async"
	"github.com/frostyard/chairlift/internal/config"
	"github.com/frostyard/chairlift/internal/nbc"
	"github.com/frostyard/chairlift/internal/pages"
	"github.com/frostyard/chairlift/internal/widgets"
	"github.com/jwijenbergh/puregotk/v4/adw"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

// Page implements the System page.
type Page struct {
	toolbarView *adw.ToolbarView
	prefsPage   *adw.PreferencesPage

	config    *config.Config
	toaster   pages.Toaster
	launchApp func(string) // Callback for launching apps
	openURL   func(string) // Callback for opening URLs

	// Goroutine lifecycle management
	ctx    context.Context
	cancel context.CancelFunc
}

// New creates a new System page with the given dependencies.
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

	// Build groups based on config
	p.buildSystemInfoGroup()
	p.buildNBCStatusGroup()
	p.buildSystemHealthGroup()
}

func (p *Page) buildSystemInfoGroup() {
	if !p.config.IsGroupEnabled("system_page", "system_info_group") {
		return
	}

	group := adw.NewPreferencesGroup()
	group.SetTitle("System Information")
	group.SetDescription("View system details and hardware information")

	expander := adw.NewExpanderRow()
	expander.SetTitle("Operating System Details")

	p.loadOSRelease(expander)

	group.Add(&expander.Widget)
	p.prefsPage.Add(group)
}

func (p *Page) loadOSRelease(expander *adw.ExpanderRow) {
	entries, err := ParseOSRelease()
	if err != nil {
		row := adw.NewActionRow()
		row.SetTitle("OS Information")
		row.SetSubtitle("Not available")
		expander.AddRow(&row.Widget)
		return
	}

	for _, entry := range entries {
		row := adw.NewActionRow()
		row.SetTitle(entry.DisplayKey)
		row.SetSubtitle(entry.Value)

		// Make URL rows clickable
		if entry.IsURL {
			row.SetActivatable(true)
			icon := gtk.NewImageFromIconName("adw-external-link-symbolic")
			row.AddSuffix(&icon.Widget)

			// IMPORTANT: Capture URL in local variable before closure
			url := entry.Value
			activatedCb := func(_ adw.ActionRow) {
				p.openURL(url)
			}
			row.ConnectActivated(&activatedCb)
		}

		expander.AddRow(&row.Widget)
	}
}

func (p *Page) buildNBCStatusGroup() {
	if !IsNBCAvailable() {
		return
	}
	if !p.config.IsGroupEnabled("system_page", "nbc_status_group") {
		return
	}

	group := adw.NewPreferencesGroup()
	group.SetTitle("NBC Status")
	group.SetDescription("View NBC system status information")

	expander := widgets.NewAsyncExpanderRow("NBC Status Details", "Loading...")

	group.Add(&expander.Expander.Widget)
	p.prefsPage.Add(group)

	// Start async load
	p.loadNBCStatus(expander)
}

func (p *Page) loadNBCStatus(expander *widgets.AsyncExpanderRow) {
	expander.StartLoading("Fetching NBC status")

	go func() {
		// Use page context - will be cancelled when Destroy() is called
		ctx, cancel := context.WithTimeout(p.ctx, nbc.DefaultTimeout)
		defer cancel()

		status, err := FetchNBCStatus(ctx)

		// Check if page was destroyed while we were fetching
		select {
		case <-p.ctx.Done():
			return // Page destroyed, don't update UI
		default:
		}

		async.RunOnMain(func() {
			// Double-check before UI update
			if p.ctx.Err() != nil {
				return // Page destroyed
			}

			if err != nil {
				expander.SetError(fmt.Sprintf("Failed to load NBC status: %v", err))
				return
			}

			expander.SetContent("Loaded")

			// Image
			if status.Image != "" {
				row := widgets.NewInfoRow("Image", status.Image)
				expander.Expander.AddRow(&row.Widget)
			}

			// Digest (shortened)
			if status.Digest != "" {
				digest := status.Digest
				if len(digest) > 19 {
					digest = digest[:19] + "..."
				}
				row := widgets.NewInfoRow("Digest", digest)
				expander.Expander.AddRow(&row.Widget)
			}

			// Device
			if status.Device != "" {
				row := widgets.NewInfoRow("Device", status.Device)
				expander.Expander.AddRow(&row.Widget)
			}

			// Active Slot
			if status.ActiveSlot != "" {
				row := widgets.NewInfoRow("Active Slot", status.ActiveSlot)
				expander.Expander.AddRow(&row.Widget)
			}

			// Filesystem Type
			if status.FilesystemType != "" {
				row := widgets.NewInfoRow("Filesystem", status.FilesystemType)
				expander.Expander.AddRow(&row.Widget)
			}

			// Root Mount Mode
			if status.RootMountMode != "" {
				row := widgets.NewInfoRow("Root Mount", status.RootMountMode)
				expander.Expander.AddRow(&row.Widget)
			}
		})
	}()
}

func (p *Page) buildSystemHealthGroup() {
	if !p.config.IsGroupEnabled("system_page", "health_group") {
		return
	}

	group := adw.NewPreferencesGroup()
	group.SetTitle("System Health")
	group.SetDescription("Overview of system health and diagnostics")

	groupCfg := p.config.GetGroupConfig("system_page", "health_group")
	appID := "io.missioncenter.MissionCenter"
	if groupCfg != nil && groupCfg.AppID != "" {
		appID = groupCfg.AppID
	}

	// Capture appID for closure
	appToLaunch := appID
	row := widgets.NewLinkRow(
		"System Performance",
		"Monitor CPU, memory, and system resources",
		func() { p.launchApp(appToLaunch) },
	)

	group.Add(&row.Widget)
	p.prefsPage.Add(group)
}
