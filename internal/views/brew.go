package views

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/frostyard/chairlift/internal/frostui"
	"github.com/frostyard/chairlift/internal/homebrew"
	"github.com/frostyard/chairlift/internal/navigation"
	"github.com/frostyard/chairlift/internal/views/actionmsg"
	"github.com/frostyard/chairlift/internal/views/actionstate"
)

// The configuration groups of Homebrew. Each one is independent of the
// others ("Config-driven visibility is real"): brew_bundles_group works
// without brew_group, brew_trust_group without brew_updates_group.
const (
	brewAppsPage    = "applications_page"
	brewUpdatesPage = "updates_page"

	groupBrew        = "brew_group"         // installed formulae and casks, uninstall, pin, Brewfile export
	groupBrewSearch  = "brew_search_group"  // search and install
	groupBrewBundles = "brew_bundles_group" // the configured Brewfile bundles
	groupBrewUpdates = "brew_updates_group" // outdated packages, upgrades, brew update
	groupBrewTrust   = "brew_trust_group"   // untrusted taps and brew trust
)

// brewJobSource is the Source of every Homebrew job and activity entry.
const brewJobSource = string(SourceBrew)

// brewBackend is everything Brew asks of Homebrew. The constructor fills it
// with internal/homebrew; tests put fakes in. Every function runs on a
// worker goroutine, never on the UI thread: even IsInstalledCached may run
// `brew --version` the first time.
type brewBackend struct {
	installed func() bool
	dryRun    func() bool

	formulae  func() ([]homebrew.Package, error)
	casks     func() ([]homebrew.Package, error)
	outdated  func() ([]homebrew.Package, error)
	untrusted func() ([]homebrew.UntrustedTap, error)
	search    func(query string) ([]homebrew.SearchResult, error)

	install   func(name string, cask bool) error
	uninstall func(name string, cask bool) error
	upgrade   func(name string) error
	update    func() error
	pin       func(name string) error
	unpin     func(name string) error
	// trust runs `brew trust` for a tap's packages: per user, never pkexec.
	trust func(tap homebrew.UntrustedTap) error

	bundles       func(paths []string) ([]homebrew.Bundle, error)
	bundleInstall func(path string) error
	bundleDump    func(path string, force bool) error
	// readBrewfile reads a bundle's Brewfile to list its packages.
	readBrewfile func(path string) (string, error)
	home         func() (string, error)
}

func homebrewBackend() brewBackend {
	return brewBackend{
		installed:     homebrew.IsInstalledCached,
		dryRun:        homebrew.IsDryRun,
		formulae:      homebrew.ListInstalledFormulae,
		casks:         homebrew.ListInstalledCasks,
		outdated:      homebrew.ListOutdated,
		untrusted:     homebrew.ListUntrustedTaps,
		search:        homebrew.Search,
		install:       homebrew.Install,
		uninstall:     homebrew.Uninstall,
		upgrade:       homebrew.Upgrade,
		update:        homebrew.Update,
		pin:           homebrew.Pin,
		unpin:         homebrew.Unpin,
		trust:         homebrew.TrustPackages,
		bundles:       homebrew.AvailableBundles,
		bundleInstall: homebrew.BundleInstall,
		bundleDump:    homebrew.BundleDump,
		readBrewfile:  readBrewfile,
		home:          os.UserHomeDir,
	}
}

// maxBrewfileBytes caps how much of a bundle's Brewfile is read to list its
// packages.
const maxBrewfileBytes = 256 << 10

func readBrewfile(path string) (string, error) {
	f, err := os.Open(path) //nolint:gosec // a Brewfile homebrew.AvailableBundles found in a configured directory
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxBrewfileBytes))
	return string(data), err
}

// brewPresence is whether Homebrew is installed, as far as Brew knows.
type brewPresence int

const (
	brewUnknown brewPresence = iota
	brewInstalled
	brewMissing
)

// brewAction is the state of one action control: an installed package's
// row (pin, unpin, uninstall), an outdated package's upgrade, a search
// result's install, a tap's trust. Its gate keeps two actions from
// overlapping; running and completed are what the control says meanwhile.
type brewAction struct {
	gate actionstate.Gate
	// taken is true from the click that took the gate (its confirmation
	// open) until the action ends.
	taken bool
	// running labels the action in flight, as "Uninstalling…"; "" while
	// none runs.
	running string
	// completed labels an action that succeeded live, as "Uninstalled":
	// the control stays closed until a reload that loaded the row's list
	// replaces the row (brewKeepRows keeps it over a failed one).
	completed string
}

// take starts an action: it reports false while another action of the
// control is asked, runs, or succeeded.
func (a *brewAction) take() bool {
	if !a.gate.TryStart() {
		return false
	}
	a.taken = true
	return true
}

// inUse reports whether the control is closed: an action is asked, runs
// or ended.
func (a *brewAction) inUse() bool { return a.taken || a.completed != "" }

// restore reopens the control after a cancel, a failure or a preview.
func (a *brewAction) restore() {
	a.gate.Reset()
	a.taken = false
	a.running = ""
}

