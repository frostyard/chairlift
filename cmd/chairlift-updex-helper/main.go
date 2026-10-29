// chairlift-updex-helper is a privileged helper binary for updex write operations.
// It is invoked via pkexec from the main chairlift application to perform
// operations that require root access (enable/disable features, update).
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/frostyard/chairlift/internal/updexhelper"
	"github.com/frostyard/updex/v2/updex"
)

const defaultTimeout = 5 * time.Minute

// resultOut is the real stdout. main keeps it for the JSON result and
// points os.Stdout at stderr, so nothing else can write to the result stream.
var resultOut = os.Stdout

func main() {
	// The updex SDK runs systemd-sysext, systemd-sysusers and systemd-tmpfiles
	// with cmd.Stdout = os.Stdout (sysext.DefaultRunner.exec), read when the SDK
	// builds the command. Their text would come before the JSON result and make
	// stdout unparsable for the GUI. Redirect os.Stdout to stderr before the SDK
	// runs; only the JSON result goes to resultOut.
	os.Stdout = os.Stderr

	invocation, err := updexhelper.ParseInvocation(os.Args[1:])
	if err != nil {
		fatal(err.Error())
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	client := updex.NewClient(updex.ClientConfig{})

	switch invocation.Command {
	case updexhelper.CommandEnableFeature:
		result, err := client.EnableFeature(ctx, invocation.Feature, updexhelper.EnableOptions(invocation.DryRun))
		outputJSON(result, err)
	case updexhelper.CommandDisableFeature:
		result, err := client.DisableFeature(ctx, invocation.Feature, updexhelper.DisableOptions(invocation.DryRun))
		outputJSON(result, err)
	case updexhelper.CommandUpdate:
		results, err := client.UpdateFeatures(ctx, updexhelper.UpdateOptions(invocation.DryRun))
		if err := updexhelper.WriteUpdateResult(resultOut, results, err); err != nil {
			// Results, when any, are already on stdout. The exit code tells
			// the GUI which failure this is (see updexhelper.ExitCode).
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(updexhelper.ExitCode(err))
		}
	}
}

func outputJSON(v any, err error) {
	if err := updexhelper.WriteResult(resultOut, v, err); err != nil {
		fatal(err.Error())
	}
}

func fatal(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(updexhelper.ExitFailure)
}
