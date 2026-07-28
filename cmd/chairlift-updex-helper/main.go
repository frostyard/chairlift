// chairlift-updex-helper is a privileged helper binary for updex write operations.
// It is invoked via pkexec from the main chairlift application to perform
// operations that require root access (enable/disable features, update).
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/frostyard/chairlift/internal/updexhelper"
	"github.com/frostyard/updex/updex"
)

const defaultTimeout = 5 * time.Minute

func main() {
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
		outputJSON(results, err)
	}
}

func outputJSON(v any, err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	enc := json.NewEncoder(os.Stdout)
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(os.Stderr, "failed to encode JSON: %v\n", err)
		os.Exit(1)
	}
}

func fatal(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}
