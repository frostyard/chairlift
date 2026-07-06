// ChairLift - A modern GTK4/Libadwaita system management tool
// Written in Go using puregotk bindings
package main

import (
	"log"
	"os"
	"time"

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
	processStart := time.Now()
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("main: process start")

	// Set version info for use by the rest of the application
	version.Version = buildVersion
	version.Commit = buildCommit
	version.Date = buildDate
	version.BuiltBy = buildBy

	application := app.New()
	defer application.Unref()
	log.Printf("main: application created in %s", time.Since(processStart))

	if code := application.Run(int32(len(os.Args)), os.Args); code > 0 {
		os.Exit(int(code))
	}
}
