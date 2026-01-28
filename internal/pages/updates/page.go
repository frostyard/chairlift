package updates

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/frostyard/chairlift/internal/async"
	"github.com/frostyard/chairlift/internal/config"
	"github.com/frostyard/chairlift/internal/nbc"
	"github.com/frostyard/chairlift/internal/operations"
	"github.com/frostyard/chairlift/internal/pages"
	"github.com/frostyard/chairlift/internal/pm"
	"github.com/jwijenbergh/puregotk/v4/adw"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

// Page implements the Updates page.
type Page struct {
	toolbarView *adw.ToolbarView
	prefsPage   *adw.PreferencesPage

	config  *config.Config
	toaster pages.Toaster

	// Goroutine lifecycle management
	ctx    context.Context
	cancel context.CancelFunc

	// NBC Update widgets
	nbcCheckRow       *adw.ActionRow
	nbcUpdateExpander *adw.ExpanderRow
	nbcUpdateBtn      *gtk.Button
	nbcDownloadBtn    *gtk.Button

	// Flatpak Updates widgets
	flatpakUpdatesExpander *adw.ExpanderRow
	flatpakUpdateRows      []*adw.ActionRow

	// Homebrew Updates widgets
	outdatedExpander *adw.ExpanderRow

	// Update count tracking
	nbcUpdateCount     int
	flatpakUpdateCount int
	brewUpdateCount    int
	updateCountMu      sync.Mutex

	// Badge update callback
	onBadgeUpdate func(total int)
}

// New creates a new Updates page with the given dependencies.
func New(deps pages.Deps, onBadgeUpdate func(int)) *Page {
	ctx, cancel := context.WithCancel(context.Background())

	p := &Page{
		config:        deps.Config,
		toaster:       deps.Toaster,
		ctx:           ctx,
		cancel:        cancel,
		onBadgeUpdate: onBadgeUpdate,
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
	p.buildNBCUpdatesGroup()
	p.buildFlatpakUpdatesGroup()
	p.buildHomebrewUpdatesGroup()
}

func (p *Page) buildNBCUpdatesGroup() {
	if !IsNBCAvailable() {
		return
	}
	if !p.config.IsGroupEnabled("updates_page", "nbc_updates_group") {
		return
	}

	group := adw.NewPreferencesGroup()
	group.SetTitle("System Updates")
	group.SetDescription("Check for and install NBC system updates")

	// Check for updates row
	p.nbcCheckRow = adw.NewActionRow()
	p.nbcCheckRow.SetTitle("Check for Updates")
	p.nbcCheckRow.SetSubtitle("Checking...")

	checkBtn := gtk.NewButtonWithLabel("Check")
	checkBtn.SetValign(gtk.AlignCenterValue)
	checkClickedCb := func(btn gtk.Button) {
		p.onNBCCheckUpdateClicked()
	}
	checkBtn.ConnectClicked(&checkClickedCb)
	p.nbcCheckRow.AddSuffix(&checkBtn.Widget)
	group.Add(&p.nbcCheckRow.Widget)

	// Update now row with progress expander
	p.nbcUpdateExpander = adw.NewExpanderRow()
	p.nbcUpdateExpander.SetTitle("Install System Update")
	p.nbcUpdateExpander.SetSubtitle("Checking for updates...")

	p.nbcDownloadBtn = gtk.NewButtonWithLabel("Download")
	p.nbcDownloadBtn.SetValign(gtk.AlignCenterValue)
	p.nbcDownloadBtn.SetSensitive(false) // Disabled until we check for updates
	downloadClickedCb := func(btn gtk.Button) {
		p.onNBCDownloadClicked(p.nbcUpdateExpander, p.nbcDownloadBtn)
	}
	p.nbcDownloadBtn.ConnectClicked(&downloadClickedCb)
	p.nbcUpdateExpander.AddSuffix(&p.nbcDownloadBtn.Widget)

	p.nbcUpdateBtn = gtk.NewButtonWithLabel("Update")
	p.nbcUpdateBtn.SetValign(gtk.AlignCenterValue)
	p.nbcUpdateBtn.AddCssClass("suggested-action")
	p.nbcUpdateBtn.SetSensitive(false) // Disabled until we check for updates
	updateClickedCb := func(btn gtk.Button) {
		p.onNBCUpdateClicked(p.nbcUpdateExpander, p.nbcUpdateBtn)
	}
	p.nbcUpdateBtn.ConnectClicked(&updateClickedCb)
	p.nbcUpdateExpander.AddSuffix(&p.nbcUpdateBtn.Widget)
	group.Add(&p.nbcUpdateExpander.Widget)

	p.prefsPage.Add(group)

	// Check for updates on startup
	go p.checkNBCUpdateAvailability()
}

func (p *Page) buildFlatpakUpdatesGroup() {
	if !p.config.IsGroupEnabled("updates_page", "flatpak_updates_group") {
		return
	}

	group := adw.NewPreferencesGroup()
	group.SetTitle("Flatpak Updates")
	group.SetDescription("Available updates for Flatpak applications")

	p.flatpakUpdatesExpander = adw.NewExpanderRow()
	p.flatpakUpdatesExpander.SetTitle("Available Updates")
	p.flatpakUpdatesExpander.SetSubtitle("Loading...")
	group.Add(&p.flatpakUpdatesExpander.Widget)

	p.prefsPage.Add(group)

	// Load flatpak updates asynchronously
	go p.loadFlatpakUpdates()
}

func (p *Page) buildHomebrewUpdatesGroup() {
	if !p.config.IsGroupEnabled("updates_page", "brew_updates_group") {
		return
	}

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
		p.onUpdateHomebrewClicked(updateBtn)
	}
	updateBtn.ConnectClicked(&updateClickedCb)

	updateRow.AddSuffix(&updateBtn.Widget)
	group.Add(&updateRow.Widget)

	// Outdated packages expander
	p.outdatedExpander = adw.NewExpanderRow()
	p.outdatedExpander.SetTitle("Outdated Packages")
	p.outdatedExpander.SetSubtitle("Loading...")
	group.Add(&p.outdatedExpander.Widget)

	p.prefsPage.Add(group)

	// Load outdated packages asynchronously
	go p.loadOutdatedPackages()
}

