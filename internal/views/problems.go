package views

import (
	"context"
	"log"
	"os"
	"sync"
	"time"

	"github.com/frostyard/chairlift/internal/problems"
	"github.com/frostyard/chairlift/internal/reporting"
	"github.com/frostyard/chairlift/internal/selfreport"
	"github.com/frostyard/chairlift/internal/version"
	"github.com/frostyard/chairlift/internal/views/actionmsg"
	"github.com/frostyard/chairlift/internal/views/actionstate"
	"github.com/frostyard/chairlift/internal/views/pageview"
)

// problemsPage is the Problems page's configuration page (ADR-0015).
const problemsPage = "problems_page"

// sendPace is the wait between 2 report requests: at most 10 requests in a
// minute, which is the intake API's rate limit.
const sendPace = 7 * time.Second

// Problems holds the problem records of this computer and the opt-in
// reporting state (ADR-0015, docs/specs/problems-page.md,
// docs/specs/problem-reporting.md): the system problems of snosi-health,
// the user's crashes and app problem records, and the consent, sent log
// and sends of internal/reporting, the only sender. Nothing is sent until
// the user turns reporting on.
//
// It belongs to the UI thread, except the values under reportingMu: the
// send goroutine reads the consent before every request.
type Problems struct {
	env *Env

	// The backends, replaceable in tests. getenv reads XDG_CONFIG_HOME,
	// XDG_STATE_HOME, HOME and the fixture hook; the rest run on worker
	// goroutines, except saveConsent (see applyReportingConsent).
	getenv      func(string) string
	uid         int
	loc         *time.Location
	version     string
	loadSnap    func(ctx context.Context, opts problems.Options) problems.Snapshot
	loadSeen    func(path string) (*problems.Seen, error)
	loadConsent func(path string) (reporting.Consent, error)
	saveConsent func(path string, c reporting.Consent) error
	loadState   func(path string) (*reporting.State, error)
	forItem     func(it problems.Item, client string) (reporting.Report, error)
	sendReport  func(ctx context.Context, in reporting.RunInput, r reporting.Report) (reporting.Entry, error)
	runReports  func(ctx context.Context, in reporting.RunInput) reporting.RunResult
	newSender   func(endpoint string) reporting.Sender
	installInfo func(ctx context.Context) (age, channel string, ok bool)
	pace        time.Duration

	// The page's groups. A disabled group's data is never loaded.
	systemOn, appsOn, reportingOn bool

	started bool
	// loaded is true once the first load published the problems.
	loaded  bool
	loading bool
	refresh actionstate.RefreshGate

	seen         *problems.Seen
	outcome      problems.SourceOutcome
	generated    time.Time
	system, apps []problems.Item
	systemHidden int
	appsHidden   int

	// Per-card state, by problem ID.
	open       map[string]bool
	dismissing map[string]bool
	// reportBusy is true while a problem's report is built or sent.
	reportBusy map[string]bool
	// reported holds the problems sent in this run: their card shows
	// "Sent" (pageview.ReportedLabel).
	reported map[string]bool

	// Reporting (ADR-0015 decision 10). reportingConsent, reportingState,
	// reportingItems and reportingEnvelope are guarded by reportingMu.
	reportingMu       sync.Mutex
	reportingConsent  reporting.Consent
	reportingState    *reporting.State
	reportingItems    []problems.Item
	reportingEnvelope problems.Envelope

	reportingConsentAt string
	reportingStateAt   string
	reportingEndpoint  string
	// reportingEndpointErr is set when the configured report endpoint is
	// not a valid URL: every send path is off then.
	reportingEndpointErr error
	reportingClient      string
	// reportingLoaded is true once the first consent load published its
	// value: the switch and the mode stay disabled until then, and the
	// handlers ignore a change before it.
	reportingLoaded bool
	// reportingApplying is true while the page itself applies a consent,
	// so nothing it does can start a second save.
	reportingApplying bool
	reportingRunning  bool

	sentOpen bool
	dialog   reportDialog
}

// NewProblems returns the problems state; nothing loads until Start.
func NewProblems(env *Env) *Problems {
	return &Problems{
		env:         env,
		getenv:      os.Getenv,
		uid:         os.Getuid(),
		loc:         time.Local,
		version:     version.Version,
		loadSnap:    problems.Load,
		loadSeen:    problems.LoadSeen,
		loadConsent: reporting.LoadConsent,
		saveConsent: reporting.SaveConsent,
		loadState:   reporting.LoadState,
		forItem:     reporting.ForItem,
		sendReport:  reporting.SendReport,
		runReports:  reporting.Run,
		newSender:   func(endpoint string) reporting.Sender { return reporting.NewHTTPSender(endpoint) },
		installInfo: reporting.DefaultInstallInfo,
		pace:        sendPace,
		systemOn:    env.Enabled(problemsPage, "problems_system_group"),
		appsOn:      env.Enabled(problemsPage, "problems_apps_group"),
		reportingOn: env.Enabled(problemsPage, "problems_reporting_group"),
		open:        map[string]bool{},
		dismissing:  map[string]bool{},
		reportBusy:  map[string]bool{},
		reported:    map[string]bool{},
	}
}

