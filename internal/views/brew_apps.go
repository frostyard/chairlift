package views

import (
	"fmt"
	"strings"

	"github.com/frostyard/chairlift/internal/frostui"
	"github.com/frostyard/chairlift/internal/homebrew"
	"github.com/frostyard/chairlift/internal/views/actionmsg"
	"github.com/frostyard/chairlift/internal/views/actionstate"
)

// brewInventory is the installed formulae and casks (brew_group).
type brewInventory struct {
	formulae, casks []homebrew.Package
	// formulaeErr and casksErr are the last failed list's error, "" after
	// a list that worked. A failed list keeps the rows it had.
	formulaeErr, casksErr string
	// loaded is true once a load ended, even one that failed.
	loaded bool

	// refresh orders the reloads: only the newest one publishes.
	refresh actionstate.RefreshGate
	loading uint64 // the generation in flight, 0 when none

	// rows are the installed packages' controls by key ("formula:gh"):
	// one gate per row, shared by its pin, unpin, uninstall and update.
	rows map[string]*brewAction
}

func (inv *brewInventory) find(kind homebrew.PackageKind, name string) (homebrew.Package, bool) {
	list := inv.formulae
	if kind == homebrew.Cask {
		list = inv.casks
	}
	for _, p := range list {
		if p.Name == name {
			return p, true
		}
	}
	return homebrew.Package{}, false
}

// kinds maps each installed name to its kind, for the Updates page's
// detail line: brew outdated does not say.
func (inv *brewInventory) kinds() map[string]homebrew.PackageKind {
	m := make(map[string]homebrew.PackageKind, len(inv.formulae)+len(inv.casks))
	for _, p := range inv.casks {
		m[p.Name] = homebrew.Cask
	}
	for _, p := range inv.formulae {
		m[p.Name] = homebrew.Formula
	}
	return m
}

// installedKey reports whether a package of kind and name is installed, as
// the last inventory load found.
func (b *Brew) installedKey(kind homebrew.PackageKind, name string) bool {
	_, ok := b.inv.find(kind, name)
	return ok
}

// rowInUse reports whether an installed row named name has an action in
// flight or done: the upgrade of that name waits for it.
func (b *Brew) rowInUse(name string) bool {
	for _, kind := range []homebrew.PackageKind{homebrew.Formula, homebrew.Cask} {
		if a := b.inv.rows[brewKey(kind, name)]; a != nil && a.inUse() {
			return true
		}
	}
	return false
}

// upgradeInUse reports whether the upgrade of name runs or ended.
func (b *Brew) upgradeInUse(name string) bool {
	a := b.updates.upgrades[name]
	return (a != nil && a.inUse()) || b.pending(name)
}

// AppsStatus is the state of the installed-packages load.
func (b *Brew) AppsStatus() AppsStatus {
	if !b.enabled(brewAppsPage, groupBrew) || b.missing() {
		return AppsStatus{}
	}
	st := AppsStatus{Available: true, Loading: !b.inv.loaded || b.inv.loading != 0}
	var errs []string
	if b.inv.formulaeErr != "" {
		errs = append(errs, "Could not list formulae: "+b.inv.formulaeErr)
	}
	if b.inv.casksErr != "" {
		errs = append(errs, "Could not list casks: "+b.inv.casksErr)
	}
	st.Err = strings.Join(errs, "; ")
	return st
}

