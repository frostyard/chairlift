package views

import (
	"log"

	"codeberg.org/puregotk/puregotk/v4/adw"
	"codeberg.org/puregotk/puregotk/v4/gtk"
)

// buildHelpPage builds the Help page content
func (uh *UserHome) buildHelpPage() {
	page := uh.helpPrefsPage
	if page == nil {
		return
	}

	// Help Resources group
	if uh.config.IsGroupEnabled("help_page", "help_resources_group") {
		group := adw.NewPreferencesGroup()
		group.SetTitle("Help &amp; Resources")
		group.SetDescription("Get help and learn more about ChairLift")

		groupCfg := uh.config.GetGroupConfig("help_page", "help_resources_group")

		// Website row
		if groupCfg != nil && groupCfg.Website != "" {
			row := adw.NewActionRow()
			row.SetTitle("Website")
			row.SetSubtitle(groupCfg.Website)
			row.SetActivatable(true)

			icon := gtk.NewImageFromIconName("adw-external-link-symbolic")
			row.AddSuffix(&icon.Widget)

			url := groupCfg.Website
			activatedCb := func(row adw.ActionRow) {
				uh.openURL(url)
			}
			row.ConnectActivated(&activatedCb)

			group.Add(&row.Widget)
		}

		// Issues row
		if groupCfg != nil && groupCfg.Issues != "" {
			row := adw.NewActionRow()
			row.SetTitle("Report Issues")
			row.SetSubtitle(groupCfg.Issues)
			row.SetActivatable(true)

			icon := gtk.NewImageFromIconName("adw-external-link-symbolic")
			row.AddSuffix(&icon.Widget)

			url := groupCfg.Issues
			activatedCb := func(row adw.ActionRow) {
				uh.openURL(url)
			}
			row.ConnectActivated(&activatedCb)

			group.Add(&row.Widget)
		}

		// Chat row
		if groupCfg != nil && groupCfg.Chat != "" {
			row := adw.NewActionRow()
			row.SetTitle("Community Discussions")
			row.SetSubtitle(groupCfg.Chat)
			row.SetActivatable(true)

			icon := gtk.NewImageFromIconName("adw-external-link-symbolic")
			row.AddSuffix(&icon.Widget)

			url := groupCfg.Chat
			activatedCb := func(row adw.ActionRow) {
				uh.openURL(url)
			}
			row.ConnectActivated(&activatedCb)

			group.Add(&row.Widget)
		}

		page.Add(group)
	}
}

// openURL opens a URL in the default browser
func (uh *UserHome) openURL(url string) {
	log.Printf("Opening URL: %s", url)
	// Use gtk_show_uri or xdg-open
}
