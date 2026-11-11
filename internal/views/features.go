package views

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/frostyard/chairlift/internal/updex"
	"github.com/frostyard/chairlift/internal/views/actionmsg"
	"github.com/frostyard/chairlift/internal/views/actionstate"
	"github.com/frostyard/chairlift/internal/views/featurestatus"
	"github.com/frostyard/frostui"
)

// The Features page's configuration page and its one group.
const (
	featuresPage  = "features_page"
	featuresGroup = "features_group"
)

// featuresUpdateKey is the Key of the job that updates every feature at
// once. No feature can have it: a feature's name is the name of its
// .feature file, which never holds a slash.
const featuresUpdateKey = "/update"

// featureRow is one feature of the last listing, copied from updex's
// FeatureInfo.
type featureRow struct {
	Name        string
	Description string
	On          bool
	// Masked features cannot be turned on or off: updex refuses them.
	Masked bool
	// Version is the version merged now, or else the one linked for the
	// next boot, when every component agrees on it; "" otherwise.
	Version string
}

// Features holds updex's features (system extensions) and the features
// update source. It belongs to the UI thread.
//
// It ports the GTK Features page (features_page.go before the MyGo port):
// it lists the features as the ordinary user, checks the enabled ones for
// updates with the read-only client (which compares with the booted image
// when bootc refuses an unprivileged status, ADR-0011), and changes them
// only through the fixed root helper, `pkexec /usr/bin/chairlift-updex-helper
// enable-feature|disable-feature <name>` and `... update`, which
// internal/updex runs.
type Features struct {
	env *Env

	// The backends, replaced in tests. installed is
	// updex.IsInstalledCached: it lists the feature definitions once.
	installed func() bool
	list      func(ctx context.Context) ([]updex.Feature, error)
	check     func(ctx context.Context) ([]updex.FeatureCheck, error)
	enable    func(ctx context.Context, name string) error
	disable   func(ctx context.Context, name string) error
	update    func(ctx context.Context) ([]updex.FeatureUpdate, error)

	// probed is true once installed answered; missing when updex has no
	// features here, which shows the "not available" card.
	probed, missing bool

	// The listing. listGate keeps an older listing that ends late from
	// replacing a newer one.
	listGate actionstate.RefreshGate
	loading  bool
	loaded   bool
	listErr  string
	features []featureRow
	// baseSubtitles holds each row's line before any update check answered
	// for it. A check gives it back to every feature it did not answer for,
	// so a stale "update available" from an earlier check cannot outlive
	// the update that removed it.
	baseSubtitles map[string]string

	// The update check. featureCheck is its generation gate, so an older
	// check that finishes late cannot replace a newer one's rows.
	featureCheck actionstate.RefreshGate
	checking     bool
	checked      bool
	checkFailed  bool
	checkErr     string
	checkedAt    time.Time
	summary      featurestatus.Summary
	// checks are the last completed check's results by feature, for the
	// versions an update brings.
	checks map[string][]updex.CheckResult
	// completed holds features updated live since the last successful check.
	// A failed recheck must not offer their completed update again.
	completed map[string]bool

	// asking holds the features whose confirmation is up, so a second
	// click does not queue a second question.
	asking map[string]bool

	source *featureSource
}

// NewFeatures returns the features state; nothing loads until Start.
func NewFeatures(env *Env) *Features {
	f := &Features{
		env:       env,
		installed: updex.IsInstalledCached,
		list:      updex.ListFeatures,
		check:     updex.CheckFeatures,
		enable:    updex.EnableFeature,
		disable:   updex.DisableFeature,
		update:    updex.UpdateFeatures,
		asking:    map[string]bool{},
		completed: map[string]bool{},
	}
	f.source = &featureSource{f: f}
	return f
}

// enabled reports whether the configuration shows the features group.
func (f *Features) enabled() bool { return f.env.Enabled(featuresPage, featuresGroup) }

// Start begins the first load: whether updex has features here, then the
// listing, then the update check. A disabled group loads nothing.
func (f *Features) Start() {
	if !f.enabled() {
		return
	}
	f.load()
}

// featureListing is what a listing goroutine hands back.
type featureListing struct {
	missing  bool
	features []updex.Feature
	err      error
}

