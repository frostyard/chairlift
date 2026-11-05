package reporting

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	"github.com/frostyard/chairlift/internal/problems"
)

// RunInput is everything 1 automatic send needs. The GUI and
// `chairlift --send-pending-reports` use the same function with the same
// rules (docs/specs/problem-reporting.md).
type RunInput struct {
	// Now returns the time of this run; nil means time.Now.
	Now func() time.Time
	// GroupOn is problems_reporting_group of the configuration.
	GroupOn bool
	// Consent reads the consent again before EACH request, so turning
	// reporting off stops a run at once.
	Consent func() Consent
	// StatePath is the sent log.
	StatePath string
	// Items are the loaded problems (system and apps).
	Items []problems.Item
	// Envelope is problems.json's image envelope, for the count.
	Envelope problems.Envelope
	// Client is problems.Client(version); "" is not a release build, and
	// then nothing is sent.
	Client string
	// InstallInfo gives the count's install age and channel; nil means
	// DefaultInstallInfo.
	InstallInfo func(ctx context.Context) (age, channel string, ok bool)
	// Sender sends a body; it is never called under DryRun.
	Sender Sender
	// DryRun builds every report, logs it and sends nothing.
	DryRun bool
	// Pace is the wait between 2 requests: 7 s in production (at most 10 in
	// a minute), 0 in tests.
	Pace time.Duration
	// Log takes the run's log lines; nil discards them.
	Log func(string)
}

// RunResult is the outcome of 1 Run.
type RunResult struct {
	// Sent are the entries that were written to the sent log.
	Sent []Entry
	// Previews are the reports a dry run would send.
	Previews []Report
	// Skipped names the rule that stopped the run; empty when it ran.
	Skipped string
	// Err is the failure that stopped the run.
	Err error
}

// Run sends the reports that this machine's consent and budget allow, in
// item order, then the weekly count. It stops at the first failed send and
// pauses automatic sending for 24 hours (State.Fail).
func Run(ctx context.Context, in RunInput) RunResult {
	var res RunResult
	logf := in.Log
	if logf == nil {
		logf = func(string) {}
	}
	nowf := in.Now
	if nowf == nil {
		nowf = time.Now
	}
	if !in.GroupOn {
		res.Skipped = "group off"
		return res
	}
	consent := in.Consent
	if consent == nil {
		consent = func() Consent { return Consent{} }
	}
	c := consent()
	if !c.Allows() {
		res.Skipped = "consent"
		return res
	}
	if c.Mode != ModeAuto {
		res.Skipped = "ask mode"
		return res
	}
	if in.Client == "" {
		res.Skipped = "not a release build"
		return res
	}
	if in.DryRun {
		return runDry(ctx, in, logf, nowf)
	}
	if in.Sender == nil {
		// No endpoint is configured, so there is nowhere to send: nothing is
		// read, written or sent.
		res.Skipped = "no endpoint"
		return res
	}

	dir := filepath.Dir(in.StatePath)
	unlock, err := Lock(dir)
	if err != nil {
		if errors.Is(err, ErrBusy) {
			res.Skipped = "busy"
			return res
		}
		res.Skipped = "busy"
		res.Err = err
		return res
	}
	defer unlock()

	st, err := LoadState(in.StatePath)
	if err != nil {
		// The caps cannot be enforced and a send cannot be recorded: send
		// nothing.
		logf("reporting: " + err.Error())
		res.Skipped = "state unreadable"
		return res
	}
	now := nowf()
	if st.Paused(now) {
		res.Skipped = "paused until " + st.RetryAfter.UTC().Format(time.RFC3339)
		return res
	}

	allowed := func() bool {
		next := consent()
		return next.Allows() && next.Mode == ModeAuto
	}
	requests := 0
	// pace waits between 2 requests, and reports a cancelled context.
	pace := func() error {
		if requests == 0 {
			return nil
		}
		if !wait(ctx, in.Pace) {
			return ctx.Err()
		}
		return nil
	}

	left := st.Remaining(now)
	for _, it := range in.Items {
		if left == 0 {
			break
		}
		if !it.Reportable || it.Kind == problems.KindCrash {
			continue
		}
		if st.WasSent(it.ID, it.Week) {
			continue
		}
		if !allowed() {
			logf("reporting: consent changed; stopping")
			return res
		}
		r, err := ForItem(it, in.Client)
		if err != nil {
			logf("reporting: " + err.Error())
			continue
		}
		if err := pace(); err != nil {
			logf("reporting: stopped: " + err.Error())
			return res
		}
		// The consent is read again after the wait and just before the
		// request: turning reporting off during the 7-second pace stops the
		// next request at once, and that is not a failure.
		if !allowed() {
			logf("reporting: consent changed; stopping")
			return res
		}
		entry, withdrawn, sendErr := deliver(ctx, st, in.StatePath, in.Sender, r, false, nowf, allowed, logf)
		if withdrawn {
			logf("reporting: consent changed; stopping")
			return res
		}
		if sendErr != nil {
			res.Err = sendErr
			return res
		}
		requests++
		res.Sent = append(res.Sent, entry)
		left--
	}

	if !st.CountDue(nowf()) || !in.Envelope.Valid {
		return res
	}
	info := in.InstallInfo
	if info == nil {
		info = DefaultInstallInfo
	}
	age, channel, ok := info(ctx)
	if !ok {
		return res
	}
	r, err := NewCountReport(in.Envelope, in.Client, age, channel, nowf())
	if err != nil {
		logf("reporting: " + err.Error())
		return res
	}
	if err := pace(); err != nil {
		logf("reporting: stopped: " + err.Error())
		return res
	}
	if !allowed() {
		logf("reporting: consent changed; stopping")
		return res
	}
	entry, withdrawn, sendErr := deliver(ctx, st, in.StatePath, in.Sender, r, true, nowf, allowed, logf)
	if withdrawn {
		logf("reporting: consent changed; stopping")
		return res
	}
	if sendErr != nil {
		res.Err = sendErr
		return res
	}
	res.Sent = append(res.Sent, entry)
	return res
}

