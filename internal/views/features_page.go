package views

import (
	"fmt"
	"log"

	"github.com/frostyard/chairlift/internal/updex"

	sgtk "github.com/frostyard/snowkit/gtk"

	"codeberg.org/puregotk/puregotk/v4/adw"
	"codeberg.org/puregotk/puregotk/v4/gtk"
)

// buildFeaturesPage builds the Features page content
func (uh *UserHome) buildFeaturesPage() {
	page := uh.featuresPrefsPage
	if page == nil {
		return
	}

	// Features group - only show if updex is available
	installed := updex.IsInstalled()
	if installed && uh.config.IsGroupEnabled("features_page", "features_group") {
		uh.featuresGroup = adw.NewPreferencesGroup()
		uh.featuresGroup.SetTitle("Features")
		uh.featuresGroup.SetDescription("Loading features...")

		// Add Update button as header suffix
		updateBtn := gtk.NewButtonWithLabel("Update")
		updateBtn.SetValign(gtk.AlignCenterValue)
		updateBtn.AddCssClass("suggested-action")
		updateClickedCb := func(btn gtk.Button) {
			uh.onUpdateFeaturesClicked(updateBtn)
		}
		updateBtn.ConnectClicked(&updateClickedCb)
		uh.featuresGroup.SetHeaderSuffix(&updateBtn.Widget)

		page.Add(uh.featuresGroup)

		// Load features asynchronously
		go uh.loadFeatures()
	} else if !installed {
		group := adw.NewPreferencesGroup()
		group.SetTitle("Features")
		group.SetDescription("Manage system features")

		row := adw.NewActionRow()
		row.SetTitle("Feature Manager Not Available")
		row.SetSubtitle("The updex command is not installed on this system")
		group.Add(&row.Widget)
		page.Add(group)
	}
}

// loadFeatures loads feature information asynchronously
func (uh *UserHome) loadFeatures() {
	ctx, cancel := updex.DefaultContext()
	defer cancel()

	features, err := updex.ListFeatures(ctx)

	sgtk.RunOnMainThread(func() {
		if uh.featuresGroup == nil {
			return
		}

		if err != nil {
			uh.featuresGroup.SetDescription(fmt.Sprintf("Error: %v", err))
			return
		}

		if len(features) == 0 {
			uh.featuresGroup.SetDescription("No features available")
			return
		}

		uh.featuresGroup.SetDescription(fmt.Sprintf("%d features available", len(features)))
		uh.featureRows = make(map[string]*adw.ActionRow)

		for _, feat := range features {
			row := adw.NewActionRow()
			row.SetTitle(feat.Description)
			row.SetSubtitle(feat.Name)

			toggle := gtk.NewSwitch()
			toggle.SetActive(feat.Enabled)
			toggle.SetValign(gtk.AlignCenterValue)

			featName := feat.Name
			sw := toggle
			stateSetCb := func(_ gtk.Switch, state bool) bool {
				uh.onFeatureToggled(featName, state, sw)
				return true // block visual change until confirmed
			}
			toggle.ConnectStateSet(&stateSetCb)

			row.AddSuffix(&toggle.Widget)
			row.SetActivatableWidget(&toggle.Widget)
			uh.featuresGroup.Add(&row.Widget)
			uh.featureRows[feat.Name] = row
		}

		// Check for updates after rendering the feature list
		go uh.checkFeatureUpdates(len(features))
	})
}

// checkFeatureUpdates checks enabled features for available updates
func (uh *UserHome) checkFeatureUpdates(totalFeatures int) {
	ctx, cancel := updex.DefaultContext()
	defer cancel()

	checks, err := updex.CheckFeatures(ctx)

	sgtk.RunOnMainThread(func() {
		if err != nil {
			log.Printf("Feature update check failed: %v", err)
			return
		}

		updateCount := 0
		for _, check := range checks {
			row, ok := uh.featureRows[check.Feature]
			if !ok || len(check.Results) == 0 {
				continue
			}

			result := check.Results[0]
			if result.UpdateAvailable {
				row.SetSubtitle(fmt.Sprintf("%s — v%s → v%s available", check.Feature, result.CurrentVersion, result.NewestVersion))
				updateCount++
			} else {
				row.SetSubtitle(fmt.Sprintf("%s — v%s", check.Feature, result.CurrentVersion))
			}
		}

		if uh.featuresGroup != nil && updateCount > 0 {
			uh.featuresGroup.SetDescription(fmt.Sprintf("%d features available (%d updates)", totalFeatures, updateCount))
		}
	})
}

// onFeatureToggled handles enabling/disabling a feature
func (uh *UserHome) onFeatureToggled(name string, enabled bool, toggle *gtk.Switch) {
	go func() {
		ctx, cancel := updex.DefaultContext()
		defer cancel()

		var err error
		if enabled {
			err = updex.EnableFeature(ctx, name)
		} else {
			err = updex.DisableFeature(ctx, name)
		}

		sgtk.RunOnMainThread(func() {
			if err != nil {
				// Revert switch to previous state
				toggle.SetActive(!enabled)
				uh.toastAdder.ShowErrorToast(fmt.Sprintf("Failed to update %s: %v", name, err))
				return
			}

			// Confirm the visual state change
			toggle.SetActive(enabled)

			if enabled {
				uh.toastAdder.ShowToast(fmt.Sprintf("%s enabled. Update to download, reboot to apply.", name))
			} else {
				uh.toastAdder.ShowToast(fmt.Sprintf("%s disabled. Update to apply, reboot to complete.", name))
			}
		})
	}()
}

// onUpdateFeaturesClicked handles the Update button click
func (uh *UserHome) onUpdateFeaturesClicked(button *gtk.Button) {
	button.SetSensitive(false)
	button.SetLabel("Updating...")

	go func() {
		ctx, cancel := updex.DefaultContext()
		defer cancel()

		err := updex.UpdateFeatures(ctx)

		sgtk.RunOnMainThread(func() {
			button.SetSensitive(true)
			button.SetLabel("Update")

			if err != nil {
				uh.toastAdder.ShowErrorToast(fmt.Sprintf("Update failed: %v", err))
				return
			}

			uh.toastAdder.ShowToast("Features updated. Changes apply after reboot.")
		})
	}()
}
