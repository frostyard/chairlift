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
	if len(os.Args) < 2 {
		fatal("usage: chairlift-updex-helper <command> [args...]")
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	client := updex.NewClient(updex.ClientConfig{})
	dryRun := updexhelper.HasDryRunFlag(os.Args[2:])

	switch os.Args[1] {
	case "enable-feature":
		if len(os.Args) < 3 {
			fatal("usage: chairlift-updex-helper enable-feature <name> [--dry-run]")
		}
		result, err := client.EnableFeature(ctx, os.Args[2], updexhelper.EnableOptions(dryRun))
		outputJSON(result, err)
	case "disable-feature":
		if len(os.Args) < 3 {
			fatal("usage: chairlift-updex-helper disable-feature <name> [--dry-run]")
		}
		result, err := client.DisableFeature(ctx, os.Args[2], updexhelper.DisableOptions(dryRun))
		outputJSON(result, err)
	case "update":
		results, err := client.UpdateFeatures(ctx, updexhelper.UpdateOptions(dryRun))
		outputJSON(results, err)
	default:
		fatal("unknown command: " + os.Args[1])
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
