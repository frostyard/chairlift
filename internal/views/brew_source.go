package views

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/chairlift/internal/frostui"
	"github.com/frostyard/chairlift/internal/homebrew"
	"github.com/frostyard/chairlift/internal/views/actionmsg"
	"github.com/frostyard/chairlift/internal/views/actionstate"
	"github.com/frostyard/chairlift/internal/views/pageview"
	"github.com/frostyard/chairlift/internal/views/trustmsg"
)

// brewUpdates is the Homebrew update source's state: the outdated packages
// (brew_updates_group) and the untrusted taps (brew_trust_group).
type brewUpdates struct {
	outdated []homebrew.Package
	// known is true once an outdated check worked.
	known bool
	// err is the last outdated check's failure; that check kept the rows
	// and count it had.
	err     string
	checked time.Time
	refresh actionstate.RefreshGate
	loading uint64 // the generation in flight, 0 when none
	// upgrades are the outdated packages' upgrade controls, by name.
	upgrades map[string]*brewAction

	// meta is the "brew update" control.
	meta brewAction

	taps        []homebrew.UntrustedTap
	tapsRefresh actionstate.RefreshGate
	tapsLoading uint64
	// trusts are the taps' trust controls, by tap name.
	trusts map[string]*brewAction
	// dismissed are the taps whose banner the user closed, until the next
	// check.
	dismissed map[string]bool
}

// untrustedPackages maps the short name of every installed package of an
// untrusted tap to that tap.
func (u *brewUpdates) untrustedPackages() map[string]string {
	m := map[string]string{}
	for _, t := range u.taps {
		for _, list := range [][]string{t.Formulae, t.Casks} {
			for _, q := range list {
				m[pageview.BrewShortName(q)] = t.Name
				m[q] = t.Name
			}
		}
	}
	return m
}

func (u *brewUpdates) findTap(name string) (homebrew.UntrustedTap, bool) {
	for _, t := range u.taps {
		if t.Name == name {
			return t, true
		}
	}
	return homebrew.UntrustedTap{}, false
}

func (u *brewUpdates) removeTap(name string) {
	out := u.taps[:0]
	for _, t := range u.taps {
		if t.Name != name {
			out = append(out, t)
		}
	}
	u.taps = out
}

// removeOutdated drops the outdated package name and reports whether it was
// there: an upgrade that a reload already retired removes nothing.
func (u *brewUpdates) removeOutdated(name string) bool {
	for i, p := range u.outdated {
		if p.Name == name {
			u.outdated = append(u.outdated[:i:i], u.outdated[i+1:]...)
			return true
		}
	}
	return false
}

// sourceEnabled reports whether the Updates page has a Homebrew card: its
// outdated packages or its untrusted taps.
func (b *Brew) sourceEnabled() bool {
	return b.enabled(brewUpdatesPage, groupBrewUpdates) || b.enabled(brewUpdatesPage, groupBrewTrust)
}

// Source is the Homebrew update source, nil when neither of its groups is
// enabled. Its Status says whether Homebrew is installed.
func (b *Brew) Source() UpdateSource {
	if !b.sourceEnabled() {
		return nil
	}
	return b.src
}

// loadOutdated starts a versioned reload of the outdated packages. Only the
// newest reload publishes; done, when not nil, runs for every reload, the
// superseded ones too, so an action waiting on it never stays closed. It is
// reachable from a tap's trust (brew_trust_group) and from the Apps page's
// actions (brew_group), so it checks its own group first.
func (b *Brew) loadOutdated(done func(bool)) {
	finish := func(ok bool) {
		if done != nil {
			done(ok)
		}
	}
	if !b.enabled(brewUpdatesPage, groupBrewUpdates) {
		finish(false)
		return
	}
	generation := b.updates.refresh.Begin()
	b.updates.loading = generation
	be := b.be
	type result struct {
		missing  bool
		packages []homebrew.Package
		err      error
	}
	Go(b.env, func() result {
		if !be.installed() {
			return result{missing: true}
		}
		packages, err := be.outdated()
		return result{packages: packages, err: err}
	}, func(r result) {
		if !b.updates.refresh.IsCurrent(generation) {
			finish(false)
			return
		}
		u := &b.updates
		u.loading = 0
		u.checked = b.env.Now()
		if r.missing {
			b.have = brewMissing
			u.outdated, u.known = nil, false
			finish(false)
			return
		}
		b.have = brewInstalled
		refresh := actionstate.OutdatedRefresh(r.err == nil, len(u.outdated), len(r.packages))
		if r.err != nil {
			// The rows and the count stay as the last check found them.
			u.err = fmt.Sprintf("Error refreshing updates: %v", r.err)
			finish(false)
			return
		}
		u.err = ""
		if refresh.ReplaceRows {
			u.outdated, u.known = r.packages, true
			u.upgrades = brewKeepRunning(u.upgrades)
		}
		finish(true)
	})
}