// Apps are the installed formulae and casks, for the Apps table.
func (b *Brew) Apps() []AppEntry {
	if !b.enabled(brewAppsPage, groupBrew) || b.missing() {
		return nil
	}
	taps := b.updates.untrustedPackages()
	outdated := map[string]string{} // name → the version an upgrade brings
	if b.enabled(brewUpdatesPage, groupBrewUpdates) {
		for _, p := range b.updates.outdated {
			outdated[p.Name] = p.Latest
			if outdated[p.Name] == "" {
				outdated[p.Name] = "newer" // brew did not name it
			}
		}
	}
	out := make([]AppEntry, 0, len(b.inv.formulae)+len(b.inv.casks))
	add := func(kind homebrew.PackageKind, p homebrew.Package) {
		key := brewKey(kind, p.Name)
		e := AppEntry{Key: key, Kind: AppFormula, Name: p.Name, ID: p.Name, Version: p.Version,
			Origin: taps[p.Name], Pinned: p.Pinned}
		if kind == homebrew.Cask {
			e.Kind = AppCask
		}
		row := b.inv.rows[key]
		upgrading := b.upgradeInUse(p.Name)
		e.Busy = b.pending(key) || upgrading || (row != nil && row.inUse())
		if latest, ok := outdated[p.Name]; ok {
			e.Update = latest
		}
		if row != nil && row.completed != "" {
			out = append(out, e)
			return
		}
		if e.Update != "" && !p.Pinned && taps[p.Name] == "" && !upgrading {
			e.Actions = append(e.Actions, ActUpdate)
		}
		if kind == homebrew.Formula {
			if p.Pinned {
				e.Actions = append(e.Actions, ActUnpin)
			} else {
				e.Actions = append(e.Actions, ActPin)
			}
		}
		e.Actions = append(e.Actions, ActUninstall)
		out = append(out, e)
	}
	for _, p := range b.inv.formulae {
		add(homebrew.Formula, p)
	}
	for _, p := range b.inv.casks {
		add(homebrew.Cask, p)
	}
	return out
}

// StateLabel says what happens to the entry or update item with key now:
// "Uninstalling…", "Pinned", "Upgrading…"; "" when nothing does.
func (b *Brew) StateLabel(key string) string {
	if a := b.inv.rows[key]; a != nil && a.inUse() {
		if a.running != "" {
			return a.running
		}
		return a.completed
	}
	name := key
	if _, n, ok := parseBrewKey(key); ok {
		name = n
	}
	if a := b.updates.upgrades[name]; a != nil && a.inUse() {
		if a.running != "" {
			return a.running
		}
		return a.completed
	}
	return ""
}

// RefreshApps reloads the installed formulae and then the casks, under one
// generation: a reload that a newer one overtook publishes nothing. A list
// that fails keeps the rows it had and reports its error.
func (b *Brew) RefreshApps() {
	if !b.enabled(brewAppsPage, groupBrew) {
		return
	}
	generation := b.inv.refresh.Begin()
	b.inv.loading = generation
	be := b.be
	type result struct {
		missing                bool
		formulae, casks        []homebrew.Package
		formulaeErr, casksErrs error
	}
	Go(b.env, func() result {
		if !be.installed() {
			return result{missing: true}
		}
		var r result
		r.formulae, r.formulaeErr = be.formulae()
		r.casks, r.casksErrs = be.casks()
		return r
	}, func(r result) {
		if !b.inv.refresh.IsCurrent(generation) {
			return
		}
		b.inv.loading = 0
		if r.missing {
			b.have = brewMissing
			return
		}
		b.have = brewInstalled
		if r.formulaeErr != nil {
			b.inv.formulaeErr = r.formulaeErr.Error()
		} else {
			b.inv.formulae, b.inv.formulaeErr = r.formulae, ""
		}
		if r.casksErrs != nil {
			b.inv.casksErr = r.casksErrs.Error()
		} else {
			b.inv.casks, b.inv.casksErr = r.casks, ""
		}
		b.inv.loaded = true
		b.inv.rows = brewKeepRunning(b.inv.rows)
	})
}

// Act does action to the installed package with key, asking first as the
// old Applications page did: pin, unpin and uninstall confirm; an update
// runs the upgrade the Updates page runs.
func (b *Brew) Act(key string, act AppAction) {
	if !b.enabled(brewAppsPage, groupBrew) || b.have != brewInstalled {
		return
	}
	kind, name, ok := parseBrewKey(key)
	if !ok {
		return
	}
	p, found := b.inv.find(kind, name)
	if !found {
		return
	}
	switch act {
	case ActUpdate:
		if b.rowInUse(name) {
			return
		}
		b.src.Update(name)
	case ActPin, ActUnpin:
		pin := act == ActPin
		if kind != homebrew.Formula || p.Pinned == pin {
			return
		}
		b.askPin(name, pin)
	case ActUninstall:
		b.askUninstall(kind, name)
	}
}

