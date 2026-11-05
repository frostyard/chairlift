package reporting

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	"github.com/frostyard/chairlift/internal/problems"
)

// The failures of a manual send from the Report dialog.
var (
	// ErrOff is returned when problems_reporting_group is disabled.
	ErrOff = errors.New("reporting: problem reporting is off")
	// ErrNoConsent is returned when the user has not turned reporting on
	// for this schema generation.
	ErrNoConsent = errors.New("reporting: problem reporting is not turned on")
	// ErrNoClient is returned when ChairLift cannot name itself as a release
	// build, so the report would fail the v1 client pattern.
	ErrNoClient = errors.New("reporting: this version cannot send reports")
	// ErrDailyLimit is returned when 20 reports were sent in the last 24
	// hours.
	ErrDailyLimit = errors.New("reporting: the daily report limit is reached")
	// ErrDryRun is returned when a dry run reaches a send.
	ErrDryRun = errors.New("reporting: a dry run sends nothing")
)

// SendReport sends 1 report that the user asked for, and records it in the
// sent log. It requires the reporting group, a consent that Allows (either
// mode), a release client, a kind that is not crash, and 1 report left of
// the daily limit; the lock keeps the GUI and the command line apart.
//
// A pause after an earlier failure does not apply: the user asked for this
// report now. A failed send pauses automatic sending for 24 hours. Callers
// decide DryRun with actionmsg.Report and never call SendReport under a dry
// run; the guard is defense in depth (ADR-0009).
func SendReport(ctx context.Context, in RunInput, r Report) (Entry, error) {
	if in.DryRun {
		return Entry{}, ErrDryRun
	}
	if !in.GroupOn {
		return Entry{}, ErrOff
	}
	consent := in.Consent
	if consent == nil {
		consent = func() Consent { return Consent{} }
	}
	if !consent().Allows() {
		return Entry{}, ErrNoConsent
	}
	if in.Client == "" {
		return Entry{}, ErrNoClient
	}
	if r.Kind == problems.KindCrash {
		return Entry{}, ErrLocalOnly
	}
	nowf := in.Now
	if nowf == nil {
		nowf = time.Now
	}
	logf := in.Log
	if logf == nil {
		logf = func(string) {}
	}
	unlock, err := Lock(filepath.Dir(in.StatePath))
	if err != nil {
		return Entry{}, err
	}
	defer unlock()
	st, err := LoadState(in.StatePath)
	if err != nil {
		// The daily cap cannot be checked, the send cannot be recorded, and
		// saving would overwrite the log the user has: send nothing.
		logf("reporting: " + err.Error())
		return Entry{}, err
	}
	save := func() {
		if err := st.Save(in.StatePath); err != nil {
			logf("reporting: " + err.Error())
		}
	}
	now := nowf()
	if st.Remaining(now) == 0 {
		return Entry{}, ErrDailyLimit
	}
	if err := in.Sender.Send(ctx, r.Body); err != nil {
		st.Fail(nowf())
		save()
		return Entry{}, err
	}
	entry := Entry{
		ReportID:  r.ReportID,
		Kind:      r.Kind,
		ProblemID: r.ProblemID,
		Week:      r.Week,
		SentAt:    nowf().UTC(),
	}
	st.Record(entry)
	save()
	return entry, nil
}