// dataOn reports whether either problem group is on.
func (p *Problems) dataOn() bool { return p.systemOn || p.appsOn }

// Start begins the first load: the problems of the enabled groups and,
// with the reporting group, the consent and the sent log. With both
// problem groups off it logs the page's built marker at once (ADR-0008);
// the reporting group still loads its consent, or a user could not
// withdraw it.
func (p *Problems) Start() {
	if p.started {
		return
	}
	p.started = true
	// The client string is computed even when it is not a release, as the
	// GTK page did; reporting.Run refuses a client that is not one.
	p.reportingClient, _ = problems.Client(p.version)
	if p.reportingOn {
		p.setUpReporting()
	}
	if !p.dataOn() {
		log.Println(problems.BuiltMarker(0, 0, "disabled"))
		if p.reportingOn {
			p.loadProblems(true)
		}
		return
	}
	p.loadProblems(true)
}

// Reload reads the problems again. It keeps the consent and the sent log
// in memory: only the first load publishes them, so a reload can never
// overwrite a newer choice of the user.
func (p *Problems) Reload() {
	if !p.started || p.loading || !p.dataOn() {
		return
	}
	p.loadProblems(false)
}

// Summary is what the Overview and the status bar show: Known after the
// first load of a problem group; Count, the problems shown; Attention, the
// shown system problems. snosi-health's records are about the system that
// ChairLift manages (a failed service, an older image, a failed integrity
// check), while the apps group lists the user's own programs' crashes.
func (p *Problems) Summary() ProblemSummary {
	if !p.loaded || !p.dataOn() {
		return ProblemSummary{}
	}
	return ProblemSummary{Known: true, Count: len(p.system) + len(p.apps), Attention: len(p.system)}
}

// problemsLoad is what 1 load read off the UI thread.
type problemsLoad struct {
	now     time.Time
	snap    problems.Snapshot
	seen    *problems.Seen
	consent reporting.Consent
	state   *reporting.State
}

// loadProblems reads every source on a worker goroutine, then publishes
// the problems on the UI thread and logs the page's built marker. The
// first load also reads the consent and the sent log; it runs with only
// the reporting group on too, because the reporting card needs them. A
// reload reuses the dismiss store of the first load.
func (p *Problems) loadProblems(first bool) {
	systemOn, appsOn, reportingOn := p.systemOn, p.appsOn, p.reportingOn
	dataOn := systemOn || appsOn
	dryRun := p.env.DryRun
	now := p.env.Now()
	getenv, uid, seen := p.getenv, p.uid, p.seen
	consentAt, stateAt := p.reportingConsentAt, p.reportingStateAt
	load, loadSeen, loadConsent, loadState := p.loadSnap, p.loadSeen, p.loadConsent, p.loadState
	gen := p.refresh.Begin()
	p.loading = true

	Go(p.env, func() problemsLoad {
		res := problemsLoad{now: now, seen: seen}
		if dataOn {
			fixtures := problems.DecideFixtures(getenv(problems.FixturesEnv), dryRun)
			if fixtures.Log != "" {
				log.Println(fixtures.Log)
			}
			if res.seen == nil {
				res.seen = problems.NewSeen("")
				if path, err := problems.SeenPath(getenv); err != nil {
					log.Printf("problems: dismissals are not kept: %v", err)
				} else if loaded, err := loadSeen(path); err != nil {
					log.Printf("problems: dismiss store not read: %v", err)
					res.seen = loaded
				} else {
					res.seen = loaded
				}
			}
			res.snap = load(context.Background(), problems.Options{
				Now:      now,
				UID:      uid,
				System:   systemOn,
				Apps:     appsOn,
				Fixtures: fixtures,
				Seen:     res.seen,
			})
			for _, line := range res.snap.LogLines() {
				log.Println(line)
			}
			if err := res.snap.SourceInvalid(); err != nil {
				selfreport.Error(selfreport.CodeProblemsSourceInvalid, err)
			}
		}
		if first && reportingOn {
			if consentAt == "" {
				log.Println("problems: problem reporting is off: the consent file has no path")
			} else if loaded, err := loadConsent(consentAt); err != nil {
				log.Printf("problems: problem reporting is off: %v", err)
			} else {
				res.consent = loaded
			}
			if stateAt != "" {
				loaded, err := loadState(stateAt)
				if err != nil {
					log.Printf("problems: %v", err)
				}
				res.state = loaded
			}
		}
		return res
	}, func(res problemsLoad) {
		if first && reportingOn {
			// Only the first load publishes the consent and the sent log, and
			// only it makes the controls usable: a later value would overwrite
			// a choice the user made since.
			p.reportingMu.Lock()
			p.reportingConsent = res.consent
			p.reportingState = res.state
			p.reportingMu.Unlock()
			p.reportingLoaded = true
		}
		if !p.refresh.IsCurrent(gen) {
			return
		}
		p.loading = false
		if !dataOn {
			return
		}
		p.loaded = true
		p.seen = res.seen
		snap := res.snap
		p.outcome, p.generated = snap.System.Outcome, snap.System.Generated
		p.system, p.systemHidden = snap.SystemItems, snap.SystemHidden
		p.apps, p.appsHidden = snap.AppItems, snap.AppsHidden

		items := append(append([]problems.Item(nil), snap.SystemItems...), snap.AppItems...)
		p.reportingMu.Lock()
		p.reportingItems = items
		p.reportingEnvelope = snap.System.Envelope
		p.reportingMu.Unlock()

		systemCount, appsCount, source := 0, 0, "disabled"
		if systemOn {
			systemCount, source = len(snap.SystemItems), snap.System.Outcome.String()
		}
		if appsOn {
			appsCount = len(snap.AppItems)
		}
		log.Println(problems.BuiltMarker(systemCount, appsCount, source))

		consent := p.consentCopy()
		if reportingOn && consent.Allows() && consent.Mode == reporting.ModeAuto && p.reportingEndpointErr == nil {
			p.runAutomaticReports(false)
		}
	})
}

