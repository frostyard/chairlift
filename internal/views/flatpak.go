package views

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/chairlift/internal/flatpak"
	"github.com/frostyard/chairlift/internal/views/actionmsg"
	"github.com/frostyard/chairlift/internal/views/actionstate"
	"github.com/frostyard/chairlift/internal/views/flatpakstatus"
	"github.com/frostyard/chairlift/internal/views/pageview"
	"github.com/frostyard/frostui"
)

// Flatpak holds the installed Flatpak apps and the Flatpak update source.
//
// The installed apps are those of the installations whose groups are on:
// "flatpak_user_group" and "flatpak_system_group" of applications_page.
// The update inventory is updates_page's "flatpak_updates_group", which
// queries both installations, as the old Updates page did. Nothing loads
// for a disabled group, and nothing loads at all without the flatpak
// command.
//
// Both loads are generation-guarded with actionstate.RefreshGate: a load
// takes its generation on the UI thread and only the newest may publish,
// so a slow reload that a later one overtook cannot bring back a row the
// later one retired. An update load in which no installation returned a
// usable result (flatpakstatus.Result.Authoritative false) keeps the last
// known rows, and with them the count, through actionstate.OutdatedRefresh.
// A per-app update does not wait for a reload either: a failure or a dry
// run leaves its row (and the row's button comes back as the job ends), a
// live success removes the row, which lowers the count, before the reload.
// A live uninstall removes the app's update row too, and retires an update
// check in flight with a newer one: the older check began before the
// uninstall and could list the app again.
type Flatpak struct {
	env *Env

	// The backend, replaceable in tests. present, the lists and the
	// changes run on goroutines; launch runs on the UI thread and only
	// starts the app; dryRun is read on the UI thread.
	present     func() bool
	listApps    func(user bool) ([]flatpak.Application, error)
	listUpdates func(user bool) ([]flatpak.UpdateInfo, error)
	update      func(appID string, user bool) error
	uninstall   func(appID string, user bool) error
	launch      func(appID string) error
	dryRun      func() bool

	// tool is what the last load learned of the flatpak command.
	tool fpTool

	// The installed apps of each enabled installation. An installation
	// whose list failed keeps the apps it had, as the old page kept its
	// rows, and says why in its error.
	userApps, systemApps []flatpak.Application
	userErr, systemErr   error
	appsLoading          bool
	appsRefresh          actionstate.RefreshGate

	// updates is the last authoritative update inventory, user
	// installation first; its length is the Flatpak update count.
	updates         []flatpak.UpdateInfo
	updatesStatus   flatpakstatus.Result
	updatesFailed   bool
	updatesChecking bool
	updatesChecked  time.Time
	// flatpakUpdatesRefresh bounds overlapping update reloads. Every live
	// update kicks a reload, so two finishing close together race; only
	// the newest generation may publish rows and count.
	flatpakUpdatesRefresh actionstate.RefreshGate
}

// fpTool is whether the flatpak command is there.
type fpTool int

const (
	fpToolUnknown fpTool = iota
	fpToolPresent
	fpToolMissing
)

// NewFlatpak returns the Flatpak state, wired to internal/flatpak.
func NewFlatpak(env *Env) *Flatpak {
	return &Flatpak{
		env:         env,
		present:     flatpak.IsInstalledCached,
		listApps:    fpListApps,
		listUpdates: flatpak.ListUpdates,
		update:      flatpak.Update,
		uninstall:   flatpak.Uninstall,
		launch:      launchDesktopApp,
		dryRun:      flatpak.IsDryRun,
	}
}

func fpListApps(user bool) ([]flatpak.Application, error) {
	if user {
		return flatpak.ListUserApplications()
	}
	return flatpak.ListSystemApplications()
}

func (f *Flatpak) userOn() bool    { return f.env.Enabled("applications_page", "flatpak_user_group") }
func (f *Flatpak) systemOn() bool  { return f.env.Enabled("applications_page", "flatpak_system_group") }
func (f *Flatpak) updatesOn() bool { return f.env.Enabled("updates_page", "flatpak_updates_group") }

// Start begins the first load of each enabled group.
func (f *Flatpak) Start() {
	if f.userOn() || f.systemOn() {
		f.loadApps()
	}
	if f.updatesOn() {
		f.loadUpdates()
	}
}

