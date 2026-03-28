package views

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/frostyard/chairlift/internal/flatpak"
	"github.com/frostyard/chairlift/internal/homebrew"
	"github.com/frostyard/chairlift/internal/nbc"

	sgtk "github.com/frostyard/snowkit/gtk"

	"codeberg.org/puregotk/puregotk/v4/adw"
	"codeberg.org/puregotk/puregotk/v4/gtk"
)

// buildUpdatesPage builds the Updates page content
func (uh *UserHome) buildUpdatesPage() {
	page := uh.updatesPrefsPage
	if page == nil {
		return
	}

	// NBC System Updates group - only show if NBC is available
	if _, err := os.Stat("/run/nbc-booted"); err == nil {
		if uh.config.IsGroupEnabled("updates_page", "nbc_updates_group") {
			group := adw.NewPreferencesGroup()
			group.SetTitle("System Updates")
			group.SetDescription("Check for and install NBC system updates")

			// Check for updates row
			uh.nbcCheckRow = adw.NewActionRow()
			uh.nbcCheckRow.SetTitle("Check for Updates")
			uh.nbcCheckRow.SetSubtitle("Checking...")

			checkBtn := gtk.NewButtonWithLabel("Check")
			checkBtn.SetValign(gtk.AlignCenterValue)
			checkClickedCb := func(btn gtk.Button) {
				uh.onNBCCheckUpdateClicked()
			}
			checkBtn.ConnectClicked(&checkClickedCb)
			uh.nbcCheckRow.AddSuffix(&checkBtn.Widget)
			group.Add(&uh.nbcCheckRow.Widget)

			// Update now row with progress expander
			uh.nbcUpdateExpander = adw.NewExpanderRow()
			uh.nbcUpdateExpander.SetTitle("Install System Update")
			uh.nbcUpdateExpander.SetSubtitle("Checking for updates...")

			uh.nbcDownloadBtn = gtk.NewButtonWithLabel("Download")
			uh.nbcDownloadBtn.SetValign(gtk.AlignCenterValue)
			uh.nbcDownloadBtn.SetSensitive(false) // Disabled until we check for updates
			downloadClickedCb := func(btn gtk.Button) {
				uh.onNBCDownloadClicked(uh.nbcUpdateExpander, uh.nbcDownloadBtn)
			}
			uh.nbcDownloadBtn.ConnectClicked(&downloadClickedCb)
			uh.nbcUpdateExpander.AddSuffix(&uh.nbcDownloadBtn.Widget)

			uh.nbcUpdateBtn = gtk.NewButtonWithLabel("Update")
			uh.nbcUpdateBtn.SetValign(gtk.AlignCenterValue)
			uh.nbcUpdateBtn.AddCssClass("suggested-action")
			uh.nbcUpdateBtn.SetSensitive(false) // Disabled until we check for updates
			updateClickedCb := func(btn gtk.Button) {
				uh.onNBCUpdateClicked(uh.nbcUpdateExpander, uh.nbcUpdateBtn)
			}
			uh.nbcUpdateBtn.ConnectClicked(&updateClickedCb)
			uh.nbcUpdateExpander.AddSuffix(&uh.nbcUpdateBtn.Widget)
			group.Add(&uh.nbcUpdateExpander.Widget)

			page.Add(group)

			// Check for updates on startup
			go uh.checkNBCUpdateAvailability()
		}
	}

	// Flatpak Updates group
	if uh.config.IsGroupEnabled("updates_page", "flatpak_updates_group") {
		group := adw.NewPreferencesGroup()
		group.SetTitle("Flatpak Updates")
		group.SetDescription("Available updates for Flatpak applications")

		uh.flatpakUpdatesExpander = adw.NewExpanderRow()
		uh.flatpakUpdatesExpander.SetTitle("Available Updates")
		uh.flatpakUpdatesExpander.SetSubtitle("Loading...")
		group.Add(&uh.flatpakUpdatesExpander.Widget)

		page.Add(group)

		// Load flatpak updates asynchronously
		go uh.loadFlatpakUpdates()
	}

	// Homebrew Updates group
	if uh.config.IsGroupEnabled("updates_page", "brew_updates_group") {
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
			uh.onUpdateHomebrewClicked()
		}
		updateBtn.ConnectClicked(&updateClickedCb)

		updateRow.AddSuffix(&updateBtn.Widget)
		group.Add(&updateRow.Widget)

		// Outdated packages expander
		uh.outdatedExpander = adw.NewExpanderRow()
		uh.outdatedExpander.SetTitle("Outdated Packages")
		uh.outdatedExpander.SetSubtitle("Loading...")
		group.Add(&uh.outdatedExpander.Widget)

		page.Add(group)

		// Load outdated packages asynchronously
		go uh.loadOutdatedPackages()
	}
}

