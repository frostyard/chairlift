package views

import (
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/frostyard/chairlift/internal/bootc"
	"github.com/frostyard/chairlift/internal/flatpak"
	"github.com/frostyard/chairlift/internal/fwupd"
	"github.com/frostyard/chairlift/internal/homebrew"
	"github.com/frostyard/chairlift/internal/sysupdate"
	"github.com/frostyard/chairlift/internal/views/actionmsg"
	"github.com/frostyard/chairlift/internal/views/actionstate"
	"github.com/frostyard/chairlift/internal/views/badgestate"
	"github.com/frostyard/chairlift/internal/views/flatpakstatus"
	"github.com/frostyard/chairlift/internal/views/pageview"
	"github.com/frostyard/chairlift/internal/views/trustmsg"

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

	// Native A/B (systemd-sysupdate) System Updates group - built hidden,
	// shown asynchronously on native A/B hosts that ship the snosi stager.
	// Mutually exclusive with the bootc group at runtime: the bootc gate
	// requires the bootc binary (absent on native A/B images) and this gate
	// requires the native-ab marker (absent on bootc images), so at most one
	// "System Updates" group ever becomes visible.
	if uh.config.IsGroupEnabled("updates_page", "sysupdate_updates_group") {
		group := adw.NewPreferencesGroup()
		group.SetTitle("System Updates")
		group.SetDescription("Download and stage system image updates; staged updates apply on restart")
		group.SetVisible(false)

		uh.sysupdateStageExpander = adw.NewExpanderRow()
		uh.sysupdateStageExpander.SetTitle("System Update")
		uh.sysupdateStageExpander.SetSubtitle("Checking status...")

		uh.sysupdateStageBtn = gtk.NewButtonWithLabel("Check for Updates")
		uh.sysupdateStageBtn.SetValign(gtk.AlignCenterValue)
		uh.sysupdateStageBtn.AddCssClass("suggested-action")
		sysupdateClickedCb := func(btn gtk.Button) {
			uh.onSysupdateStageClicked()
		}
		uh.sysupdateStageBtn.ConnectClicked(&sysupdateClickedCb)
		uh.sysupdateStageExpander.AddSuffix(&uh.sysupdateStageBtn.Widget)

		uh.sysupdateRollbackRow = adw.NewActionRow()
		uh.sysupdateRollbackRow.SetTitle("Previous Version")
		uh.sysupdateRollbackRow.SetSubtitle("Checking status...")

		group.Add(&uh.sysupdateStageExpander.Widget)
		group.Add(&uh.sysupdateRollbackRow.Widget)
		page.Add(group)

		go uh.loadSysupdateUpdateStatus(group)
	}

	// Firmware Updates group (fwupd) - built hidden, shown asynchronously
	// when the fwupdmgr client is present, even when there are no updates (the
	// expander then reads "Firmware is up to date"). Every call is
	// unprivileged: the fwupd daemon authorizes the install with its own
	// PolicyKit actions (ADR-0012).
	if uh.config.IsGroupEnabled("updates_page", "firmware_updates_group") {
		group := adw.NewPreferencesGroup()
		group.SetTitle("Firmware Updates")
		group.SetDescription("Device firmware from the Linux Vendor Firmware Service, installed through fwupd")
		group.SetVisible(false)

		uh.firmwareGroup = group
		uh.firmwareExpander = adw.NewExpanderRow()
		uh.firmwareExpander.SetTitle("Available Updates")
		uh.firmwareExpander.SetSubtitle("Checking...")
		group.Add(&uh.firmwareExpander.Widget)

		page.Add(group)

		go uh.loadFirmwareUpdates()
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

		// Load flatpak updates asynchronously. loadFlatpakUpdates takes its
		// refresh generation here, then queries on a goroutine.
		uh.loadFlatpakUpdates()
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
		updateGate := &actionstate.Gate{}
		updateClickedCb := func(btn gtk.Button) {
			if !updateGate.TryStart() {
				return
			}
			btn.SetSensitive(false)
			btn.SetLabel("Updating...")
			go uh.updateHomebrew(btn, updateGate)
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
		uh.loadOutdatedPackages()
	}

	// Untrusted Homebrew Taps group - hidden unless untrusted taps with
	// installed packages exist (Homebrew 6 tap trust).
	if uh.config.IsGroupEnabled("updates_page", "brew_trust_group") {
		uh.brewTrustGroup = adw.NewPreferencesGroup()
		uh.brewTrustGroup.SetTitle("Untrusted Homebrew Taps")
		uh.brewTrustGroup.SetDescription("Homebrew ignores packages from untrusted taps during upgrades. Trust a tap to resume updates for its packages.")
		uh.brewTrustGroup.SetVisible(false)
		page.Add(uh.brewTrustGroup)

		go uh.loadUntrustedTaps()
	}
}

// loadUntrustedTaps populates the Untrusted Taps group. Runs in a
// goroutine; the group stays hidden when there is nothing actionable.
func (uh *UserHome) loadUntrustedTaps() {
	if !homebrew.IsInstalledCached() {
		return
	}

	taps, err := homebrew.ListUntrustedTaps()
	if err != nil {
		log.Printf("untrusted tap check failed: %v", err)
		return
	}
	if len(taps) == 0 {
		return
	}

	sgtk.RunOnMainThread(func() {
		uh.brewTrustRows = make(map[string]*adw.ActionRow)
		for _, tap := range taps {
			t := tap // capture
			presentation := pageview.UntrustedTap(t.Name, t.Formulae, t.Casks)
			row := adw.NewActionRow()
			row.SetTitle(presentation.Title)
			row.SetSubtitle(presentation.Subtitle)

			trustBtn := gtk.NewButtonWithLabel("Trust")
			trustBtn.SetValign(gtk.AlignCenterValue)
			btn := trustBtn
			clickedCb := func(_ gtk.Button) {
				uh.confirmTrustTap(t, btn)
			}
			trustBtn.ConnectClicked(&clickedCb)
			row.AddSuffix(&trustBtn.Widget)

			uh.brewTrustGroup.Add(&row.Widget)
			uh.brewTrustRows[t.Name] = row
		}
		uh.brewTrustGroup.SetVisible(true)
	})
}

// confirmTrustTap shows a confirmation dialog before trusting a tap's packages.
func (uh *UserHome) confirmTrustTap(tap homebrew.UntrustedTap, button *gtk.Button) {
	dialog := adw.NewAlertDialog(
		fmt.Sprintf("Trust packages from %s?", tap.Name),
		"Trusting allows this tap's package definitions to run code during installs and upgrades. Only trust taps you recognize.",
	)
	dialog.AddResponse("cancel", "Cancel")
	dialog.AddResponse("trust", "Trust")
	dialog.SetResponseAppearance("trust", adw.ResponseSuggestedValue)

	responseCb := func(_ adw.AlertDialog, response string) {
		if response != "trust" {
			return
		}
		button.SetSensitive(false)
		button.SetLabel("Trusting...")
		go uh.trustTap(tap, button)
	}
	dialog.ConnectResponse(&responseCb)
	dialog.Present(&uh.updatesPrefsPage.Widget)
}

// trustTap runs brew trust and updates the UI on completion.
func (uh *UserHome) trustTap(tap homebrew.UntrustedTap, button *gtk.Button) {
	err := homebrew.TrustPackages(tap)

	sgtk.RunOnMainThread(func() {
		if err != nil {
			button.SetSensitive(true)
			button.SetLabel("Trust")
			uh.toastAdder.ShowErrorToast(fmt.Sprintf("Failed to trust %s: %v", tap.Name, err))
			return
		}

		decision := actionmsg.TapTrust(homebrew.IsDryRun(), tap.Name)
		if decision.MutateUI {
			if row, ok := uh.brewTrustRows[tap.Name]; ok {
				uh.brewTrustGroup.Remove(&row.Widget)
				delete(uh.brewTrustRows, tap.Name)
			}
			if len(uh.brewTrustRows) == 0 {
				uh.brewTrustGroup.SetVisible(false)
			}
			uh.toastAdder.ShowToast(decision.Toast)

			// Newly trusted packages may now appear as outdated.
			uh.loadOutdatedPackages()
		} else {
			// Dry-run: nothing was actually trusted, so the row must not
			// disappear from the Untrusted Taps list. Reset the button
			// instead of leaving it stuck on "Trusting...".
			button.SetSensitive(true)
			button.SetLabel("Trust")
			uh.toastAdder.ShowToast(decision.Toast)
		}
	})
}

// loadOutdatedPackages loads outdated Homebrew packages asynchronously.
// It is reachable from trustTap (a newly-trusted tap's packages may now be
// outdated) as well as from buildUpdatesPage, so it must stay nil-safe
// against brew_updates_group being disabled — trustTap only depends on
// brew_trust_group and has no way to know whether outdatedExpander exists.
func (uh *UserHome) loadOutdatedPackages() {
	uh.loadOutdatedPackagesWithDone(nil)
}

// loadOutdatedPackagesWithDone starts a versioned refresh. Only the newest
// request may replace rows/count, while every supplied completion callback
// still runs so a superseded action cannot remain disabled.
func (uh *UserHome) loadOutdatedPackagesWithDone(done func(bool)) {
	generation := uh.brewRefresh.Begin()
	go uh.loadOutdatedPackagesGeneration(generation, done)
}

func (uh *UserHome) loadOutdatedPackagesGeneration(generation uint64, done func(bool)) {
	if uh.outdatedExpander == nil {
		if done != nil {
			sgtk.RunOnMainThread(func() {
				done(false)
			})
		}
		return
	}

	if !homebrew.IsInstalledCached() {
		sgtk.RunOnMainThread(func() {
			if !uh.brewRefresh.IsCurrent(generation) {
				if done != nil {
					done(false)
				}
				return
			}
			uh.updateCounts.Set(badgestate.Homebrew, 0)
			uh.updateBadgeCount()
			uh.outdatedRows.Clear(func(row *adw.ActionRow) {
				uh.outdatedExpander.Remove(&row.Widget)
			})
			uh.outdatedExpander.SetSubtitle("Homebrew not installed")
			uh.outdatedExpander.SetEnableExpansion(false)
			if done != nil {
				done(false)
			}
		})
		return
	}

	packages, err := homebrew.ListOutdated()
	currentCount := uh.updateCounts.Get(badgestate.Homebrew)
	refresh := actionstate.OutdatedRefresh(err == nil, currentCount, len(packages))
	if err != nil {
		sgtk.RunOnMainThread(func() {
			if !uh.brewRefresh.IsCurrent(generation) {
				if done != nil {
					done(false)
				}
				return
			}
			uh.outdatedExpander.SetSubtitle(fmt.Sprintf("Error refreshing updates: %v", err))
			if done != nil {
				done(false)
			}
		})
		return
	}

	sgtk.RunOnMainThread(func() {
		if !uh.brewRefresh.IsCurrent(generation) {
			if done != nil {
				done(false)
			}
			return
		}
		presentation := actionstate.OutdatedPresentation(refresh.Count)
		uh.updateCounts.Set(badgestate.Homebrew, refresh.Count)
		uh.updateBadgeCount()
		if refresh.ReplaceRows {
			uh.outdatedRows.Clear(func(row *adw.ActionRow) {
				uh.outdatedExpander.Remove(&row.Widget)
			})
		}

		uh.outdatedExpander.SetSubtitle(presentation.Subtitle)
		uh.outdatedExpander.SetEnableExpansion(presentation.Expandable)
		for _, pkg := range packages {
			row := adw.NewActionRow()
			row.SetTitle(pkg.Name)
			row.SetSubtitle(pkg.Version)

			upgradeBtn := gtk.NewButtonWithLabel("Upgrade")
			upgradeBtn.SetValign(gtk.AlignCenterValue)
			upgradeGate := &actionstate.Gate{}
			pkgName := pkg.Name
			clickedCb := func(btn gtk.Button) {
				if !upgradeGate.TryStart() {
					return
				}
				btn.SetSensitive(false)
				btn.SetLabel("Upgrading...")
				go func() {
					err := homebrew.Upgrade(pkgName)
					dryRun := homebrew.IsDryRun()
					decision := actionstate.PackageUpgrade(err == nil, dryRun)
					if err != nil {
						var trustErr *homebrew.UntrustedTapError
						msg := fmt.Sprintf("Upgrade failed: %v", err)
						if errors.As(err, &trustErr) {
							// uh.brewTrustGroup is only ever assigned once, in
							// buildUpdatesPage on the main thread before this
							// goroutine (or any goroutine) starts, so reading
							// it here is race-free.
							msg = trustmsg.UpgradeMessage(pkgName, uh.brewTrustGroup != nil)
						}
						sgtk.RunOnMainThread(func() {
							if decision.RestoreControl {
								upgradeGate.Reset()
								btn.SetSensitive(true)
								btn.SetLabel("Upgrade")
							}
							uh.toastAdder.ShowErrorToast(msg)
						})
						return
					}
					sgtk.RunOnMainThread(func() {
						uh.toastAdder.ShowToast(actionmsg.Upgrade(dryRun, pkgName))
						if decision.RemoveRow {
							upgradeGate.Complete()
						}
						if decision.RemoveRow && uh.outdatedRows.Remove(row, func(row *adw.ActionRow) {
							uh.outdatedExpander.Remove(&row.Widget)
						}) {
							remaining := uh.updateCounts.Add(badgestate.Homebrew, -1).Count
							presentation := actionstate.OutdatedPresentation(remaining)
							uh.outdatedExpander.SetSubtitle(presentation.Subtitle)
							uh.outdatedExpander.SetEnableExpansion(presentation.Expandable)
							uh.updateBadgeCount()
						}
						if decision.RestoreControl {
							upgradeGate.Reset()
							btn.SetSensitive(true)
							btn.SetLabel("Upgrade")
						}
						if decision.Refresh {
							uh.loadOutdatedPackages()
						}
					})
				}()
			}
			upgradeBtn.ConnectClicked(&clickedCb)

			row.AddSuffix(&upgradeBtn.Widget)
			uh.outdatedExpander.AddRow(&row.Widget)
			uh.outdatedRows.Add(row)
		}
		if done != nil {
			done(true)
		}
	})
}

// loadFlatpakUpdates starts a generation-guarded refresh of the Flatpak
// update inventory. Only the newest request may publish, so a slow reload
// that a later one overtook cannot re-add a row the later one retired or
// overwrite a newer badge count. Call it on the main thread so concurrent
// completions take generations in the order they finished.
func (uh *UserHome) loadFlatpakUpdates() {
	generation := uh.flatpakUpdatesRefresh.Begin()
	go uh.loadFlatpakUpdatesGeneration(generation)
}

// loadFlatpakUpdatesGeneration runs one Flatpak update refresh on a worker
// goroutine and publishes it on the main thread only while generation is
// still current.
func (uh *UserHome) loadFlatpakUpdatesGeneration(generation uint64) {
	if !flatpak.IsInstalledCached() {
		sgtk.RunOnMainThread(func() {
			if !uh.flatpakUpdatesRefresh.IsCurrent(generation) {
				return
			}
			uh.updateCounts.Set(badgestate.Flatpak, 0)
			uh.updateBadgeCount()
			if uh.flatpakUpdatesExpander != nil {
				uh.flatpakUpdatesExpander.SetSubtitle("Flatpak not installed")
			}
		})
		return
	}

	// Collect updates from both user and system installations
	var allUpdates []flatpak.UpdateInfo

	// Load user updates. The error is kept as a value, not just logged: the
	// expander subtitle has to say that half the picture is missing. The
	// updates are kept even with an error: a remote failure
	// (*flatpak.RemoteError) still returns the healthy remotes' updates,
	// while an aborted query returns none.
	userUpdates, userErr := flatpak.ListUpdates(true)
	if userErr != nil {
		log.Printf("Error loading user flatpak updates: %v", userErr)
	}
	allUpdates = append(allUpdates, userUpdates...)

	// Load system updates
	systemUpdates, systemErr := flatpak.ListUpdates(false)
	if systemErr != nil {
		log.Printf("Error loading system flatpak updates: %v", systemErr)
	}
	allUpdates = append(allUpdates, systemUpdates...)

	status := flatpakstatus.Subtitle(len(allUpdates), userErr != nil, systemErr != nil)

	sgtk.RunOnMainThread(func() {
		if !uh.flatpakUpdatesRefresh.IsCurrent(generation) {
			return
		}

		// A load in which both installation queries failed learned nothing,
		// so it keeps the last known count and rows instead of publishing an
		// empty inventory.
		refresh := actionstate.OutdatedRefresh(
			status.Authoritative,
			uh.updateCounts.Get(badgestate.Flatpak),
			len(allUpdates),
		)
		uh.updateCounts.Set(badgestate.Flatpak, refresh.Count)
		uh.updateBadgeCount()

		if uh.flatpakUpdatesExpander == nil {
			return
		}

		if !refresh.ReplaceRows {
			// Say the check failed, but leave the previously discovered
			// rows and their expansion reachable.
			uh.flatpakUpdatesExpander.SetSubtitle(status.Subtitle)
			return
		}

		// Clear existing rows
		for _, row := range uh.flatpakUpdateRows {
			uh.flatpakUpdatesExpander.Remove(&row.Widget)
		}
		uh.flatpakUpdateRows = nil

		uh.flatpakUpdatesExpander.SetSubtitle(status.Subtitle)
		uh.flatpakUpdatesExpander.SetEnableExpansion(status.Expandable)

		if len(allUpdates) == 0 {
			return
		}

		for _, update := range allUpdates {
			presentation := pageview.FlatpakUpdate(
				update.Name,
				update.ApplicationID,
				update.NewVersion,
				update.Installation,
			)
			row := adw.NewActionRow()
			row.SetTitle(presentation.Title)
			row.SetSubtitle(presentation.Subtitle)

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
						uh.toastAdder.ShowToast(actionmsg.Update(flatpak.IsDryRun(), appID))
						// Refresh the updates list. Called on the main
						// thread so concurrent completions take generations
						// in the order they finished.
						uh.loadFlatpakUpdates()
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

// installableCount counts the updates fwupd reports as updatable now. Blocked
// devices stay in the list but not in the badge.
func installableCount(updates []fwupd.Update) int {
	count := 0
	for _, update := range updates {
		if update.Installable {
			count++
		}
	}
	return count
}

// loadFirmwareUpdates loads the firmware update list. Runs in a goroutine; the
// group stays hidden when fwupdmgr is not installed. The load is
// generation-guarded so a slow query cannot replace a newer list.
func (uh *UserHome) loadFirmwareUpdates() {
	group := uh.firmwareGroup
	if uh.firmwareExpander == nil || group == nil {
		return
	}

	if !fwupd.IsInstalledCached() {
		return // group stays hidden
	}

	generation := uh.firmwareRefresh.Begin()

	updates, err := fwupd.ListUpdates()
	if err != nil {
		log.Printf("Error loading firmware updates: %v", err)
	}

	sgtk.RunOnMainThread(func() {
		if !uh.firmwareRefresh.IsCurrent(generation) {
			return
		}
		group.SetVisible(true)
		if err != nil {
			// No rows this time, but rows from an earlier load stay
			// reachable. The count is not lowered: a failed query says
			// nothing about the devices, and it must never claim the
			// firmware is up to date.
			uh.applyFirmwareStatus(pageview.FirmwareUpdatesSubtitle(0, err, uh.firmwareRows.Len()))
			return
		}
		uh.publishFirmwareList(updates)
	})
}

// publishFirmwareList replaces the firmware rows from an authoritative device
// list, sets the badge to the installable count, and sets the subtitle from the
// full list. It is the single place that publishes firmware rows, so a load and
// a verified install cannot drift apart. Main thread only.
func (uh *UserHome) publishFirmwareList(updates []fwupd.Update) {
	expander := uh.firmwareExpander
	if expander == nil {
		return
	}

	// The badge counts what the user can install; the expander subtitle
	// counts everything fwupd lists.
	uh.updateCounts.Set(badgestate.Firmware, installableCount(updates))
	uh.updateBadgeCount()

	uh.firmwareRows.Clear(func(row *adw.ActionRow) {
		expander.Remove(&row.Widget)
	})
	for _, update := range updates {
		uh.addFirmwareRow(expander, update)
	}

	uh.applyFirmwareStatus(pageview.FirmwareUpdatesSubtitle(len(updates), nil, uh.firmwareRows.Len()))
}

// applyFirmwareStatus sets the firmware expander's subtitle and expansion
// state. Main thread only; the expander is nil when the group is disabled.
func (uh *UserHome) applyFirmwareStatus(status pageview.Status) {
	if uh.firmwareExpander == nil {
		return
	}
	uh.firmwareExpander.SetSubtitle(status.Subtitle)
	uh.firmwareExpander.SetEnableExpansion(status.Expandable)
}

// addFirmwareRow adds one device's update row. An installable device gets an
// Install button; a device fwupd cannot update now gets no button and shows
// the reason instead. Main thread only.
func (uh *UserHome) addFirmwareRow(expander *adw.ExpanderRow, update fwupd.Update) {
	var presentation pageview.Row
	if update.Installable {
		presentation = pageview.FirmwareUpdate(
			update.DeviceName,
			update.Vendor,
			update.CurrentVersion,
			update.NewVersion,
		)
	} else {
		presentation = pageview.FirmwareBlockedUpdate(
			update.DeviceName,
			update.Vendor,
			update.CurrentVersion,
			update.NewVersion,
			update.Reason,
		)
	}

	row := adw.NewActionRow()
	row.SetTitle(presentation.Title)
	row.SetSubtitle(presentation.Subtitle)

	if update.Installable {
		installBtn := gtk.NewButtonWithLabel("Install")
		installBtn.SetValign(gtk.AlignCenterValue)
		clickedCb := func(_ gtk.Button) {
			if !uh.firmwareInstallGate.TryStart() {
				uh.toastAdder.ShowToast("Another firmware update is installing")
				return
			}
			uh.confirmFirmwareInstall(update, installBtn)
		}
		installBtn.ConnectClicked(&clickedCb)
		row.AddSuffix(&installBtn.Widget)
	}

	expander.AddRow(&row.Widget)
	uh.firmwareRows.Add(row)
}

// confirmFirmwareInstall asks for confirmation before installing one device's
// firmware, then runs the install. Every outcome warns about power and says
// that ChairLift will not restart the computer. The page-wide install gate is
// already held when this runs.
func (uh *UserHome) confirmFirmwareInstall(update fwupd.Update, button *gtk.Button) {
	heading, body := pageview.FirmwareInstallConfirmation(
		update.DeviceName,
		update.NewVersion,
		update.NeedsReboot,
		update.NeedsShutdown,
	)
	dialog := adw.NewAlertDialog(heading, body)
	dialog.AddResponse("cancel", "Cancel")
	dialog.AddResponse("install", "Install")
	dialog.SetResponseAppearance("install", adw.ResponseDestructiveValue)
	dialog.SetCloseResponse("cancel")

	responseCb := func(_ adw.AlertDialog, response string) {
		if response != "install" {
			// Cancelled: the page must offer an install again.
			uh.firmwareInstallGate.Reset()
			return
		}
		button.SetSensitive(false)
		button.SetLabel("Installing...")
		go uh.installFirmwareUpdate(update, button)
	}
	dialog.ConnectResponse(&responseCb)
	dialog.Present(&uh.updatesPrefsPage.Widget)
}

// installFirmwareUpdate installs one device's firmware, then asks fwupd what
// happened. fwupdmgr can exit 0 without a flash, so the decision comes from
// actionmsg.FirmwareInstall and the verification list, not from the exit
// status. Rows are never removed by row identity: a verified list is published
// whole, so a device that still has an update stays visible. The page-wide gate
// is reset last, in every case.
func (uh *UserHome) installFirmwareUpdate(update fwupd.Update, button *gtk.Button) {
	report, installErr := fwupd.InstallAndVerify(update)
	dryRun := fwupd.IsDryRun()
	decision := actionmsg.FirmwareInstall(
		dryRun,
		installErr,
		report,
		update.DeviceName,
		update.NeedsReboot,
		update.NeedsShutdown,
	)

	sgtk.RunOnMainThread(func() {
		if decision.PublishList {
			// A newer generation makes any load still in flight stale, so it
			// cannot replace this verified list.
			uh.firmwareRefresh.Begin()
			uh.publishFirmwareList(report.Updates)
		} else if report.ListErr != nil {
			// The rows cannot be rebuilt, so the old ones stay and the
			// expander says why the list is out of date.
			uh.applyFirmwareStatus(pageview.FirmwareUpdatesSubtitle(0, report.ListErr, uh.firmwareRows.Len()))
		}
		if decision.CompletedLabel != "" {
			// The install finished but the new rows could not be read: the old
			// button records it and stays insensitive.
			button.SetLabel(decision.CompletedLabel)
		}
		if decision.RestoreControl {
			button.SetSensitive(true)
			button.SetLabel("Install")
		}
		if decision.IsError {
			uh.toastAdder.ShowErrorToast(decision.Toast)
		} else {
			uh.toastAdder.ShowToast(decision.Toast)
		}
		uh.firmwareInstallGate.Reset()
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
	if staged {
		uh.updateCounts.Set(badgestate.Bootc, 1)
	} else {
		uh.updateCounts.Set(badgestate.Bootc, 0)
	}
	uh.updateBadgeCount()

	sgtk.RunOnMainThread(func() {
		group.SetVisible(true)
		if err != nil {
			uh.bootcStageExpander.SetSubtitle(fmt.Sprintf("Error: %v", err))
			return
		}
		version := ""
		if staged {
			version = status.Status.Staged.Version()
		}
		uh.bootcStageExpander.SetSubtitle(pageview.BootcUpdateSubtitle(staged, version))
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
		if staged {
			uh.updateCounts.Set(badgestate.Bootc, 1)
		} else {
			uh.updateCounts.Set(badgestate.Bootc, 0)
		}
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

			version := ""
			if staged {
				version = status.Status.Staged.Version()
			}
			expander.SetSubtitle(pageview.BootcStageResultSubtitle(staged, version, lastMessage))
			uh.toastAdder.ShowToast(actionmsg.BootcStage(bootc.IsDryRun(), staged))
		})
	}()
}

// loadSysupdateUpdateStatus gates the native A/B updates group and reflects
// the /run/snosi state files in the expander subtitle, rollback row, and
// update badge. The reads are unprivileged: the stager's state files are
// world-readable and the rollback candidate comes from partition labels.
func (uh *UserHome) loadSysupdateUpdateStatus(group *adw.PreferencesGroup) {
	if !sysupdate.IsNativeABCached() || !sysupdate.StageScriptAvailable() {
		return // group stays hidden
	}

	ctx, cancel := sysupdate.DefaultContext()
	defer cancel()

	status := sysupdate.GetStatus()
	rollbackVersion, _ := sysupdate.RollbackVersion(ctx)

	if status.IsStaged() {
		uh.updateCounts.Set(badgestate.Sysupdate, 1)
	} else {
		uh.updateCounts.Set(badgestate.Sysupdate, 0)
	}
	uh.updateBadgeCount()

	outcome, version, checkedAt := status.Presentation()
	sgtk.RunOnMainThread(func() {
		group.SetVisible(true)
		uh.sysupdateStageExpander.SetSubtitle(pageview.SysupdateUpdateSubtitle(outcome, version, checkedAt))
		uh.sysupdateRollbackRow.SetSubtitle(pageview.SysupdateRollbackSubtitle(rollbackVersion))
	})
}

// onSysupdateStageClicked runs the snosi stager with streamed log output.
// The script checks, downloads, and stages in one idempotent operation; the
// staged version applies at the next reboot.
func (uh *UserHome) onSysupdateStageClicked() {
	button := uh.sysupdateStageBtn
	expander := uh.sysupdateStageExpander

	button.SetSensitive(false)
	button.SetLabel("Working...")
	expander.SetExpanded(true)
	expander.SetSubtitle("Checking for updates...")

	// Remove rows from any previous run before adding new ones, otherwise
	// repeated clicks stack duplicate Progress/Details rows.
	if uh.sysupdateActivityRow != nil {
		expander.Remove(&uh.sysupdateActivityRow.Widget)
	}
	if uh.sysupdateLogExpander != nil {
		expander.Remove(&uh.sysupdateLogExpander.Widget)
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
	uh.sysupdateActivityRow = activityRow

	logExpander := adw.NewExpanderRow()
	logExpander.SetTitle("Details")
	logExpander.SetSubtitle("View output")
	expander.AddRow(&logExpander.Widget)
	uh.sysupdateLogExpander = logExpander

	go func() {
		ctx, cancel := sysupdate.DefaultContext()
		defer cancel()

		progressCh := make(chan sysupdate.ProgressEvent)

		var stageErr error
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			stageErr = sysupdate.StageUpdate(ctx, progressCh)
		}()

		var lastMessage string
		for event := range progressCh {
			evt := event
			if evt.Type == sysupdate.EventMessage {
				lastMessage = evt.Message
			}
			sgtk.RunOnMainThread(func() {
				switch evt.Type {
				case sysupdate.EventMessage:
					msgRow := adw.NewActionRow()
					msgRow.SetTitle(evt.Message)
					msgRow.SetSubtitle(time.Now().Format("15:04:05"))
					logExpander.AddRow(&msgRow.Widget)
					activityRow.SetSubtitle(evt.Message)
				case sysupdate.EventComplete:
					activityRow.SetSubtitle("Complete")
				}
			})
		}

		wg.Wait()

		// Re-read the state files so the subtitle, rollback row, and badge
		// reflect reality (staged vs already-current) rather than guessing
		// from output. A stage fills the inactive slot with the newer
		// version, so the rollback candidate must be recomputed too.
		status := sysupdate.GetStatus()
		rollbackCtx, rollbackCancel := sysupdate.DefaultContext()
		rollbackVersion, _ := sysupdate.RollbackVersion(rollbackCtx)
		rollbackCancel()

		staged := status.IsStaged()
		if staged {
			uh.updateCounts.Set(badgestate.Sysupdate, 1)
		} else {
			uh.updateCounts.Set(badgestate.Sysupdate, 0)
		}
		uh.updateBadgeCount()

		_, version, _ := status.Presentation()
		sgtk.RunOnMainThread(func() {
			spinner.Stop()
			button.SetSensitive(true)
			button.SetLabel("Check for Updates")
			uh.sysupdateRollbackRow.SetSubtitle(pageview.SysupdateRollbackSubtitle(rollbackVersion))

			if stageErr != nil {
				expander.SetSubtitle(fmt.Sprintf("Update failed: %v", stageErr))
				uh.toastAdder.ShowErrorToast(fmt.Sprintf("Update failed: %v", stageErr))
				return
			}

			expander.SetSubtitle(pageview.SysupdateStageResultSubtitle(staged, version, lastMessage))
			uh.toastAdder.ShowToast(actionmsg.SysupdateStage(sysupdate.IsDryRun(), staged))
		})
	}()
}

// updateHomebrew updates Homebrew metadata, then refreshes the outdated list
// before restoring the top-level action.
func (uh *UserHome) updateHomebrew(button gtk.Button, gate *actionstate.Gate) {
	err := homebrew.Update()
	dryRun := homebrew.IsDryRun()
	decision := actionstate.MetadataUpdate(err == nil, dryRun)
	if err != nil {
		sgtk.RunOnMainThread(func() {
			if decision.RestoreControl {
				gate.Reset()
				button.SetSensitive(true)
				button.SetLabel("Update")
			}
			uh.toastAdder.ShowErrorToast(fmt.Sprintf("Update failed: %v", err))
		})
		return
	}

	sgtk.RunOnMainThread(func() {
		uh.toastAdder.ShowToast(actionmsg.SelfUpdate(dryRun, "Homebrew"))
		if decision.RestoreControl {
			gate.Reset()
			button.SetSensitive(true)
			button.SetLabel("Update")
		}
		if decision.Refresh {
			uh.loadOutdatedPackagesWithDone(func(bool) {
				gate.Reset()
				button.SetSensitive(true)
				button.SetLabel("Update")
			})
		}
	})
}
