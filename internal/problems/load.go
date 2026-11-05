package problems

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	stdproblems "github.com/frostyard/std/problems"
)

// FixturesEnv names the dry-run test hook: an absolute folder that holds
// problems.json, details.json, coredumps.json (coredumpctl's JSON output)
// and journal.json (journalctl -o json output). ChairLift honors it only
// with --dry-run; then it runs no command for the page.
const FixturesEnv = "CHAIRLIFT_PROBLEMS_FIXTURES"

// CommandTimeout bounds each command of a load.
const CommandTimeout = 10 * time.Second

// maxJournalFixtureBytes bounds the journal.json fixture.
const maxJournalFixtureBytes = 8 << 20

// FixtureDecision is the outcome of DecideFixtures.
type FixtureDecision struct {
	// Use is true when the page reads its sources from Dir.
	Use bool
	Dir string
	// Log is the line to log once when the variable is set but ignored;
	// empty otherwise.
	Log string
}

// DecideFixtures decides whether the value of FixturesEnv applies. An empty
// value is no hook. Without --dry-run, or with a path that is not
// absolute, the value is ignored and Log says why.
func DecideFixtures(value string, dryRun bool) FixtureDecision {
	switch {
	case value == "":
		return FixtureDecision{}
	case !dryRun:
		return FixtureDecision{Log: "problems: " + FixturesEnv + " ignored: it needs --dry-run"}
	case !filepath.IsAbs(value):
		return FixtureDecision{Log: "problems: " + FixturesEnv + " ignored: not an absolute path"}
	default:
		return FixtureDecision{Use: true, Dir: filepath.Clean(value)}
	}
}

// Options select what Load reads.
type Options struct {
	// Now is the time of the load.
	Now time.Time
	// UID is the user whose crashes the apps group shows.
	UID int
	// System and Apps say which groups the page builds.
	System bool
	Apps   bool
	// Fixtures, when Use is true, replaces every source (dry-run only).
	Fixtures FixtureDecision
	// Seen is the dismiss store; nil hides nothing.
	Seen *Seen
	// Coredumpctl and Journalctl are the command names; empty means
	// "coredumpctl" and std's default "journalctl".
	Coredumpctl string
	Journalctl  string
}

// Snapshot is the result of 1 load.
type Snapshot struct {
	System      SystemFile
	SystemItems []Item
	AppItems    []Item
	// SystemHidden and AppsHidden count the dismissed items of each group;
	// Merged counts the system crashes that the merge rule moved to the
	// apps group.
	SystemHidden int
	AppsHidden   int
	Merged       int
	// DetailsErr, CoredumpErr and JournalErr are source failures that the
	// page logs; they show no row.
	DetailsErr  error
	CoredumpErr error
	JournalErr  error
	// JournalDropped counts journal entries that std's reader dropped
	// because they failed validation (forged records).
	JournalDropped int
}

// Load reads every source of the page and applies the merge rule and the
// dismiss store. problems.json is read when either group is built, because
// its envelope is the image identity of the user items too.
func Load(ctx context.Context, opts Options) Snapshot {
	var snap Snapshot
	if !opts.System && !opts.Apps {
		return snap
	}
	problemsPath, detailsPath := ProblemsPath, DetailsPath
	if opts.Fixtures.Use {
		problemsPath = filepath.Join(opts.Fixtures.Dir, "problems.json")
		detailsPath = filepath.Join(opts.Fixtures.Dir, "details.json")
	}
	snap.System = ReadSystem(problemsPath)
	env := snap.System.Envelope

	system := snap.System.Items
	if opts.System && len(system) > 0 {
		details, err := readDetails(detailsPath)
		snap.DetailsErr = err
		applyDetails(system, details)
	}

	var crashes, apps []Item
	if opts.Apps {
		crashes, snap.CoredumpErr = loadCrashes(ctx, opts, env)
		apps, snap.JournalDropped, snap.JournalErr = loadAppRecords(ctx, opts, env)
	}
	if opts.System {
		system, snap.Merged = MergeCrashes(system, crashes)
		snap.SystemItems, snap.SystemHidden = FilterSeen(system, opts.Seen)
	}
	if opts.Apps {
		userItems := append(append([]Item(nil), apps...), crashes...)
		sortItems(userItems)
		if len(userItems) > MaxItems {
			userItems = userItems[:MaxItems]
		}
		snap.AppItems, snap.AppsHidden = FilterSeen(userItems, opts.Seen)
	}
	return snap
}