// updateBadgeCount updates the total update count and notifies the parent via callback.
func (p *Page) updateBadgeCount() {
	p.updateCountMu.Lock()
	total := p.nbcUpdateCount + p.flatpakUpdateCount + p.brewUpdateCount
	p.updateCountMu.Unlock()

	if p.onBadgeUpdate != nil {
		async.RunOnMain(func() {
			select {
			case <-p.ctx.Done():
				return // Page destroyed
			default:
			}
			p.onBadgeUpdate(total)
		})
	}
}

// onNBCCheckUpdateClicked checks if an NBC system update is available
func (p *Page) onNBCCheckUpdateClicked() {
	if p.nbcCheckRow != nil {
		p.nbcCheckRow.SetSubtitle("Checking for updates...")
	}
	if p.nbcUpdateBtn != nil {
		p.nbcUpdateBtn.SetSensitive(false)
	}
	if p.nbcDownloadBtn != nil {
		p.nbcDownloadBtn.SetSensitive(false)
	}

	go p.checkNBCUpdateAvailability()
}

// checkNBCUpdateAvailability checks for NBC updates and updates the UI accordingly
func (p *Page) checkNBCUpdateAvailability() {
	ctx, cancel := nbc.DefaultContext()
	defer cancel()

	result, err := CheckNBCUpdate(ctx)
	if err != nil {
		p.updateCountMu.Lock()
		p.nbcUpdateCount = 0
		p.updateCountMu.Unlock()
		p.updateBadgeCount()

		async.RunOnMain(func() {
			select {
			case <-p.ctx.Done():
				return // Page destroyed
			default:
			}
			if p.nbcCheckRow != nil {
				p.nbcCheckRow.SetSubtitle(fmt.Sprintf("Error: %v", err))
			}
			if p.nbcUpdateExpander != nil {
				p.nbcUpdateExpander.SetSubtitle("Failed to check for updates")
			}
			if p.nbcUpdateBtn != nil {
				p.nbcUpdateBtn.SetSensitive(false)
			}
			if p.nbcDownloadBtn != nil {
				p.nbcDownloadBtn.SetSensitive(false)
			}
		})
		return
	}

	// Update the badge count
	if result.UpdateNeeded {
		p.updateCountMu.Lock()
		p.nbcUpdateCount = 1
		p.updateCountMu.Unlock()
	} else {
		p.updateCountMu.Lock()
		p.nbcUpdateCount = 0
		p.updateCountMu.Unlock()
	}
	p.updateBadgeCount()

	async.RunOnMain(func() {
		select {
		case <-p.ctx.Done():
			return // Page destroyed
		default:
		}

		if result.UpdateNeeded {
			if p.nbcCheckRow != nil {
				digest := result.NewDigest
				if len(digest) > 19 {
					digest = digest[:19] + "..."
				}
				p.nbcCheckRow.SetSubtitle(fmt.Sprintf("Update available: %s", digest))
			}
			if p.nbcUpdateExpander != nil {
				p.nbcUpdateExpander.SetSubtitle("Update available - click to install")
			}
			if p.nbcUpdateBtn != nil {
				p.nbcUpdateBtn.SetSensitive(true)
			}
			if p.nbcDownloadBtn != nil {
				p.nbcDownloadBtn.SetSensitive(true)
			}
		} else {
			if p.nbcCheckRow != nil {
				p.nbcCheckRow.SetSubtitle("System is up to date")
			}
			if p.nbcUpdateExpander != nil {
				p.nbcUpdateExpander.SetSubtitle("No updates available")
			}
			if p.nbcUpdateBtn != nil {
				p.nbcUpdateBtn.SetSensitive(false)
			}
			if p.nbcDownloadBtn != nil {
				p.nbcDownloadBtn.SetSensitive(false)
			}
		}
	})
}