// load lists the features and then checks them, on a fresh generation.
func (f *Features) load() {
	if !f.enabled() {
		return
	}
	generation := f.listGate.Begin()
	f.loading = true
	installed, list := f.installed, f.list
	Go(f.env, func() featureListing {
		if !installed() {
			return featureListing{missing: true}
		}
		ctx, cancel := updex.DefaultContext()
		defer cancel()
		features, err := list(ctx)
		return featureListing{features: features, err: err}
	}, func(r featureListing) {
		if !f.listGate.IsCurrent(generation) {
			return
		}
		f.loading = false
		f.probed = true
		if r.missing {
			f.missing = true
			return
		}
		f.missing = false
		if r.err != nil {
			// The rows of an earlier listing stay: a failed reload must
			// not replace what is known with nothing.
			log.Printf("features: list failed: %v", r.err)
			f.listErr = r.err.Error()
			return
		}
		f.listErr = ""
		f.loaded = true
		f.features = make([]featureRow, 0, len(r.features))
		f.baseSubtitles = make(map[string]string, len(r.features))
		for _, feat := range r.features {
			row := featureRow{
				Name:        feat.Name,
				Description: feat.Description,
				On:          feat.Enabled && !feat.Masked,
				Masked:      feat.Masked,
				Version:     featureVersion(feat),
			}
			f.features = append(f.features, row)
			f.baseSubtitles[feat.Name] = baseSubtitle(row)
		}
		if len(f.features) > 0 {
			f.recheckFeatureUpdates()
		}
	})
}

// featureVersion is the version every component of feat runs, or else the
// one every component links for the next boot; "" when they disagree or
// none is known.
func featureVersion(feat updex.Feature) string {
	running := make([]string, len(feat.TransferStatus))
	linked := make([]string, len(feat.TransferStatus))
	for i, status := range feat.TransferStatus {
		running[i], linked[i] = status.RunningVersion, status.LinkedVersion
	}
	if v := agreedVersion(running); v != "" {
		return v
	}
	return agreedVersion(linked)
}

// agreedVersion is the version every entry names; "" when there is none,
// one is empty or "unknown" (merged without a recorded version), or they
// differ.
func agreedVersion(versions []string) string {
	if len(versions) == 0 {
		return ""
	}
	for _, v := range versions {
		if v == "" || v == "unknown" || v != versions[0] {
			return ""
		}
	}
	return versions[0]
}

// baseSubtitle is a row's line while no check answered for it: empty, as
// the switch says on or off and the version has its own column, except
// for a masked feature, whose switch cannot say why it is off.
func baseSubtitle(row featureRow) string {
	if row.Masked {
		return "Masked by the administrator: it cannot be turned on here"
	}
	return ""
}

// featureCheckResult is what a check goroutine hands back.
type featureCheckResult struct {
	checks []updex.FeatureCheck
	err    error
}

// recheckFeatureUpdates runs the feature update check again on a fresh
// generation. It is the reason a row stops reading "update available"
// after an update: the startup check says that before anything is staged,
// and only a new check reads the staged, not yet linked image as waiting
// for a newer OS image. UI thread only.
func (f *Features) recheckFeatureUpdates() {
	if !f.enabled() || !f.loaded || len(f.features) == 0 {
		return
	}
	generation := f.featureCheck.Begin()
	f.checking = true
	check := f.check
	Go(f.env, func() featureCheckResult {
		ctx, cancel := updex.DefaultContext()
		defer cancel()
		checks, err := check(ctx)
		return featureCheckResult{checks: checks, err: err}
	}, func(r featureCheckResult) {
		f.checkFeatureUpdates(generation, r.checks, r.err)
	})
}

// checkFeatureUpdates applies a check's result to the rows, when the check
// is still the newest one. UI thread only.
func (f *Features) checkFeatureUpdates(generation uint64, checks []updex.FeatureCheck, err error) {
	if !f.featureCheck.IsCurrent(generation) {
		return
	}
	f.checking = false
	f.checkedAt = f.env.Now()
	if err != nil {
		// The rows keep what the last check said; only the description
		// says that this one failed.
		log.Printf("Feature update check failed: %v", err)
		f.checkFailed = true
		f.checkErr = err.Error()
		return
	}
	names := make([]string, 0, len(f.features))
	for _, row := range f.features {
		names = append(names, row.Name)
	}
	// Every name the check does not answer for falls back to its base
	// subtitle (subtitle reads f.baseSubtitles[name]), because the summary
	// replaces the last one whole.
	f.summary = featurestatus.Summarize(names, checks)
	clear(f.completed)
	f.checks = make(map[string][]updex.CheckResult, len(checks))
	for _, c := range checks {
		f.checks[c.Feature] = c.Results
	}
	f.checked = true
	f.checkFailed = false
	f.checkErr = ""
}

