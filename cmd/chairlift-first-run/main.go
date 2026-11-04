// chairlift-first-run starts ChairLift's first run once for each user
// (ADR-0014). It is a pure-Go program: it imports no GTK binding, so it can
// exit before any GTK library loads when the user already finished the
// first run. Every decision lives in internal/firstrun.Gate; this file only
// reads the process state, prints, and execs.
package main

import (
	"fmt"
	"os"
	"syscall"

	"github.com/frostyard/chairlift/internal/firstrun"
)

func main() {
	home, _ := os.UserHomeDir()
	result := firstrun.Gate(firstrun.GateEnv{
		Args:   os.Args[1:],
		UID:    os.Getuid(),
		Getenv: os.Getenv,
		Home:   home,
	})
	// A failed write to a closed stream changes nothing the gate decides.
	_, _ = fmt.Fprint(os.Stdout, result.Stdout)
	_, _ = fmt.Fprint(os.Stderr, result.Stderr)
	if result.ExecPath != "" {
		err := syscall.Exec(result.ExecPath, result.ExecArgv, os.Environ())
		_, _ = fmt.Fprintf(os.Stderr, "first-run: cannot start %s: %v\n", result.ExecPath, err)
		os.Exit(1)
	}
	os.Exit(result.Code)
}