// loadOutdatedPackages loads outdated Homebrew packages asynchronously
func (uh *UserHome) loadOutdatedPackages() {
	if !homebrew.IsInstalledCached() {
		uh.updateCountMu.Lock()
		uh.brewUpdateCount = 0
		uh.updateCountMu.Unlock()
		uh.updateBadgeCount()

		sgtk.RunOnMainThread(func() {
			uh.outdatedExpander.SetSubtitle("Homebrew not installed")
		})
		return
	}

	packages, err := homebrew.ListOutdated()
	if err != nil {
		uh.updateCountMu.Lock()
		uh.brewUpdateCount = 0
		uh.updateCountMu.Unlock()
		uh.updateBadgeCount()

		sgtk.RunOnMainThread(func() {
			uh.outdatedExpander.SetSubtitle(fmt.Sprintf("Error: %v", err))
		})
		return
	}

	// Update the badge count
	uh.updateCountMu.Lock()
	uh.brewUpdateCount = len(packages)
	uh.updateCountMu.Unlock()
	uh.updateBadgeCount()

	sgtk.RunOnMainThread(func() {
		uh.outdatedExpander.SetSubtitle(fmt.Sprintf("%d packages available", len(packages)))
		for _, pkg := range packages {
			row := adw.NewActionRow()
			row.SetTitle(pkg.Name)
			row.SetSubtitle(pkg.Version)

			upgradeBtn := gtk.NewButtonWithLabel("Upgrade")
			upgradeBtn.SetValign(gtk.AlignCenterValue)
			pkgName := pkg.Name
			clickedCb := func(btn gtk.Button) {
				go func() {
					if err := homebrew.Upgrade(pkgName); err != nil {
						sgtk.RunOnMainThread(func() {
							uh.toastAdder.ShowErrorToast(fmt.Sprintf("Upgrade failed: %v", err))
						})
						return
					}
					sgtk.RunOnMainThread(func() {
						uh.toastAdder.ShowToast(fmt.Sprintf("%s upgraded", pkgName))
					})
				}()
			}
			upgradeBtn.ConnectClicked(&clickedCb)

			row.AddSuffix(&upgradeBtn.Widget)
			uh.outdatedExpander.AddRow(&row.Widget)
		}
	})
}

