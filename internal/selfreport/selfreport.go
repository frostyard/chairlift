// Package selfreport records ChairLift's own problems (ADR-0015): its panics
// and a few coded internal errors, as frostyard.problem/1 journal records
// written by github.com/frostyard/std/problems. Records stay in the local
// journal; ChairLift's Problems page shows them in the apps group like any
// other frostyard app's records.
//
// The package imports no GTK binding, so every package of ChairLift can
// record a coded error from its real error path.
package selfreport

import (
	"log"
	"sync"

	stdproblems "github.com/frostyard/std/problems"
)

// AppID is ChairLift's S8 app ID.
const AppID = "org.frostyard.ChairLift"

// ChairLift's error codes. Each code is a constant here and nowhere else,
// matches ^[a-z][a-z0-9_.]{0,63}$, has a hint in internal/problems, and is
// recorded at 1 real internal-fault path (TestCodesAreRecordedAtTheirPaths).
const (
	// CodeConfigLoadFailed: the authoritative configuration file could not
	// be read or validated, so every group is off (ADR-0003).
	CodeConfigLoadFailed = "config.load_failed"
	// CodeProblemsSourceInvalid: /var/lib/snosi-health/problems.json exists
	// but is malformed.
	CodeProblemsSourceInvalid = "problems.source_invalid"
	// CodeFirstRunCompleteFailed: the first-run completion markers could not
	// be written.
	CodeFirstRunCompleteFailed = "firstrun.complete_failed"
	// CodeUpdexResultsUnreadable: the updex helper exited 0 but its results
	// could not be read.
	CodeUpdexResultsUnreadable = "updex.results_unreadable"
)

// Codes lists every code of this package.
var Codes = []string{
	CodeConfigLoadFailed,
	CodeProblemsSourceInvalid,
	CodeFirstRunCompleteFailed,
	CodeUpdexResultsUnreadable,
}

var (
	mu       sync.Mutex
	recorder *stdproblems.Recorder
)

// Init makes the process recorder for the release version and returns it.
// main calls it once, right after it sets version.Version, and then defers
// RecoverPanic on the result. Init also calls CrashOutput, which records the
// crash of an earlier run at this start and covers the panics of every
// goroutine and fatal errors of this run.
//
// A version that is not a release <semver> ("dev", a snapshot) makes the
// recorder inert: it writes nothing. Init logs that once.
func Init(version string, opts ...stdproblems.Option) *stdproblems.Recorder {
	rec := stdproblems.New(AppID, version, stdproblems.ComponentGUI, opts...)
	mu.Lock()
	recorder = rec
	mu.Unlock()
	if err := rec.Err(); err != nil {
		log.Printf("problems: ChairLift's own problem records are off: %v", err)
		return rec
	}
	if err := rec.CrashOutput(); err != nil {
		log.Printf("problems: crash output is not recorded: %v", err)
	}
	return rec
}

// Recorder returns the process recorder, or nil before Init (a nil
// recorder is inert).
func Recorder() *stdproblems.Recorder {
	mu.Lock()
	defer mu.Unlock()
	return recorder
}

// Error records a coded error with err's text as the local MESSAGE. It is
// logging, not a system change, so it also runs under --dry-run. Before
// Init, or with an inert recorder, it writes nothing.
func Error(code string, err error) {
	Recorder().Error(code, err)
}
