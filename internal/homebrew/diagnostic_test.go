package homebrew

import (
	"strings"
	"testing"
)

// bundleFailureStdout and bundleFailureStderr are the two streams a failing
// `brew bundle install` produces for a flatpak-only Brewfile. Brew replays
// the flatpak subprocess's own output on stdout and prints its restatements
// on stderr, so the cause is on stdout and only progress precedes it.
const (
	bundleFailureStdout = `Installing io.podman_desktop.PodmanDesktop
error: Remote "flathub" not found`
	bundleFailureStderr = `Error: Installing io.podman_desktop.PodmanDesktop has failed!
Error: ` + "`brew bundle`" + ` failed! 1 Brewfile dependency failed to install`
)

func TestSummarizeDiagnosticPrefersTheInstallerErrorOverProgress(t *testing.T) {
	got := summarizeDiagnostic(joinCommandStreams(bundleFailureStdout, bundleFailureStderr))
	want := `error: Remote "flathub" not found`
	if got != want {
		t.Errorf("summarizeDiagnostic = %q, want %q", got, want)
	}
}

// Keeping stderr alone (the runner's old behavior) leaves only brew's
// restatement, which never names a cause.
func TestSummarizeDiagnosticOnStderrAloneLosesTheCause(t *testing.T) {
	got := summarizeDiagnostic(bundleFailureStderr)
	if strings.Contains(got, "flathub") {
		t.Fatalf("summary = %q; the fixture is meant to omit the cause", got)
	}
	if got != "Error: Installing io.podman_desktop.PodmanDesktop has failed!" {
		t.Errorf("summarizeDiagnostic = %q, want brew's first restatement", got)
	}
}

func TestSummarizeDiagnosticPrefersTheFirstErrorLine(t *testing.T) {
	output := strings.Join([]string{
		"==> Downloading https://example.invalid/bottle",
		"fatal: unable to access 'https://example.invalid/': Could not resolve host",
		"Error: Failure while executing; `git clone` exited with 128.",
	}, "\n")
	want := "fatal: unable to access 'https://example.invalid/': Could not resolve host"
	if got := summarizeDiagnostic(output); got != want {
		t.Errorf("summarizeDiagnostic = %q, want the first error line %q", got, want)
	}
}

func TestSummarizeDiagnosticFallsBackToAFailureLine(t *testing.T) {
	output := strings.Join([]string{
		"Installing mas-cli",
		"Installing mas-cli has failed!",
		"Homebrew Bundle failed! 1 Brewfile dependency failed to install.",
	}, "\n")
	want := "Installing mas-cli has failed!"
	if got := summarizeDiagnostic(output); got != want {
		t.Errorf("summarizeDiagnostic = %q, want %q", got, want)
	}
}

func TestSummarizeDiagnosticFallsBackToTheLastLine(t *testing.T) {
	output := "first line\nsecond line\nlast line\n\n"
	if got := summarizeDiagnostic(output); got != "last line" {
		t.Errorf("summarizeDiagnostic = %q, want %q", got, "last line")
	}
}

func TestSummarizeDiagnosticReturnsEmptyForBlankOutput(t *testing.T) {
	for _, output := range []string{"", "\n", "   \n\t\n", "\r\n"} {
		if got := summarizeDiagnostic(output); got != "" {
			t.Errorf("summarizeDiagnostic(%q) = %q, want an empty summary", output, got)
		}
	}
}

func TestSummarizeDiagnosticKeepsOnlyTheFinalStateOfARewrittenLine(t *testing.T) {
	output := "##O#- #\r####  10.0%\r#####  99.9%\nError: bottle is corrupt"
	if got := summarizeDiagnostic(output); got != "Error: bottle is corrupt" {
		t.Errorf("summarizeDiagnostic = %q, want the error line", got)
	}
	rewritesOnly := "####  10.0%\r#####  99.9%"
	if got := summarizeDiagnostic(rewritesOnly); got != "#####  99.9%" {
		t.Errorf("summarizeDiagnostic = %q, want the final rewrite state", got)
	}
}

func TestSummarizeDiagnosticKeepsCRLFTerminatedLines(t *testing.T) {
	output := "Warning: tap is shallow\r\nError: bottle is corrupt\r\n"
	if got := summarizeDiagnostic(output); got != "Error: bottle is corrupt" {
		t.Errorf("summarizeDiagnostic = %q, want the CRLF-terminated error line", got)
	}
}