// onNBCUpdateClicked initiates an NBC system update with progress display
func (p *Page) onNBCUpdateClicked(expander *adw.ExpanderRow, button *gtk.Button) {
	// Disable button and expand to show progress
	button.SetSensitive(false)
	button.SetLabel("Updating...")
	expander.SetExpanded(true)
	expander.SetSubtitle("Starting update...")

	// Create a progress bar row
	progressRow := adw.NewActionRow()
	progressRow.SetTitle("Progress")
	progressRow.SetSubtitle("Initializing...")

	progressBar := gtk.NewProgressBar()
	progressBar.SetHexpand(true)
	progressBar.SetValign(gtk.AlignCenterValue)
	progressBar.SetFraction(0)
	progressRow.AddSuffix(&progressBar.Widget)
	expander.AddRow(&progressRow.Widget)

	// Create a log expander for detailed messages
	logExpander := adw.NewExpanderRow()
	logExpander.SetTitle("Details")
	logExpander.SetSubtitle("View detailed progress messages")
	expander.AddRow(&logExpander.Widget)

	go func() {
		ctx, cancel := nbc.DefaultContext()
		defer cancel()

		progressCh := make(chan nbc.ProgressEvent)

		// Start processing progress events in a separate goroutine
		var updateErr error
		var wg sync.WaitGroup
		wg.Add(1)

		go func() {
			defer wg.Done()
			updateErr = nbc.Update(ctx, nbc.UpdateOptions{Auto: true}, progressCh)
		}()

		// Process progress events
		for event := range progressCh {
			evt := event // capture for closure
			async.RunOnMain(func() {
				select {
				case <-p.ctx.Done():
					return // Page destroyed
				default:
				}

				switch evt.Type {
				case nbc.EventTypeStep:
					// Update main progress
					progress := float64(evt.Step) / float64(evt.TotalSteps)
					progressBar.SetFraction(progress)
					progressRow.SetSubtitle(fmt.Sprintf("Step %d/%d: %s", evt.Step, evt.TotalSteps, evt.StepName))
					expander.SetSubtitle(fmt.Sprintf("[%d/%d] %s", evt.Step, evt.TotalSteps, evt.StepName))

				case nbc.EventTypeProgress:
					// Update progress bar with percentage
					progressBar.SetFraction(float64(evt.Percent) / 100.0)
					if evt.Message != "" {
						progressRow.SetSubtitle(fmt.Sprintf("%d%% - %s", evt.Percent, evt.Message))
					}

				case nbc.EventTypeMessage:
					// Add message to log
					msgRow := adw.NewActionRow()
					msgRow.SetTitle(evt.Message)
					msgRow.SetSubtitle(time.Now().Format("15:04:05"))
					logExpander.AddRow(&msgRow.Widget)

				case nbc.EventTypeWarning:
					// Add warning to log with icon
					warnRow := adw.NewActionRow()
					warnRow.SetTitle(evt.Message)
					warnRow.SetSubtitle("Warning")
					warnIcon := gtk.NewImageFromIconName("dialog-warning-symbolic")
					warnRow.AddPrefix(&warnIcon.Widget)
					logExpander.AddRow(&warnRow.Widget)

				case nbc.EventTypeError:
					// Add error to log with icon
					errRow := adw.NewActionRow()
					errRow.SetTitle(evt.Message)
					errRow.SetSubtitle("Error")
					errIcon := gtk.NewImageFromIconName("dialog-error-symbolic")
					errRow.AddPrefix(&errIcon.Widget)
					logExpander.AddRow(&errRow.Widget)
					logExpander.SetExpanded(true)

				case nbc.EventTypeComplete:
					// Update with success
					progressBar.SetFraction(1.0)
					progressRow.SetSubtitle("Complete")
					expander.SetSubtitle("Update complete - please reboot")

					// Add completion message
					completeRow := adw.NewActionRow()
					completeRow.SetTitle(evt.Message)
					completeRow.SetSubtitle("Complete")
					completeIcon := gtk.NewImageFromIconName("object-select-symbolic")
					completeRow.AddPrefix(&completeIcon.Widget)
					logExpander.AddRow(&completeRow.Widget)
				}
			})
		}

		// Wait for update to finish
		wg.Wait()

		// Handle final result
		async.RunOnMain(func() {
			select {
			case <-p.ctx.Done():
				return // Page destroyed
			default:
			}

			button.SetSensitive(true)
			button.SetLabel("Update")

			if updateErr != nil {
				expander.SetSubtitle(fmt.Sprintf("Update failed: %v", updateErr))
				p.toaster.ShowErrorToast(fmt.Sprintf("Update failed: %v", updateErr))
			} else {
				p.toaster.ShowToast("System update complete! Please reboot to apply changes.")
			}
		})
	}()
}