// loadTaps reloads the untrusted taps that have installed packages. A
// failed check is logged and keeps the taps it had, as the old page did.
func (b *Brew) loadTaps() {
	if !b.enabled(brewUpdatesPage, groupBrewTrust) {
		return
	}
	generation := b.updates.tapsRefresh.Begin()
	b.updates.tapsLoading = generation
	be := b.be
	type result struct {
		missing bool
		taps    []homebrew.UntrustedTap
		err     error
	}
	Go(b.env, func() result {
		if !be.installed() {
			return result{missing: true}
		}
		taps, err := be.untrusted()
		return result{taps: taps, err: err}
	}, func(r result) {
		if !b.updates.tapsRefresh.IsCurrent(generation) {
			return
		}
		u := &b.updates
		u.tapsLoading = 0
		u.checked = b.env.Now()
		if r.missing {
			b.have = brewMissing
			return
		}
		b.have = brewInstalled
		if r.err != nil {
			log.Printf("untrusted tap check failed: %v", r.err)
			return
		}
		u.taps = r.taps
		u.trusts = brewKeepRunning(u.trusts)
	})
}

// UpgradeAll queues an upgrade of every outdated package that is not held.
func (b *Brew) UpgradeAll() {
	jobs := b.src.Jobs()
	if len(jobs) == 0 {
		b.env.Toast("Nothing to upgrade")
		return
	}
	b.env.Jobs.Enqueue(jobs...)
}

// upgradeJob returns the job that upgrades name, or nil when its upgrade or
// its installed row's action is already in use.
func (b *Brew) upgradeJob(name, from string) *frostui.Job {
	a := brewActionFor(b.updates.upgrades, name)
	if b.rowInUse(name) || !a.take() {
		return nil
	}
	a.running = "Upgrading…"
	upgrade := b.be.upgrade
	return &frostui.Job{
		Source: brewJobSource,
		Key:    name,
		Title:  "Upgrading " + name,
		Run:    func(context.Context, *frostui.Progress) error { return upgrade(name) },
		Done:   func(err error) { b.finishUpgrade(name, from, a, err) },
	}
}

// finishUpgrade applies an upgrade's result as the old outdated row did: a
// failure and a preview reopen the control and keep the row; a live success
// removes the row, which lowers the count, and reloads the outdated list.
func (b *Brew) finishUpgrade(name, from string, a *brewAction, err error) {
	if brewCanceled(err) {
		a.restore()
		return
	}
	dryRun := b.dryRun()
	decision := actionstate.PackageUpgrade(err == nil, dryRun)
	if err != nil {
		msg := fmt.Sprintf("Upgrade failed: %v", err)
		var trustErr *homebrew.UntrustedTapError
		if errors.As(err, &trustErr) {
			msg = trustmsg.UpgradeMessage(name, b.enabled(brewUpdatesPage, groupBrewTrust))
			// The tap's banner shows what to trust.
			b.loadTaps()
		}
		if decision.RestoreControl {
			a.restore()
		}
		b.env.ToastError(msg)
		return
	}
	b.notify(dryRun, actionmsg.Upgrade(dryRun, name))
	if decision.RemoveRow {
		a.complete("Upgraded")
		b.updates.removeOutdated(name)
		detail := ""
		if from != "" {
			detail = "from " + from
		}
		b.env.Record(brewJobSource, "Upgraded "+name, detail)
	}
	if decision.RestoreControl {
		a.restore()
	}
	if decision.Refresh {
		b.loadOutdated(nil)
		// The installed versions changed; reload them once no other
		// upgrade waits.
		if !b.upgrading() {
			b.RefreshApps()
		}
	}
}

// upgrading reports whether an upgrade runs or waits.
func (b *Brew) upgrading() bool {
	for _, a := range b.updates.upgrades {
		if a.taken {
			return true
		}
	}
	return false
}