// loadFlatpakUpdates loads available Flatpak updates asynchronously
func (uh *UserHome) loadFlatpakUpdates() {
	if !flatpak.IsInstalledCached() {
		uh.updateCountMu.Lock()
		uh.flatpakUpdateCount = 0
		uh.updateCountMu.Unlock()
		uh.updateBadgeCount()

		sgtk.RunOnMainThread(func() {
			if uh.flatpakUpdatesExpander != nil {
				uh.flatpakUpdatesExpander.SetSubtitle("Flatpak not installed")
			}
		})
		return
	}

	// Collect updates from both user and system installations
	var allUpdates []flatpak.UpdateInfo

	// Load user updates
	userUpdates, err := flatpak.ListUpdates(true)
	if err != nil {
		log.Printf("Error loading user flatpak updates: %v", err)
	} else {
		allUpdates = append(allUpdates, userUpdates...)
	}

	// Load system updates
	systemUpdates, err := flatpak.ListUpdates(false)
	if err != nil {
		log.Printf("Error loading system flatpak updates: %v", err)
	} else {
		allUpdates = append(allUpdates, systemUpdates...)
	}

	// Update the badge count
	uh.updateCountMu.Lock()
	uh.flatpakUpdateCount = len(allUpdates)
	uh.updateCountMu.Unlock()
	uh.updateBadgeCount()

	sgtk.RunOnMainThread(func() {
		if uh.flatpakUpdatesExpander == nil {
			return
		}

		// Clear existing rows
		for _, row := range uh.flatpakUpdateRows {
			uh.flatpakUpdatesExpander.Remove(&row.Widget)
		}
		uh.flatpakUpdateRows = nil

		if len(allUpdates) == 0 {
			uh.flatpakUpdatesExpander.SetSubtitle("All applications are up to date")
			uh.flatpakUpdatesExpander.SetEnableExpansion(false)
			return
		}

		uh.flatpakUpdatesExpander.SetSubtitle(fmt.Sprintf("%d updates available", len(allUpdates)))
		uh.flatpakUpdatesExpander.SetEnableExpansion(true)

		for _, update := range allUpdates {
			row := adw.NewActionRow()
			row.SetTitle(update.Name)
			subtitle := update.ApplicationID
			if update.NewVersion != "" {
				subtitle = fmt.Sprintf("%s → %s", update.ApplicationID, update.NewVersion)
			}
			if update.Installation == "user" {
				subtitle += " (user)"
			}
			row.SetSubtitle(subtitle)

			// Add update button
			updateBtn := gtk.NewButtonWithLabel("Update")
			updateBtn.SetValign(gtk.AlignCenterValue)
			updateBtn.AddCssClass("suggested-action")

			appID := update.ApplicationID
			isUser := update.Installation == "user"
			clickedCb := func(btn gtk.Button) {
				btn.SetSensitive(false)
				btn.SetLabel("Updating...")
				go func() {
					if err := flatpak.Update(appID, isUser); err != nil {
						sgtk.RunOnMainThread(func() {
							btn.SetSensitive(true)
							btn.SetLabel("Update")
							uh.toastAdder.ShowErrorToast(fmt.Sprintf("Update failed: %v", err))
						})
						return
					}
					sgtk.RunOnMainThread(func() {
						uh.toastAdder.ShowToast(fmt.Sprintf("%s updated", appID))
						// Refresh the updates list
						go uh.loadFlatpakUpdates()
					})
				}()
			}
			updateBtn.ConnectClicked(&clickedCb)

			row.AddSuffix(&updateBtn.Widget)
			uh.flatpakUpdatesExpander.AddRow(&row.Widget)
			uh.flatpakUpdateRows = append(uh.flatpakUpdateRows, row)
		}
	})
}

// onNBCCheckUpdateClicked checks if an NBC system update is available
func (uh *UserHome) onNBCCheckUpdateClicked() {
	if uh.nbcCheckRow != nil {
		uh.nbcCheckRow.SetSubtitle("Checking for updates...")
	}
	if uh.nbcUpdateBtn != nil {
		uh.nbcUpdateBtn.SetSensitive(false)
	}
	if uh.nbcDownloadBtn != nil {
		uh.nbcDownloadBtn.SetSensitive(false)
	}

	go uh.checkNBCUpdateAvailability()
}