// deliver makes 1 request for rep, in the order that keeps the sent log
// honest (docs/specs/problem-reporting.md):
//
//  1. reserve: record the entry (or the count week) and save it together with
//     a conservative pause (now + 24 h);
//  2. read the consent again, just before the request: another process may
//     have turned reporting off, and then the reservation is rolled back and
//     the run stops without a failure;
//  3. send;
//  4. settle: a success restores the earlier pause and saves; a failure
//     forgets the entry, keeps the pause and saves.
//
// A process that exits during a request therefore leaves the entry and the
// pause on disk: a later run sends nothing for 24 hours and no report is
// sent twice. withdrawn reports that step 2 stopped the send.
func deliver(
	ctx context.Context,
	st *State,
	statePath string,
	sender Sender,
	rep Report,
	count bool,
	nowf func() time.Time,
	allowed func() bool,
	logf func(string),
) (Entry, bool, error) {
	now := nowf()
	entry := Entry{
		ReportID:  rep.ReportID,
		Kind:      rep.Kind,
		ProblemID: rep.ProblemID,
		Week:      rep.Week,
		SentAt:    now.UTC(),
	}
	priorRetry, priorWeek := st.RetryAfter, st.CountWeek
	// undo takes the reservation back, the pause included.
	undo := func() {
		st.Forget(entry.ReportID)
		if count {
			st.SetCountWeek(priorWeek)
		}
		st.RetryAfter = priorRetry
	}

	if count {
		st.RecordCount(rep.Week, entry)
	} else {
		st.Record(entry)
	}
	st.Fail(now)
	if err := saveState(st, statePath); err != nil {
		undo()
		return Entry{}, false, err
	}

	if !allowed() {
		undo()
		if err := saveState(st, statePath); err != nil {
			logf("reporting: " + err.Error())
		}
		return Entry{}, true, nil
	}

	if err := sender.Send(ctx, rep.Body); err != nil {
		// The pause of the reservation stays: a failed send pauses automatic
		// sending for 24 hours.
		st.Forget(entry.ReportID)
		if count {
			st.SetCountWeek(priorWeek)
		}
		if saveErr := saveState(st, statePath); saveErr != nil {
			// The entry and the pause stay on disk, which only under-sends.
			logf("reporting: " + saveErr.Error())
			return Entry{}, false, errors.Join(err, saveErr)
		}
		return Entry{}, false, err
	}

	st.RetryAfter = priorRetry
	if err := saveState(st, statePath); err != nil {
		// The pause stays on disk, which only under-sends.
		logf("reporting: " + err.Error())
	}
	return entry, false, nil
}

// runDry builds every report of a run that would send, logs it, and sends
// nothing. It reads and writes no state and takes no lock, so a dry run
// cannot change anything (ADR-0009).
func runDry(ctx context.Context, in RunInput, logf func(string), nowf func() time.Time) RunResult {
	var res RunResult
	now := nowf()
	for _, it := range in.Items {
		if len(res.Previews) >= MaxPerDay {
			break
		}
		if !it.Reportable || it.Kind == problems.KindCrash {
			continue
		}
		r, err := ForItem(it, in.Client)
		if err != nil {
			logf("reporting: " + err.Error())
			continue
		}
		logf("[DRY-RUN] would send report " + r.Kind)
		res.Previews = append(res.Previews, r)
	}
	if !in.Envelope.Valid {
		return res
	}
	info := in.InstallInfo
	if info == nil {
		info = DefaultInstallInfo
	}
	age, channel, ok := info(ctx)
	if !ok {
		return res
	}
	r, err := NewCountReport(in.Envelope, in.Client, age, channel, now)
	if err != nil {
		logf("reporting: " + err.Error())
		return res
	}
	logf("[DRY-RUN] would send report " + problems.KindCount)
	res.Previews = append(res.Previews, r)
	return res
}

// wait sleeps for d, or returns false when ctx ends first.
func wait(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return true
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
