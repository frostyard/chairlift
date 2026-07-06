package views

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/frostyard/chairlift/internal/bootc"

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

	// bootc Status group - built hidden, shown asynchronously if this host
	// is booted from a bootc deployment (bootc status requires an exec, so
	// the gate must not run synchronously during page construction).
	if uh.config.IsGroupEnabled("system_page", "bootc_status_group") {
		group := adw.NewPreferencesGroup()
		group.SetTitle("System Image")
		group.SetDescription("bootc deployment status")
		group.SetVisible(false)

		bootcExpander := adw.NewExpanderRow()
		bootcExpander.SetTitle("Deployment Details")
		bootcExpander.SetSubtitle("Loading...")

		group.Add(&bootcExpander.Widget)
		page.Add(group)

		// Gate + load asynchronously
		go uh.loadBootcStatus(group, bootcExpander)
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

// loadBootcStatus checks the bootc boot gate and populates the status
// expander. Runs in a goroutine; shows the group only on bootc hosts.
func (uh *UserHome) loadBootcStatus(group *adw.PreferencesGroup, expander *adw.ExpanderRow) {
	if !bootc.IsBootcBootedCached() {
		return // group stays hidden on non-bootc hosts
	}

	ctx, cancel := bootc.DefaultContext()
	defer cancel()

	status, err := bootc.GetStatus(ctx)

	sgtk.RunOnMainThread(func() {
		group.SetVisible(true)

		if err != nil {
			expander.SetSubtitle(fmt.Sprintf("Error: %v", err))
			return
		}

		expander.SetSubtitle("Loaded")

		addRow := func(title, subtitle string) {
			row := adw.NewActionRow()
			row.SetTitle(title)
			row.SetSubtitle(subtitle)
			expander.AddRow(&row.Widget)
		}

		booted := status.Status.Booted
		if booted.ImageRef() != "" {
			addRow("Image", booted.ImageRef())
		}
		if booted.Version() != "" {
			addRow("Version", booted.Version())
		}
		if booted.Timestamp() != "" {
			addRow("Built", booted.Timestamp())
		}
		if digest := booted.Digest(); digest != "" {
			if len(digest) > 19 {
				digest = digest[:19] + "..."
			}
			addRow("Digest", digest)
		}

		if staged := status.Status.Staged; staged != nil {
			subtitle := "Restart to apply"
			if staged.Version() != "" {
				subtitle = fmt.Sprintf("%s — restart to apply", staged.Version())
			}
			addRow("Staged Update", subtitle)
		}

		if rollback := status.Status.Rollback; rollback != nil {
			subtitle := rollback.Version()
			if subtitle == "" {
				subtitle = "Available"
			}
			addRow("Rollback", subtitle)
		}
	})
}