// checkNBCUpdateAvailability checks for NBC updates and updates the UI accordingly
func (uh *UserHome) checkNBCUpdateAvailability() {
	ctx, cancel := nbc.DefaultContext()
	defer cancel()

	result, err := nbc.CheckUpdate(ctx)
	if err != nil {
		uh.updateCountMu.Lock()
		uh.nbcUpdateCount = 0
		uh.updateCountMu.Unlock()
		uh.updateBadgeCount()

		sgtk.RunOnMainThread(func() {
			if uh.nbcCheckRow != nil {
				uh.nbcCheckRow.SetSubtitle(fmt.Sprintf("Error: %v", err))
			}
			if uh.nbcUpdateExpander != nil {
				uh.nbcUpdateExpander.SetSubtitle("Failed to check for updates")
			}
			if uh.nbcUpdateBtn != nil {
				uh.nbcUpdateBtn.SetSensitive(false)
			}
			if uh.nbcDownloadBtn != nil {
				uh.nbcDownloadBtn.SetSensitive(false)
			}
		})
		return
	}

	// Update the badge count
	if result.UpdateNeeded {
		uh.updateCountMu.Lock()
		uh.nbcUpdateCount = 1
		uh.updateCountMu.Unlock()
	} else {
		uh.updateCountMu.Lock()
		uh.nbcUpdateCount = 0
		uh.updateCountMu.Unlock()
	}
	uh.updateBadgeCount()

	sgtk.RunOnMainThread(func() {
		if result.UpdateNeeded {
			if uh.nbcCheckRow != nil {
				digest := result.NewDigest
				if len(digest) > 19 {
					digest = digest[:19] + "..."
				}
				uh.nbcCheckRow.SetSubtitle(fmt.Sprintf("Update available: %s", digest))
			}
			if uh.nbcUpdateExpander != nil {
				uh.nbcUpdateExpander.SetSubtitle("Update available - click to install")
			}
			if uh.nbcUpdateBtn != nil {
				uh.nbcUpdateBtn.SetSensitive(true)
			}
			if uh.nbcDownloadBtn != nil {
				uh.nbcDownloadBtn.SetSensitive(true)
			}
		} else {
			if uh.nbcCheckRow != nil {
				uh.nbcCheckRow.SetSubtitle("System is up to date")
			}
			if uh.nbcUpdateExpander != nil {
				uh.nbcUpdateExpander.SetSubtitle("No updates available")
			}
			if uh.nbcUpdateBtn != nil {
				uh.nbcUpdateBtn.SetSensitive(false)
			}
			if uh.nbcDownloadBtn != nil {
				uh.nbcDownloadBtn.SetSensitive(false)
			}
		}
	})
}

// nbcOperationFunc is the signature shared by nbc.Update and nbc.Download.
type nbcOperationFunc func(ctx context.Context, progressCh chan<- nbc.ProgressEvent) error

// nbcOperationParams captures the per-operation differences for runNBCOperation.
type nbcOperationParams struct {
	activeLabel   string // button label while running (e.g. "Updating...")
	resetLabel    string // button label after completion (e.g. "Update")
	startSubtitle string // expander subtitle at start (e.g. "Starting update...")
	completionMsg string // expander subtitle on EventTypeComplete
	successToast  string // toast shown on success
	failurePrefix string // prefix for failure messages (e.g. "Update")
	onFinished    func() // optional extra work on the main thread after completion
}

