package views

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/chairlift/internal/frostui"
	"github.com/frostyard/chairlift/internal/views/pageview"
)

// updatesPage is the Updates page: the header with "Check now" and "Update
// everything", the update run across every source as a pipeline, and one
// card per source with a row per item.
//
// The page owns no update state: each source holds its own items and
// status, and the job queue holds every job's state. The page only keeps
// the last batch of jobs, so the rows and the pipeline still say what a
// run did after it ended (a source drops an item it updated), until the
// next batch or "Check now".
type updatesPage struct {
	env *Env
	// sources are the sources that apply, in run order: Store.Sources in
	// the app, fakes in tests.
	sources func() []UpdateSource

	// run is the newest batch of jobs, in queue order, kept after it ends.
	run []*frostui.Job
	// shown is what each source's card lists: the source's items, and in
	// their place the items the run updated that the source dropped.
	shown map[SourceID][]UpdateItem
}

// NewUpdatesPage returns the Updates page.
func NewUpdatesPage(s *Store) Page { return newUpdatesPage(s.Env, s.Sources) }

func newUpdatesPage(env *Env, sources func() []UpdateSource) *updatesPage {
	return &updatesPage{env: env, sources: sources, shown: map[SourceID][]UpdateItem{}}
}

// View draws the page.
func (p *updatesPage) View(c *ui.Context) {
	srcs := p.sources()
	p.track(srcs)
	frostui.PageHeader(c, "Updates", p.subtitle(srcs), func() { p.actions(c, srcs) })
	if len(srcs) == 0 {
		frostui.Card(c).Children(func() {
			frostui.Empty(c, "software-update-available", "No update source applies here",
				"The system image, features, Flatpak, Homebrew and firmware are each turned off in the configuration, or their tools are not installed on this computer.")
		})
		return
	}
	if p.env.Jobs.Busy() || updatesAnyItems(srcs) {
		p.runCard(c, srcs)
	}
	for _, src := range srcs {
		p.sourceCard(c, src)
	}
}

// track notes the batch that runs now and merges each source's items into
// what its card shows.
func (p *updatesPage) track(srcs []UpdateSource) {
	p.noteBatch()
	for _, src := range srcs {
		id := src.ID()
		p.shown[id] = updatesMerge(p.shown[id], src.Items(), func(key string) bool { return p.ranJob(id, key) != nil })
	}
}

// noteBatch keeps the update jobs of the batch the queue runs, if any; other
// pages' jobs (a cleanup, a bundle) are not the Updates page's to show.
func (p *updatesPage) noteBatch() {
	if !p.env.Jobs.Busy() {
		return
	}
	p.run = p.run[:0:0]
	for _, j := range p.env.Jobs.BatchJobs() {
		switch SourceID(j.Source) {
		case SourceImage, SourceFeatures, SourceFlatpak, SourceBrew, SourceFirmware:
			p.run = append(p.run, j)
		}
	}
}

// updatesMerge returns the items a card lists: prev's order, each item
// replaced by its current form; items that left cur only while kept says
// so (the run updated them); then the new items of cur.
func updatesMerge(prev, cur []UpdateItem, kept func(key string) bool) []UpdateItem {
	now := make(map[string]UpdateItem, len(cur))
	for _, it := range cur {
		now[it.Key] = it
	}
	out := make([]UpdateItem, 0, len(cur))
	seen := make(map[string]bool, len(prev))
	for _, it := range prev {
		if c, ok := now[it.Key]; ok {
			out = append(out, c)
			seen[it.Key] = true
		} else if kept(it.Key) {
			out = append(out, it)
			seen[it.Key] = true
		}
	}
	for _, it := range cur {
		if !seen[it.Key] {
			out = append(out, it)
		}
	}
	return out
}

// ranJob is the newest job of the kept batch for an item.
func (p *updatesPage) ranJob(id SourceID, key string) *frostui.Job {
	for i := len(p.run) - 1; i >= 0; i-- {
		if j := p.run[i]; j.Source == string(id) && j.Key == key {
			return j
		}
	}
	return nil
}