// systemStatus and appsStatus are the sections' status (ADR-0015 outcomes).
func (p *Problems) systemStatus(now time.Time) pageview.ProblemsStatus {
	return pageview.ProblemsSystemStatus(p.outcome, p.generated, len(p.system), p.systemHidden, now, p.loc)
}

func (p *Problems) appsStatus() pageview.ProblemsStatus {
	return pageview.ProblemsAppsStatus(len(p.apps), p.appsHidden)
}

// copyDetails puts a problem's local details on the clipboard
// (problems.DetailsText: the hint, the values and the local text).
func (p *Problems) copyDetails(write func(string), item problems.Item, title string) {
	write(problems.DetailsText(item))
	p.env.Toast(actionmsg.ProblemDetailsCopied(title))
}

// dismiss hides a problem until it happens again (ADR-0015 decision 6).
// actionmsg.ProblemDismiss decides it: a dry run writes and hides nothing;
// a live dismissal writes the dismiss store on a worker goroutine and
// hides the card after the write succeeded. A failed write keeps it.
func (p *Problems) dismiss(item problems.Item, title string) {
	decision := actionmsg.ProblemDismiss(p.env.DryRun, item.ID, title)
	if decision.Log != "" {
		log.Println(decision.Log)
	}
	if !decision.Persist {
		p.env.Toast(decision.Toast)
		return
	}
	if p.dismissing[item.ID] || p.seen == nil {
		return
	}
	p.dismissing[item.ID] = true
	seen, id := p.seen, item.ID
	at := problems.DismissAt(item, p.env.Now())
	Go(p.env, func() error {
		return seen.Dismiss(id, at)
	}, func(err error) {
		delete(p.dismissing, id)
		if err != nil {
			log.Printf("problems: dismiss %s: %v", id, err)
			p.env.ToastError(actionmsg.ProblemDismissFailed(title, err))
			return
		}
		if decision.Hide {
			p.hide(id)
		}
		p.env.ToastSuccess(decision.Toast)
		p.env.Record("problems", "Dismissed a problem", title)
	})
}

// hide removes a dismissed problem from its section, counts it as hidden,
// and takes it out of what an automatic send may report, as
// `chairlift --send-pending-reports` never reports a dismissed problem.
// It builds new slices: a send goroutine may hold the old ones.
func (p *Problems) hide(id string) {
	var n int
	p.system, n = withoutID(p.system, id)
	p.systemHidden += n
	p.apps, n = withoutID(p.apps, id)
	p.appsHidden += n
	delete(p.open, id)
	p.reportingMu.Lock()
	p.reportingItems, _ = withoutID(p.reportingItems, id)
	p.reportingMu.Unlock()
}

// withoutID returns a new slice of items without those with id, and how
// many it dropped.
func withoutID(items []problems.Item, id string) ([]problems.Item, int) {
	out := make([]problems.Item, 0, len(items))
	for _, it := range items {
		if it.ID != id {
			out = append(out, it)
		}
	}
	return out, len(items) - len(out)
}
