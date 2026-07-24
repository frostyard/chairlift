package updexhelper

import "testing"

// TestHasDryRunFlag covers representative arg slices: the flag present, the
// flag absent, and the flag present among other args.
func TestHasDryRunFlag(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{
			name: "flag present alone",
			args: []string{"--dry-run"},
			want: true,
		},
		{
			name: "flag absent, no args",
			args: []string{},
			want: false,
		},
		{
			name: "flag absent, other args present",
			args: []string{"some-feature"},
			want: false,
		},
		{
			name: "flag present among other args",
			args: []string{"some-feature", "--dry-run"},
			want: true,
		},
		{
			name: "flag present before other args",
			args: []string{"--dry-run", "some-feature"},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasDryRunFlag(tt.args); got != tt.want {
				t.Errorf("HasDryRunFlag(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

// TestEnableOptions asserts DryRun is set to exactly the bool passed, for
// both true and false.
func TestEnableOptions(t *testing.T) {
	for _, dryRun := range []bool{true, false} {
		got := EnableOptions(dryRun)
		if got.DryRun != dryRun {
			t.Errorf("EnableOptions(%v).DryRun = %v, want %v", dryRun, got.DryRun, dryRun)
		}
	}
}

// TestDisableOptions asserts DryRun is set to exactly the bool passed, for
// both true and false.
func TestDisableOptions(t *testing.T) {
	for _, dryRun := range []bool{true, false} {
		got := DisableOptions(dryRun)
		if got.DryRun != dryRun {
			t.Errorf("DisableOptions(%v).DryRun = %v, want %v", dryRun, got.DryRun, dryRun)
		}
	}
}

// TestUpdateOptions asserts DryRun is set to exactly the bool passed, for
// both true and false. This is the direct fix for
// cmd/chairlift-updex-helper/main.go's update case previously constructing
// a zero-value updex.UpdateFeaturesOptions{} and silently dropping
// --dry-run.
func TestUpdateOptions(t *testing.T) {
	for _, dryRun := range []bool{true, false} {
		got := UpdateOptions(dryRun)
		if got.DryRun != dryRun {
			t.Errorf("UpdateOptions(%v).DryRun = %v, want %v", dryRun, got.DryRun, dryRun)
		}
	}
}