// UpdateMetadata runs `brew update`, then reloads the outdated packages
// before it reopens its control, as the old Update Homebrew row did.
func (b *Brew) UpdateMetadata() {
	if !b.enabled(brewUpdatesPage, groupBrewUpdates) || b.have != brewInstalled {
		return
	}
	m := &b.updates.meta
	if !m.take() {
		return
	}
	m.running = "Updating…"
	b.enqueue("metadata", "Updating Homebrew metadata", b.be.update, func(err error) {
		if brewCanceled(err) {
			m.restore()
			return
		}
		dryRun := b.dryRun()
		decision := actionstate.MetadataUpdate(err == nil, dryRun)
		if err != nil {
			if decision.RestoreControl {
				m.restore()
			}
			b.env.ToastError(fmt.Sprintf("Update failed: %v", err))
			return
		}
		b.notify(dryRun, actionmsg.SelfUpdate(dryRun, "Homebrew"))
		if decision.RestoreControl {
			m.restore()
		}
		if !dryRun {
			b.env.Record(brewJobSource, "Updated Homebrew metadata", "brew update")
		}
		if decision.Refresh {
			b.loadOutdated(func(bool) { m.restore() })
		}
	})
}

// AskTrust asks before it trusts every installed package of the tap named
// name. Trust is per user (`brew trust`); it never uses pkexec.
func (b *Brew) AskTrust(name string) {
	if !b.enabled(brewUpdatesPage, groupBrewTrust) {
		return
	}
	tap, ok := b.updates.findTap(name)
	if !ok {
		return
	}
	a := brewActionFor(b.updates.trusts, name)
	if !a.take() {
		return
	}
	row := pageview.UntrustedTap(tap.Name, tap.Formulae, tap.Casks)
	b.env.Dialogs.Ask(frostui.Confirm{
		Title:     fmt.Sprintf("Trust packages from %s?", tap.Name),
		Body:      "Trusting allows this tap's package definitions to run code during installs and upgrades. Only trust taps you recognize.",
		Details:   []frostui.KeyValue{{Key: "Packages", Value: row.Subtitle}},
		Action:    "Trust",
		OnCancel:  a.restore,
		OnConfirm: func() { b.runTrust(tap, row.Subtitle, a) },
	})
}

func (b *Brew) runTrust(tap homebrew.UntrustedTap, packages string, a *brewAction) {
	a.running = "Trusting…"
	tap.Formulae = append([]string(nil), tap.Formulae...)
	tap.Casks = append([]string(nil), tap.Casks...)
	trust := b.be.trust
	b.enqueue("tap:"+tap.Name, "Trusting "+tap.Name, func() error { return trust(tap) }, func(err error) {
		if brewCanceled(err) {
			a.restore()
			return
		}
		if err != nil {
			a.restore()
			b.env.ToastError(fmt.Sprintf("Failed to trust %s: %v", tap.Name, err))
			return
		}
		decision := actionmsg.TapTrust(b.dryRun(), tap.Name)
		if !decision.MutateUI {
			// A dry run trusted nothing: the tap and its banner stay.
			a.restore()
			b.env.Toast(decision.Toast)
			return
		}
		a.complete("Trusted")
		b.updates.removeTap(tap.Name)
		delete(b.updates.trusts, tap.Name)
		b.env.ToastSuccess(decision.Toast)
		b.env.Record(brewJobSource, "Trusted "+tap.Name, packages)
		// Newly trusted packages may now show as outdated.
		b.loadOutdated(nil)
	})
}

// brewSource is the Homebrew update source of the Updates page.
type brewSource struct{ b *Brew }

func (s *brewSource) ID() SourceID  { return SourceBrew }
func (s *brewSource) Title() string { return "Homebrew" }
func (s *brewSource) Icon() string  { return "package-x-generic" }
func (s *brewSource) Blurb() string {
	return "Formulae and casks installed for you. Pinned formulae and packages of untrusted taps are held."
}

func (s *brewSource) Status() SourceStatus {
	b := s.b
	st := SourceStatus{Available: b.sourceEnabled() && !b.missing()}
	if !st.Available {
		return st
	}
	u := &b.updates
	st.Checking = b.have == brewUnknown || u.loading != 0 || u.tapsLoading != 0
	st.Err = u.err
	st.CheckedAt = u.checked
	items := s.Items()
	held := 0
	for _, it := range items {
		if it.Held {
			held++
		}
	}
	st.Note = pageview.BrewSourceNote(u.known && b.enabled(brewUpdatesPage, groupBrewUpdates), len(items), held, len(s.visibleTaps()))
	return st
}

