package firstrun

import (
	"testing"
	"time"

	"github.com/frostyard/chairlift/internal/i18n"
	"github.com/frostyard/chairlift/internal/reporting"
)

// TestReportingStatusMsgid proves the step's status text follows the consent
// mode, and reads as off for a stale consent.
func TestReportingStatusMsgid(t *testing.T) {
	tests := []struct {
		name    string
		consent reporting.Consent
		want    string
	}{
		{"off", reporting.Consent{}, i18n.MsgReportingOff},
		{"on and asking", reporting.Decide(true, reporting.ModeAsk, time.Now()), i18n.MsgReportingOn},
		{"on and automatic", reporting.Decide(true, reporting.ModeAuto, time.Now()), i18n.MsgReportingOnAuto},
		{"stale reads as off", reporting.Consent{Format: 1, Enabled: true, Mode: reporting.ModeAuto, Schema: "frostyard.problem/0"}, i18n.MsgReportingOff},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ReportingStatusMsgid(tt.consent); got != tt.want {
				t.Errorf("ReportingStatusMsgid() = %q, want %q", got, tt.want)
			}
		})
	}
	if i18n.MsgReportingOnAuto == i18n.MsgReportingOn {
		t.Error("the automatic and Ask texts are the same, so the step would not say what happens")
	}
}