func (b *Brew) askPin(name string, pin bool) {
	key := brewKey(homebrew.Formula, name)
	row := brewActionFor(b.inv.rows, key)
	if b.upgradeInUse(name) || !row.take() {
		return
	}
	verb := "Unpin"
	description := "Unpinned formulae resume receiving upgrades."
	if pin {
		verb = "Pin"
		description = "Pinned formulae are skipped by Homebrew upgrades until they are unpinned."
	}
	b.env.Dialogs.Ask(frostui.Confirm{
		Title:     fmt.Sprintf("%s %s?", verb, name),
		Body:      description,
		Action:    verb,
		OnCancel:  row.restore,
		OnConfirm: func() { b.runPin(name, pin, row) },
	})
}

func (b *Brew) runPin(name string, pin bool, row *brewAction) {
	verb, completed, errorPrefix := "Unpin", "Unpinned", "Unpin failed"
	run := b.be.unpin
	if pin {
		verb, completed, errorPrefix = "Pin", "Pinned", "Pin failed"
		run = b.be.pin
	}
	row.running = verb + "ning…"
	b.enqueue(brewKey(homebrew.Formula, name), fmt.Sprintf("%sning %s", verb, name), func() error { return run(name) }, func(err error) {
		dryRun := b.dryRun()
		decision := actionstate.PackagePin(err == nil, dryRun)
		if b.finishMutation(row, decision, err, errorPrefix, actionmsg.Pin(dryRun, name, pin), completed, dryRun) {
			b.env.Record(brewJobSource, completed+" "+name, "")
		}
	})
}

func (b *Brew) askUninstall(kind homebrew.PackageKind, name string) {
	key := brewKey(kind, name)
	row := brewActionFor(b.inv.rows, key)
	if b.upgradeInUse(name) || !row.take() {
		return
	}
	b.env.Dialogs.Ask(frostui.Confirm{
		Title:       fmt.Sprintf("Uninstall %s?", name),
		Body:        fmt.Sprintf("Homebrew will remove the %s %s and its package-managed files.", strings.ToLower(kind.DisplayName()), name),
		Action:      "Uninstall",
		Destructive: true,
		OnCancel:    row.restore,
		OnConfirm:   func() { b.runUninstall(kind, name, row) },
	})
}

func (b *Brew) runUninstall(kind homebrew.PackageKind, name string, row *brewAction) {
	row.running = "Uninstalling…"
	uninstall := b.be.uninstall
	cask := kind == homebrew.Cask
	b.enqueue(brewKey(kind, name), "Uninstalling "+name, func() error { return uninstall(name, cask) }, func(err error) {
		dryRun := b.dryRun()
		decision := actionstate.PackageUninstall(err == nil, dryRun)
		if b.finishMutation(row, decision, err, "Uninstall failed", actionmsg.Uninstall(dryRun, name), "Uninstalled", dryRun) {
			b.env.Record(brewJobSource, "Uninstalled "+name, strings.ToLower(kind.DisplayName()))
		}
	})
}

// finishMutation applies an installed row's action result, as the old
// finishHomebrewPackageMutation did: a failure or a preview reopens the
// row's controls, a live success closes them and reloads the inventory (and
// the outdated list, which a pin or an uninstall changes). It reports a
// live success.
func (b *Brew) finishMutation(row *brewAction, decision actionstate.Decision, err error, errorPrefix, toast, completed string, dryRun bool) bool {
	if brewCanceled(err) {
		row.restore()
		return false
	}
	if decision.RestoreControl {
		row.restore()
	}
	if err != nil {
		b.env.ToastError(fmt.Sprintf("%s: %v", errorPrefix, err))
		return false
	}
	if decision.CompleteControl {
		row.complete(completed)
	}
	b.notify(dryRun, toast)
	if decision.Refresh {
		b.RefreshApps()
		b.loadOutdated(nil)
	}
	return decision.CompleteControl
}