// subtitle is the line under a feature's title: what the last check said
// of it, or its base subtitle when the check did not answer for it.
func (f *Features) subtitle(name string) string {
	if status, ok := f.summary.Rows[name]; ok {
		return status.Subtitle
	}
	return f.baseSubtitles[name]
}

// status is the last check's status of a feature; ok is false when the
// check did not answer for it.
func (f *Features) status(name string) (featurestatus.Status, bool) {
	status, ok := f.summary.Rows[name]
	return status, ok
}

// groupDescription says how many features there are and what the last
// check found, as the old features group's description did.
func (f *Features) groupDescription() string {
	total := len(f.features)
	switch {
	case f.checkFailed:
		return featurestatus.GroupDescriptionCheckFailed(total)
	case f.checked:
		return featurestatus.GroupDescription(total, f.summary.Updates, f.summary.Waiting)
	}
	return featureCountDescription(total)
}

// row returns the listed feature called name.
func (f *Features) row(name string) (featureRow, bool) {
	for _, row := range f.features {
		if row.Name == name {
			return row, true
		}
	}
	return featureRow{}, false
}

// updatable are the features the last check found an update for, in
// listing order, excluding successful live updates since that check.
func (f *Features) updatable() []featureRow {
	var out []featureRow
	for _, row := range f.features {
		if status, ok := f.summary.Rows[row.Name]; ok && status.HasUpdate && !f.completed[row.Name] {
			out = append(out, row)
		}
	}
	return out
}

// onCount is how many listed features are on.
func (f *Features) onCount() int {
	n := 0
	for _, row := range f.features {
		if row.On {
			n++
		}
	}
	return n
}

// available reports whether the features apply here: the group is on and
// updex has features.
func (f *Features) available() bool {
	return f.enabled() && f.probed && !f.missing
}

// busy reports whether a job for the feature waits or runs.
func (f *Features) busy(name string) bool {
	return f.env.Jobs.Pending(string(SourceFeatures), name)
}

// updating reports whether the features update waits or runs.
func (f *Features) updating() bool {
	return f.env.Jobs.Pending(string(SourceFeatures), featuresUpdateKey)
}

// Summary is what the Overview shows.
func (f *Features) Summary() FeatureSummary {
	if !f.available() || !f.loaded {
		return FeatureSummary{}
	}
	return FeatureSummary{Known: true, On: f.onCount(), Total: len(f.features)}
}

// Refresh lists the features again when the listing never worked, and
// otherwise checks them for updates again, unless a load or check runs.
func (f *Features) Refresh() {
	if !f.enabled() || f.loading || f.checking {
		return
	}
	if !f.loaded || f.listErr != "" {
		f.load()
		return
	}
	f.recheckFeatureUpdates()
}

// AskToggle asks whether to turn a feature on (on true) or off, and queues
// the change when the user agrees. Nothing changes before: the switch and
// the tile show the listing until the job ran.
func (f *Features) AskToggle(name string, on bool) {
	row, ok := f.row(name)
	if !ok || row.Masked || row.On == on || f.busy(name) || f.asking[name] {
		return
	}
	title := featureTitle(row)
	verb := "off"
	body := fmt.Sprintf("The next feature update removes %s from /usr. Its data in /var stays, so turning it on again picks up where it was. Changes apply after a reboot.", name)
	if on {
		verb = "on"
		body = fmt.Sprintf("The next feature update downloads %s and merges it into /usr, and it stays across system updates. Changes apply after a reboot.", name)
	}
	note := "Needs administrator authentication."
	if f.env.DryRun {
		note = "Dry run: ChairLift only logs the command it would run."
	}
	f.asking[name] = true
	f.env.Dialogs.Ask(frostui.Confirm{
		Title:       fmt.Sprintf("Turn %s %s?", verb, title),
		Body:        body,
		Note:        note,
		Action:      "Turn " + verb,
		Destructive: !on,
		OnConfirm: func() {
			delete(f.asking, name)
			f.toggle(name, title, on)
		},
		OnCancel: func() { delete(f.asking, name) },
	})
}

