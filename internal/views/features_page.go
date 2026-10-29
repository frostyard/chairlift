package views

import (
	"fmt"
	"log"

	"github.com/frostyard/chairlift/internal/updex"
	"github.com/frostyard/chairlift/internal/views/actionmsg"
	"github.com/frostyard/chairlift/internal/views/featurestatus"
	"github.com/frostyard/chairlift/internal/views/pageview"

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

		uh.featuresGroup.SetDescription(pageview.FeatureGroupDescription(len(features)))
		uh.featureRows = make(map[string]*adw.ActionRow)
		uh.featureBaseSubtitles = make(map[string]string, len(features))

		for _, feat := range features {
			presentation := pageview.Feature(feat.Name, feat.Description)
			row := adw.NewActionRow()
			row.SetTitle(presentation.Title)
			row.SetSubtitle(presentation.Subtitle)

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
			uh.featureBaseSubtitles[feat.Name] = presentation.Subtitle
		}

		// Check for updates after rendering the feature list.
		generation := uh.featureCheck.Begin()
		go uh.checkFeatureUpdates(generation, len(features))
	})
}

// checkFeatureUpdates checks enabled features for available updates and applies
// the result to the rows of the generation it belongs to. generation comes from
// uh.featureCheck, so a check that a newer one has already replaced publishes
// nothing.
func (uh *UserHome) checkFeatureUpdates(generation uint64, totalFeatures int) {
	ctx, cancel := updex.DefaultContext()
	defer cancel()

	checks, err := updex.CheckFeatures(ctx)

	sgtk.RunOnMainThread(func() {
		if !uh.featureCheck.IsCurrent(generation) {
			return
		}

		if err != nil {
			log.Printf("Feature update check failed: %v", err)
			if uh.featuresGroup != nil {
				uh.featuresGroup.SetDescription(featurestatus.GroupDescriptionCheckFailed(totalFeatures))
			}
			return
		}

		names := make([]string, 0, len(uh.featureRows))
		for name := range uh.featureRows {
			names = append(names, name)
		}

		summary := featurestatus.Summarize(names, checks)
		for name, row := range uh.featureRows {
			// A feature the check did not answer for gets its base subtitle
			// back: the earlier check's text, or "update available" before the
			// update, would otherwise stay after an update changed the state.
			if status, ok := summary.Rows[name]; ok {
				row.SetSubtitle(status.Subtitle)
				continue
			}
			row.SetSubtitle(uh.featureBaseSubtitles[name])
		}

		if uh.featuresGroup != nil {
			uh.featuresGroup.SetDescription(featurestatus.GroupDescription(totalFeatures, summary.Updates, summary.Waiting))
		}
	})
}

// recheckFeatureUpdates runs the feature update check again on a fresh
// generation. It is the reason a row stops reading "update available" after an
// update: the check that first ran at startup says that before any image is
// staged, and only a new check reads the staged, not yet linked image as
// waiting for a newer OS image. Main thread only, because it reads the row map.
func (uh *UserHome) recheckFeatureUpdates() {
	if uh.featuresGroup == nil || uh.featureRows == nil {
		return
	}

	generation := uh.featureCheck.Begin()
	go uh.checkFeatureUpdates(generation, len(uh.featureRows))
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

			decision := actionmsg.FeatureToggle(updex.IsDryRun(), enabled, name)
			if decision.Confirm {
				// Confirm the visual state change
				toggle.SetActive(enabled)
			} else {
				// Nothing actually changed under dry-run; revert to the
				// pre-click state.
				toggle.SetActive(!enabled)
			}

			uh.toastAdder.ShowToast(decision.Toast)
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

		results, err := updex.UpdateFeatures(ctx)
		logFeatureUpdate(results, err)
		decision := actionmsg.FeatureUpdate(updex.IsDryRun(), results, err)

		sgtk.RunOnMainThread(func() {
			button.SetSensitive(true)
			button.SetLabel("Update")

			if decision.Persistent {
				uh.toastAdder.ShowErrorToast(decision.Toast)
			} else {
				uh.toastAdder.ShowToast(decision.Toast)
			}

			// The rows still show the state of the startup check, which read
			// "update available" because nothing was staged yet. Check again so
			// they show what the update did.
			if decision.Recheck {
				uh.recheckFeatureUpdates()
			}
		})
	}()
}

// logFeatureUpdate logs the update's error, each feature's classified outcome
// and each transfer's result, so a held or failed feature is diagnosable from
// the log after its toast is gone. It touches no widget.
func logFeatureUpdate(results []updex.FeatureUpdate, err error) {
	if err != nil {
		log.Printf("Feature update returned an error: %v", err)
	}
	summary := updex.SummarizeUpdate(results)
	for _, outcome := range summary.Features {
		log.Printf("Feature update: feature=%q outcome=%s reason=%q", outcome.Feature, outcome.Class, outcome.Reason)
	}
	for _, feature := range results {
		for _, t := range feature.Results {
			log.Printf(
				"Feature update: feature=%q component=%q version=%q downloaded=%v linked=%v installed=%v held=%v needs_image=%q not_linked=%v error=%q message=%q",
				feature.Feature, t.Component, t.Version, t.Downloaded, t.Linked, t.Installed, t.Held, t.NeedsImage, t.NotLinked, t.Error, t.NextActionMessage,
			)
		}
	}
	if summary.RefreshSkipped != "" {
		log.Printf("Feature update: sysext refresh skipped: %s", summary.RefreshSkipped)
	}
}
