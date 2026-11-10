package frostui

import (
	"github.com/egoist/mygo/ui"
)

// StepState is the state of one item of a Pipeline.
type StepState int

const (
	// StepReady waits for a run.
	StepReady StepState = iota
	// StepQueued is in the run, after the items before it.
	StepQueued
	// StepRunning runs now.
	StepRunning
	// StepDone ended well.
	StepDone
	// StepFailed ended with an error.
	StepFailed
	// StepHeld is left out of the run, as a package of an untrusted tap
	// or firmware that waits for a confirmation.
	StepHeld
	// StepSkipped was left out by the user or canceled.
	StepSkipped
)

// StateOf maps a job's state to a step's.
func StateOf(j *Job) StepState {
	if j == nil {
		return StepReady
	}
	switch j.State {
	case JobQueued:
		return StepQueued
	case JobRunning:
		return StepRunning
	case JobDone:
		return StepDone
	case JobFailed:
		return StepFailed
	}
	return StepSkipped
}

// Step is one item of a pipeline stage: a package, a device, an image.
type Step struct {
	Title string
	Note  string
	State StepState
}

// Stage is one column of a Pipeline: an update source.
type Stage struct {
	Title, Icon string
	Note        string
	Steps       []Step
	// Empty is what the stage says when it has no steps ("Up to date").
	Empty string
}

// Pipeline draws stages side by side, left to right, each a column of
// steps, with arrows between the stages: one update run across sources.
func Pipeline(c *ui.Context, stages []Stage) ui.Element {
	t := c.Theme()
	return ui.Row(c).Gap(SpaceS).AlignItems(ui.Start).Role(ui.RoleList).Label("Update run").Children(func() {
		for i, st := range stages {
			if i > 0 {
				ui.Box(c.Key(i*2)).Padding(34, 0, 0, 0).Children(func() {
					Icon(c, "go-next").TextColor(t.TextMuted)
				})
			}
			ui.Column(c.Key(i*2 + 1)).Grow(1).Basis(0).Gap(6).AlignItems(ui.Stretch).Children(func() {
				ui.Row(c).Gap(6).Padding(0, 2, 4, 2).AlignItems(ui.Center).Children(func() {
					stageIcon(c, st)
					ui.Text(c, st.Title).Bold().FontSize(13).SingleLine().Grow(1)
					if st.Note != "" {
						Caption(c, st.Note).SingleLine()
					}
				})
				if len(st.Steps) == 0 {
					empty := st.Empty
					if empty == "" {
						empty = "Nothing to do"
					}
					ui.Row(c).Padding(8, 10).Radius(RadiusChip).Border(1, t.Border.Alpha(0.6)).Children(func() {
						Muted(c, empty).SingleLine()
					})
				}
				for j, step := range st.Steps {
					stepChip(c.Key(j), step)
				}
			})
		}
	})
}

func stageIcon(c *ui.Context, st Stage) {
	running, failed, done, all := false, false, 0, 0
	for _, s := range st.Steps {
		switch s.State {
		case StepRunning, StepQueued:
			running = running || s.State == StepRunning
		case StepFailed:
			failed = true
		case StepDone:
			done++
		}
		if s.State != StepHeld {
			all++
		}
	}
	t := c.Theme()
	switch {
	case running:
		ui.Spinner(c).Label(st.Title + " running")
	case failed:
		Icon(c, "dialog-error").TextColor(t.Danger)
	case all > 0 && done == all:
		Icon(c, "object-select").TextColor(t.Success)
	default:
		Icon(c, st.Icon).TextColor(t.TextMuted)
	}
}

func stepChip(c *ui.Context, s Step) {
	t := c.Theme()
	border, bg := t.Border, Panel(t)
	opacity := float32(1)
	switch s.State {
	case StepRunning:
		border, bg = t.Accent, t.Accent.Alpha(0.12)
	case StepDone:
		border = t.Success.Alpha(0.5)
	case StepFailed:
		border, bg = t.Danger, t.Danger.Alpha(0.1)
	case StepHeld, StepSkipped:
		opacity = 0.55
	}
	ui.Row(c).Gap(SpaceS).Padding(7, 10).Radius(RadiusChip).Background(bg).Border(1, border).Opacity(opacity).
		AlignItems(ui.Center).Role(ui.RoleListItem).Label(s.Title + ", " + stepWord(s.State)).Children(func() {
		switch s.State {
		case StepRunning:
			ui.Spinner(c).Label("Running")
		case StepDone:
			Icon(c, "object-select").TextColor(t.Success)
		case StepFailed:
			Icon(c, "dialog-error").TextColor(t.Danger)
		case StepHeld:
			Icon(c, "changes-prevent").TextColor(t.TextMuted)
		case StepSkipped:
			Icon(c, "media-skip-forward").TextColor(t.TextMuted)
		case StepQueued:
			Icon(c, "content-loading").TextColor(t.TextMuted)
		default:
			Icon(c, "software-update-available").TextColor(t.TextMuted)
		}
		ui.Text(c, s.Title).FontSize(12.5).SingleLine().Grow(1)
		if s.Note != "" {
			Caption(c, s.Note).SingleLine()
		}
	})
}

func stepWord(s StepState) string {
	return [...]string{"ready", "queued", "running", "done", "failed", "held", "skipped"}[s]
}

// JobCard is the rail's card for the running job: its title, the batch's
// progress and "Job 2 of 5". It draws nothing while the queue is idle.
func JobCard(c *ui.Context, q *Jobs) {
	cur := q.Current()
	if cur == nil {
		return
	}
	t := c.Theme()
	done, total := q.Batch()
	ui.Column(c).Gap(6).Padding(10, 12).Radius(RadiusChip).Background(t.Accent.Alpha(0.10)).Border(1, t.Accent.Alpha(0.35)).
		AlignItems(ui.Stretch).Role(ui.RoleStatus).Label(cur.Title).Children(func() {
		ui.Row(c).Gap(SpaceS).AlignItems(ui.Center).Children(func() {
			ui.Spinner(c).Label("Working")
			ui.Text(c, cur.Title).FontSize(12.5).Bold().SingleLine().Grow(1)
			stop := "Stop: drop the queued jobs and cancel this one"
			if cur.Unstoppable {
				stop = "Stop: drop the queued jobs; this one cannot be stopped and finishes first"
			}
			if !q.Stopping() && IconButton(c, "process-stop", stop).Clicked() {
				q.CancelAll()
			}
		})
		ui.Progress(c, q.BatchFraction()).Label("Progress")
		status := cur.Status
		if q.Stopping() {
			status = "Finishing; the queued jobs were dropped"
		} else if status == "" {
			status = sprintf("Job %d of %d", done+1, total)
		}
		Caption(c, status).SingleLine()
	})
}
