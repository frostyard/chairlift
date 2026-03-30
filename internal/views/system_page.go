package views

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/frostyard/chairlift/internal/nbc"

	sgtk "github.com/frostyard/snowkit/gtk"

	"codeberg.org/puregotk/puregotk/v4/adw"
	"codeberg.org/puregotk/puregotk/v4/gtk"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// buildSystemPage builds the System page content
func (uh *UserHome) buildSystemPage() {
	page := uh.systemPrefsPage
	if page == nil {
		return
	}

	// System Information group
	if uh.config.IsGroupEnabled("system_page", "system_info_group") {
		group := adw.NewPreferencesGroup()
		group.SetTitle("System Information")
		group.SetDescription("View system details and hardware information")

		// OS Release expander
		osExpander := adw.NewExpanderRow()
		osExpander.SetTitle("Operating System Details")

		uh.loadOSRelease(osExpander)
		group.Add(&osExpander.Widget)
		page.Add(group)
	}

	// NBC Status group - only show if NBC is booted
	if _, err := os.Stat("/run/nbc-booted"); err == nil {
		if uh.config.IsGroupEnabled("system_page", "nbc_status_group") {
			group := adw.NewPreferencesGroup()
			group.SetTitle("NBC Status")
			group.SetDescription("View NBC system status information")

			nbcExpander := adw.NewExpanderRow()
			nbcExpander.SetTitle("NBC Status Details")
			nbcExpander.SetSubtitle("Loading...")

			group.Add(&nbcExpander.Widget)
			page.Add(group)

			// Load NBC status asynchronously
			uh.loadNBCStatus(nbcExpander)
		}
	}

	// System Health group
	if uh.config.IsGroupEnabled("system_page", "health_group") {
		group := adw.NewPreferencesGroup()
		group.SetTitle("System Health")
		group.SetDescription("Overview of system health and diagnostics")

		perfRow := adw.NewActionRow()
		perfRow.SetTitle("System Performance")
		perfRow.SetSubtitle("Monitor CPU, memory, and system resources")
		perfRow.SetActivatable(true)

		icon := gtk.NewImageFromIconName("adw-external-link-symbolic")
		perfRow.AddSuffix(&icon.Widget)

		groupCfg := uh.config.GetGroupConfig("system_page", "health_group")
		appID := "io.missioncenter.MissionCenter"
		if groupCfg != nil && groupCfg.AppID != "" {
			appID = groupCfg.AppID
		}

		activatedCb := func(row adw.ActionRow) {
			uh.launchApp(appID)
		}
		perfRow.ConnectActivated(&activatedCb)

		group.Add(&perfRow.Widget)
		page.Add(group)
	}
}

// loadOSRelease loads /etc/os-release into the expander
func (uh *UserHome) loadOSRelease(expander *adw.ExpanderRow) {
	file, err := os.Open("/etc/os-release")
	if err != nil {
		row := adw.NewActionRow()
		row.SetTitle("OS Information")
		row.SetSubtitle("Not available")
		expander.AddRow(&row.Widget)
		return
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		key := parts[0]
		value := strings.Trim(parts[1], "\"'")

		// Convert key to readable format
		readableKey := strings.ReplaceAll(key, "_", " ")
		readableKey = cases.Title(language.English).String(strings.ToLower(readableKey))

		row := adw.NewActionRow()
		row.SetTitle(readableKey)
		row.SetSubtitle(value)

		// Make URL rows clickable
		if strings.HasSuffix(key, "URL") {
			row.SetActivatable(true)
			icon := gtk.NewImageFromIconName("adw-external-link-symbolic")
			row.AddSuffix(&icon.Widget)

			url := value
			activatedCb := func(row adw.ActionRow) {
				uh.openURL(url)
			}
			row.ConnectActivated(&activatedCb)
		}

		expander.AddRow(&row.Widget)
	}
}