// toggle queues the job that turns a feature on or off through the root
// helper (enable-feature or disable-feature).
func (f *Features) toggle(name, title string, on bool) {
	if f.busy(name) {
		return
	}
	run := f.disable
	jobTitle := "Turning off " + title
	if on {
		run = f.enable
		jobTitle = "Turning on " + title
	}
	// The helper runs as root through pkexec: a stop or a deadline could
	// signal only pkexec, so the job is Unstoppable, has no deadline and
	// finishes first.
	f.env.Jobs.Enqueue(&frostui.Job{
		Source: string(SourceFeatures), Key: name, Title: jobTitle, Unstoppable: true,
		Run: func(ctx context.Context, _ *frostui.Progress) error {
			return run(ctx, name)
		},
		Done: func(err error) {
			if errors.Is(err, context.Canceled) {
				// Dropped from the queue before the helper ran: read back
				// what the system has now.
				f.env.Toast("Canceled: " + jobTitle)
				f.load()
				return
			}
			f.toggled(name, title, on, err)
		},
	})
}

// toggled applies a finished toggle job on the UI thread.
func (f *Features) toggled(name, title string, on bool, err error) {
	if err != nil {
		// The rows still show the listing, so nothing needs restoring.
		f.env.ToastError(fmt.Sprintf("Failed to update %s: %v", name, err))
		return
	}
	decision := actionmsg.FeatureToggle(f.env.DryRun, on, name)
	f.env.Toast(decision.Toast)
	if !decision.Confirm {
		// Nothing changed under dry run: the rows keep the listing.
		return
	}
	for i := range f.features {
		if f.features[i].Name == name {
			f.features[i].On = on
		}
	}
	action := "Turned off "
	if on {
		action = "Turned on "
	}
	f.env.Record(string(SourceFeatures), action+title, decision.Toast)
	// The listing and the check say what the change did.
	f.load()
}

// UpdateAll queues the features update. One run of the helper updates
// every feature that is on, so there is never more than one such job. It
// asks nothing first, as the old page's Update button did not.
func (f *Features) UpdateAll() {
	if !f.available() || f.updating() {
		return
	}
	f.env.Jobs.Enqueue(f.updateJob())
}

// updateJob is the job that updates every feature that is on, through the
// root helper's update command.
func (f *Features) updateJob() *frostui.Job {
	update := f.update
	// Run writes these on its goroutine; Done reads them on the UI thread
	// after the queue posted the job's end.
	var (
		ran       bool
		results   []updex.FeatureUpdate
		updateErr error
	)
	return &frostui.Job{
		Source: string(SourceFeatures),
		Key:    featuresUpdateKey,
		Title:  "Updating features",
		// The helper runs as root through pkexec: the job has no deadline
		// and finishes first.
		Unstoppable: true,
		Run: func(ctx context.Context, p *frostui.Progress) error {
			ran = true
			p.Set(-1, "Updating features")
			results, updateErr = update(ctx)
			logFeatureUpdate(results, updateErr, p.Log)
			return updateErr
		},
		Done: func(err error) {
			if !ran && errors.Is(err, context.Canceled) {
				// Dropped from the queue: the helper never ran.
				f.env.Toast("Canceled: Updating features")
				return
			}
			f.updated(results, updateErr)
		},
	}
}

// updated applies a finished features update on the UI thread.
func (f *Features) updated(results []updex.FeatureUpdate, err error) {
	decision := actionmsg.FeatureUpdate(f.env.DryRun, results, err)
	if decision.Persistent {
		f.env.ToastError(decision.Toast)
	} else {
		f.env.Toast(decision.Toast)
	}
	if changed(results, err) {
		f.env.Record(string(SourceFeatures), "Updated features", decision.Toast)
	}
	if !f.env.DryRun && err == nil {
		for _, outcome := range updex.SummarizeUpdate(results).Of(updex.ClassUpdated) {
			f.completed[outcome.Feature] = true
		}
	}
	// The rows still show the state of the last check, which read "update
	// available" because nothing was staged yet. Check again so they show
	// what the update did.
	if decision.Recheck {
		f.recheckFeatureUpdates()
	}
}

// changed reports whether an update run changed something: a feature the
// classified outcomes call updated, or a merge whose activation did not
// finish. An all-current or empty run changed nothing.
func changed(results []updex.FeatureUpdate, err error) bool {
	var activation *updex.ActivationIncompleteError
	if errors.As(err, &activation) {
		return true
	}
	return len(updex.SummarizeUpdate(results).Of(updex.ClassUpdated)) > 0
}