// onNBCDownloadClicked initiates an NBC system update download with progress display
func (p *Page) onNBCDownloadClicked(expander *adw.ExpanderRow, button *gtk.Button) {
	// Disable buttons and expand to show progress
	button.SetSensitive(false)
	button.SetLabel("Downloading...")
	if p.nbcUpdateBtn != nil {
		p.nbcUpdateBtn.SetSensitive(false)
	}
	expander.SetExpanded(true)
	expander.SetSubtitle("Starting download...")

	// Create a progress bar row
	progressRow := adw.NewActionRow()
	progressRow.SetTitle("Progress")
	progressRow.SetSubtitle("Initializing...")

	progressBar := gtk.NewProgressBar()
	progressBar.SetHexpand(true)
	progressBar.SetValign(gtk.AlignCenterValue)
	progressBar.SetFraction(0)
	progressRow.AddSuffix(&progressBar.Widget)
	expander.AddRow(&progressRow.Widget)

	// Create a log expander for detailed messages
	logExpander := adw.NewExpanderRow()
	logExpander.SetTitle("Details")
	logExpander.SetSubtitle("View detailed progress messages")
	expander.AddRow(&logExpander.Widget)

	go func() {
		ctx, cancel := nbc.DefaultContext()
		defer cancel()

		progressCh := make(chan nbc.ProgressEvent)

		// Start processing progress events in a separate goroutine
		var downloadErr error
		var wg sync.WaitGroup
		wg.Add(1)

		go func() {
			defer wg.Done()
			downloadErr = nbc.Download(ctx, nbc.DownloadOptions{ForUpdate: true}, progressCh)
		}()

		// Process progress events
		for event := range progressCh {
			evt := event // capture for closure
			async.RunOnMain(func() {
				select {
				case <-p.ctx.Done():
					return // Page destroyed
				default:
				}

				switch evt.Type {
				case nbc.EventTypeStep:
					// Update main progress
					progress := float64(evt.Step) / float64(evt.TotalSteps)
					progressBar.SetFraction(progress)
					progressRow.SetSubtitle(fmt.Sprintf("Step %d/%d: %s", evt.Step, evt.TotalSteps, evt.StepName))
					expander.SetSubtitle(fmt.Sprintf("[%d/%d] %s", evt.Step, evt.TotalSteps, evt.StepName))

				case nbc.EventTypeProgress:
					// Update progress bar with percentage
					progressBar.SetFraction(float64(evt.Percent) / 100.0)
					if evt.Message != "" {
						progressRow.SetSubtitle(fmt.Sprintf("%d%% - %s", evt.Percent, evt.Message))
					}

				case nbc.EventTypeMessage:
					// Add message to log
					msgRow := adw.NewActionRow()
					msgRow.SetTitle(evt.Message)
					msgRow.SetSubtitle(time.Now().Format("15:04:05"))
					logExpander.AddRow(&msgRow.Widget)

				case nbc.EventTypeWarning:
					// Add warning to log with icon
					warnRow := adw.NewActionRow()
					warnRow.SetTitle(evt.Message)
					warnRow.SetSubtitle("Warning")
					warnIcon := gtk.NewImageFromIconName("dialog-warning-symbolic")
					warnRow.AddPrefix(&warnIcon.Widget)
					logExpander.AddRow(&warnRow.Widget)

				case nbc.EventTypeError:
					// Add error to log with icon
					errRow := adw.NewActionRow()
					errRow.SetTitle(evt.Message)
					errRow.SetSubtitle("Error")
					errIcon := gtk.NewImageFromIconName("dialog-error-symbolic")
					errRow.AddPrefix(&errIcon.Widget)
					logExpander.AddRow(&errRow.Widget)
					logExpander.SetExpanded(true)

				case nbc.EventTypeComplete:
					// Update with success
					progressBar.SetFraction(1.0)
					progressRow.SetSubtitle("Complete")
					expander.SetSubtitle("Download complete - ready to install")

					// Add completion message
					completeRow := adw.NewActionRow()
					completeRow.SetTitle(evt.Message)
					completeRow.SetSubtitle("Complete")
					completeIcon := gtk.NewImageFromIconName("object-select-symbolic")
					completeRow.AddPrefix(&completeIcon.Widget)
					logExpander.AddRow(&completeRow.Widget)
				}
			})
		}

		// Wait for download to finish
		wg.Wait()

		// Handle final result
		async.RunOnMain(func() {
			select {
			case <-p.ctx.Done():
				return // Page destroyed
			default:
			}

			button.SetSensitive(true)
			button.SetLabel("Download")

			if downloadErr != nil {
				expander.SetSubtitle(fmt.Sprintf("Download failed: %v", downloadErr))
				p.toaster.ShowErrorToast(fmt.Sprintf("Download failed: %v", downloadErr))
				if p.nbcUpdateBtn != nil {
					p.nbcUpdateBtn.SetSensitive(true)
				}
			} else {
				p.toaster.ShowToast("Update downloaded! Click Update to install.")
				// Keep update button enabled so user can install the downloaded update
				if p.nbcUpdateBtn != nil {
					p.nbcUpdateBtn.SetSensitive(true)
				}
			}
		})
	}()
}

