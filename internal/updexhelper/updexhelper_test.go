package updexhelper

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

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