// loadNBCStatus loads NBC status information asynchronously into the expander
func (uh *UserHome) loadNBCStatus(expander *adw.ExpanderRow) {
	// Add loading row
	loadingRow := adw.NewActionRow()
	loadingRow.SetTitle("Loading...")
	loadingRow.SetSubtitle("Fetching NBC status")

	spinner := gtk.NewSpinner()
	spinner.Start()
	loadingRow.AddPrefix(&spinner.Widget)
	expander.AddRow(&loadingRow.Widget)

	go func() {
		ctx, cancel := nbc.DefaultContext()
		defer cancel()

		status, err := nbc.GetStatus(ctx)

		sgtk.RunOnMainThread(func() {
			// Remove loading row
			expander.Remove(&loadingRow.Widget)

			if err != nil {
				errorRow := adw.NewActionRow()
				errorRow.SetTitle("Error")
				errorRow.SetSubtitle(fmt.Sprintf("Failed to load NBC status: %v", err))
				errorIcon := gtk.NewImageFromIconName("dialog-error-symbolic")
				errorRow.AddPrefix(&errorIcon.Widget)
				expander.AddRow(&errorRow.Widget)
				expander.SetSubtitle("Failed to load")
				return
			}

			// Display status information
			expander.SetSubtitle("Loaded")

			// Image
			if status.Image != "" {
				row := adw.NewActionRow()
				row.SetTitle("Image")
				row.SetSubtitle(status.Image)
				expander.AddRow(&row.Widget)
			}

			// Digest
			if status.Digest != "" {
				row := adw.NewActionRow()
				row.SetTitle("Digest")
				// Show shortened digest
				digest := status.Digest
				if len(digest) > 19 {
					digest = digest[:19] + "..."
				}
				row.SetSubtitle(digest)
				expander.AddRow(&row.Widget)
			}

			// Device
			if status.Device != "" {
				row := adw.NewActionRow()
				row.SetTitle("Device")
				row.SetSubtitle(status.Device)
				expander.AddRow(&row.Widget)
			}

			// Active Slot
			if status.ActiveSlot != "" {
				row := adw.NewActionRow()
				row.SetTitle("Active Slot")
				row.SetSubtitle(status.ActiveSlot)
				expander.AddRow(&row.Widget)
			}

			// Filesystem Type
			if status.FilesystemType != "" {
				row := adw.NewActionRow()
				row.SetTitle("Filesystem")
				row.SetSubtitle(status.FilesystemType)
				expander.AddRow(&row.Widget)
			}

			// Root Mount Mode
			if status.RootMountMode != "" {
				row := adw.NewActionRow()
				row.SetTitle("Root Mount")
				row.SetSubtitle(status.RootMountMode)
				expander.AddRow(&row.Widget)
			}

			// Staged Update
			if status.StagedUpdate != nil {
				row := adw.NewActionRow()
				row.SetTitle("Staged Update")
				digest := status.StagedUpdate.ImageDigest
				if len(digest) > 19 {
					digest = digest[:19] + "..."
				}
				row.SetSubtitle(fmt.Sprintf("Ready: %s", digest))
				applyButton := gtk.NewButtonWithLabel("Apply")
				applyButton.SetValign(gtk.AlignCenterValue)
				applyButton.AddCssClass("suggested-action")
				btn := applyButton // capture for closure
				applyClickedCb := func(_ gtk.Button) {
					uh.onSystemUpdateClicked(btn)
				}
				applyButton.ConnectClicked(&applyClickedCb)
				row.AddSuffix(&applyButton.Widget)
				expander.AddRow(&row.Widget)
			}
		})
	}()
}

// onSystemUpdateClicked handles the system update button click using the nbc package
func (uh *UserHome) onSystemUpdateClicked(button *gtk.Button) {
	// Disable the button while updating
	if button != nil {
		button.SetSensitive(false)
		button.SetLabel("Updating...")
	}
	uh.toastAdder.ShowToast("Starting system update...")

	go func() {
		ctx, cancel := nbc.DefaultContext()
		defer cancel()

		progressCh := make(chan nbc.ProgressEvent)

		var updateErr error
		var wg sync.WaitGroup
		wg.Add(1)

		go func() {
			defer wg.Done()
			updateErr = nbc.Update(ctx, nbc.UpdateOptions{Auto: true}, progressCh)
		}()

		// Process progress events and show key updates via toasts
		var lastStep string
		for event := range progressCh {
			evt := event
			if evt.Type == nbc.EventTypeStep && evt.StepName != lastStep {
				lastStep = evt.StepName
				sgtk.RunOnMainThread(func() {
					uh.toastAdder.ShowToast(fmt.Sprintf("[%d/%d] %s", evt.Step, evt.TotalSteps, evt.StepName))
				})
			} else if evt.Type == nbc.EventTypeError {
				sgtk.RunOnMainThread(func() {
					uh.toastAdder.ShowErrorToast(evt.Message)
				})
			}
		}

		wg.Wait()

		sgtk.RunOnMainThread(func() {
			// Re-enable the button
			if button != nil {
				button.SetSensitive(true)
				button.SetLabel("Update")
			}

			if updateErr != nil {
				uh.toastAdder.ShowErrorToast(fmt.Sprintf("Update failed: %v", updateErr))
			} else {
				uh.toastAdder.ShowToast("Update complete! Reboot now to apply changes.")
			}
		})
	}()
}
