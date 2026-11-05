package firstrun

import (
	"io/fs"
	"testing"
	"time"

	"github.com/frostyard/chairlift/internal/problems"
)

// TestStepReportingIsTheOptionalStep proves the ADR-0015 step is outside
// ADR-0014's Steps, has its own name, and does not show by default.
func TestStepReportingIsTheOptionalStep(t *testing.T) {
	if StepReporting.String() != "reporting" {
		t.Errorf("StepReporting.String() = %q, want reporting", StepReporting.String())
	}
	for _, step := range Steps {
		if step == StepReporting {
			t.Error("Steps lists StepReporting; it is the optional ADR-0015 step")
		}
	}
	if Step(99).String() != "unknown" {
		t.Errorf("Step(99).String() = %q, want unknown", Step(99).String())
	}
	f := NewFlow(true)
	f.SetConnectivity(ConnOnline)
	if f.Visible(StepReporting) {
		t.Error("NewFlow shows the reporting step, want it hidden")
	}
	if got := f.VisibleSteps(); len(got) != len(Steps) {
		t.Errorf("VisibleSteps() = %v, want ADR-0014's %d steps", got, len(Steps))
	}
}

// TestFlowOrdersTheReportingStepBetweenStyleAndApps pins the order of the
// optional step: Style, Reporting, Apps, Progress.
func TestFlowOrdersTheReportingStepBetweenStyleAndApps(t *testing.T) {
	f := NewFlowWithReporting(true, true)
	f.SetConnectivity(ConnOnline)
	want := []Step{StepWelcome, StepConnectivity, StepStyle, StepReporting, StepApps, StepProgress, StepDone}
	got := f.VisibleSteps()
	if len(got) != len(want) {
		t.Fatalf("VisibleSteps() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("VisibleSteps() = %v, want %v", got, want)
		}
	}

	// Next walks Style → Reporting → Apps, and Back walks the same way.
	for f.Current() != StepStyle {
		if !f.Next() {
			t.Fatalf("stuck at %v", f.Current())
		}
	}
	if !f.Next() || f.Current() != StepReporting {
		t.Fatalf("Next from Style = %v, want Reporting", f.Current())
	}
	if !f.Next() || f.Current() != StepApps {
		t.Fatalf("Next from Reporting = %v, want Apps", f.Current())
	}
	if !f.Back() || f.Current() != StepReporting {
		t.Fatalf("Back from Apps = %v, want Reporting", f.Current())
	}
	if !f.Back() || f.Current() != StepStyle {
		t.Fatalf("Back from Reporting = %v, want Style", f.Current())
	}
}

// TestFlowHidesReportingWhenUnavailable proves the step is skipped: Next
// from Style goes straight to Apps.
func TestFlowHidesReportingWhenUnavailable(t *testing.T) {
	f := NewFlowWithReporting(true, false)
	f.SetConnectivity(ConnOnline)
	for f.Current() != StepStyle {
		if !f.Next() {
			t.Fatalf("stuck at %v", f.Current())
		}
	}
	if !f.Next() || f.Current() != StepApps {
		t.Fatalf("Next from Style = %v, want Apps", f.Current())
	}
}

// TestFlowReportingStepKeepsCloseAndBackRules proves the optional step
// behaves like Style: Back leaves it, and it never blocks a close.
func TestFlowReportingStepKeepsCloseAndBackRules(t *testing.T) {
	f := NewFlowWithReporting(true, true)
	f.SetConnectivity(ConnOnline)
	for f.Current() != StepReporting {
		if !f.Next() {
			t.Fatalf("stuck at %v", f.Current())
		}
	}
	if !f.CanClose() {
		t.Error("CanClose() = false on the reporting step")
	}
	if !f.CanBack() || !f.CanNext() {
		t.Error("the reporting step must allow Back and Next")
	}
}

// TestReportingAvailable proves the step's rule: the group must be enabled
// AND the system problem file must exist as a regular file.
func TestReportingAvailable(t *testing.T) {
	regular := func(string) (fs.FileInfo, error) { return fakeFileInfo{}, nil }
	missing := func(string) (fs.FileInfo, error) { return nil, fs.ErrNotExist }
	directory := func(string) (fs.FileInfo, error) { return fakeFileInfo{dir: true}, nil }

	tests := []struct {
		name    string
		groupOn bool
		stat    func(string) (fs.FileInfo, error)
		want    bool
	}{
		{"group on and the file exists", true, regular, true},
		{"group off", false, regular, false},
		{"no problem file", true, missing, false},
		{"the path is a directory", true, directory, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ReportingAvailable(tt.groupOn, tt.stat); got != tt.want {
				t.Errorf("ReportingAvailable() = %v, want %v", got, tt.want)
			}
		})
	}

	var asked string
	ReportingAvailable(true, func(path string) (fs.FileInfo, error) {
		asked = path
		return fakeFileInfo{}, nil
	})
	if asked != problems.ProblemsPath {
		t.Errorf("ReportingAvailable read %q, want %q", asked, problems.ProblemsPath)
	}
}

// fakeFileInfo is a file's metadata.
type fakeFileInfo struct {
	dir bool
}

func (f fakeFileInfo) Name() string { return "problems.json" }
func (f fakeFileInfo) Size() int64  { return 0 }
func (f fakeFileInfo) Mode() fs.FileMode {
	if f.dir {
		return fs.ModeDir | 0o755
	}
	return 0o644
}
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return f.dir }
func (f fakeFileInfo) Sys() any           { return nil }