func (f *Flatpak) setTool(present bool) {
	if present {
		f.tool = fpToolPresent
	} else {
		f.tool = fpToolMissing
	}
}

// --- Installed apps ---------------------------------------------------------

type fpAppsResult struct {
	present            bool
	user, system       []flatpak.Application
	userErr, systemErr error
}

// RefreshApps reloads the installed apps.
func (f *Flatpak) RefreshApps() {
	if !f.userOn() && !f.systemOn() {
		return
	}
	f.loadApps()
}

// loadApps starts a generation-guarded load of the enabled installations.
// Call it on the UI thread, so loads take generations in the order they
// were asked for.
func (f *Flatpak) loadApps() {
	generation := f.appsRefresh.Begin()
	f.appsLoading = true
	user, system := f.userOn(), f.systemOn()
	present, list := f.present, f.listApps
	Go(f.env, func() fpAppsResult {
		var r fpAppsResult
		if r.present = present(); !r.present {
			return r
		}
		if user {
			if r.user, r.userErr = list(true); r.userErr != nil {
				log.Printf("Error loading user flatpak applications: %v", r.userErr)
			}
		}
		if system {
			if r.system, r.systemErr = list(false); r.systemErr != nil {
				log.Printf("Error loading system flatpak applications: %v", r.systemErr)
			}
		}
		return r
	}, func(r fpAppsResult) { f.applyApps(generation, r) })
}

// applyApps publishes one load, while its generation is the newest.
func (f *Flatpak) applyApps(generation uint64, r fpAppsResult) {
	if !f.appsRefresh.IsCurrent(generation) {
		return
	}
	f.appsLoading = false
	f.setTool(r.present)
	if !r.present {
		f.userApps, f.systemApps, f.userErr, f.systemErr = nil, nil, nil, nil
		return
	}
	if r.userErr == nil {
		f.userApps = r.user
	}
	if r.systemErr == nil {
		f.systemApps = r.system
	}
	f.userErr, f.systemErr = r.userErr, r.systemErr
}

// AppsStatus is the state of the installed-apps load.
func (f *Flatpak) AppsStatus() AppsStatus {
	st := AppsStatus{
		Available: (f.userOn() || f.systemOn()) && f.tool != fpToolMissing,
		Loading:   f.appsLoading,
	}
	var errs []string
	if f.userErr != nil && f.userOn() {
		errs = append(errs, "user installation: "+f.userErr.Error())
	}
	if f.systemErr != nil && f.systemOn() {
		errs = append(errs, "system installation: "+f.systemErr.Error())
	}
	st.Err = strings.Join(errs, "; ")
	return st
}

// Apps are the installed Flatpak apps of the enabled installations, for the
// Apps table: system apps first, then the user's.
func (f *Flatpak) Apps() []AppEntry {
	if f.tool == fpToolMissing {
		return nil
	}
	updates := map[string]string{}
	for _, u := range f.updates {
		to := u.NewVersion
		if to == "" {
			to = "latest"
		}
		updates[fpUpdateKey(u)] = to
	}
	var out []AppEntry
	seen := map[string]bool{}
	add := func(user bool, apps []flatpak.Application) {
		kind := AppFlatpakSystem
		if user {
			kind = AppFlatpakUser
		}
		for _, a := range apps {
			key := fpKey(user, a.ApplicationID, a.Branch)
			if seen[key] {
				continue
			}
			seen[key] = true
			e := AppEntry{
				Key:     key,
				Kind:    kind,
				Name:    fpName(a.Name, a.ApplicationID),
				ID:      a.ApplicationID,
				Version: a.Version,
				Origin:  a.Origin,
				Update:  updates[key],
				Busy:    f.env.Jobs.Pending(string(SourceFlatpak), key),
			}
			if e.Update != "" {
				e.Actions = append(e.Actions, ActUpdate)
			}
			e.Actions = append(e.Actions, ActOpen, ActUninstall)
			out = append(out, e)
		}
	}
	if f.systemOn() {
		add(false, f.systemApps)
	}
	if f.userOn() {
		add(true, f.userApps)
	}
	return out
}

// Act does action to the entry with key: an update queues the same job as
// the Updates page's row, Open starts the app, and Uninstall asks first.
func (f *Flatpak) Act(key string, action AppAction) {
	var entry AppEntry
	found := false
	for _, e := range f.Apps() {
		if e.Key == key {
			entry, found = e, true
			break
		}
	}
	if !found {
		return
	}
	switch action {
	case ActUpdate:
		f.queueUpdate(key)
	case ActOpen:
		f.LaunchApp(entry.ID)
	case ActUninstall:
		f.confirmUninstall(entry)
	}
}

