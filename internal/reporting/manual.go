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
	// ErrAlreadySent is returned for a report that this week's sent log
	// already holds: a problem report sent in the same week, or a count of
	// the same ISO week.
	ErrAlreadySent = errors.New("reporting: this report was sent already")
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
	if in.Sender == nil {
		return Entry{}, ErrNoEndpoint
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
	now := nowf()
	if st.Remaining(now) == 0 {
		return Entry{}, ErrDailyLimit
	}
	// A count is due once per ISO week, and a problem report once per week:
	// the dialog must not send either twice, also after a restart.
	if r.Kind == problems.KindCount {
		if r.Week != "" && st.CountWeek == r.Week {
			return Entry{}, ErrAlreadySent
		}
	} else if st.WasSent(r.ProblemID, r.Week) {
		return Entry{}, ErrAlreadySent
	}

	entry := Entry{
		ReportID:  r.ReportID,
		Kind:      r.Kind,
		ProblemID: r.ProblemID,
		Week:      r.Week,
		SentAt:    nowf().UTC(),
	}
	// Record and save before the request, as the automatic path does, so a
	// process that exits during the request can only under-send. A failed
	// request forgets the entry again.
	priorWeek := st.CountWeek
	record := func() {
		if r.Kind == problems.KindCount {
			st.RecordCount(r.Week, entry)
			return
		}
		st.Record(entry)
	}
	undo := func() {
		st.Forget(entry.ReportID)
		if r.Kind == problems.KindCount {
			st.SetCountWeek(priorWeek)
		}
	}
	record()
	if err := st.Save(in.StatePath); err != nil {
		undo()
		logf("reporting: " + err.Error())
		return Entry{}, err
	}
	if err := in.Sender.Send(ctx, r.Body); err != nil {
		undo()
		st.Fail(nowf())
		if saveErr := st.Save(in.StatePath); saveErr != nil {
			logf("reporting: " + saveErr.Error())
			return Entry{}, errors.Join(err, saveErr)
		}
		return Entry{}, err
	}
	return entry, nil
}