// runNBCOperation handles the shared progress UI scaffolding for NBC operations.
func (uh *UserHome) runNBCOperation(expander *adw.ExpanderRow, button *gtk.Button, opFunc nbcOperationFunc, params nbcOperationParams) {
	button.SetSensitive(false)
	button.SetLabel(params.activeLabel)
	expander.SetExpanded(true)
	expander.SetSubtitle(params.startSubtitle)

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

		var opErr error
		var wg sync.WaitGroup
		wg.Add(1)

		go func() {
			defer wg.Done()
			opErr = opFunc(ctx, progressCh)
		}()

		// Process progress events
		for event := range progressCh {
			evt := event // capture for closure
			sgtk.RunOnMainThread(func() {
				switch evt.Type {
				case nbc.EventTypeStep:
					progress := float64(evt.Step) / float64(evt.TotalSteps)
					progressBar.SetFraction(progress)
					progressRow.SetSubtitle(fmt.Sprintf("Step %d/%d: %s", evt.Step, evt.TotalSteps, evt.StepName))
					expander.SetSubtitle(fmt.Sprintf("[%d/%d] %s", evt.Step, evt.TotalSteps, evt.StepName))

				case nbc.EventTypeProgress:
					progressBar.SetFraction(float64(evt.Percent) / 100.0)
					if evt.Message != "" {
						progressRow.SetSubtitle(fmt.Sprintf("%d%% - %s", evt.Percent, evt.Message))
					}

				case nbc.EventTypeMessage:
					msgRow := adw.NewActionRow()
					msgRow.SetTitle(evt.Message)
					msgRow.SetSubtitle(time.Now().Format("15:04:05"))
					logExpander.AddRow(&msgRow.Widget)

				case nbc.EventTypeWarning:
					warnRow := adw.NewActionRow()
					warnRow.SetTitle(evt.Message)
					warnRow.SetSubtitle("Warning")
					warnIcon := gtk.NewImageFromIconName("dialog-warning-symbolic")
					warnRow.AddPrefix(&warnIcon.Widget)
					logExpander.AddRow(&warnRow.Widget)

				case nbc.EventTypeError:
					errRow := adw.NewActionRow()
					errRow.SetTitle(evt.Message)
					errRow.SetSubtitle("Error")
					errIcon := gtk.NewImageFromIconName("dialog-error-symbolic")
					errRow.AddPrefix(&errIcon.Widget)
					logExpander.AddRow(&errRow.Widget)
					logExpander.SetExpanded(true)

				case nbc.EventTypeComplete:
					progressBar.SetFraction(1.0)
					progressRow.SetSubtitle("Complete")
					expander.SetSubtitle(params.completionMsg)

					completeRow := adw.NewActionRow()
					completeRow.SetTitle(evt.Message)
					completeRow.SetSubtitle("Complete")
					completeIcon := gtk.NewImageFromIconName("object-select-symbolic")
					completeRow.AddPrefix(&completeIcon.Widget)
					logExpander.AddRow(&completeRow.Widget)
				}
			})
		}

		wg.Wait()

		sgtk.RunOnMainThread(func() {
			button.SetSensitive(true)
			button.SetLabel(params.resetLabel)

			if opErr != nil {
				expander.SetSubtitle(fmt.Sprintf("%s failed: %v", params.failurePrefix, opErr))
				uh.toastAdder.ShowErrorToast(fmt.Sprintf("%s failed: %v", params.failurePrefix, opErr))
			} else {
				uh.toastAdder.ShowToast(params.successToast)
			}

			if params.onFinished != nil {
				params.onFinished()
			}
		})
	}()
}

// onNBCUpdateClicked initiates an NBC system update with progress display
func (uh *UserHome) onNBCUpdateClicked(expander *adw.ExpanderRow, button *gtk.Button) {
	uh.runNBCOperation(expander, button,
		func(ctx context.Context, progressCh chan<- nbc.ProgressEvent) error {
			return nbc.Update(ctx, nbc.UpdateOptions{Auto: true}, progressCh)
		},
		nbcOperationParams{
			activeLabel:   "Updating...",
			resetLabel:    "Update",
			startSubtitle: "Starting update...",
			completionMsg: "Update complete - please reboot",
			successToast:  "System update complete! Please reboot to apply changes.",
			failurePrefix: "Update",
		},
	)
}

// onNBCDownloadClicked initiates an NBC system update download with progress display
func (uh *UserHome) onNBCDownloadClicked(expander *adw.ExpanderRow, button *gtk.Button) {
	if uh.nbcUpdateBtn != nil {
		uh.nbcUpdateBtn.SetSensitive(false)
	}

	uh.runNBCOperation(expander, button,
		func(ctx context.Context, progressCh chan<- nbc.ProgressEvent) error {
			return nbc.Download(ctx, nbc.DownloadOptions{ForUpdate: true}, progressCh)
		},
		nbcOperationParams{
			activeLabel:   "Downloading...",
			resetLabel:    "Download",
			startSubtitle: "Starting download...",
			completionMsg: "Download complete - ready to install",
			successToast:  "Update downloaded! Click Update to install.",
			failurePrefix: "Download",
			onFinished: func() {
				if uh.nbcUpdateBtn != nil {
					uh.nbcUpdateBtn.SetSensitive(true)
				}
			},
		},
	)
}

// onUpdateHomebrewClicked handles the Homebrew update button click
func (uh *UserHome) onUpdateHomebrewClicked() {
	go func() {
		if err := homebrew.Update(); err != nil {
			sgtk.RunOnMainThread(func() {
				uh.toastAdder.ShowErrorToast(fmt.Sprintf("Update failed: %v", err))
			})
			return
		}
		sgtk.RunOnMainThread(func() {
			uh.toastAdder.ShowToast("Homebrew updated successfully")
		})
	}()
}
