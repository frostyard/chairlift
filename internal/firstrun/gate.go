package firstrun

import (
	"fmt"
	"strings"
)

// ChairLiftPath is the fixed path the gate execs. The gate never searches
// PATH: the snosi autostart entry runs /usr/bin/chairlift-first-run, and the
// first run belongs to the packaged ChairLift beside it.
const ChairLiftPath = "/usr/bin/chairlift"

// GateEnv is everything Gate reads from the process.
type GateEnv struct {
	// Args are the command-line arguments without the program name.
	Args []string
	// UID is the real user ID.
	UID int
	// Getenv reads an environment variable (os.Getenv in production).
	Getenv func(string) string
	// Home is the user's home directory ("" when unknown).
	Home string
}

// GateResult is what cmd/chairlift-first-run does: print Stdout and Stderr,
// then exec ExecPath with ExecArgv when ExecPath is set, else exit with Code.
type GateResult struct {
	Code     int
	Stdout   string
	Stderr   string
	ExecPath string
	ExecArgv []string
}

// gateUsage is the --help text of chairlift-first-run. The marker names
// come from markers.go, the only file that spells them.
var gateUsage = `Usage: chairlift-first-run [--dry-run] [--mark-done]

Start ChairLift's first run (chairlift --first-run) once for each user.
It exits at once, without loading GTK, when the user already finished the
first run (${XDG_CONFIG_HOME:-~/.config}/` + markerDirName + "/" + markerName + ` or
${XDG_CONFIG_HOME:-~/.config}/` + legacyMarkerName + ` exists) or runs as root.

Options:
  --dry-run     Pass --dry-run to ChairLift; with --mark-done, write nothing.
  --mark-done   Record the first run as finished and exit (no GTK).
  -h, --help    Show this help and exit.
`

// gateOptions are the parsed arguments of chairlift-first-run.
type gateOptions struct {
	help     bool
	dryRun   bool
	markDone bool
}

// parseGateArgs parses the arguments. Every argument must be a known option;
// repeating one is allowed.
func parseGateArgs(args []string) (gateOptions, error) {
	var opts gateOptions
	for _, arg := range args {
		switch arg {
		case "-h", "--help":
			opts.help = true
		case "--dry-run":
			opts.dryRun = true
		case "--mark-done":
			opts.markDone = true
		default:
			return gateOptions{}, fmt.Errorf("unknown argument %q", arg)
		}
	}
	return opts, nil
}

// Gate decides what chairlift-first-run does. In order:
//
//   - an unknown argument prints the usage to stderr and exits 2;
//   - --help or -h prints the usage to stdout and exits 0;
//   - UID 0 exits 0: the first run and its markers are per-user, and root
//     must not write markers in root's home (also for --mark-done);
//   - an unknown config home exits 1;
//   - --mark-done calls Complete (MarkDone, or the dry-run preview with
//     --dry-run) and exits 0, or 1 when it fails;
//   - an existing marker exits 0;
//   - else it execs /usr/bin/chairlift --first-run [--dry-run].
//
// Each skip logs one "first-run: ..." line to stderr, so the user journal
// shows why no window opened.
func Gate(env GateEnv) GateResult {
	opts, err := parseGateArgs(env.Args)
	if err != nil {
		return GateResult{Code: 2, Stderr: "chairlift-first-run: " + err.Error() + "\n" + gateUsage}
	}
	if opts.help {
		return GateResult{Code: 0, Stdout: gateUsage}
	}
	if env.UID == 0 {
		return GateResult{Code: 0, Stderr: "first-run: skipped for root: the first run and its markers are per-user\n"}
	}

	paths, err := ResolvePaths(env.Getenv, env.Home)
	if err != nil {
		return GateResult{Code: 1, Stderr: "first-run: cannot find the config home: " + err.Error() + "\n"}
	}

	if opts.markDone {
		decision, err := Complete(opts.dryRun, paths)
		if err != nil {
			return GateResult{Code: 1, Stderr: decision.Log + "\n"}
		}
		return GateResult{Code: 0, Stderr: decision.Log + "\n"}
	}

	if IsDone(paths) {
		return GateResult{Code: 0, Stderr: "first-run: already done\n"}
	}

	argv := []string{"chairlift", "--first-run"}
	if opts.dryRun {
		argv = append(argv, "--dry-run")
	}
	return GateResult{
		ExecPath: ChairLiftPath,
		ExecArgv: argv,
		Stderr:   "first-run: starting " + ChairLiftPath + " " + strings.Join(argv[1:], " ") + "\n",
	}
}