// jobOf is the job that shows on an item: the queue's own, else the kept
// batch's.
func (p *updatesPage) jobOf(id SourceID, key string) *frostui.Job {
	if j := p.env.Jobs.Find(string(id), key); j != nil {
		return j
	}
	return p.ranJob(id, key)
}

// updatesAnyItems reports whether any source lists anything.
func updatesAnyItems(srcs []UpdateSource) bool {
	for _, src := range srcs {
		if len(src.Items()) > 0 {
			return true
		}
	}
	return false
}

// updatesDefault counts what a source's default Jobs covers: its items that
// are neither held nor need a confirmation.
func updatesDefault(src UpdateSource) int {
	n := 0
	for _, it := range src.Items() {
		if !it.Held && !it.Confirm {
			n++
		}
	}
	return n
}

// sourcePending reports whether a job of src waits or runs.
func (p *updatesPage) sourcePending(src UpdateSource) bool {
	for _, it := range src.Items() {
		if p.env.Jobs.Pending(string(src.ID()), it.Key) {
			return true
		}
	}
	return false
}

func (p *updatesPage) subtitle(srcs []UpdateSource) string {
	ready, held := 0, 0
	checking, image := false, false
	var newest time.Time
	var failed []string
	for _, src := range srcs {
		st := src.Status()
		if st.Err != "" && !st.Checking {
			failed = append(failed, src.Title())
		}
		checking = checking || st.Checking
		if st.CheckedAt.After(newest) {
			newest = st.CheckedAt
		}
		image = image || src.ID() == SourceImage
		for _, it := range src.Items() {
			if it.Held {
				held++
			} else {
				ready++
			}
		}
	}
	var parts []string
	switch {
	case len(srcs) == 0:
		return "No update source applies to this computer"
	case ready+held == 0 && checking && newest.IsZero():
		return "Checking for updates…"
	case ready+held == 0 && !checking && len(failed) == len(srcs):
		// Every check failed (offline, say): nothing is known.
		return "Could not check for updates · Check now tries again"
	case ready+held == 0 && len(failed) > 0:
		parts = append(parts, "Nothing found")
	case ready+held == 0:
		parts = append(parts, "Everything is up to date")
	case held > 0:
		parts = append(parts, fmt.Sprintf("%d ready, %d held", ready, held))
	default:
		parts = append(parts, fmt.Sprintf("%d ready", ready))
	}
	if len(failed) > 0 {
		parts = append(parts, "could not check "+strings.Join(failed, ", "))
	}
	if checking {
		parts = append(parts, "checking now")
	} else if !newest.IsZero() {
		parts = append(parts, strings.TrimPrefix(pageview.LastChecked(newest, p.env.Now(), time.Local), "Last "))
	}
	if image {
		parts = append(parts, "staged system updates apply on restart")
	}
	return strings.Join(parts, " · ")
}

func (p *updatesPage) actions(c *ui.Context, srcs []UpdateSource) {
	checking := updatesChecking(srcs)
	check := frostui.LabeledButton(c, "view-refresh", "Check now", false).Disabled(checking)
	if checking {
		check.Tooltip("A check runs")
	}
	if check.Clicked() {
		p.checkNow()
	}
	reason := p.everythingReason(srcs)
	all := frostui.LabeledButton(c, "software-update-available", "Update everything", true).Disabled(reason != "")
	if reason != "" {
		all.Tooltip(reason)
	}
	if all.Clicked() {
		p.updateEverything()
	}
}

func updatesChecking(srcs []UpdateSource) bool {
	for _, src := range srcs {
		if src.Status().Checking {
			return true
		}
	}
	return false
}

// everythingReason says why "Update everything" cannot run now, "" when it
// can.
func (p *updatesPage) everythingReason(srcs []UpdateSource) string {
	if p.env.Jobs.Busy() {
		return "Wait for the running job to finish"
	}
	for _, src := range srcs {
		if updatesDefault(src) > 0 {
			return ""
		}
	}
	return "Nothing to update"
}