// complete closes the control for good after a live success.
func (a *brewAction) complete(label string) {
	a.gate.Complete()
	a.taken = false
	a.running = ""
	a.completed = label
}

// brewKeepRunning carries actions over a reload of their rows: one asked or
// in flight stays, so the reloaded row shares its gate and still shows it
// running; a finished one is dropped, so the reloaded row starts fresh.
func brewKeepRunning(old map[string]*brewAction) map[string]*brewAction {
	next := make(map[string]*brewAction, len(old))
	for key, a := range old {
		if a.taken {
			next[key] = a
		}
	}
	return next
}

// brewActionFor returns the action of key in m, making it.
func brewActionFor(m map[string]*brewAction, key string) *brewAction {
	a := m[key]
	if a == nil {
		a = &brewAction{}
		m[key] = a
	}
	return a
}

// Brew holds Homebrew's packages, taps and bundles and the Homebrew update
// source. It belongs to the UI thread; its loaders and jobs call Homebrew on
// worker goroutines and hand the results back through Env.Post.
type Brew struct {
	env *Env
	be  brewBackend

	have brewPresence

	inv     brewInventory // brew_group: installed formulae and casks
	updates brewUpdates   // brew_updates_group and brew_trust_group
	search  brewSearch    // brew_search_group
	bundles brewBundles   // brew_bundles_group

	// export is the Brewfile export (brew_group).
	export brewAction

	src *brewSource

	// searchRequested is set by the "Search Homebrew…" command until the
	// Apps page takes it to show its Find more tab.
	searchRequested bool
}

// NewBrew returns the Homebrew state. Nothing runs until Start.
func NewBrew(env *Env) *Brew {
	return newBrew(env, homebrewBackend())
}

func newBrew(env *Env, be brewBackend) *Brew {
	b := &Brew{env: env, be: be}
	b.inv.rows = map[string]*brewAction{}
	b.updates.upgrades = map[string]*brewAction{}
	b.updates.trusts = map[string]*brewAction{}
	b.updates.dismissed = map[string]bool{}
	b.search.rows = map[string]*brewAction{}
	b.src = &brewSource{b: b}
	return b
}

func (b *Brew) enabled(page, group string) bool { return b.env.Enabled(page, group) }

// anyEnabled reports whether any Homebrew group is on.
func (b *Brew) anyEnabled() bool {
	return b.enabled(brewAppsPage, groupBrew) || b.enabled(brewAppsPage, groupBrewSearch) ||
		b.enabled(brewAppsPage, groupBrewBundles) || b.enabled(brewUpdatesPage, groupBrewUpdates) ||
		b.enabled(brewUpdatesPage, groupBrewTrust)
}

// Start checks for Homebrew on a worker, then loads what the enabled groups
// show: outdated packages, untrusted taps, installed packages, bundles.
func (b *Brew) Start() {
	if !b.anyEnabled() {
		return
	}
	installed := b.be.installed
	Go(b.env, installed, func(ok bool) {
		b.setPresence(ok)
		if ok {
			b.loadOutdated(nil)
			b.loadTaps()
			b.RefreshApps()
		}
		// Bundles show without Homebrew too, with their installs off,
		// as bundleview.Present says.
		b.loadBundles()
	})
}

func (b *Brew) setPresence(installed bool) {
	if installed {
		b.have = brewInstalled
	} else {
		b.have = brewMissing
	}
}

// missing reports whether Homebrew is known to be absent.
func (b *Brew) missing() bool { return b.have == brewMissing }

// dryRun is the backend's dry-run flag, read once per decision as the old
// pages did with homebrew.IsDryRun.
func (b *Brew) dryRun() bool { return b.be.dryRun() }

// notify shows a finished action's message: a preview in dry run, a success
// otherwise.
func (b *Brew) notify(dryRun bool, message string) {
	if dryRun {
		b.env.Toast(message)
		return
	}
	b.env.ToastSuccess(message)
}

// brewCanceled reports whether a job ended because the queue was canceled.
func brewCanceled(err error) bool { return errors.Is(err, context.Canceled) }

// enqueue queues one Homebrew job.
func (b *Brew) enqueue(key, title string, run func() error, done func(err error)) {
	b.env.Run(brewJobSource, key, title, func(context.Context, *frostui.Progress) error { return run() }, done)
}

// pending reports whether a Homebrew job of key waits or runs.
func (b *Brew) pending(key string) bool {
	return b.env.Jobs != nil && b.env.Jobs.Pending(brewJobSource, key)
}

// brewKey is an installed package's key in the Apps table, and the key of
// its jobs: "formula:gh", "cask:firefox".
func brewKey(kind homebrew.PackageKind, name string) string {
	return string(kind) + ":" + name
}

// parseBrewKey splits a brewKey.
func parseBrewKey(key string) (homebrew.PackageKind, string, bool) {
	kind, name, ok := strings.Cut(key, ":")
	if !ok || name == "" || (kind != string(homebrew.Formula) && kind != string(homebrew.Cask)) {
		return "", "", false
	}
	return homebrew.PackageKind(kind), name, true
}

