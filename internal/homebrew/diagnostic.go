package homebrew

import "strings"

const (
	// diagnosticSummaryLimit bounds, in runes, the distilled failure line
	// carried in a state-changing command's Error message. The message ends
	// up in a toast, so an unbounded line would be unreadable however it is
	// rendered.
	diagnosticSummaryLimit = 240
	// diagnosticSummaryTail is how many runes of an over-long line are kept
	// from its end. Elision takes the middle rather than the end because the
	// two ends carry different information: a diagnostic opens with what
	// failed and often closes with the path or reference it failed on.
	diagnosticSummaryTail = 60
	// diagnosticEllipsis marks where the middle of an over-long line was cut.
	diagnosticEllipsis = " … "
)

// joinCommandStreams combines a failed command's stdout and stderr into one
// diagnostic text, stdout first, omitting a stream that is blank.
//
// The order matters for summarizeDiagnostic. `brew bundle` runs each entry's
// real installer (flatpak, mas, whalebrew, ...) with its stderr merged into
// its stdout, buffers that output, and replays it on stdout when the entry
// fails, while brew's own restatements ("<verb> <name> has failed!" and
// "`brew bundle` failed! N Brewfile dependencies failed to install") go to
// stderr. Reading stdout first means the first error line found is the
// installer's root cause rather than brew's count of failed entries.
func joinCommandStreams(stdout, stderr string) string {
	parts := make([]string, 0, 2)
	for _, part := range []string{stdout, stderr} {
		if strings.TrimSpace(part) != "" {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, "\n")
}

// summarizeDiagnostic reduces captured brew output to the single most
// informative line, in three tiers: the first explicit error line
// (isErrorLine), else the first line reporting a failure (isFailureLine),
// else the last non-blank line. The chosen line is shortened by elideMiddle.
// Blank output summarizes to "".
//
// A failing `brew bundle install` prints progress for every entry it
// attempts before it prints why one failed, so the raw text starts with
// lines like "Installing io.podman_desktop.PodmanDesktop" and buries the
// cause further down; a single-line toast of the whole block showed only
// the progress line.
func summarizeDiagnostic(output string) string {
	lines := diagnosticLines(output)
	if len(lines) == 0 {
		return ""
	}
	for _, match := range []func(string) bool{isErrorLine, isFailureLine} {
		for _, line := range lines {
			if match(line) {
				return elideMiddle(line)
			}
		}
	}
	return elideMiddle(lines[len(lines)-1])
}

// diagnosticLines splits captured output into trimmed, non-empty lines.
// Carriage returns are progress rewrites of a single terminal line, so only
// the final state of such a line is kept. A trailing CR is a line ending
// (CRLF), not a rewrite, so it is trimmed before the last rewrite is chosen;
// otherwise a CRLF-terminated line would collapse to nothing.
func diagnosticLines(output string) []string {
	var lines []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimRight(line, "\r")
		if idx := strings.LastIndex(line, "\r"); idx >= 0 {
			line = line[idx+1:]
		}
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// isErrorLine reports whether a line is a tool's own error report:
// brew's "Error: ...", the lowercase "error: ..." of the installers brew
// delegates to (flatpak and other bundle extensions), git's "fatal: ...",
// and brew's "Failure while executing ..." wrapper. Matching is
// case-insensitive.
func isErrorLine(line string) bool {
	lower := strings.ToLower(line)
	return strings.HasPrefix(lower, "error:") ||
		strings.HasPrefix(lower, "fatal:") ||
		strings.Contains(lower, "failure while executing")
}

// isFailureLine reports whether a line states that something failed without
// an error prefix. `brew bundle` reports a failing entry this way
// ("Installing <name> has failed!") and names the entry, which its trailing
// "Homebrew Bundle failed!" count does not.
func isFailureLine(line string) bool {
	lower := strings.ToLower(line)
	return strings.Contains(lower, "failed") || strings.Contains(lower, "failure")
}

// elideMiddle shortens a line longer than diagnosticSummaryLimit runes by
// replacing its middle with diagnosticEllipsis, keeping the last
// diagnosticSummaryTail runes, so the result is exactly
// diagnosticSummaryLimit runes. It counts runes, not bytes, so a cut never
// lands inside a multi-byte character. Shorter lines are returned unchanged.
func elideMiddle(line string) string {
	runes := []rune(line)
	if len(runes) <= diagnosticSummaryLimit {
		return line
	}
	head := diagnosticSummaryLimit - diagnosticSummaryTail - len([]rune(diagnosticEllipsis))
	return string(runes[:head]) + diagnosticEllipsis + string(runes[len(runes)-diagnosticSummaryTail:])
}
