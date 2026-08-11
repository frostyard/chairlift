package sysupdate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseUpdateCheckOutcomes(t *testing.T) {
	tests := []struct {
		name string
		data string
		want UpdateCheck
	}{
		{
			name: "current",
			data: "outcome=current\nchecked_at=2026-08-10T20:08:01-06:00\nimage=snow-ab\nrunning_version=20260810191856\nremote_version=\n",
			want: UpdateCheck{
				Outcome:        OutcomeCurrent,
				CheckedAt:      "2026-08-10T20:08:01-06:00",
				Image:          "snow-ab",
				RunningVersion: "20260810191856",
			},
		},
		{
			name: "staged",
			data: "outcome=staged\nchecked_at=2026-08-10T20:08:01-06:00\nimage=snow-ab\nrunning_version=20260810191856\nremote_version=20260810200801\n",
			want: UpdateCheck{
				Outcome:        OutcomeStaged,
				CheckedAt:      "2026-08-10T20:08:01-06:00",
				Image:          "snow-ab",
				RunningVersion: "20260810191856",
				RemoteVersion:  "20260810200801",
			},
		},
		{
			name: "failed",
			data: "outcome=failed\nchecked_at=2026-08-10T20:08:01-06:00\nimage=snow-ab\nrunning_version=20260810191856\nremote_version=\n",
			want: UpdateCheck{
				Outcome:        OutcomeFailed,
				CheckedAt:      "2026-08-10T20:08:01-06:00",
				Image:          "snow-ab",
				RunningVersion: "20260810191856",
			},
		},
		{
			// held-rollback is bootc-transport-only; on native it must map
			// to unknown, not crash or masquerade as a real outcome.
			name: "unknown outcome value",
			data: "outcome=held-rollback\nchecked_at=t\nimage=i\nrunning_version=\nremote_version=\n",
			want: UpdateCheck{Outcome: OutcomeUnknown, CheckedAt: "t", Image: "i"},
		},
		{
			name: "empty data",
			data: "",
			want: UpdateCheck{Outcome: OutcomeUnknown},
		},
		{
			name: "unknown keys and blank lines ignored, first occurrence wins",
			data: "\nfuture_field=x\noutcome=current\noutcome=failed\n\n",
			want: UpdateCheck{Outcome: OutcomeCurrent},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseUpdateCheck([]byte(tt.data)); got != tt.want {
				t.Errorf("ParseUpdateCheck = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestParseStagedUpdateTransportShapes(t *testing.T) {
	tests := []struct {
		name        string
		data        string
		want        StagedUpdate
		wantDisplay string
	}{
		{
			name:        "native version shape",
			data:        "image=snow-ab\nversion=20260810200801\nstaged_at=2026-08-10T20:08:01-06:00\n",
			want:        StagedUpdate{Image: "snow-ab", Version: "20260810200801", StagedAt: "2026-08-10T20:08:01-06:00"},
			wantDisplay: "20260810200801",
		},
		{
			name:        "bootc digest shape",
			data:        "image=ghcr.io/frostyard/snow:latest\ndigest=sha256:0123456789abcdef0123\nstaged_at=t\n",
			want:        StagedUpdate{Image: "ghcr.io/frostyard/snow:latest", Digest: "sha256:0123456789abcdef0123", StagedAt: "t"},
			wantDisplay: "0123456789ab",
		},
		{
			name:        "empty file",
			data:        "",
			want:        StagedUpdate{},
			wantDisplay: "",
		},
		{
			name:        "malformed version falls back to empty display",
			data:        "image=snow-ab\nversion=not-a-version\n",
			want:        StagedUpdate{Image: "snow-ab", Version: "not-a-version"},
			wantDisplay: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseStagedUpdate([]byte(tt.data))
			if got != tt.want {
				t.Errorf("ParseStagedUpdate = %+v, want %+v", got, tt.want)
			}
			if display := got.DisplayVersion(); display != tt.wantDisplay {
				t.Errorf("DisplayVersion = %q, want %q", display, tt.wantDisplay)
			}
		})
	}
}

func TestValidVersionGrammar(t *testing.T) {
	valid := []string{"20260810200801", "00000000000000"}
	invalid := []string{"", "2026081020080", "202608102008011", "2026081020080a", "sha256:abc", "20260810 20080"}
	for _, v := range valid {
		if !ValidVersion(v) {
			t.Errorf("ValidVersion(%q) = false, want true", v)
		}
	}
	for _, v := range invalid {
		if ValidVersion(v) {
			t.Errorf("ValidVersion(%q) = true, want false", v)
		}
	}
}

func TestStatusPresentationDecisionTable(t *testing.T) {
	check := func(outcome CheckOutcome, remote string) *UpdateCheck {
		return &UpdateCheck{Outcome: outcome, CheckedAt: "2026-08-10T20:08:01-06:00", RemoteVersion: remote}
	}
	staged := &StagedUpdate{Image: "snow-ab", Version: "20260810200801"}

	tests := []struct {
		name                         string
		status                       Status
		wantOutcome, wantVer, wantAt string
		wantStaged                   bool
	}{
		{
			name:        "semaphore present wins",
			status:      Status{Check: check(OutcomeCurrent, ""), Staged: staged},
			wantOutcome: "staged", wantVer: "20260810200801", wantStaged: true,
		},
		{
			name:        "semaphore only",
			status:      Status{Staged: staged},
			wantOutcome: "staged", wantVer: "20260810200801", wantStaged: true,
		},
		{
			name:        "outcome staged without semaphore uses remote version",
			status:      Status{Check: check(OutcomeStaged, "20260810200801")},
			wantOutcome: "staged", wantVer: "20260810200801", wantStaged: true,
		},
		{
			name:        "current",
			status:      Status{Check: check(OutcomeCurrent, "")},
			wantOutcome: "current", wantAt: "2026-08-10T20:08:01-06:00",
		},
		{
			name:        "failed",
			status:      Status{Check: check(OutcomeFailed, "")},
			wantOutcome: "failed", wantAt: "2026-08-10T20:08:01-06:00",
		},
		{
			name:   "both files absent is idle",
			status: Status{},
		},
		{
			name:   "unknown outcome is idle",
			status: Status{Check: check(OutcomeUnknown, "")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outcome, version, checkedAt := tt.status.Presentation()
			if outcome != tt.wantOutcome || version != tt.wantVer || checkedAt != tt.wantAt {
				t.Errorf("Presentation() = (%q, %q, %q), want (%q, %q, %q)",
					outcome, version, checkedAt, tt.wantOutcome, tt.wantVer, tt.wantAt)
			}
			if got := tt.status.IsStaged(); got != tt.wantStaged {
				t.Errorf("IsStaged() = %v, want %v", got, tt.wantStaged)
			}
		})
	}
}

func TestReadUpdateCheckFromMissingFileIsNotAnError(t *testing.T) {
	check, err := readUpdateCheckFrom(filepath.Join(t.TempDir(), "absent"))
	if err != nil {
		t.Fatalf("readUpdateCheckFrom(absent) error = %v, want nil", err)
	}
	if check != nil {
		t.Errorf("readUpdateCheckFrom(absent) = %+v, want nil", check)
	}
}

func TestReadStagedUpdateFromMissingFileIsNotAnError(t *testing.T) {
	staged, err := readStagedUpdateFrom(filepath.Join(t.TempDir(), "absent"))
	if err != nil {
		t.Fatalf("readStagedUpdateFrom(absent) error = %v, want nil", err)
	}
	if staged != nil {
		t.Errorf("readStagedUpdateFrom(absent) = %+v, want nil", staged)
	}
}

func TestReadUpdateCheckFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update-check")
	data := "outcome=staged\nchecked_at=t\nimage=snow-ab\nrunning_version=20260810191856\nremote_version=20260810200801\n"
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	check, err := readUpdateCheckFrom(path)
	if err != nil {
		t.Fatalf("readUpdateCheckFrom: %v", err)
	}
	if check == nil || check.Outcome != OutcomeStaged || check.RemoteVersion != "20260810200801" {
		t.Errorf("readUpdateCheckFrom = %+v", check)
	}
}