func TestSummarizeDiagnosticKeepsBothEndsOfAnOverlongLine(t *testing.T) {
	line := "Error: " + strings.Repeat("x", diagnosticSummaryLimit*2) + " /usr/share/snow/bundles/dev.Brewfile"
	got := summarizeDiagnostic(line)
	if n := len([]rune(got)); n != diagnosticSummaryLimit {
		t.Errorf("summary length = %d runes, want %d", n, diagnosticSummaryLimit)
	}
	if !strings.HasPrefix(got, "Error: xxx") {
		t.Errorf("summary = %q, want it to keep the head of the line", got)
	}
	if !strings.HasSuffix(got, "dev.Brewfile") {
		t.Errorf("summary = %q, want it to keep the tail of the line", got)
	}
	if !strings.Contains(got, diagnosticEllipsis) {
		t.Errorf("summary = %q, want it to mark the elided middle", got)
	}
}

func TestDiagnosticLinesSplitsTrimsAndDropsBlankLines(t *testing.T) {
	got := diagnosticLines("  one  \n\n two\r\n###\r three\r\n \n")
	want := []string{"one", "two", "three"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("diagnosticLines = %q, want %q", got, want)
	}
	if got := diagnosticLines(""); len(got) != 0 {
		t.Errorf("diagnosticLines(\"\") = %q, want none", got)
	}
}

func TestErrorLineRecognizesToolErrorReports(t *testing.T) {
	for _, tt := range []struct {
		line string
		want bool
	}{
		{"Error: No such keg", true},
		{`error: Remote "flathub" not found`, true},
		{"ERROR: shouting", true},
		{"fatal: not a git repository", true},
		{"==> Failure while executing `git fetch`", true},
		{"Installing mas-cli has failed!", false},
		{"Warning: Error: in the middle", false},
		{"Installing foo", false},
	} {
		if got := isErrorLine(tt.line); got != tt.want {
			t.Errorf("isErrorLine(%q) = %v, want %v", tt.line, got, tt.want)
		}
	}
}

func TestFailureLineRecognizesFailureReports(t *testing.T) {
	for _, tt := range []struct {
		line string
		want bool
	}{
		{"Installing mas-cli has failed!", true},
		{"Homebrew Bundle FAILED", true},
		{"network failure", true},
		{"Installing mas-cli", false},
		{"", false},
	} {
		if got := isFailureLine(tt.line); got != tt.want {
			t.Errorf("isFailureLine(%q) = %v, want %v", tt.line, got, tt.want)
		}
	}
}

func TestElideMiddleCutsOnRuneBoundaries(t *testing.T) {
	line := "Error: " + strings.Repeat("é", diagnosticSummaryLimit*2) + " done"
	got := elideMiddle(line)
	if strings.ContainsRune(got, '�') {
		t.Errorf("elideMiddle = %q, want no replacement characters", got)
	}
	if n := len([]rune(got)); n != diagnosticSummaryLimit {
		t.Errorf("elideMiddle length = %d runes, want %d", n, diagnosticSummaryLimit)
	}
	if !strings.HasSuffix(got, " done") {
		t.Errorf("elideMiddle = %q, want the tail kept", got)
	}
}

func TestElideMiddleLeavesShortLinesAlone(t *testing.T) {
	for _, line := range []string{
		`Error: No formulae or casks found for "demo".`,
		strings.Repeat("y", diagnosticSummaryLimit),
	} {
		if got := elideMiddle(line); got != line {
			t.Errorf("elideMiddle(%d runes) changed a line within the limit", len([]rune(line)))
		}
	}
}

func TestJoinCommandStreamsReadsStdoutBeforeStderr(t *testing.T) {
	if got := joinCommandStreams("from stdout", "from stderr"); got != "from stdout\nfrom stderr" {
		t.Errorf("joinCommandStreams = %q, want stdout first", got)
	}
}

func TestJoinCommandStreamsSkipsBlankStreams(t *testing.T) {
	for _, tt := range []struct {
		name           string
		stdout, stderr string
		want           string
	}{
		{name: "empty stderr", stdout: "out", stderr: "", want: "out"},
		{name: "blank stderr", stdout: "out", stderr: " \n", want: "out"},
		{name: "empty stdout", stdout: "", stderr: "err", want: "err"},
		{name: "both blank", stdout: "", stderr: "\n", want: ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := joinCommandStreams(tt.stdout, tt.stderr); got != tt.want {
				t.Errorf("joinCommandStreams = %q, want %q", got, tt.want)
			}
		})
	}
}