// LaunchApp starts the desktop app with the ID appID (a Flatpak app, or
// the configured software center) and does not wait for it; a failure is
// an error toast. It changes nothing on the system, so a dry run launches
// too, as the old page did.
func (f *Flatpak) LaunchApp(appID string) {
	log.Printf("Launching app: %s", appID)
	if err := f.launch(appID); err != nil {
		log.Printf("Failed to launch app %s: %v", appID, err)
		f.env.ToastError(fmt.Sprintf("Failed to launch %s", appID))
	}
}

func (f *Flatpak) confirmUninstall(e AppEntry) {
	if f.env.Jobs.Pending(string(SourceFlatpak), e.Key) {
		return
	}
	user := e.Kind == AppFlatpakUser
	where, note := "the system installation, for everyone on this computer", "Removing an app for everyone may ask for administrator authentication."
	if user {
		where, note = "your user installation", ""
	}
	f.env.Dialogs.Ask(frostui.Confirm{
		Title:       fmt.Sprintf("Uninstall %s?", e.Name),
		Body:        fmt.Sprintf("Flatpak will remove %s from %s. Its data in ~/.var/app stays.", e.ID, where),
		Note:        note,
		Details:     fpConfirmDetails(e),
		Action:      "Uninstall",
		Destructive: true,
		OnConfirm:   func() { f.env.Jobs.Enqueue(f.uninstallJob(e)) },
	})
}

func fpConfirmDetails(e AppEntry) []frostui.KeyValue {
	rows := []frostui.KeyValue{{Key: "ID", Value: e.ID, Mono: true}}
	if e.Version != "" {
		rows = append(rows, frostui.KeyValue{Key: "Version", Value: e.Version})
	}
	if e.Origin != "" {
		rows = append(rows, frostui.KeyValue{Key: "Origin", Value: e.Origin})
	}
	return rows
}

func (f *Flatpak) uninstallJob(e AppEntry) *frostui.Job {
	key, appID, name, user := e.Key, e.ID, e.Name, e.Kind == AppFlatpakUser
	uninstall := f.uninstall
	return &frostui.Job{
		Source: string(SourceFlatpak),
		Key:    key,
		Title:  "Uninstalling " + name,
		Run: func(context.Context, *frostui.Progress) error {
			return uninstall(appID, user)
		},
		Done: func(err error) { f.finishUninstall(key, appID, name, err) },
	}
}

// finishUninstall applies an uninstall's result: a failure or a dry run
// leaves everything as it was; a live success drops the app's update row,
// records it and reloads the installed apps.
//
// An update check in flight began before the uninstall, so it may still
// list the app: a live success starts a newer check, whose generation
// retires the one in flight, so its result cannot bring the row back. The
// newer check, which began after the uninstall, publishes the truth.
func (f *Flatpak) finishUninstall(key, appID, name string, err error) {
	dryRun := f.dryRun()
	decision := actionstate.PackageUninstall(err == nil, dryRun)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			f.env.ToastError(fmt.Sprintf("Uninstall failed: %v", err))
		}
		return
	}
	if !decision.Refresh {
		f.env.Toast(actionmsg.Uninstall(dryRun, appID))
		return
	}
	f.env.ToastSuccess(actionmsg.Uninstall(dryRun, appID))
	f.env.Record(string(SourceFlatpak), "Uninstalled "+name, appID)
	// An app that is gone has nothing to update.
	f.removeUpdate(key)
	if f.updatesChecking {
		f.loadUpdates()
	}
	f.loadApps()
}

// --- Updates ----------------------------------------------------------------

type fpUpdatesResult struct {
	present bool
	updates []flatpak.UpdateInfo
	status  flatpakstatus.Result
	failed  bool
}