// loadCrashes reads the user's crashes: the coredumps.json fixture, or
// coredumpctl with CommandTimeout.
func loadCrashes(ctx context.Context, opts Options, env Envelope) ([]Item, error) {
	var data []byte
	var err error
	if opts.Fixtures.Use {
		data, err = readFixture(filepath.Join(opts.Fixtures.Dir, "coredumps.json"), maxCommandOutput)
	} else {
		bin := opts.Coredumpctl
		if bin == "" {
			bin = "coredumpctl"
		}
		cctx, cancel := context.WithTimeout(ctx, CommandTimeout)
		defer cancel()
		data, err = RunCoredumpctl(cctx, bin)
	}
	if err != nil {
		return nil, err
	}
	entries, err := ParseCoredumps(data)
	if err != nil {
		return nil, err
	}
	return CrashItems(entries, opts.UID, env), nil
}

// loadAppRecords reads the app problem records of the user journal through
// std's reader: the journal.json fixture through ParseJSON, or Read with
// CommandTimeout.
func loadAppRecords(ctx context.Context, opts Options, env Envelope) ([]Item, int, error) {
	var res stdproblems.Result
	var err error
	if opts.Fixtures.Use {
		var f *os.File
		f, err = os.Open(filepath.Join(opts.Fixtures.Dir, "journal.json"))
		if errors.Is(err, fs.ErrNotExist) {
			return nil, 0, nil
		}
		if err != nil {
			return nil, 0, err
		}
		defer func() { _ = f.Close() }()
		res, err = stdproblems.ParseJSON(io.LimitReader(f, maxJournalFixtureBytes))
	} else {
		ropts := JournalReadOptions(opts.Now)
		ropts.Journalctl = opts.Journalctl
		cctx, cancel := context.WithTimeout(ctx, CommandTimeout)
		defer cancel()
		res, err = stdproblems.Read(cctx, ropts)
	}
	if err != nil {
		return nil, 0, err
	}
	return AppItems(res, env), res.Dropped, nil
}

// readFixture reads a fixture file of at most max bytes; a missing file is
// empty.
func readFixture(path string, limit int) ([]byte, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > limit {
		return nil, fmt.Errorf("%s: larger than %d bytes", path, limit)
	}
	return data, nil
}

// LogLines returns the lines the page logs after a load: source failures
// and dropped counts. They show nowhere else.
func (s Snapshot) LogLines() []string {
	var lines []string
	if s.System.Err != nil {
		lines = append(lines, fmt.Sprintf("problems: system checks %s: %v", s.System.Outcome, s.System.Err))
	}
	if s.System.Dropped > 0 || s.System.Truncated > 0 {
		lines = append(lines, fmt.Sprintf("problems: problems.json: dropped %d invalid and %d past the limit of %d", s.System.Dropped, s.System.Truncated, MaxItems))
	}
	if s.DetailsErr != nil {
		lines = append(lines, fmt.Sprintf("problems: details.json not used: %v", s.DetailsErr))
	}
	if s.CoredumpErr != nil {
		lines = append(lines, fmt.Sprintf("problems: crash list not read: %v", s.CoredumpErr))
	}
	if s.JournalErr != nil {
		lines = append(lines, fmt.Sprintf("problems: app problem records not read: %v", s.JournalErr))
	}
	if s.JournalDropped > 0 {
		lines = append(lines, fmt.Sprintf("problems: dropped %d journal records that failed validation", s.JournalDropped))
	}
	if s.Merged > 0 {
		lines = append(lines, fmt.Sprintf("problems: %d system crashes shown once in the apps group", s.Merged))
	}
	return lines
}

// SourceInvalid returns the error of a problems.json that exists but is
// malformed: ChairLift records it as the coded error
// problems.source_invalid. It is nil for every other outcome.
func (s Snapshot) SourceInvalid() error {
	if s.System.Outcome == SourceMalformed {
		return s.System.Err
	}
	return nil
}

// BuiltMarker returns the log line that the page writes after it is
// built: "problems: page built system=<n> apps=<n> source=<outcome>". The
// first 3 words are a readiness marker of the e2e test (ADR-0008).
func BuiltMarker(system, apps int, source string) string {
	return fmt.Sprintf("problems: page built system=%d apps=%d source=%s", system, apps, source)
}