func (p *Page) onUpdateHomebrewClicked(button *gtk.Button) {
	// Disable button and show working state
	button.SetSensitive(false)
	button.SetLabel("Updating...")

	// Start tracked operation (visible in operations popover)
	op := operations.Start("Update Homebrew", operations.CategoryUpdate, false)

	// Wire retry capability - enables Retry button in operations popover
	op.RetryFunc = func() {
		p.onUpdateHomebrewClicked(button)
	}

	go func() {
		err := pm.HomebrewUpdate()

		async.RunOnMain(func() {
			select {
			case <-p.ctx.Done():
				return // Page destroyed
			default:
			}

			// Restore button state
			button.SetSensitive(true)
			button.SetLabel("Update")

			// Complete the tracked operation
			op.Complete(err)

			if err != nil {
				userErr := async.NewUserError("Couldn't update Homebrew", err)
				p.toaster.ShowErrorToast(userErr.FormatForUser())
				log.Printf("Homebrew update error details: %v", err)
				return
			}
			p.toaster.ShowToast("Homebrew updated successfully")
			// Refresh outdated packages list
			go p.loadOutdatedPackages()
		})
	}()
}

func (p *Page) loadOutdatedPackages() {
	if !pm.HomebrewIsInstalled() {
		p.updateCountMu.Lock()
		p.brewUpdateCount = 0
		p.updateCountMu.Unlock()
		p.updateBadgeCount()

		async.RunOnMain(func() {
			select {
			case <-p.ctx.Done():
				return // Page destroyed
			default:
			}
			if p.outdatedExpander != nil {
				p.outdatedExpander.SetSubtitle("Homebrew not installed")
			}
		})
		return
	}

	packages, err := pm.ListHomebrewOutdated()
	if err != nil {
		p.updateCountMu.Lock()
		p.brewUpdateCount = 0
		p.updateCountMu.Unlock()
		p.updateBadgeCount()

		async.RunOnMain(func() {
			select {
			case <-p.ctx.Done():
				return // Page destroyed
			default:
			}
			if p.outdatedExpander != nil {
				p.outdatedExpander.SetSubtitle(fmt.Sprintf("Error: %v", err))
			}
		})
		return
	}

	// Update the badge count
	p.updateCountMu.Lock()
	p.brewUpdateCount = len(packages)
	p.updateCountMu.Unlock()
	p.updateBadgeCount()

	async.RunOnMain(func() {
		select {
		case <-p.ctx.Done():
			return // Page destroyed
		default:
		}

		if p.outdatedExpander == nil {
			return
		}

		p.outdatedExpander.SetSubtitle(fmt.Sprintf("%d packages available", len(packages)))
		for _, pkg := range packages {
			pkg := pkg // Capture for closure
			row := adw.NewActionRow()
			row.SetTitle(pkg.Name)
			row.SetSubtitle(pkg.Version)

			upgradeBtn := gtk.NewButtonWithLabel("Upgrade")
			upgradeBtn.SetValign(gtk.AlignCenterValue)
			clickedCb := func(btn gtk.Button) {
				go func() {
					if err := pm.HomebrewUpgrade(pkg.Name); err != nil {
						async.RunOnMain(func() {
							select {
							case <-p.ctx.Done():
								return
							default:
							}
							p.toaster.ShowErrorToast(fmt.Sprintf("Upgrade failed: %v", err))
						})
						return
					}
					async.RunOnMain(func() {
						select {
						case <-p.ctx.Done():
							return
						default:
						}
						p.toaster.ShowToast(fmt.Sprintf("%s upgraded", pkg.Name))
					})
				}()
			}
			upgradeBtn.ConnectClicked(&clickedCb)

			row.AddSuffix(&upgradeBtn.Widget)
			p.outdatedExpander.AddRow(&row.Widget)
		}
	})
}

