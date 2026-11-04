package firstrun

import (
	"reflect"
	"testing"
)

func TestStepNames(t *testing.T) {
	want := []string{"welcome", "connectivity", "style", "apps", "progress", "done"}
	if len(Steps) != len(want) {
		t.Fatalf("Steps has %d entries, want %d", len(Steps), len(want))
	}
	for i, step := range Steps {
		if step.String() != want[i] {
			t.Errorf("Steps[%d] = %q, want %q", i, step, want[i])
		}
	}
	if Step(99).String() != "unknown" {
		t.Errorf("Step(99).String() = %q", Step(99).String())
	}
}

func TestFlowVisibleSteps(t *testing.T) {
	all := []Step{StepWelcome, StepConnectivity, StepStyle, StepApps, StepProgress, StepDone}
	noApps := []Step{StepWelcome, StepConnectivity, StepStyle, StepProgress, StepDone}
	tests := []struct {
		name          string
		appsAvailable bool
		conn          Connectivity
		want          []Step
	}{
		{"apps loaded and online", true, ConnOnline, all},
		{"apps loaded but offline", true, ConnOffline, noApps},
		{"apps loaded, still checking", true, ConnChecking, noApps},
		{"no apps list or disabled group, online", false, ConnOnline, noApps},
		{"no apps list, offline", false, ConnOffline, noApps},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := NewFlow(tt.appsAvailable)
			f.SetConnectivity(tt.conn)
			if got := f.VisibleSteps(); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("VisibleSteps() = %v, want %v", got, tt.want)
			}
			if f.Visible(Step(-1)) || f.Visible(Step(6)) {
				t.Error("an out-of-range step is visible")
			}
		})
	}
}

// TestFlowTransitions drives a flow through scripts of moves and checks the
// step after each move.
func TestFlowTransitions(t *testing.T) {
	type move struct {
		do       string // "next", "back", "online", "offline", "checking", "finish"
		wantOK   bool
		wantStep Step
	}
	tests := []struct {
		name          string
		appsAvailable bool
		moves         []move
	}{
		{
			name:          "online with apps: every step, Back blocked from Progress",
			appsAvailable: true,
			moves: []move{
				{"back", false, StepWelcome},
				{"next", true, StepConnectivity},
				{"next", false, StepConnectivity}, // still checking
				{"online", true, StepConnectivity},
				{"next", true, StepStyle},
				{"back", true, StepConnectivity},
				{"next", true, StepStyle},
				{"next", true, StepApps},
				{"back", true, StepStyle},
				{"next", true, StepApps},
				{"next", true, StepProgress},
				{"back", false, StepProgress},
				{"next", false, StepProgress}, // actions still run
				{"finish", true, StepProgress},
				{"next", true, StepDone},
				{"back", false, StepDone},
				{"next", false, StepDone},
			},
		},
		{
			name:          "offline user can finish without Apps",
			appsAvailable: true,
			moves: []move{
				{"next", true, StepConnectivity},
				{"offline", true, StepConnectivity},
				{"next", true, StepStyle},
				{"next", true, StepProgress},
				{"finish", true, StepProgress},
				{"next", true, StepDone},
			},
		},
		{
			name:          "no apps list: Style goes to Progress, Back from Style to Connectivity",
			appsAvailable: false,
			moves: []move{
				{"next", true, StepConnectivity},
				{"online", true, StepConnectivity},
				{"next", true, StepStyle},
				{"next", true, StepProgress},
				{"finish", true, StepProgress},
				{"next", true, StepDone},
			},
		},
		{
			name:          "a re-check that goes offline hides Apps",
			appsAvailable: true,
			moves: []move{
				{"next", true, StepConnectivity},
				{"online", true, StepConnectivity},
				{"checking", true, StepConnectivity},
				{"next", false, StepConnectivity},
				{"back", true, StepWelcome},
				{"next", true, StepConnectivity},
				{"offline", true, StepConnectivity},
				{"next", true, StepStyle},
				{"next", true, StepProgress},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := NewFlow(tt.appsAvailable)
			if f.Current() != StepWelcome || f.Connectivity() != ConnChecking || f.ProgressFinished() {
				t.Fatalf("NewFlow() = %+v, want Welcome, checking, progress not finished", f)
			}
			for i, m := range tt.moves {
				ok := true
				switch m.do {
				case "next":
					can := f.CanNext()
					ok = f.Next()
					if can != ok {
						t.Fatalf("move %d: CanNext() = %v but Next() = %v", i, can, ok)
					}
				case "back":
					can := f.CanBack()
					ok = f.Back()
					if can != ok {
						t.Fatalf("move %d: CanBack() = %v but Back() = %v", i, can, ok)
					}
				case "online":
					f.SetConnectivity(ConnOnline)
				case "offline":
					f.SetConnectivity(ConnOffline)
				case "checking":
					f.SetConnectivity(ConnChecking)
				case "finish":
					f.FinishProgress()
					if !f.ProgressFinished() {
						t.Fatalf("move %d: ProgressFinished() = false after FinishProgress", i)
					}
				}
				if ok != m.wantOK || f.Current() != m.wantStep {
					t.Fatalf("move %d (%s) = %v at %v, want %v at %v", i, m.do, ok, f.Current(), m.wantOK, m.wantStep)
				}
			}
		})
	}
}

func TestFlowNeighbourHelpers(t *testing.T) {
	f := NewFlow(true)
	f.SetConnectivity(ConnOffline)
	if got, ok := f.nextVisible(StepStyle); !ok || got != StepProgress {
		t.Errorf("nextVisible(Style) offline = %v, %v; want Progress", got, ok)
	}
	if got, ok := f.previousVisible(StepProgress); !ok || got != StepStyle {
		t.Errorf("previousVisible(Progress) offline = %v, %v; want Style", got, ok)
	}
	if _, ok := f.nextVisible(StepDone); ok {
		t.Error("nextVisible(Done) found a step")
	}
	if _, ok := f.previousVisible(StepWelcome); ok {
		t.Error("previousVisible(Welcome) found a step")
	}
	f.SetConnectivity(ConnOnline)
	if got, _ := f.nextVisible(StepStyle); got != StepApps {
		t.Errorf("nextVisible(Style) online = %v, want Apps", got)
	}
}
