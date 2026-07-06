package views

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/frostyard/chairlift/internal/bootc"
	"github.com/frostyard/chairlift/internal/flatpak"
	"github.com/frostyard/chairlift/internal/homebrew"

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

	// bootc System Updates group - built hidden, shown asynchronously on
	// bootc hosts that ship the update-stage script.
	if uh.config.IsGroupEnabled("updates_page", "bootc_updates_group") {
		group := adw.NewPreferencesGroup()
		group.SetTitle("System Updates")
		group.SetDescription("Download and stage system image updates; staged updates apply on restart")
		group.SetVisible(false)

		uh.bootcStageExpander = adw.NewExpanderRow()
		uh.bootcStageExpander.SetTitle("System Update")
		uh.bootcStageExpander.SetSubtitle("Checking status...")

		uh.bootcStageBtn = gtk.NewButtonWithLabel("Check for Updates")
		uh.bootcStageBtn.SetValign(gtk.AlignCenterValue)
		uh.bootcStageBtn.AddCssClass("suggested-action")
		stageClickedCb := func(btn gtk.Button) {
			uh.onBootcStageClicked()
		}
		uh.bootcStageBtn.ConnectClicked(&stageClickedCb)
		uh.bootcStageExpander.AddSuffix(&uh.bootcStageBtn.Widget)

		group.Add(&uh.bootcStageExpander.Widget)
		page.Add(group)

		go uh.loadBootcUpdateStatus(group)
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

// loadBootcUpdateStatus gates the bootc updates group and reflects the
// current staged/booted state in the expander subtitle and update badge.
func (uh *UserHome) loadBootcUpdateStatus(group *adw.PreferencesGroup) {
	if !bootc.IsBootcBootedCached() || !bootc.StageScriptAvailable() {
		return // group stays hidden
	}

	ctx, cancel := bootc.DefaultContext()
	defer cancel()

	status, err := bootc.GetStatus(ctx)

	staged := err == nil && status.Status.Staged != nil
	uh.updateCountMu.Lock()
	if staged {
		uh.bootcUpdateCount = 1
	} else {
		uh.bootcUpdateCount = 0
	}
	uh.updateCountMu.Unlock()
	uh.updateBadgeCount()

	sgtk.RunOnMainThread(func() {
		group.SetVisible(true)
		if err != nil {
			uh.bootcStageExpander.SetSubtitle(fmt.Sprintf("Error: %v", err))
			return
		}
		if staged {
			version := status.Status.Staged.Version()
			if version != "" {
				uh.bootcStageExpander.SetSubtitle(fmt.Sprintf("Update %s staged — restart to apply", version))
			} else {
				uh.bootcStageExpander.SetSubtitle("Update staged — restart to apply")
			}
		} else {
			uh.bootcStageExpander.SetSubtitle("Check for and download the latest system image")
		}
	})
}

// onBootcStageClicked runs the stage script with streamed log output.
// The script checks, downloads, and stages in one idempotent operation.
func (uh *UserHome) onBootcStageClicked() {
	button := uh.bootcStageBtn
	expander := uh.bootcStageExpander

	button.SetSensitive(false)
	button.SetLabel("Working...")
	expander.SetExpanded(true)
	expander.SetSubtitle("Checking for updates...")

	// Remove rows from any previous run before adding new ones, otherwise
	// repeated clicks stack duplicate Progress/Details rows.
	if uh.bootcActivityRow != nil {
		expander.Remove(&uh.bootcActivityRow.Widget)
	}
	if uh.bootcLogExpander != nil {
		expander.Remove(&uh.bootcLogExpander.Widget)
	}

	// Activity row with a spinner (the stage script emits no percentages,
	// so progress is indeterminate).
	activityRow := adw.NewActionRow()
	activityRow.SetTitle("Progress")
	activityRow.SetSubtitle("Running...")
	spinner := gtk.NewSpinner()
	spinner.Start()
	activityRow.AddSuffix(&spinner.Widget)
	expander.AddRow(&activityRow.Widget)
	uh.bootcActivityRow = activityRow

	logExpander := adw.NewExpanderRow()
	logExpander.SetTitle("Details")
	logExpander.SetSubtitle("View output")
	expander.AddRow(&logExpander.Widget)
	uh.bootcLogExpander = logExpander

	go func() {
		ctx, cancel := bootc.DefaultContext()
		defer cancel()

		progressCh := make(chan bootc.ProgressEvent)

		var stageErr error
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			stageErr = bootc.StageUpdate(ctx, progressCh)
		}()

		var lastMessage string
		for event := range progressCh {
			evt := event
			if evt.Type == bootc.EventMessage {
				lastMessage = evt.Message
			}
			sgtk.RunOnMainThread(func() {
				switch evt.Type {
				case bootc.EventMessage:
					msgRow := adw.NewActionRow()
					msgRow.SetTitle(evt.Message)
					msgRow.SetSubtitle(time.Now().Format("15:04:05"))
					logExpander.AddRow(&msgRow.Widget)
					activityRow.SetSubtitle(evt.Message)
				case bootc.EventError:
					errRow := adw.NewActionRow()
					errRow.SetTitle(evt.Message)
					errRow.SetSubtitle("Error")
					errIcon := gtk.NewImageFromIconName("dialog-error-symbolic")
					errRow.AddPrefix(&errIcon.Widget)
					logExpander.AddRow(&errRow.Widget)
					logExpander.SetExpanded(true)
				case bootc.EventComplete:
					activityRow.SetSubtitle("Complete")
				}
			})
		}

		wg.Wait()

		// Re-read status so the subtitle and badge reflect reality
		// (staged vs already-current) rather than guessing from output.
		statusCtx, statusCancel := bootc.DefaultContext()
		status, statusErr := bootc.GetStatus(statusCtx)
		statusCancel()

		staged := statusErr == nil && status.Status.Staged != nil
		uh.updateCountMu.Lock()
		if staged {
			uh.bootcUpdateCount = 1
		} else {
			uh.bootcUpdateCount = 0
		}
		uh.updateCountMu.Unlock()
		uh.updateBadgeCount()

		sgtk.RunOnMainThread(func() {
			spinner.Stop()
			button.SetSensitive(true)
			button.SetLabel("Check for Updates")

			if stageErr != nil {
				expander.SetSubtitle(fmt.Sprintf("Update failed: %v", stageErr))
				uh.toastAdder.ShowErrorToast(fmt.Sprintf("Update failed: %v", stageErr))
				return
			}

			if staged {
				version := status.Status.Staged.Version()
				if version != "" {
					expander.SetSubtitle(fmt.Sprintf("Update %s staged — restart to apply", version))
				} else {
					expander.SetSubtitle("Update staged — restart to apply")
				}
				uh.toastAdder.ShowToast("System update staged. Restart to apply.")
			} else {
				subtitle := "System is up to date"
				if lastMessage != "" {
					subtitle = lastMessage
				}
				expander.SetSubtitle(subtitle)
				uh.toastAdder.ShowToast("System is up to date")
			}
		})
	}()
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