// loadUpdates starts a generation-guarded refresh of the update
// inventory. Only the newest request may publish, so a slow reload that a
// later one overtook cannot re-add a row the later one retired. Call it on
// the UI thread so concurrent completions take generations in the order
// they finished.
func (f *Flatpak) loadUpdates() {
	generation := f.flatpakUpdatesRefresh.Begin()
	f.updatesChecking = true
	present, list := f.present, f.listUpdates
	Go(f.env, func() fpUpdatesResult {
		var r fpUpdatesResult
		if r.present = present(); !r.present {
			return r
		}
		// The updates are kept even with an error: a remote failure
		// (*flatpak.RemoteError) still returns the healthy remotes'
		// updates, while an aborted query returns none.
		userUpdates, userErr := list(true)
		if userErr != nil {
			log.Printf("Error loading user flatpak updates: %v", userErr)
		}
		systemUpdates, systemErr := list(false)
		if systemErr != nil {
			log.Printf("Error loading system flatpak updates: %v", systemErr)
		}
		r.updates = append(append(r.updates, userUpdates...), systemUpdates...)
		r.status = flatpakstatus.Subtitle(len(r.updates), userErr != nil, systemErr != nil)
		r.failed = userErr != nil || systemErr != nil
		return r
	}, func(r fpUpdatesResult) { f.applyUpdates(generation, r) })
}

// applyUpdates publishes one update load, while its generation is the
// newest.
func (f *Flatpak) applyUpdates(generation uint64, r fpUpdatesResult) {
	if !f.flatpakUpdatesRefresh.IsCurrent(generation) {
		return
	}
	f.updatesChecking = false
	f.updatesChecked = f.env.Now()
	f.setTool(r.present)
	if !r.present {
		f.updates, f.updatesFailed = nil, false
		f.updatesStatus = flatpakstatus.Result{Subtitle: "Flatpak not installed"}
		return
	}
	f.updatesStatus, f.updatesFailed = r.status, r.failed
	// A load in which no installation returned a usable result learned
	// nothing, so it keeps the last known rows, and with them the count,
	// instead of publishing an empty inventory. The count is the rows'
	// length, so the two cannot drift apart.
	refresh := actionstate.OutdatedRefresh(r.status.Authoritative, len(f.updates), len(r.updates))
	if !refresh.ReplaceRows {
		return
	}
	f.updates = r.updates
}

// removeUpdate retires the update row with key, reporting whether there
// was one.
func (f *Flatpak) removeUpdate(key string) bool {
	for i, u := range f.updates {
		if fpUpdateKey(u) == key {
			f.updates = append(f.updates[:i:i], f.updates[i+1:]...)
			return true
		}
	}
	return false
}

// Source is the Flatpak update source, nil when flatpak_updates_group is
// off.
func (f *Flatpak) Source() UpdateSource {
	if !f.updatesOn() {
		return nil
	}
	return fpSource{f}
}

func (f *Flatpak) sourceStatus() SourceStatus {
	st := SourceStatus{
		Available: f.updatesOn() && f.tool != fpToolMissing,
		Checking:  f.updatesChecking,
		CheckedAt: f.updatesChecked,
	}
	if f.updatesFailed {
		st.Err = f.updatesStatus.Subtitle
	} else {
		st.Note = f.updatesStatus.Subtitle
	}
	return st
}

// items are the update rows, with the installed version when the
// installation's apps are listed.
func (f *Flatpak) items() []UpdateItem {
	if len(f.updates) == 0 {
		return nil
	}
	installed := map[string]string{}
	for _, a := range f.userApps {
		installed[fpKey(true, a.ApplicationID, a.Branch)] = a.Version
	}
	for _, a := range f.systemApps {
		installed[fpKey(false, a.ApplicationID, a.Branch)] = a.Version
	}
	items := make([]UpdateItem, 0, len(f.updates))
	seen := map[string]bool{}
	for _, u := range f.updates {
		key := fpUpdateKey(u)
		if seen[key] {
			continue
		}
		seen[key] = true
		row := pageview.FlatpakUpdate(u.Name, u.ApplicationID, u.NewVersion, u.Installation)
		detail := u.ApplicationID
		if u.Installation == "user" {
			detail += " (user)"
		}
		items = append(items, UpdateItem{
			Key:    key,
			Title:  fpName(row.Title, u.ApplicationID),
			Detail: detail,
			From:   installed[key],
			To:     u.NewVersion,
			Action: "Update",
		})
	}
	return items
}