// Items are the outdated packages. A pinned formula and a package of an
// untrusted tap are held: Homebrew would skip them.
func (s *brewSource) Items() []UpdateItem {
	b := s.b
	if !b.enabled(brewUpdatesPage, groupBrewUpdates) || b.missing() {
		return nil
	}
	taps := b.updates.untrustedPackages()
	kinds := b.inv.kinds()
	items := make([]UpdateItem, 0, len(b.updates.outdated))
	for _, p := range b.updates.outdated {
		it := UpdateItem{Key: p.Name, Title: p.Name, From: p.Version, Action: "Upgrade"}
		var detail []string
		if kind, ok := kinds[p.Name]; ok {
			detail = append(detail, strings.ToLower(kind.DisplayName()))
		}
		tap := taps[p.Name]
		if tap != "" {
			detail = append(detail, tap)
		}
		it.Detail = strings.Join(detail, " · ")
		switch {
		case tap != "":
			it.Held, it.HeldWhy = true, "tap not trusted"
		case p.Pinned:
			it.Held, it.HeldWhy = true, "pinned"
		}
		items = append(items, it)
	}
	return items
}

// upgradable are the items of keys (every item, without keys) that an
// upgrade may start for now: not held, and no upgrade of theirs in use.
func (s *brewSource) upgradable(keys []string) []UpdateItem {
	b := s.b
	items := s.Items()
	byKey := make(map[string]UpdateItem, len(items))
	for _, it := range items {
		byKey[it.Key] = it
	}
	if len(keys) == 0 {
		for _, it := range items {
			keys = append(keys, it.Key)
		}
	}
	var out []UpdateItem
	for _, key := range keys {
		it, ok := byKey[key]
		if !ok || it.Held || b.upgradeInUse(key) || b.rowInUse(key) {
			continue
		}
		out = append(out, it)
	}
	return out
}

func (s *brewSource) Refresh() {
	b := s.b
	if b.missing() || s.Status().Checking {
		return
	}
	b.updates.dismissed = map[string]bool{}
	b.loadOutdated(nil)
	b.loadTaps()
}

func (s *brewSource) Jobs(keys ...string) []*frostui.Job {
	var jobs []*frostui.Job
	for _, it := range s.upgradable(keys) {
		if j := s.b.upgradeJob(it.Key, it.From); j != nil {
			jobs = append(jobs, j)
		}
	}
	return jobs
}

func (s *brewSource) Update(key string) {
	if jobs := s.Jobs(key); len(jobs) > 0 {
		s.b.env.Jobs.Enqueue(jobs...)
	}
}

// visibleTaps are the untrusted taps whose banner shows.
func (s *brewSource) visibleTaps() []homebrew.UntrustedTap {
	b := s.b
	if !b.enabled(brewUpdatesPage, groupBrewTrust) || b.missing() {
		return nil
	}
	var out []homebrew.UntrustedTap
	for _, t := range b.updates.taps {
		if !b.updates.dismissed[t.Name] {
			out = append(out, t)
		}
	}
	return out
}

// Extra draws a warning banner for each untrusted tap, whose action trusts
// it, and the control that updates Homebrew's own metadata.
func (s *brewSource) Extra(c *ui.Context) {
	b := s.b
	taps := s.visibleTaps()
	meta := b.enabled(brewUpdatesPage, groupBrewUpdates) && b.have == brewInstalled
	if len(taps) == 0 && !meta {
		return
	}
	ui.Column(c).Gap(frostui.SpaceS).AlignItems(ui.Stretch).Children(func() {
		for _, tap := range taps {
			name := tap.Name
			text := pageview.BrewTrustBanner(name, tap.Formulae, tap.Casks)
			label := "Trust tap"
			busy := false
			if a := b.updates.trusts[name]; a != nil && a.running != "" {
				text, label, busy = fmt.Sprintf("Trusting %s…", name), "", true
			}
			ui.Column(c).Key("tap:" + name).AlignItems(ui.Stretch).Children(func() {
				acted, closed := frostui.Banner(c, frostui.Warning, "changes-prevent", text, label, !busy)
				if acted {
					b.AskTrust(name)
				}
				if closed {
					b.updates.dismissed[name] = true
				}
			})
		}
		if meta {
			b.drawMetadata(c)
		}
	})
}

// drawMetadata is the "brew update" row: Homebrew's own definitions.
func (b *Brew) drawMetadata(c *ui.Context) {
	t := c.Theme()
	m := &b.updates.meta
	ui.Row(c).Gap(frostui.SpaceM).Padding(frostui.SpaceS, 0).AlignItems(ui.Center).Children(func() {
		frostui.Icon(c, "view-refresh").TextColor(t.TextMuted)
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			frostui.Body(c, "Homebrew metadata")
			frostui.Muted(c, "Update Homebrew itself and all formulae definitions")
		})
		label := "Update metadata"
		if m.running != "" {
			label = m.running
		}
		if ui.Button(c, label).Label("Update Homebrew metadata").Disabled(m.inUse() || b.pending("metadata")).Clicked() {
			b.UpdateMetadata()
		}
	})
}
