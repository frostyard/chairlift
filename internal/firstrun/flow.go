package firstrun

// Step is one step of the first-run window.
type Step int

// The steps. Steps lists ADR-0014's steps, which are the values 0 to 5;
// StepReporting is the optional ADR-0015 problem-reporting step, and the
// flow's order (flowOrder) places it between Style and Apps.
const (
	StepWelcome Step = iota
	StepConnectivity
	StepStyle
	StepApps
	StepProgress
	StepDone
	// StepReporting is the optional problem-reporting consent step
	// (ADR-0015). It shows only when the reporting group is enabled and the
	// system problem file exists.
	StepReporting
)

// Steps lists ADR-0014's steps in order. StepReporting is not in it: it is
// the optional ADR-0015 step, and flowOrder is the order of the window.
var Steps = []Step{StepWelcome, StepConnectivity, StepStyle, StepApps, StepProgress, StepDone}

// flowOrder is every step in the order the window walks them, with the
// optional reporting step between Style and Apps.
var flowOrder = []Step{StepWelcome, StepConnectivity, StepStyle, StepReporting, StepApps, StepProgress, StepDone}

// String returns the step's stable name; the window uses it as the stack
// child name.
func (s Step) String() string {
	switch s {
	case StepWelcome:
		return "welcome"
	case StepConnectivity:
		return "connectivity"
	case StepStyle:
		return "style"
	case StepApps:
		return "apps"
	case StepProgress:
		return "progress"
	case StepDone:
		return "done"
	case StepReporting:
		return "reporting"
	}
	return "unknown"
}

// Connectivity is the status of the connectivity check. It is status only,
// not a gate: offline users can finish.
type Connectivity int

// The connectivity states.
const (
	ConnChecking Connectivity = iota
	ConnOnline
	ConnOffline
)

// Flow is the first run's step state machine. It is not safe for concurrent
// use: the window drives it from the GTK main thread only.
type Flow struct {
	current            Step
	appsAvailable      bool
	reportingAvailable bool
	conn               Connectivity
	progressDone       bool
}

// NewFlow starts at Welcome with the connectivity check pending.
// appsAvailable is true when the application list loaded and its config
// group is enabled. The optional reporting step does not show.
func NewFlow(appsAvailable bool) *Flow {
	return NewFlowWithReporting(appsAvailable, false)
}

// NewFlowWithReporting is NewFlow, and adds the optional problem-reporting
// step (ADR-0015) when reportingAvailable is true.
func NewFlowWithReporting(appsAvailable, reportingAvailable bool) *Flow {
	return &Flow{
		current:            StepWelcome,
		appsAvailable:      appsAvailable,
		reportingAvailable: reportingAvailable,
		conn:               ConnChecking,
	}
}

// Current returns the step that shows.
func (f *Flow) Current() Step { return f.current }

// Connectivity returns the last connectivity status.
func (f *Flow) Connectivity() Connectivity { return f.conn }

// SetConnectivity records a connectivity status.
func (f *Flow) SetConnectivity(c Connectivity) { f.conn = c }

// FinishProgress records that every Progress action ended.
func (f *Flow) FinishProgress() { f.progressDone = true }

// ProgressFinished reports whether every Progress action ended.
func (f *Flow) ProgressFinished() bool { return f.progressDone }

// Visible reports whether a step takes part in the flow. Apps is visible only
// when the list loaded, its config group is enabled, and the connectivity
// check said online; Reporting is visible only when the reporting group is
// enabled and the system problem file exists (ADR-0015); every other step is
// always visible.
func (f *Flow) Visible(s Step) bool {
	switch s {
	case StepApps:
		return f.appsAvailable && f.conn == ConnOnline
	case StepReporting:
		return f.reportingAvailable
	}
	return s >= StepWelcome && s <= StepDone
}

// VisibleSteps lists the visible steps in order.
func (f *Flow) VisibleSteps() []Step {
	var steps []Step
	for _, s := range flowOrder {
		if f.Visible(s) {
			steps = append(steps, s)
		}
	}
	return steps
}

// nextVisible returns the first visible step after s.
func (f *Flow) nextVisible(s Step) (Step, bool) {
	at := indexOfStep(s)
	if at < 0 {
		return s, false
	}
	for _, n := range flowOrder[at+1:] {
		if f.Visible(n) {
			return n, true
		}
	}
	return s, false
}

// previousVisible returns the last visible step before s.
func (f *Flow) previousVisible(s Step) (Step, bool) {
	at := indexOfStep(s)
	if at < 0 {
		return s, false
	}
	for i := at - 1; i >= 0; i-- {
		if f.Visible(flowOrder[i]) {
			return flowOrder[i], true
		}
	}
	return s, false
}

// indexOfStep returns s's position in flowOrder, or -1 for a step that is
// not in it.
func indexOfStep(s Step) int {
	for i, candidate := range flowOrder {
		if candidate == s {
			return i
		}
	}
	return -1
}

// CanClose reports whether the window may close now. It is false only on
// the Progress step before FinishProgress: the Flatpak actions still run and
// the completion markers are not written yet, so a close would lose them.
// The window blocks close-request and holds the application while it is
// false.
func (f *Flow) CanClose() bool {
	return f.current != StepProgress || f.progressDone
}

// CanNext reports whether Next is possible: never from Done, not from
// Connectivity while the check runs (the result decides whether Apps shows),
// and not from Progress until its actions finished.
func (f *Flow) CanNext() bool {
	switch f.current {
	case StepDone:
		return false
	case StepConnectivity:
		if f.conn == ConnChecking {
			return false
		}
	case StepProgress:
		if !f.progressDone {
			return false
		}
	}
	_, ok := f.nextVisible(f.current)
	return ok
}

// CanBack reports whether Back is possible: never from Welcome, Progress or
// Done.
func (f *Flow) CanBack() bool {
	switch f.current {
	case StepWelcome, StepProgress, StepDone:
		return false
	}
	_, ok := f.previousVisible(f.current)
	return ok
}

// Next moves to the next visible step when CanNext allows it, and reports
// whether it moved.
func (f *Flow) Next() bool {
	if !f.CanNext() {
		return false
	}
	f.current, _ = f.nextVisible(f.current)
	return true
}

// Back moves to the previous visible step when CanBack allows it, and reports
// whether it moved.
func (f *Flow) Back() bool {
	if !f.CanBack() {
		return false
	}
	f.current, _ = f.previousVisible(f.current)
	return true
}