// logFeatureUpdate logs the update's error, each feature's classified
// outcome and each transfer's result, so a held or failed feature is
// diagnosable from the log after its toast is gone. Each outcome also goes
// to the console through console, which may be nil.
func logFeatureUpdate(results []updex.FeatureUpdate, err error, console func(frostui.Level, string, ...any)) {
	say := func(level frostui.Level, format string, args ...any) {
		if console != nil {
			console(level, format, args...)
		}
	}
	if err != nil {
		log.Printf("Feature update returned an error: %v", err)
	}
	summary := updex.SummarizeUpdate(results)
	for _, outcome := range summary.Features {
		log.Printf("Feature update: feature=%q outcome=%s reason=%q", outcome.Feature, outcome.Class, outcome.Reason)
		level := frostui.LevelInfo
		switch outcome.Class {
		case updex.ClassFailed:
			level = frostui.LevelError
		case updex.ClassWaitingForImage, updex.ClassHeld:
			level = frostui.LevelWarn
		}
		if outcome.Reason != "" {
			say(level, "%s: %s (%s)", outcome.Feature, outcome.Class, outcome.Reason)
		} else {
			say(level, "%s: %s", outcome.Feature, outcome.Class)
		}
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
		say(frostui.LevelWarn, "sysext refresh skipped: %s", summary.RefreshSkipped)
	}
}

// updateVersions are the versions an update of the feature brings, when
// exactly one of its components has one.
func (f *Features) updateVersions(name string) (from, to string) {
	var updates []updex.CheckResult
	for _, r := range f.checks[name] {
		if r.UpdateAvailable {
			updates = append(updates, r)
		}
	}
	if len(updates) != 1 {
		return "", ""
	}
	return updates[0].CurrentVersion, updates[0].NewestVersion
}

// Source is the features update source. It is never nil; its Status says
// whether it applies.
func (f *Features) Source() UpdateSource { return f.source }

// featureSource is the Features part of the Updates page.
type featureSource struct{ f *Features }

func (s *featureSource) ID() SourceID  { return SourceFeatures }
func (s *featureSource) Title() string { return "Features" }
func (s *featureSource) Icon() string  { return "application-x-addon" }
func (s *featureSource) Blurb() string {
	return "System extensions that add to /usr without changing the image"
}

func (s *featureSource) Status() SourceStatus {
	f := s.f
	st := SourceStatus{
		Available: f.available(),
		Checking:  f.loading || f.checking,
		CheckedAt: f.checkedAt,
	}
	switch {
	case f.listErr != "":
		st.Err = f.listErr
	case f.checkFailed:
		st.Err = f.checkErr
	}
	if f.loaded {
		parts := []string{fmt.Sprintf("%d of %d on", f.onCount(), len(f.features))}
		if f.summary.Waiting > 0 {
			parts = append(parts, fmt.Sprintf("%d waiting for a newer OS image", f.summary.Waiting))
		}
		if len(f.updatable()) > 1 {
			parts = append(parts, "one update covers them all")
		}
		st.Note = strings.Join(parts, " · ")
	}
	return st
}

// Items are the features the last check found an update for. Their Action
// is "Update", and every one of them is updated by the same single run of
// the helper, which updates every feature that is on.
func (s *featureSource) Items() []UpdateItem {
	f := s.f
	if !f.available() {
		return nil
	}
	rows := f.updatable()
	items := make([]UpdateItem, 0, len(rows))
	for _, row := range rows {
		from, to := f.updateVersions(row.Name)
		items = append(items, UpdateItem{
			Key:    row.Name,
			Title:  featureTitle(row),
			Detail: row.Name,
			From:   from,
			To:     to,
			Action: "Update",
			// One helper run updates every feature: its job is each row's.
			JobKey: featuresUpdateKey,
		})
	}
	return items
}

func (s *featureSource) Refresh() { s.f.Refresh() }

// Jobs returns the one job that updates every feature that is on, whatever
// keys it is given: the helper cannot update one feature alone. It returns
// none when nothing has an update or the update already waits or runs.
func (s *featureSource) Jobs(keys ...string) []*frostui.Job {
	f := s.f
	if !f.available() || f.updating() || len(f.updatable()) == 0 {
		return nil
	}
	return []*frostui.Job{f.updateJob()}
}

// Update queues the features update; key does not matter (see Jobs).
func (s *featureSource) Update(string) { s.f.UpdateAll() }
