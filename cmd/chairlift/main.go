// ChairLift - A modern GTK4/Libadwaita system management tool
// Written in Go using puregotk bindings
package main

import (
	"log"
	"os"

	"github.com/frostyard/chairlift/internal/app"
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
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Set version info for use by the rest of the application
	version.Version = buildVersion
	version.Commit = buildCommit
	version.Date = buildDate
	version.BuiltBy = buildBy

	application := app.New()
	defer application.Unref()

	if code := application.Run(len(os.Args), os.Args); code > 0 {
		os.Exit(code)
	}
}