// TakeSearchRequest reports, once, that the "Search Homebrew…" command ran,
// so the Apps page shows its Find more tab.
func (b *Brew) TakeSearchRequest() bool {
	r := b.searchRequested
	b.searchRequested = false
	return r
}

// ExportAvailable reports whether the installed packages can be exported
// to ~/Brewfile now (brew_group, Homebrew installed).
func (b *Brew) ExportAvailable() bool {
	return b.enabled(brewAppsPage, groupBrew) && b.have == brewInstalled
}

// ExportBrewfile writes every installed package to ~/Brewfile with `brew
// bundle dump --force`, after asking: the file is replaced.
func (b *Brew) ExportBrewfile() {
	if !b.ExportAvailable() || !b.export.take() {
		return
	}
	home, err := b.be.home()
	if err != nil {
		b.export.restore()
		b.env.ToastError(fmt.Sprintf("Bundle dump failed: %v", err))
		return
	}
	path := filepath.Join(home, "Brewfile")
	b.env.Dialogs.Ask(frostui.Confirm{
		Title:     "Export installed packages?",
		Body:      fmt.Sprintf("Homebrew writes every installed formula, cask and tap to %s, replacing the file if it exists.", path),
		Action:    "Export",
		OnCancel:  b.export.restore,
		OnConfirm: func() { b.runExport(path) },
	})
}

func (b *Brew) runExport(path string) {
	b.export.running = "Exporting…"
	dump := b.be.bundleDump
	b.enqueue("brewfile", "Exporting Homebrew packages to "+path, func() error { return dump(path, true) }, func(err error) {
		b.export.restore()
		if brewCanceled(err) {
			return
		}
		if err != nil {
			b.env.ToastError(fmt.Sprintf("Bundle dump failed: %v", err))
			return
		}
		dry := b.dryRun()
		b.notify(dry, actionmsg.BundleDump(dry, path))
		b.env.Record(brewJobSource, "Exported Homebrew packages", path)
	})
}

// Commands are Homebrew's palette commands: update the metadata, trust each
// untrusted tap, search, upgrade everything, export, install a bundle. Each
// is behind the group of the control it stands for.
func (b *Brew) Commands() []frostui.Command {
	if b.missing() {
		return nil
	}
	const group = "Homebrew"
	var cmds []frostui.Command
	if b.enabled(brewUpdatesPage, groupBrewUpdates) {
		c := frostui.Command{ID: "brew.metadata", Title: "Update Homebrew metadata", Group: group,
			Icon: "view-refresh", Hint: "brew update formulae definitions", Run: b.UpdateMetadata}
		switch {
		case b.have != brewInstalled:
			c.Disabled, c.Reason = true, "checking"
		case b.updates.meta.inUse():
			c.Disabled, c.Reason = true, "running"
		}
		cmds = append(cmds, c)

		jobs := b.src.upgradable(nil)
		up := frostui.Command{ID: "brew.upgrade-all", Title: "Upgrade all Homebrew packages", Group: group,
			Icon: "software-update-available", Hint: "brew upgrade outdated", Run: b.UpgradeAll}
		if len(jobs) == 0 {
			up.Disabled, up.Reason = true, "nothing to upgrade"
		}
		cmds = append(cmds, up)
	}
	if b.enabled(brewUpdatesPage, groupBrewTrust) {
		for _, tap := range b.updates.taps {
			name := tap.Name
			c := frostui.Command{ID: "brew.trust." + name, Title: "Trust " + name, Group: group,
				Icon: "changes-allow", Hint: "brew trust tap untrusted", Run: func() { b.AskTrust(name) }}
			if a := b.updates.trusts[name]; a != nil && a.inUse() {
				c.Disabled, c.Reason = true, "running"
			}
			cmds = append(cmds, c)
		}
	}
	if b.SearchAvailable() {
		cmds = append(cmds, frostui.Command{ID: "brew.search", Title: "Search Homebrew…", Group: group,
			Icon: "edit-find", Hint: "find install formula cask", Run: func() {
				b.searchRequested = true
				b.search.focus = true
				b.env.Navigate(navigation.Apps)
			}})
	}
	if b.ExportAvailable() {
		c := frostui.Command{ID: "brew.export", Title: "Export Homebrew packages to ~/Brewfile", Group: group,
			Icon: "document-save", Hint: "brew bundle dump", Run: b.ExportBrewfile}
		if b.export.inUse() {
			c.Disabled, c.Reason = true, "running"
		}
		cmds = append(cmds, c)
	}
	if b.BundlesAvailable() && b.have == brewInstalled {
		for _, bd := range b.bundles.items {
			c := frostui.Command{ID: "brew.bundle." + bd.Path, Title: "Install the " + bd.Name + " bundle", Group: group,
				Icon: "system-software-install", Hint: "brew bundle Brewfile " + bd.Description, Run: func() { b.AskBundle(bd.Path) }}
			if bd.running || bd.installed {
				c.Disabled, c.Reason = true, "installed"
				if bd.running {
					c.Reason = "running"
				}
			}
			cmds = append(cmds, c)
		}
	}
	return cmds
}