// checkNow checks every source, as Store.CheckAll does, and forgets the
// last run: the user asked for a fresh look.
func (p *updatesPage) checkNow() {
	if !p.env.Jobs.Busy() {
		p.run = nil
	}
	for _, src := range p.sources() {
		src.Refresh()
	}
}

// updateEverything queues every source's default jobs in source order, as
// Store.UpdateEverything does: no held item and no firmware.
func (p *updatesPage) updateEverything() {
	if p.env.Jobs.Busy() {
		p.env.Toast("Wait for the running job to finish")
		return
	}
	var jobs []*frostui.Job
	for _, src := range p.sources() {
		jobs = append(jobs, src.Jobs()...)
	}
	if len(jobs) == 0 {
		p.env.Toast("Nothing to update")
		return
	}
	p.env.Jobs.Enqueue(jobs...)
	p.noteBatch()
}

// updateSource queues one source's default jobs, leaving out any that
// already wait or run.
func (p *updatesPage) updateSource(src UpdateSource) {
	var jobs []*frostui.Job
	for _, j := range src.Jobs() {
		if !p.env.Jobs.Pending(j.Source, j.Key) {
			jobs = append(jobs, j)
		}
	}
	if len(jobs) == 0 {
		p.env.Toast("Nothing to update")
		return
	}
	p.env.Jobs.Enqueue(jobs...)
	p.noteBatch()
}

// runCard draws the update run: one pipeline stage per source.
func (p *updatesPage) runCard(c *ui.Context, srcs []UpdateSource) {
	q := p.env.Jobs
	confirm := false
	stages := make([]frostui.Stage, 0, len(srcs))
	now := p.env.Now()
	for _, src := range srcs {
		st, asks, running := p.stage(src, now)
		confirm = confirm || asks
		if running {
			c.After(time.Second) // the running job's time ticks
		}
		stages = append(stages, st)
	}
	sub := "Each source in order."
	if confirm {
		sub += " Firmware waits for you, device by device."
	}
	frostui.Card(c).Key("run").Children(func() {
		frostui.CardHeader(c, "Update run", sub, func() {
			if q.Busy() {
				done, total := q.Batch()
				ui.Column(c).Gap(6).AlignItems(ui.End).Children(func() {
					frac := q.BatchFraction()
					ui.Row(c).Gap(frostui.SpaceS).AlignItems(ui.Center).Children(func() {
						frostui.Caption(c, fmt.Sprintf("Job %d of %d", min(done+1, total), total)).SingleLine()
						ui.Text(c, strconv.Itoa(int(frac*100))+"%").FontSize(12.5).Bold().SingleLine()
					})
					ui.Progress(c, frac).Width(160).Label("Update run progress")
				})
			} else if summary := frostui.RunSummary(p.run); summary != "" {
				frostui.Caption(c, "Last run: "+summary).SingleLine()
			}
		})
		frostui.Pipeline(c, stages)
	})
}

// stage builds a source's pipeline stage from what its card shows, and
// reports whether it has an item that waits for a confirmation and whether
// one of its jobs runs.
func (p *updatesPage) stage(src UpdateSource, now time.Time) (st frostui.Stage, asks, running bool) {
	status := src.Status()
	st = frostui.Stage{Title: src.Title(), Icon: src.Icon(), Empty: "Up to date"}
	switch {
	case status.Checking:
		st.Empty = "Checking…"
	case status.Err != "":
		st.Empty = "Check failed"
	}
	var spent time.Duration
	for _, it := range p.shown[src.ID()] {
		job := p.jobOf(src.ID(), it.Key)
		step := frostui.Step{Title: it.Title, State: updatesStepState(it, job)}
		if d, ok := updatesJobTime(job, now); ok {
			step.Note = frostui.Duration(d)
			spent += d
			running = running || job.State == frostui.JobRunning
		}
		asks = asks || (it.Confirm && !it.Held)
		st.Steps = append(st.Steps, step)
	}
	if spent > 0 {
		st.Note = frostui.Duration(spent)
	}
	return st, asks, running
}