// updateJobs returns the jobs that update the rows with keys, every row
// for none (no Flatpak row is held or asks first). A row whose job waits
// or runs is left out.
func (f *Flatpak) updateJobs(keys ...string) []*frostui.Job {
	want := map[string]bool{}
	for _, k := range keys {
		want[k] = true
	}
	var jobs []*frostui.Job
	seen := map[string]bool{}
	for _, u := range f.updates {
		key := fpUpdateKey(u)
		if seen[key] || (len(keys) > 0 && !want[key]) || f.env.Jobs.Pending(string(SourceFlatpak), key) {
			continue
		}
		seen[key] = true
		jobs = append(jobs, f.updateJob(key, u))
	}
	return jobs
}

// queueUpdate queues the update of one row, unless its job already waits
// or runs.
func (f *Flatpak) queueUpdate(key string) {
	if jobs := f.updateJobs(key); len(jobs) > 0 {
		f.env.Jobs.Enqueue(jobs...)
	}
}

func (f *Flatpak) updateJob(key string, u flatpak.UpdateInfo) *frostui.Job {
	appID, user, to := u.ApplicationID, u.Installation == "user", u.NewVersion
	name := fpName(u.Name, appID)
	update := f.update
	return &frostui.Job{
		Source: string(SourceFlatpak),
		Key:    key,
		Title:  "Updating " + name,
		Run: func(context.Context, *frostui.Progress) error {
			return update(appID, user)
		},
		Done: func(err error) { f.finishUpdate(key, appID, name, to, err) },
	}
}

// finishUpdate applies one update's result on the UI thread. The row must
// not depend on the reload: a reload in which no installation returned a
// usable result keeps the old rows. So a live success retires the row now
// and then reloads; a failure or a dry run leaves the row, whose button
// comes back as its job ended, and does not reload.
func (f *Flatpak) finishUpdate(key, appID, name, to string, err error) {
	dryRun := f.dryRun()
	decision := actionstate.PackageUpgrade(err == nil, dryRun)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			f.env.ToastError(fmt.Sprintf("Update failed: %v", err))
		}
		return
	}
	if decision.RemoveRow {
		f.removeUpdate(key)
		detail := appID
		if to != "" {
			detail += " → " + to
		}
		f.env.Record(string(SourceFlatpak), "Updated "+name, detail)
		f.env.ToastSuccess(actionmsg.Update(dryRun, appID))
	} else {
		f.env.Toast(actionmsg.Update(dryRun, appID))
	}
	// Reload on the UI thread, so concurrent completions take
	// generations in the order they finished. The installed apps reload
	// too: the Apps table shows the new version.
	if decision.Refresh {
		f.loadUpdates()
		if f.userOn() || f.systemOn() {
			f.loadApps()
		}
	}
}

// fpSource is the Flatpak update source of the Updates page.
type fpSource struct{ f *Flatpak }

func (s fpSource) ID() SourceID { return SourceFlatpak }
func (s fpSource) Title() string {
	return "Flatpak"
}
func (s fpSource) Icon() string { return "system-software-install" }
func (s fpSource) Blurb() string {
	return "Apps from Flatpak remotes such as Flathub, for you and for everyone on this computer."
}
func (s fpSource) Status() SourceStatus { return s.f.sourceStatus() }
func (s fpSource) Items() []UpdateItem  { return s.f.items() }

// Refresh starts a check, unless one runs.
func (s fpSource) Refresh() {
	if !s.f.updatesOn() || s.f.updatesChecking {
		return
	}
	s.f.loadUpdates()
}

func (s fpSource) Jobs(keys ...string) []*frostui.Job { return s.f.updateJobs(keys...) }
func (s fpSource) Update(key string)                  { s.f.queueUpdate(key) }
func (s fpSource) Extra(*ui.Context)                  {}

// --- Keys and names ---------------------------------------------------------

// fpKey is the key of an installed app or of its update, shared by the
// Apps table and the Updates page so that a job shows on both:
// "flatpak:user:<id>" or "flatpak:system:<id>", with "//<branch>" for a
// branch other than stable, as a second branch of the same app needs.
func fpKey(user bool, appID, branch string) string {
	installation := "system"
	if user {
		installation = "user"
	}
	key := "flatpak:" + installation + ":" + appID
	if branch != "" && branch != "stable" {
		key += "//" + branch
	}
	return key
}

func fpUpdateKey(u flatpak.UpdateInfo) string {
	return fpKey(u.Installation == "user", u.ApplicationID, u.Branch)
}

// fpName is an app's name, or its ID when flatpak lists none.
func fpName(name, appID string) string {
	if strings.TrimSpace(name) == "" {
		return appID
	}
	return name
}
