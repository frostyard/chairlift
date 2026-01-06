// ChairLift - A modern GTK4/Libadwaita system management tool
// Written in Go using puregotk bindings
package main

import (
	"log"
	"os"

	"github.com/frostyard/chairlift/internal/app"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	application := app.New()
	defer application.Unref()

	if code := application.Run(len(os.Args), os.Args); code > 0 {
		os.Exit(code)
	}
}
