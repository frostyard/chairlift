package views

import (
	"fmt"
	"log"
	"os"

	"github.com/frostyard/chairlift/internal/flatpak"
	"github.com/frostyard/chairlift/internal/homebrew"

	"codeberg.org/puregotk/puregotk/v4/adw"
	"codeberg.org/puregotk/puregotk/v4/gtk"
)

// buildMaintenancePage builds the Maintenance page content
func (uh *UserHome) buildMaintenancePage() {
	page := uh.maintenancePrefsPage
	if page == nil {
		return
	}

	// Cleanup group
	if uh.config.IsGroupEnabled("maintenance_page", "maintenance_cleanup_group") {
		group := adw.NewPreferencesGroup()
		group.SetTitle("System Cleanup")
		group.SetDescription("Clean up system files and free disk space")

		groupCfg := uh.config.GetGroupConfig("maintenance_page", "maintenance_cleanup_group")
		if groupCfg != nil {
			for _, action := range groupCfg.Actions {
				row := adw.NewActionRow()
				row.SetTitle(action.Title)
				row.SetSubtitle(action.Script)

				if action.Sudo {
					sudoIcon := gtk.NewImageFromIconName("dialog-password-symbolic")
					row.AddPrefix(&sudoIcon.Widget)
				}

				button := gtk.NewButtonWithLabel("Run")
				button.SetValign(gtk.AlignCenterValue)
				button.AddCssClass("suggested-action")

				script := action.Script
				sudo := action.Sudo
				title := action.Title
				clickedCb := func(btn gtk.Button) {
					uh.runMaintenanceAction(title, script, sudo)
				}
				button.ConnectClicked(&clickedCb)

				row.AddSuffix(&button.Widget)
				group.Add(&row.Widget)
			}
		}

		page.Add(group)
	}

	// Homebrew Cleanup group
	if uh.config.IsGroupEnabled("maintenance_page", "maintenance_brew_group") && homebrew.IsInstalled() {
		group := adw.NewPreferencesGroup()
		group.SetTitle("Homebrew Cleanup")
		group.SetDescription("Remove old versions and clear Homebrew cache")

		row := adw.NewActionRow()
		row.SetTitle("Clean Up Homebrew")
		row.SetSubtitle("Remove outdated downloads and old package versions")

		icon := gtk.NewImageFromIconName("user-trash-symbolic")
		row.AddPrefix(&icon.Widget)

		button := gtk.NewButtonWithLabel("Clean Up")
		button.SetValign(gtk.AlignCenterValue)
		button.AddCssClass("suggested-action")

		clickedCb := func(btn gtk.Button) {
			uh.onBrewCleanupClicked(button)
		}
		button.ConnectClicked(&clickedCb)

		row.AddSuffix(&button.Widget)
		group.Add(&row.Widget)

		page.Add(group)
	}

	// Flatpak Cleanup group
	if uh.config.IsGroupEnabled("maintenance_page", "maintenance_flatpak_group") && flatpak.IsInstalled() {
		group := adw.NewPreferencesGroup()
		group.SetTitle("Flatpak Cleanup")
		group.SetDescription("Remove unused Flatpak runtimes and extensions")

		row := adw.NewActionRow()
		row.SetTitle("Remove Unused Runtimes")
		row.SetSubtitle("Uninstall unused Flatpak runtimes and extensions")

		icon := gtk.NewImageFromIconName("user-trash-symbolic")
		row.AddPrefix(&icon.Widget)

		button := gtk.NewButtonWithLabel("Clean Up")
		button.SetValign(gtk.AlignCenterValue)
		button.AddCssClass("suggested-action")

		clickedCb := func(btn gtk.Button) {
			uh.onFlatpakCleanupClicked(button)
		}
		button.ConnectClicked(&clickedCb)

		row.AddSuffix(&button.Widget)
		group.Add(&row.Widget)

		page.Add(group)
	}

	// Optimization group
	if uh.config.IsGroupEnabled("maintenance_page", "maintenance_optimization_group") {
		group := adw.NewPreferencesGroup()
		group.SetTitle("System Optimization")
		group.SetDescription("Optimize system performance")

		// Placeholder for optimization features
		row := adw.NewActionRow()
		row.SetTitle("Optimization tools")
		row.SetSubtitle("Coming soon")
		group.Add(&row.Widget)

		page.Add(group)
	}
}

// onBrewCleanupClicked handles the Homebrew cleanup button click
func (uh *UserHome) onBrewCleanupClicked(button *gtk.Button) {
	button.SetSensitive(false)
	button.SetLabel("Cleaning...")

	go func() {
		output, err := homebrew.Cleanup()

		runOnMainThread(func() {
			button.SetSensitive(true)
			button.SetLabel("Clean Up")

			if err != nil {
				uh.toastAdder.ShowErrorToast(fmt.Sprintf("Homebrew cleanup failed: %v", err))
				return
			}

			if homebrew.IsDryRun() {
				uh.toastAdder.ShowToast(output)
			} else {
				uh.toastAdder.ShowToast("Homebrew cleanup completed")
			}
		})
	}()
}

// onFlatpakCleanupClicked handles the Flatpak cleanup button click
func (uh *UserHome) onFlatpakCleanupClicked(button *gtk.Button) {
	button.SetSensitive(false)
	button.SetLabel("Cleaning...")

	go func() {
		output, err := flatpak.UninstallUnused()

		runOnMainThread(func() {
			button.SetSensitive(true)
			button.SetLabel("Clean Up")

			if err != nil {
				uh.toastAdder.ShowErrorToast(fmt.Sprintf("Flatpak cleanup failed: %v", err))
				return
			}

			if flatpak.IsDryRun() {
				uh.toastAdder.ShowToast(output)
			} else {
				uh.toastAdder.ShowToast("Flatpak cleanup completed")
			}
		})
	}()
}

// onBrewBundleDumpClicked handles the Homebrew bundle dump button click
func (uh *UserHome) onBrewBundleDumpClicked() {
	go func() {
		homeDir, _ := os.UserHomeDir()
		path := homeDir + "/Brewfile"
		if err := homebrew.BundleDump(path, true); err != nil {
			runOnMainThread(func() {
				uh.toastAdder.ShowErrorToast(fmt.Sprintf("Bundle dump failed: %v", err))
			})
			return
		}
		runOnMainThread(func() {
			uh.toastAdder.ShowToast(fmt.Sprintf("Brewfile saved to %s", path))
		})
	}()
}

// runMaintenanceAction runs a maintenance action script
func (uh *UserHome) runMaintenanceAction(title, script string, sudo bool) {
	log.Printf("Running action: %s (script: %s, sudo: %v)", title, script, sudo)
	// TODO: Execute the script
}
