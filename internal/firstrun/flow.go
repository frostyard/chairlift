package firstrun

// Step is one step of the first-run window.
type Step int

// The steps, in order.
const (
	StepWelcome Step = iota
	StepConnectivity
	StepStyle
	StepApps
	StepProgress
	StepDone
)

// Steps lists every step in order.
var Steps = []Step{StepWelcome, StepConnectivity, StepStyle, StepApps, StepProgress, StepDone}

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
	current       Step
	appsAvailable bool
	conn          Connectivity
	progressDone  bool
}

// NewFlow starts at Welcome with the connectivity check pending.
// appsAvailable is true when the application list loaded and its config
// group is enabled.
func NewFlow(appsAvailable bool) *Flow {
	return &Flow{current: StepWelcome, appsAvailable: appsAvailable, conn: ConnChecking}
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
// check said online; every other step is always visible.
func (f *Flow) Visible(s Step) bool {
	if s == StepApps {
		return f.appsAvailable && f.conn == ConnOnline
	}
	return s >= StepWelcome && s <= StepDone
}

// VisibleSteps lists the visible steps in order.
func (f *Flow) VisibleSteps() []Step {
	var steps []Step
	for _, s := range Steps {
		if f.Visible(s) {
			steps = append(steps, s)
		}
	}
	return steps
}

// nextVisible returns the first visible step after s.
func (f *Flow) nextVisible(s Step) (Step, bool) {
	for n := s + 1; n <= StepDone; n++ {
		if f.Visible(n) {
			return n, true
		}
	}
	return s, false
}

// previousVisible returns the last visible step before s.
func (f *Flow) previousVisible(s Step) (Step, bool) {
	for p := s - 1; p >= StepWelcome; p-- {
		if f.Visible(p) {
			return p, true
		}
	}
	return s, false
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