func (p *Page) loadFlatpakUpdates() {
	if !pm.FlatpakIsInstalled() {
		p.updateCountMu.Lock()
		p.flatpakUpdateCount = 0
		p.updateCountMu.Unlock()
		p.updateBadgeCount()

		async.RunOnMain(func() {
			select {
			case <-p.ctx.Done():
				return // Page destroyed
			default:
			}
			if p.flatpakUpdatesExpander != nil {
				p.flatpakUpdatesExpander.SetSubtitle("Flatpak not installed")
			}
		})
		return
	}

	// Get updates from pm library (automatically handles user/system distinction)
	allUpdates, err := pm.ListFlatpakUpdates()
	if err != nil {
		log.Printf("Error loading flatpak updates: %v", err)
		p.updateCountMu.Lock()
		p.flatpakUpdateCount = 0
		p.updateCountMu.Unlock()
		p.updateBadgeCount()

		async.RunOnMain(func() {
			select {
			case <-p.ctx.Done():
				return // Page destroyed
			default:
			}
			if p.flatpakUpdatesExpander != nil {
				p.flatpakUpdatesExpander.SetSubtitle(fmt.Sprintf("Error: %v", err))
			}
		})
		return
	}

	// Update the badge count
	p.updateCountMu.Lock()
	p.flatpakUpdateCount = len(allUpdates)
	p.updateCountMu.Unlock()
	p.updateBadgeCount()

	async.RunOnMain(func() {
		select {
		case <-p.ctx.Done():
			return // Page destroyed
		default:
		}

		if p.flatpakUpdatesExpander == nil {
			return
		}

		// Clear existing rows
		for _, row := range p.flatpakUpdateRows {
			p.flatpakUpdatesExpander.Remove(&row.Widget)
		}
		p.flatpakUpdateRows = nil

		if len(allUpdates) == 0 {
			p.flatpakUpdatesExpander.SetSubtitle("All applications are up to date")
			p.flatpakUpdatesExpander.SetEnableExpansion(false)
			return
		}

		p.flatpakUpdatesExpander.SetSubtitle(fmt.Sprintf("%d updates available", len(allUpdates)))
		p.flatpakUpdatesExpander.SetEnableExpansion(true)

		for _, update := range allUpdates {
			update := update // Capture for closure
			row := adw.NewActionRow()
			row.SetTitle(update.ID)
			subtitle := update.ID
			if update.AvailableVer != "" {
				subtitle = fmt.Sprintf("%s -> %s", update.CurrentVer, update.AvailableVer)
			}
			if !update.IsUser {
				subtitle += " (system)"
			}
			row.SetSubtitle(subtitle)

			// Add update button
			updateBtn := gtk.NewButtonWithLabel("Update")
			updateBtn.SetValign(gtk.AlignCenterValue)
			updateBtn.AddCssClass("suggested-action")

			clickedCb := func(btn gtk.Button) {
				btn.SetSensitive(false)
				btn.SetLabel("Updating...")
				go func() {
					if err := pm.FlatpakUpdate(update.ID, update.IsUser); err != nil {
						async.RunOnMain(func() {
							select {
							case <-p.ctx.Done():
								return
							default:
							}
							btn.SetSensitive(true)
							btn.SetLabel("Update")
							p.toaster.ShowErrorToast(fmt.Sprintf("Update failed: %v", err))
						})
						return
					}
					async.RunOnMain(func() {
						select {
						case <-p.ctx.Done():
							return
						default:
						}
						p.toaster.ShowToast(fmt.Sprintf("%s updated", update.ID))
						// Refresh the updates list
						go p.loadFlatpakUpdates()
					})
				}()
			}
			updateBtn.ConnectClicked(&clickedCb)

			row.AddSuffix(&updateBtn.Widget)
			p.flatpakUpdatesExpander.AddRow(&row.Widget)
			p.flatpakUpdateRows = append(p.flatpakUpdateRows, row)
		}
	})
}
