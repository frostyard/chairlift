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

	if uh.config.IsGroupEnabled("features_page", "features_group") {
		// Build the features group (shown if updex is available)
		uh.featuresGroup = adw.NewPreferencesGroup()
		uh.featuresGroup.SetTitle("Features")
		uh.featuresGroup.SetDescription("Checking feature availability...")

		// Add Update button as header suffix (disabled until availability confirmed)
		updateBtn := gtk.NewButtonWithLabel("Update")
		updateBtn.SetValign(gtk.AlignCenterValue)
		updateBtn.AddCssClass("suggested-action")
		updateBtn.SetSensitive(false)
		updateClickedCb := func(btn gtk.Button) {
			uh.onUpdateFeaturesClicked(updateBtn)
		}
		updateBtn.ConnectClicked(&updateClickedCb)
		uh.featuresGroup.SetHeaderSuffix(&updateBtn.Widget)

		page.Add(uh.featuresGroup)

		// Build the "not available" group (hidden by default)
		uh.featuresUnavailableGroup = adw.NewPreferencesGroup()
		uh.featuresUnavailableGroup.SetTitle("Features")
		uh.featuresUnavailableGroup.SetDescription("Manage system features")
		uh.featuresUnavailableGroup.SetVisible(false)

		unavailRow := adw.NewActionRow()
		unavailRow.SetTitle("Feature Manager Not Available")
		unavailRow.SetSubtitle("System features are not configured on this system")
		uh.featuresUnavailableGroup.Add(&unavailRow.Widget)
		page.Add(uh.featuresUnavailableGroup)

		// Check availability and load features asynchronously
		go uh.checkAndLoadFeatures(updateBtn)
	}
}

// checkAndLoadFeatures checks updex availability then loads features
func (uh *UserHome) checkAndLoadFeatures(updateBtn *gtk.Button) {
	if !updex.IsInstalledCached() {
		sgtk.RunOnMainThread(func() {
			if uh.featuresGroup != nil {
				uh.featuresGroup.SetVisible(false)
			}
			if uh.featuresUnavailableGroup != nil {
				uh.featuresUnavailableGroup.SetVisible(true)
			}
		})
		return
	}

	sgtk.RunOnMainThread(func() {
		updateBtn.SetSensitive(true)
	})

	uh.loadFeatures()
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
