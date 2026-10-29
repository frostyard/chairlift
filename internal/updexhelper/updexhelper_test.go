package updexhelper

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/frostyard/updex/v2/sysext"
	"github.com/frostyard/updex/v2/updex"
)

func TestParseInvocationAcceptsSupportedShapes(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want Invocation
	}{
		{
			name: "enable",
			args: []string{"enable-feature", "demo"},
			want: Invocation{Command: CommandEnableFeature, Feature: "demo"},
		},
		{
			name: "enable dry run",
			args: []string{"enable-feature", "demo", "--dry-run"},
			want: Invocation{Command: CommandEnableFeature, Feature: "demo", DryRun: true},
		},
		{
			name: "disable",
			args: []string{"disable-feature", "demo"},
			want: Invocation{Command: CommandDisableFeature, Feature: "demo"},
		},
		{
			name: "disable dry run",
			args: []string{"disable-feature", "demo", "--dry-run"},
			want: Invocation{Command: CommandDisableFeature, Feature: "demo", DryRun: true},
		},
		{
			name: "update",
			args: []string{"update"},
			want: Invocation{Command: CommandUpdate},
		},
		{
			name: "update dry run",
			args: []string{"update", "--dry-run"},
			want: Invocation{Command: CommandUpdate, DryRun: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseInvocation(tt.args)
			if err != nil {
				t.Fatalf("ParseInvocation(%v): %v", tt.args, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseInvocation(%v) = %+v, want %+v", tt.args, got, tt.want)
			}
		})
	}
}

func TestParseInvocationRejectsUnsupportedShapes(t *testing.T) {
	tests := [][]string{
		nil,
		{"unknown"},
		{"enable-feature"},
		{"enable-feature", ""},
		{"enable-feature", "--dry-run"},
		{"enable-feature", "demo", "--unknown"},
		{"enable-feature", "demo", "--dry-run", "extra"},
		{"disable-feature"},
		{"disable-feature", ""},
		{"disable-feature", "--dry-run"},
		{"disable-feature", "demo", "--unknown"},
		{"disable-feature", "demo", "--dry-run", "extra"},
		{"update", "--unknown"},
		{"update", "--dry-run", "extra"},
	}

	for i, args := range tests {
		t.Run(fmt.Sprintf("case-%d", i), func(t *testing.T) {
			if got, err := ParseInvocation(args); err == nil {
				t.Errorf("ParseInvocation(%v) = %+v, want error", args, got)
			}
		})
	}
}

func TestSupportedCommandsIsCompleteAndImmutable(t *testing.T) {
	want := []string{CommandEnableFeature, CommandDisableFeature, CommandUpdate}
	got := SupportedCommands()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SupportedCommands() = %v, want %v", got, want)
	}

	got[0] = "changed"
	if again := SupportedCommands(); !reflect.DeepEqual(again, want) {
		t.Errorf("SupportedCommands() was mutated through returned slice: %v", again)
	}
}

func TestEnableOptions(t *testing.T) {
	for _, dryRun := range []bool{true, false} {
		got := EnableOptions(dryRun)
		want := updex.EnableFeatureOptions{DryRun: dryRun}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("EnableOptions(%v) = %+v, want %+v", dryRun, got, want)
		}
	}
}

func TestDisableOptions(t *testing.T) {
	for _, dryRun := range []bool{true, false} {
		got := DisableOptions(dryRun)
		want := updex.DisableFeatureOptions{DryRun: dryRun}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("DisableOptions(%v) = %+v, want %+v", dryRun, got, want)
		}
	}
}

func TestUpdateOptions(t *testing.T) {
	for _, dryRun := range []bool{true, false} {
		got := UpdateOptions(dryRun)
		want := updex.UpdateFeaturesOptions{DryRun: dryRun}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("UpdateOptions(%v) = %+v, want %+v", dryRun, got, want)
		}
	}
}

func TestWriteResultPreservesRefreshFailure(t *testing.T) {
	refreshErr := errors.New("sysext refresh failed")
	result := updex.FeatureActionResult{
		Feature:      "desktop",
		RefreshError: refreshErr.Error(),
	}
	var out bytes.Buffer

	err := WriteResult(&out, result, refreshErr)
	if !errors.Is(err, refreshErr) {
		t.Fatalf("WriteResult error = %v, want refresh error", err)
	}
	if out.Len() != 0 {
		t.Fatalf("WriteResult wrote success output %q for refresh failure", out.String())
	}
}

func TestWriteResultEncodesSuccess(t *testing.T) {
	var out bytes.Buffer
	if err := WriteResult(&out, map[string]bool{"success": true}, nil); err != nil {
		t.Fatalf("WriteResult: %v", err)
	}
	if got := out.String(); !strings.Contains(got, `"success":true`) {
		t.Fatalf("WriteResult output = %q, want JSON success", got)
	}
}

func TestExitCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "nil", err: nil, want: 0},
		{name: "plain error", err: errors.New("boom"), want: ExitFailure},
		{name: "bare activation incomplete", err: sysext.ErrActivationIncomplete, want: ExitActivationIncomplete},
		{name: "wrapped with %w", err: fmt.Errorf("sysext refresh failed: %w", sysext.ErrActivationIncomplete), want: ExitActivationIncomplete},
		{
			name: "joined with transfer errors",
			err: errors.Join(
				errors.New("one or more components failed to update"),
				errors.New("desktop/app: download failed"),
				fmt.Errorf("sysext refresh failed: %w", sysext.ErrActivationIncomplete),
			),
			want: ExitActivationIncomplete,
		},
		{name: "joined without it", err: errors.Join(errors.New("a"), errors.New("b")), want: ExitFailure},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExitCode(tt.err); got != tt.want {
				t.Errorf("ExitCode(%v) = %d, want %d", tt.err, got, tt.want)
			}
		})
	}
	if ExitFailure != 1 || ExitActivationIncomplete != 3 {
		t.Errorf("exit codes = %d, %d; the helper contract is 1 and 3", ExitFailure, ExitActivationIncomplete)
	}
}

func TestWriteUpdateResult(t *testing.T) {
	results := []updex.UpdateFeaturesResult{{
		Feature: "desktop",
		Results: []updex.UpdateResult{{Component: "app", NeedsImage: "needs image >= 5"}},
	}}
	failure := errors.New("one or more components failed to update")

	tests := []struct {
		name       string
		results    []updex.UpdateFeaturesResult
		err        error
		wantErr    error
		wantOutput bool
	}{
		{name: "results and no error", results: results, wantOutput: true},
		{name: "results with error are still written", results: results, err: failure, wantErr: failure, wantOutput: true},
		{name: "nil results with error write nothing", results: nil, err: failure, wantErr: failure},
		{name: "nil results and no error write nothing", results: nil},
		{name: "empty results encode as an array", results: []updex.UpdateFeaturesResult{}, wantOutput: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			err := WriteUpdateResult(&out, tt.results, tt.err)
			if !errors.Is(err, tt.wantErr) || (tt.wantErr == nil && err != nil) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if (out.Len() > 0) != tt.wantOutput {
				t.Fatalf("output = %q, wantOutput %v", out.String(), tt.wantOutput)
			}
			if !tt.wantOutput {
				return
			}
			var decoded []updex.UpdateFeaturesResult
			if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
				t.Fatalf("output %q is not a JSON array of results: %v", out.String(), err)
			}
			if !reflect.DeepEqual(decoded, tt.results) {
				t.Errorf("decoded = %#v, want %#v", decoded, tt.results)
			}
		})
	}
}
