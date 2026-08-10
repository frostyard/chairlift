package views

import (
	"fmt"
	"log"
	"os"
	"os/exec"

	"github.com/frostyard/chairlift/internal/views/pageview"

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
		if groupCfg != nil {
			resources := pageview.HelpResources(groupCfg.Website, groupCfg.Issues, groupCfg.Chat)
			for _, resource := range resources {
				row := adw.NewActionRow()
				row.SetTitle(resource.Title)
				row.SetSubtitle(resource.URL)
				row.SetActivatable(true)

				icon := gtk.NewImageFromIconName("adw-external-link-symbolic")
				row.AddSuffix(&icon.Widget)

				url := resource.URL
				activatedCb := func(row adw.ActionRow) {
					uh.openURL(url)
				}
				row.ConnectActivated(&activatedCb)

				group.Add(&row.Widget)
			}
		}

		page.Add(group)
	}
}

// openURL opens a URL in the default browser using xdg-open
func (uh *UserHome) openURL(url string) {
	log.Printf("Opening URL: %s", url)

	cmd := exec.Command("xdg-open", url)
	cmd.Env = os.Environ()

	if err := cmd.Start(); err != nil {
		log.Printf("Failed to open URL %s: %v", url, err)
		uh.toastAdder.ShowErrorToast(fmt.Sprintf("Failed to open URL: %s", url))
		return
	}

	go func() {
		_ = cmd.Wait()
	}()
}