// updatesStepState is an item's step: its job's state, else held for a
// held item and for one that waits for a confirmation, else ready.
func updatesStepState(it UpdateItem, job *frostui.Job) frostui.StepState {
	if job != nil {
		return frostui.StateOf(job)
	}
	if it.Held || it.Confirm {
		return frostui.StepHeld
	}
	return frostui.StepReady
}

// updatesJobTime is how long a job ran, or runs so far.
func updatesJobTime(j *frostui.Job, now time.Time) (time.Duration, bool) {
	if j == nil || j.Started.IsZero() {
		return 0, false
	}
	end := j.Ended
	if j.State == frostui.JobRunning || end.IsZero() {
		end = now
	}
	return max(end.Sub(j.Started), 0), true
}

// sourceCard draws one source: its header, what only it shows, its status
// and a row per item.
func (p *updatesPage) sourceCard(c *ui.Context, src UpdateSource) {
	t := c.Theme()
	status := src.Status()
	items := p.shown[src.ID()]
	frostui.Card(c).Key(src.ID()).Gap(frostui.SpaceS).Children(func() {
		ui.Row(c).Gap(frostui.SpaceM).AlignItems(ui.Center).Padding(0, 0, frostui.SpaceXS, 0).Children(func() {
			frostui.IconTile(c, src.Icon(), frostui.Accent, 32)
			ui.Column(c).Grow(1).Gap(2).Children(func() {
				frostui.Heading(c, src.Title())
				frostui.Muted(c, src.Blurb()).Wrap()
			})
			if n := updatesDefault(src); n > 1 {
				b := ui.Button(c, "Update all").Label("Update all of " + src.Title())
				if p.sourcePending(src) {
					b.Disabled(true)
				}
				if b.Clicked() {
					p.updateSource(src)
				}
			}
		})
		src.Extra(c)
		switch {
		case status.Checking:
			frostui.Loading(c, "Checking for updates…")
		case status.Err != "":
			frostui.Problem(c, status.Err)
		}
		if status.Note != "" {
			frostui.Muted(c, status.Note).Wrap()
		}
		if len(items) == 0 {
			if !status.Checking && status.Err == "" {
				ui.Row(c).Gap(frostui.SpaceS).Padding(frostui.SpaceXS, 0).AlignItems(ui.Center).Children(func() {
					frostui.Icon(c, "object-select").TextColor(t.Success)
					frostui.Muted(c, "Up to date")
				})
			}
			return
		}
		ui.Column(c).AlignItems(ui.Stretch).Children(func() {
			for i, it := range items {
				if i > 0 {
					frostui.RowDivider(c).Key([2]any{"div", it.Key})
				}
				p.itemRow(c, src, it)
			}
		})
	})
}

// itemRow is one item: its name and detail, the versions, and its button
// or what its job did.
func (p *updatesPage) itemRow(c *ui.Context, src UpdateSource, it UpdateItem) {
	job := p.jobOf(src.ID(), it.Key)
	ui.Row(c).Key([2]any{"row", it.Key}).Gap(frostui.SpaceM).Padding(frostui.SpaceS, 0).AlignItems(ui.Center).Children(func() {
		ui.Avatar(c, it.Title, nil).Size(32, 32)
		ui.Column(c).Grow(1).Gap(1).Children(func() {
			frostui.Strong(c, it.Title).SingleLine()
			if it.Detail != "" {
				frostui.Muted(c, it.Detail).SingleLine()
			}
		})
		if it.From != "" || it.To != "" {
			frostui.VersionChange(c, it.From, it.To)
		}
		ui.Row(c).Width(170).Gap(frostui.SpaceS).Justify(ui.End).AlignItems(ui.Center).Children(func() {
			p.itemAction(c, src, it, job)
		})
	})
}

