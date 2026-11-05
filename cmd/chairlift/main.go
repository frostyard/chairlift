// ChairLift - A modern GTK4/Libadwaita system management tool
// Written in Go using puregotk bindings
package main

import (
	"log"
	"os"
	"time"

	"github.com/frostyard/chairlift/internal/app"
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

	application := app.New()
	defer application.Unref()
	log.Printf("main: application created in %s", time.Since(processStart))

	if code := application.Run(int32(len(os.Args)), os.Args); code > 0 {
		os.Exit(int(code))
	}
}
