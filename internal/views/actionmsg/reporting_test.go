package actionmsg

import (
	"errors"
	"testing"
)

// TestReportDecision pins the Report dialog's single decision: a dry run
// previews and marks nothing, and a live send sends and marks.
func TestReportDecision(t *testing.T) {
	tests := []struct {
		name   string
		dryRun bool
		kind   string
		want   ReportDecision
	}{
		{
			name:   "dry run previews",
			dryRun: true,
			kind:   "unit_failed",
			want: ReportDecision{
				Send:     false,
				MarkSent: false,
				Toast:    "[DRY-RUN] Preview: report unit_failed — no changes made",
				Log:      "[DRY-RUN] would send report unit_failed",
			},
		},
		{
			name:   "live sends",
			dryRun: false,
			kind:   "unit_failed",
			want: ReportDecision{
				Send:     true,
				MarkSent: true,
				Toast:    "Sent report unit_failed",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Report(tt.dryRun, tt.kind)
			if got != tt.want {
				t.Fatalf("Report(%v, %q) = %+v, want %+v", tt.dryRun, tt.kind, got, tt.want)
			}
			if got.Send != !tt.dryRun || got.MarkSent != !tt.dryRun {
				t.Errorf("Report(%v) = send %v, mark %v; want both %v", tt.dryRun, got.Send, got.MarkSent, !tt.dryRun)
			}
		})
	}
}

// TestReportFailed proves the failure toast names the kind and the error, and
// that a live failure leaves MarkSent false through Report.
func TestReportFailed(t *testing.T) {
	got := ReportFailed("unit_failed", errors.New("the report endpoint answered 503"))
	want := "Could not send the unit_failed report: the report endpoint answered 503"
	if got != want {
		t.Errorf("ReportFailed() = %q, want %q", got, want)
	}
	if Report(false, "unit_failed").MarkSent != true {
		t.Error("Report(false) does not mark on success")
	}
}

// TestReportingConsentDecision pins the switch's single decision: a dry run
// writes nothing, and a live choice persists.
func TestReportingConsentDecision(t *testing.T) {
	tests := []struct {
		name    string
		dryRun  bool
		enabled bool
		mode    string
		want    ConsentDecision
	}{
		{
			name:    "dry run on",
			dryRun:  true,
			enabled: true,
			mode:    "auto",
			want: ConsentDecision{
				Persist: false,
				Toast:   "[DRY-RUN] Preview: problem reporting on — no changes made",
				Log:     "[DRY-RUN] would save problem reporting enabled=true mode=auto",
			},
		},
		{
			name:    "dry run off",
			dryRun:  true,
			enabled: false,
			mode:    "ask",
			want: ConsentDecision{
				Persist: false,
				Toast:   "[DRY-RUN] Preview: problem reporting off — no changes made",
				Log:     "[DRY-RUN] would save problem reporting enabled=false mode=ask",
			},
		},
		{
			name:    "dry run unknown mode reads as ask",
			dryRun:  true,
			enabled: true,
			mode:    "",
			want: ConsentDecision{
				Persist: false,
				Toast:   "[DRY-RUN] Preview: problem reporting on — no changes made",
				Log:     "[DRY-RUN] would save problem reporting enabled=true mode=ask",
			},
		},
		{
			name:    "live persists",
			dryRun:  false,
			enabled: true,
			mode:    "auto",
			want: ConsentDecision{
				Persist: true,
				Toast:   "Problem reporting is on",
			},
		},
		{
			name:    "live off persists",
			dryRun:  false,
			enabled: false,
			mode:    "ask",
			want: ConsentDecision{
				Persist: true,
				Toast:   "Problem reporting is off",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ReportingConsent(tt.dryRun, tt.enabled, tt.mode)
			if got != tt.want {
				t.Fatalf("ReportingConsent(%v, %v, %q) = %+v, want %+v", tt.dryRun, tt.enabled, tt.mode, got, tt.want)
			}
			if got.Persist != !tt.dryRun {
				t.Errorf("ReportingConsent(%v).Persist = %v, want %v", tt.dryRun, got.Persist, !tt.dryRun)
			}
		})
	}
}