func (p *updatesPage) itemAction(c *ui.Context, src UpdateSource, it UpdateItem, job *frostui.Job) {
	t := c.Theme()
	switch {
	case job != nil && job.State == frostui.JobQueued:
		frostui.JobStatus(c, frostui.RowStatus{State: frostui.StepQueued, Text: "Queued", Label: "Queued: " + it.Title, Compact: true})
		return
	case job != nil && job.State == frostui.JobRunning:
		doing := updatesDoing(it.Action)
		frostui.JobStatus(c, frostui.RowStatus{State: frostui.StepRunning, Text: doing, Label: doing + " " + it.Title, Compact: true})
		return
	case job != nil && job.State == frostui.JobDone:
		frostui.JobStatus(c, frostui.RowStatus{State: frostui.StepDone, Text: updatesDone(it.Action), Compact: true})
		return
	case job != nil && job.State == frostui.JobFailed:
		msg := "Failed"
		if job.Err != nil {
			msg = job.Err.Error()
		}
		frostui.JobStatus(c, frostui.RowStatus{State: frostui.StepFailed, Text: "Failed", Tooltip: msg, Compact: true})
	}
	label := strings.TrimSuffix(it.Action, "…") + " " + it.Title
	if it.Held {
		frostui.Icon(c, "changes-prevent").TextColor(t.TextMuted).Tooltip(it.HeldWhy)
	}
	b := ui.Button(c, it.Action).Label(label)
	if it.Held {
		b.Disabled(true).Tooltip(it.HeldWhy)
	}
	if b.Clicked() {
		src.Update(it.Key)
	}
}

// updatesDoing is what a running job of an item with action does.
func updatesDoing(action string) string {
	switch strings.TrimSuffix(action, "…") {
	case "Stage":
		return "Staging"
	case "Install":
		return "Installing"
	}
	return "Updating"
}

// updatesDone is what an item with action shows once its job worked. An
// action that is already an outcome, as firmware's "Restart required",
// stays as it is.
func updatesDone(action string) string {
	switch strings.TrimSuffix(action, "…") {
	case "Stage":
		return "Staged"
	case "Install":
		return "Installed"
	case "Update", "":
		return "Updated"
	}
	return action
}

// updatesSourceCommand names the palette command that runs a source's
// default jobs.
func updatesSourceCommand(src UpdateSource) string {
	switch src.ID() {
	case SourceImage:
		return "Stage the system update"
	case SourceFeatures:
		return "Update features"
	case SourceFlatpak:
		return "Update Flatpak apps"
	case SourceBrew:
		return "Update Homebrew packages"
	}
	return "Update " + src.Title()
}

// Commands are "Update everything", one command per source with something
// to update, and one per firmware device (which asks first).
func (p *updatesPage) Commands() []frostui.Command {
	srcs := p.sources()
	reason := p.everythingReason(srcs)
	cmds := []frostui.Command{{
		ID: "update-everything", Title: "Update everything", Group: "Actions", Icon: "software-update-available",
		Hint: "upgrade all updates", Disabled: reason != "", Reason: reason, Run: p.updateEverything,
	}}
	for _, src := range srcs {
		if updatesDefault(src) > 0 {
			pending := p.sourcePending(src)
			cmds = append(cmds, frostui.Command{
				ID: "updates-source-" + string(src.ID()), Title: updatesSourceCommand(src), Group: "Actions", Icon: src.Icon(),
				Hint: src.Title(), Disabled: pending, Reason: "Its updates are running", Run: func() { p.updateSource(src) },
			})
		}
		for _, it := range src.Items() {
			if !it.Confirm || it.Held {
				continue
			}
			title := strings.TrimSuffix(it.Action, "…") + " " + it.Title
			if src.ID() == SourceFirmware {
				title = "Install firmware for " + it.Title
			}
			key := it.Key
			cmds = append(cmds, frostui.Command{
				ID: "updates-item-" + string(src.ID()) + "-" + key, Title: title, Group: "Actions", Icon: src.Icon(),
				Hint: it.Detail, Disabled: p.env.Jobs.Pending(string(src.ID()), key), Reason: "It is installing",
				Run: func() { src.Update(key) },
			})
		}
	}
	return cmds
}

// Badge is the number of items every source can update, held ones
// included: Store.UpdateCount.
func (p *updatesPage) Badge() (string, frostui.Tone) {
	n := 0
	for _, src := range p.sources() {
		n += len(src.Items())
	}
	if n == 0 {
		return "", frostui.Neutral
	}
	return strconv.Itoa(n), frostui.Accent
}
