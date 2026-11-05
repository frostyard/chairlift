package reporting

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/frostyard/chairlift/internal/config"
	"github.com/frostyard/chairlift/internal/problems"
)

// CLIInput is everything `chairlift --send-pending-reports` needs. The mode
// builds no GTK window: it reads the same consent, the same sent log and
// the same problems as the page, and then stops.
type CLIInput struct {
	// Getenv reads XDG_CONFIG_HOME, XDG_STATE_HOME and HOME; os.Getenv in
	// production.
	Getenv func(string) string
	// Config is the loaded configuration; ConfigErr is its failure, which
	// stops the mode with exit code 1.
	Config    *config.Config
	ConfigErr error
	// Version is version.Version; a version that is not a release sends
	// nothing.
	Version string
	// DryRun prints the reports instead of sending them.
	DryRun bool
	// Stdout takes the previews; Stderr takes the log lines and errors.
	Stdout io.Writer
	Stderr io.Writer
	// Load reads the problems: system and apps say which groups to read, and
	// seen is the dismiss store (nil hides nothing).
	Load func(ctx context.Context, system, apps bool, seen *problems.Seen) problems.Snapshot
	// NewSender builds the sender for an endpoint; nil means
	// NewHTTPSender.
	NewSender func(endpoint string) Sender
	// InstallInfo gives the count's install age and channel; nil means
	// DefaultInstallInfo.
	InstallInfo func(ctx context.Context) (age, channel string, ok bool)
	// Now returns the time; nil means time.Now.
	Now func() time.Time
	// Pace is the wait between 2 requests; 7 s in production.
	Pace time.Duration
}

// SendPending runs the automatic send without a window and returns the exit
// code: 1 for a configuration error or a failed send, and 0 otherwise.
//
// The mode sends nothing, and says why in 1 line, when the reporting group
// is off, when the consent is off or stale, when the consent mode is "Ask
// each time", or when this is not a release build.
func SendPending(ctx context.Context, in CLIInput) int {
	stdout, stderr := in.Stdout, in.Stderr
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	nowf := in.Now
	if nowf == nil {
		nowf = time.Now
	}
	if in.ConfigErr != nil {
		sayf(stderr, "problem reporting: configuration error: %v\n", in.ConfigErr)
		return 1
	}
	if !groupOn(in.Config) {
		say(stderr, "problem reporting is off: the reporting group is disabled")
		return 0
	}
	consentPath, err := ConsentPath(in.Getenv)
	if err != nil {
		sayf(stderr, "problem reporting is off: %v\n", err)
		return 0
	}
	consent, err := LoadConsent(consentPath)
	if err != nil {
		sayf(stderr, "problem reporting is off: %v\n", err)
		return 0
	}
	if !consent.Allows() {
		if consent.Stale() {
			say(stderr, "problem reporting is off: the report format changed; turn reporting on in ChairLift to agree again")
			return 0
		}
		say(stderr, "problem reporting is off")
		return 0
	}
	if consent.Mode != ModeAuto {
		say(stderr, "Ask each time: nothing is sent without a window")
		return 0
	}
	client, ok := problems.Client(in.Version)
	if !ok {
		sayf(stderr, "problem reporting is off: %s is not a release build\n", in.Version)
		return 0
	}
	statePath, err := StatePath(in.Getenv)
	if err != nil {
		sayf(stderr, "problem reporting is off: %v\n", err)
		return 0
	}
	endpoint, err := ParseEndpoint(configuredEndpoint(in.Config))
	if err != nil {
		sayf(stderr, "problem reporting: %v\n", err)
		return 1
	}
	if in.Load == nil {
		say(stderr, "problem reporting: no problem source is configured")
		return 1
	}
	seen, err := seenStore(in.Getenv)
	if err != nil {
		// The user dismissed problems that this mode cannot see: sending
		// would send them again, so nothing is sent.
		sayf(stderr, "problem reporting: nothing sent: the dismiss store could not be read: %v\n", err)
		return 0
	}
	logf := func(line string) { say(stderr, line) }
	snap := in.Load(ctx, systemOn(in.Config), appsOn(in.Config), seen)
	for _, line := range snap.LogLines() {
		logf(line)
	}
	items := append(append([]problems.Item(nil), snap.SystemItems...), snap.AppItems...)
	newSender := in.NewSender
	if newSender == nil {
		newSender = func(endpoint string) Sender { return NewHTTPSender(endpoint) }
	}
	run := RunInput{
		Now:     nowf,
		GroupOn: true,
		// Read the consent file again before each request, so a withdrawal
		// during the run stops it at once; a read failure is off.
		Consent:     FileConsent(consentPath),
		StatePath:   statePath,
		Items:       items,
		Envelope:    snap.System.Envelope,
		Client:      client,
		InstallInfo: in.InstallInfo,
		DryRun:      in.DryRun,
		Pace:        in.Pace,
		Log:         logf,
	}
	if !in.DryRun {
		// A dry run builds every report and calls no sender at all.
		run.Sender = newSender(endpoint)
	}
	res := Run(ctx, run)
	if in.DryRun {
		for _, r := range res.Previews {
			say(stdout, r.Text())
		}
		return 0
	}
	if res.Skipped != "" {
		logf("problem reporting: nothing sent: " + res.Skipped)
	}
	for _, e := range res.Sent {
		logf("problem reporting: sent " + e.Kind + " report " + e.ReportID)
	}
	if res.Err != nil {
		logf("problem reporting: " + res.Err.Error())
		return 1
	}
	return 0
}

// say writes 1 line to w. The mode's exit code reports the outcome, so a
// broken stream is not a second failure to report.
func say(w io.Writer, line string) {
	_, _ = fmt.Fprintln(w, line)
}

// sayf writes 1 formatted line to w.
func sayf(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}

// seenStore loads the dismiss store of this user, the way the page does:
// problems.SeenPath then problems.LoadSeen. The mode sends no problem that
// the user dismissed and the page does not show.
func seenStore(getenv func(string) string) (*problems.Seen, error) {
	path, err := problems.SeenPath(getenv)
	if err != nil {
		return nil, err
	}
	return problems.LoadSeen(path)
}

// groupOn reports whether problems_reporting_group is enabled. A nil
// configuration keeps the default, which is on.
func groupOn(cfg *config.Config) bool {
	if cfg == nil {
		return true
	}
	return cfg.IsGroupEnabled("problems_page", "problems_reporting_group")
}

// systemOn reports whether problems_system_group is enabled.
func systemOn(cfg *config.Config) bool {
	if cfg == nil {
		return true
	}
	return cfg.IsGroupEnabled("problems_page", "problems_system_group")
}

// appsOn reports whether problems_apps_group is enabled.
func appsOn(cfg *config.Config) bool {
	if cfg == nil {
		return true
	}
	return cfg.IsGroupEnabled("problems_page", "problems_apps_group")
}

// configuredEndpoint returns the endpoint of the reporting group when an
// administrator-owned configuration file set it, and "" otherwise.
func configuredEndpoint(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	raw, adminOwned := cfg.ReportEndpoint()
	if !adminOwned {
		return ""
	}
	return raw
}
