// ChairLift - A modern GTK4/Libadwaita system management tool
// Written in Go using puregotk bindings
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/frostyard/chairlift/internal/app"
	"github.com/frostyard/chairlift/internal/config"
	"github.com/frostyard/chairlift/internal/problems"
	"github.com/frostyard/chairlift/internal/reporting"
	"github.com/frostyard/chairlift/internal/selfreport"
	"github.com/frostyard/chairlift/internal/version"
)

// Build information set via ldflags by goreleaser
var (
	buildVersion = "dev"
	buildCommit  = "unknown"
	buildDate    = "unknown"
	buildBy      = "unknown"
)

// The command-line modes that end before the GTK application exists.
const (
	// sendPendingFlag runs the automatic problem-report send and exits
	// (ADR-0015).
	sendPendingFlag = "--send-pending-reports"
	// dryRunFlag and dryRunShortFlag change nothing (ADR-0009).
	dryRunFlag      = "--dry-run"
	dryRunShortFlag = "-d"
)

func main() {
	processStart := time.Now()
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("main: process start")

	// Set version info for use by the rest of the application
	version.Version = buildVersion
	version.Commit = buildCommit
	version.Date = buildDate
	version.BuiltBy = buildBy

	// Record ChairLift's own panics and crashes (ADR-0015). Init records the
	// crash of an earlier run and covers every goroutine's panics and fatal
	// errors through the crash output file; RecoverPanic must be deferred
	// directly here. A dev or snapshot version makes the recorder inert.
	recorder := selfreport.Init(version.Version)
	defer recorder.RecoverPanic()

	// --send-pending-reports sends what the consent allows and exits, with
	// no window and no GTK (ADR-0015).
	if hasArgument(os.Args[1:], sendPendingFlag) {
		os.Exit(sendPendingReports())
	}

	application := app.New()
	defer application.Unref()
	log.Printf("main: application created in %s", time.Since(processStart))

	if code := application.Run(int32(len(os.Args)), os.Args); code > 0 {
		os.Exit(int(code))
	}
}

// hasArgument reports whether args hold name.
func hasArgument(args []string, name string) bool {
	for _, arg := range args {
		if arg == name {
			return true
		}
	}
	return false
}

// sendPendingReports runs the windowless problem-report send and returns the
// exit code: 1 for a configuration error or a failed send, and 0 otherwise.
func sendPendingReports() int {
	dryRun := hasArgument(os.Args[1:], dryRunFlag) || hasArgument(os.Args[1:], dryRunShortFlag)
	cfg, cfgErr := config.Load()
	var configErr error
	if cfgErr != nil {
		configErr = cfgErr
	}
	now := time.Now
	in := reporting.CLIInput{
		Getenv:      os.Getenv,
		Config:      cfg,
		ConfigErr:   configErr,
		Version:     version.Version,
		DryRun:      dryRun,
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
		Now:         now,
		Pace:        7 * time.Second,
		InstallInfo: reporting.DefaultInstallInfo,
		NewSender:   func(endpoint string) reporting.Sender { return reporting.NewHTTPSender(endpoint) },
		Load: func(ctx context.Context, system, apps bool, seen *problems.Seen) problems.Snapshot {
			fixtures := problems.DecideFixtures(os.Getenv(problems.FixturesEnv), dryRun)
			if fixtures.Log != "" {
				fmt.Fprintln(os.Stderr, fixtures.Log)
			}
			return problems.Load(ctx, problems.Options{
				Now:      now(),
				UID:      os.Getuid(),
				System:   system,
				Apps:     apps,
				Fixtures: fixtures,
				Seen:     seen,
			})
		},
	}
	return reporting.SendPending(context.Background(), in)
}
