# ChairLift Modernization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the deprecated nbc integration with bootc (via the snosi stage script), add a Homebrew 6 tap-trust UI, bump updex to v1.2.3, and verify async startup.

**Architecture:** ChairLift is a GTK4/Libadwaita Go app using puregotk (no CGO). External tools are wrapped in `internal/<tool>` packages following a shared shape (SetDryRun, IsInstalledCached, context timeouts, custom error types); views call wrappers from goroutines and marshal UI updates via `sgtk.RunOnMainThread()`. This plan adds `internal/bootc`, deletes `internal/nbc`, and extends `internal/homebrew`.

**Tech Stack:** Go 1.24+, puregotk v4 (adw/gtk), `github.com/frostyard/snowkit` (sgtk main-thread dispatch), pkexec/PolicyKit for privileged ops.

**Spec:** `docs/superpowers/specs/2026-07-06-modernization-design.md`

## Global Constraints

- Branch: `feat/modernize`, all commits on it. Single PR → `main` at the end.
- Build must stay green after every task: `make build` (CGO_ENABLED=0) succeeds.
- Tests: `go test ./...` passes after every task (repo currently has ZERO test files; tasks below add the first ones).
- Commit messages: conventional commits (`feat:`, `fix:`, `docs:`, `chore:`), each ending with the trailer `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`.
- Wrapper package conventions (copy from existing packages, do not invent new shapes): module-level `dryRun` + `SetDryRun()/IsDryRun()`, `DefaultContext()`, `Error`/`NotFoundError` types, `IsInstalledCached()`-style `sync.Once` caching.
- UI updates ONLY inside `sgtk.RunOnMainThread(func(){...})`. GTK/adw calls from goroutines crash.
- puregotk callback pattern: callbacks are passed as POINTERS to func variables that must outlive the call, e.g. `cb := func(btn gtk.Button){...}; btn.ConnectClicked(&cb)`.
- bootc stage script path (fixed, from snow images): `/usr/libexec/bootc-update-stage`.
- Do not add any new external Go dependencies. The only go.mod change is `github.com/frostyard/updex` v1.2.0 → v1.2.3 (Task 10).
- After any source change, relevant docs must be updated before the work is "complete" (project CLAUDE.md rule) — Task 12 covers this; do not skip it.

## Verified Facts (do not re-derive)

These were verified on the dev host (Snow Linux, nbc-booted, brew 6.0.8, bootc 1.16.3):

- `bootc status --format json` works **unprivileged** and exits 0 even on non-bootc hosts, printing `{"status":{"booted":null,...}}`. The gate for "is this a bootc system" is `status.booted != null`, NOT the exit code, NOT `/run/ostree-booted` (absent on snow composefs systems).
- `brew tap-info --installed --json` entries carry `"trusted": true|false`.
- Formulae from untrusted taps are **invisible** to `brew list --full-name` and cannot be loaded by brew at all. The only reliable installed-package→tap mapping is the Cellar: `<prefix>/Cellar/<name>/<version>/INSTALL_RECEIPT.json` has `.source.tap` (e.g. `"multica-ai/tap"`). For casks: `<prefix>/Caskroom/<token>/.metadata/<version>/<timestamp>/Casks/<token>.json` has top-level `.tap`; casks installed from the API may lack this file — treat those as trusted (they're from homebrew/cask).
- `brew trust --formula <user/tap/name>` / `brew trust --cask <...>` write a per-user trust store (`~/.homebrew/trust.json`). No root, no pkexec.
- Failed brew operations mention untrusted taps with the phrases `"not trusted"` / `"untrusted tap"` (e.g. `Refusing to load formula X from untrusted tap Y`).
- puregotk pinned version has `adw.NewAlertDialog(heading, body)`, `AddResponse(id, label)`, `SetResponseAppearance(id, adw.ResponseSuggestedValue|ResponseDestructiveValue)`, `ConnectResponse(*func(AlertDialog, string))`, and `Present(*gtk.Widget)` (inherited from `adw.Dialog`).
- updex v1.2.3 compiles against existing chairlift code unchanged.

---

### Task 1: `internal/bootc` — status types, parsing, boot gate

**Files:**
- Create: `internal/bootc/bootc.go`
- Test: `internal/bootc/bootc_test.go`

**Interfaces:**
- Consumes: nothing (new package).
- Produces (used by Tasks 2–4):
  - `type Status struct { Spec SpecInfo; Status StatusInfo }` with `Status.Booted/Staged/Rollback *Deployment`
  - `func (d *Deployment) ImageRef() string`, `Version() string`, `Timestamp() string`, `Digest() string` (all nil-safe: return "" on nil receiver)
  - `func parseStatus(data []byte) (*Status, error)`
  - `func GetStatus(ctx context.Context) (*Status, error)`
  - `func IsBootcBooted(ctx context.Context) bool`
  - `func IsBootcBootedCached() bool`
  - `func SetDryRun(bool)`, `IsDryRun() bool`, `DefaultContext() (context.Context, context.CancelFunc)` (30 min)
  - `type Error struct{ Message string }`, `type NotFoundError struct{ Message string }`

- [ ] **Step 1: Write the failing test**

Create `internal/bootc/bootc_test.go`:

```go
package bootc

import "testing"

// nonBootcJSON is real output captured from `bootc status --format json`
// on a non-bootc (nbc-booted) host.
const nonBootcJSON = `{"apiVersion":"org.containers.bootc/v1","kind":"BootcHost","metadata":{"name":"host"},"spec":{"bootOrder":"default","image":null},"status":{"booted":null,"rollback":null,"rollbackQueued":false,"staged":null,"type":null,"usrOverlay":null}}`

// bootedStagedJSON follows the org.containers.bootc/v1 schema for a booted
// host with a staged update (constructed; validate on a bootc VM post-merge).
const bootedStagedJSON = `{
  "apiVersion": "org.containers.bootc/v1",
  "kind": "BootcHost",
  "metadata": {"name": "host"},
  "spec": {
    "bootOrder": "default",
    "image": {"image": "ghcr.io/frostyard/snow:stable", "transport": "containers-storage"}
  },
  "status": {
    "booted": {
      "image": {
        "image": {"image": "ghcr.io/frostyard/snow:stable", "transport": "containers-storage"},
        "version": "20260701.0",
        "timestamp": "2026-07-01T10:00:00Z",
        "imageDigest": "sha256:aaaa1111bbbb2222cccc3333dddd4444eeee5555ffff6666aaaa7777bbbb8888"
      },
      "cachedUpdate": null,
      "incompatible": false,
      "pinned": false,
      "store": "ostreeContainer"
    },
    "staged": {
      "image": {
        "image": {"image": "ghcr.io/frostyard/snow:stable", "transport": "containers-storage"},
        "version": "20260706.0",
        "timestamp": "2026-07-06T09:00:00Z",
        "imageDigest": "sha256:9999aaaa0000bbbb1111cccc2222dddd3333eeee4444ffff5555aaaa6666bbbb"
      },
      "cachedUpdate": null,
      "incompatible": false,
      "pinned": false,
      "store": "ostreeContainer"
    },
    "rollback": null,
    "rollbackQueued": false,
    "type": "bootcHost"
  }
}`

func TestParseStatusNonBootcHost(t *testing.T) {
	s, err := parseStatus([]byte(nonBootcJSON))
	if err != nil {
		t.Fatalf("parseStatus: %v", err)
	}
	if s.Status.Booted != nil {
		t.Errorf("Booted = %+v, want nil", s.Status.Booted)
	}
	if s.Status.Staged != nil {
		t.Errorf("Staged = %+v, want nil", s.Status.Staged)
	}
	if s.Booted() {
		t.Error("Booted() = true, want false")
	}
}

func TestParseStatusBootedWithStaged(t *testing.T) {
	s, err := parseStatus([]byte(bootedStagedJSON))
	if err != nil {
		t.Fatalf("parseStatus: %v", err)
	}
	if !s.Booted() {
		t.Fatal("Booted() = false, want true")
	}
	if got, want := s.Status.Booted.ImageRef(), "ghcr.io/frostyard/snow:stable"; got != want {
		t.Errorf("booted ImageRef = %q, want %q", got, want)
	}
	if got, want := s.Status.Booted.Version(), "20260701.0"; got != want {
		t.Errorf("booted Version = %q, want %q", got, want)
	}
	if s.Status.Staged == nil {
		t.Fatal("Staged = nil, want non-nil")
	}
	if got, want := s.Status.Staged.Version(), "20260706.0"; got != want {
		t.Errorf("staged Version = %q, want %q", got, want)
	}
	if got := s.Status.Staged.Digest(); got == "" {
		t.Error("staged Digest = empty, want sha256:...")
	}
}

func TestDeploymentNilSafe(t *testing.T) {
	var d *Deployment
	if d.ImageRef() != "" || d.Version() != "" || d.Timestamp() != "" || d.Digest() != "" {
		t.Error("nil Deployment accessors must return empty strings")
	}
}

func TestParseStatusMalformed(t *testing.T) {
	if _, err := parseStatus([]byte("not json")); err == nil {
		t.Error("parseStatus(garbage) = nil error, want error")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/bootc/ -v`
Expected: FAIL — `undefined: parseStatus`, `undefined: Deployment` (compile error).

- [ ] **Step 3: Write the implementation**

Create `internal/bootc/bootc.go`:

```go
// Package bootc provides an interface to bootc-based system updates.
// Status reads call `bootc status --format json` directly (unprivileged).
// Update staging is delegated to the snow-shipped workaround script
// /usr/libexec/bootc-update-stage via pkexec, because bootc's own
// registry-transport pull currently fails on snow images.
package bootc

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"sync"
	"time"
)

const (
	bootcCommand   = "bootc"
	pkexecCommand  = "pkexec"
	DefaultTimeout = 30 * time.Minute
)

var dryRun = false

// SetDryRun enables/disables dry-run mode
func SetDryRun(mode bool) {
	dryRun = mode
	log.Printf("bootc dry-run mode: %v", mode)
}

// IsDryRun returns whether dry-run mode is enabled
func IsDryRun() bool {
	return dryRun
}

// DefaultContext returns a context with the default 30-minute timeout
func DefaultContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), DefaultTimeout)
}

// Error represents a bootc-related error
type Error struct {
	Message string
}

func (e *Error) Error() string {
	return e.Message
}

// NotFoundError is returned when bootc is not installed
type NotFoundError struct {
	Message string
}

func (e *NotFoundError) Error() string {
	return e.Message
}

// ImageReference identifies a container image (org.containers.bootc/v1).
type ImageReference struct {
	Image     string `json:"image"`
	Transport string `json:"transport"`
}

// ImageStatus describes a deployed image.
type ImageStatus struct {
	Image       ImageReference `json:"image"`
	Version     string         `json:"version"`
	Timestamp   string         `json:"timestamp"`
	ImageDigest string         `json:"imageDigest"`
}

// Deployment is one entry in status (booted, staged, or rollback).
type Deployment struct {
	Image  *ImageStatus `json:"image"`
	Pinned bool         `json:"pinned"`
}

// ImageRef returns the deployment's image reference, or "" if unknown.
func (d *Deployment) ImageRef() string {
	if d == nil || d.Image == nil {
		return ""
	}
	return d.Image.Image.Image
}

// Version returns the deployment's image version, or "".
func (d *Deployment) Version() string {
	if d == nil || d.Image == nil {
		return ""
	}
	return d.Image.Version
}

// Timestamp returns the deployment's image timestamp, or "".
func (d *Deployment) Timestamp() string {
	if d == nil || d.Image == nil {
		return ""
	}
	return d.Image.Timestamp
}

// Digest returns the deployment's image digest, or "".
func (d *Deployment) Digest() string {
	if d == nil || d.Image == nil {
		return ""
	}
	return d.Image.ImageDigest
}

// SpecInfo is the host spec section of bootc status.
type SpecInfo struct {
	Image *ImageReference `json:"image"`
}

// StatusInfo is the status section of bootc status.
type StatusInfo struct {
	Booted   *Deployment `json:"booted"`
	Staged   *Deployment `json:"staged"`
	Rollback *Deployment `json:"rollback"`
}

// Status is the parsed output of `bootc status --format json`.
type Status struct {
	Spec   SpecInfo   `json:"spec"`
	Status StatusInfo `json:"status"`
}

// Booted reports whether the host is booted from a bootc deployment.
func (s *Status) Booted() bool {
	return s != nil && s.Status.Booted != nil
}

// parseStatus parses `bootc status --format json` output.
func parseStatus(data []byte) (*Status, error) {
	var s Status
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, &Error{Message: fmt.Sprintf("failed to parse bootc status JSON: %v", err)}
	}
	return &s, nil
}

// GetStatus returns the current bootc host status. Runs unprivileged.
func GetStatus(ctx context.Context) (*Status, error) {
	cmd := exec.CommandContext(ctx, bootcCommand, "status", "--format", "json")
	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, &Error{Message: "bootc status timed out"}
		}
		if execErr, ok := err.(*exec.Error); ok && execErr.Err == exec.ErrNotFound {
			return nil, &NotFoundError{Message: "bootc not found"}
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, &Error{Message: fmt.Sprintf("bootc status failed (exit %d): %s", exitErr.ExitCode(), string(exitErr.Stderr))}
		}
		return nil, &Error{Message: err.Error()}
	}
	return parseStatus(output)
}

// IsBootcBooted reports whether this host is booted from a bootc deployment.
// Note: `bootc status` exits 0 with a null booted entry on non-bootc hosts,
// so the gate is the booted field, not the exit code.
func IsBootcBooted(ctx context.Context) bool {
	status, err := GetStatus(ctx)
	if err != nil {
		return false
	}
	return status.Booted()
}

var (
	bootedOnce   sync.Once
	bootedResult bool
)

// IsBootcBootedCached returns a cached result of IsBootcBooted, running the
// check at most once. Safe to call from view goroutines during async startup.
func IsBootcBootedCached() bool {
	bootedOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		bootedResult = IsBootcBooted(ctx)
	})
	return bootedResult
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/bootc/ -v`
Expected: PASS (4 tests).

- [ ] **Step 5: Verify full build**

Run: `make build && go vet ./internal/bootc/`
Expected: both succeed.

- [ ] **Step 6: Commit**

```bash
git add internal/bootc/
git commit -m "feat: add internal/bootc package with status parsing and boot gate

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: `internal/bootc` — StageUpdate via the snosi script, streaming output

**Files:**
- Create: `internal/bootc/stage.go`
- Test: `internal/bootc/stage_test.go`

**Interfaces:**
- Consumes: `Error`, `NotFoundError`, `dryRun` from Task 1.
- Produces (used by Task 4):
  - `type EventType string`; `const EventMessage EventType = "message"`, `EventError EventType = "error"`, `EventComplete EventType = "complete"`
  - `type ProgressEvent struct { Type EventType; Message string }`
  - `func StageScriptAvailable() bool`
  - `func StageUpdate(ctx context.Context, progressCh chan<- ProgressEvent) error` — closes progressCh when done; sends each output line as EventMessage, final EventComplete on success.

- [ ] **Step 1: Write the failing test**

Create `internal/bootc/stage_test.go`:

```go
package bootc

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeScript writes an executable shell script and returns its path.
func writeScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-stage")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func collectEvents(ch <-chan ProgressEvent) []ProgressEvent {
	var events []ProgressEvent
	for e := range ch {
		events = append(events, e)
	}
	return events
}

func TestRunStageStreamingSuccess(t *testing.T) {
	script := writeScript(t, `echo "Staging update: img"
echo "Update staged; it will apply at the next reboot."`)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ch := make(chan ProgressEvent)
	done := make(chan error, 1)
	go func() { done <- runStageStreaming(ctx, ch, script) }()

	events := collectEvents(ch)
	if err := <-done; err != nil {
		t.Fatalf("runStageStreaming: %v", err)
	}

	if len(events) != 3 { // 2 messages + 1 complete
		t.Fatalf("got %d events %+v, want 3", len(events), events)
	}
	if events[0].Type != EventMessage || events[0].Message != "Staging update: img" {
		t.Errorf("event[0] = %+v", events[0])
	}
	if events[2].Type != EventComplete {
		t.Errorf("event[2] = %+v, want EventComplete", events[2])
	}
}

func TestRunStageStreamingFailure(t *testing.T) {
	script := writeScript(t, `echo "about to fail"
echo "boom" >&2
exit 3`)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ch := make(chan ProgressEvent)
	done := make(chan error, 1)
	go func() { done <- runStageStreaming(ctx, ch, script) }()

	events := collectEvents(ch)
	err := <-done
	if err == nil {
		t.Fatal("runStageStreaming = nil error, want failure")
	}
	// stdout and stderr lines both stream as messages
	var sawStdout, sawStderr bool
	for _, e := range events {
		if e.Message == "about to fail" {
			sawStdout = true
		}
		if e.Message == "boom" {
			sawStderr = true
		}
		if e.Type == EventComplete {
			t.Error("got EventComplete on failure")
		}
	}
	if !sawStdout || !sawStderr {
		t.Errorf("missing streamed lines; events: %+v", events)
	}
}

func TestStageUpdateDryRun(t *testing.T) {
	SetDryRun(true)
	defer SetDryRun(false)

	ctx := context.Background()
	ch := make(chan ProgressEvent)
	done := make(chan error, 1)
	go func() { done <- StageUpdate(ctx, ch) }()

	events := collectEvents(ch)
	if err := <-done; err != nil {
		t.Fatalf("dry-run StageUpdate: %v", err)
	}
	if len(events) == 0 || events[len(events)-1].Type != EventComplete {
		t.Errorf("dry-run should emit mock events ending in EventComplete; got %+v", events)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/bootc/ -v`
Expected: FAIL — `undefined: runStageStreaming`, `undefined: ProgressEvent`.

- [ ] **Step 3: Write the implementation**

Create `internal/bootc/stage.go`:

```go
package bootc

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
)

// StageScriptPath is the snow-shipped workaround script that pulls the OS
// image via podman and stages it with `bootc switch --transport
// containers-storage`. See the design spec for why plain `bootc upgrade`
// is not used.
const StageScriptPath = "/usr/libexec/bootc-update-stage"

// EventType classifies a ProgressEvent.
type EventType string

const (
	EventMessage  EventType = "message"
	EventError    EventType = "error"
	EventComplete EventType = "complete"
)

// ProgressEvent is a single line of progress from the stage script.
type ProgressEvent struct {
	Type    EventType
	Message string
}

// StageScriptAvailable reports whether the stage script is installed.
func StageScriptAvailable() bool {
	_, err := os.Stat(StageScriptPath)
	return err == nil
}

// StageUpdate checks for and stages a system update by running the stage
// script via pkexec. Output lines stream to progressCh as EventMessage
// events; EventComplete is sent on success. progressCh is closed when done.
// The script is idempotent: it exits 0 without staging when already current.
func StageUpdate(ctx context.Context, progressCh chan<- ProgressEvent) error {
	if dryRun {
		log.Printf("[DRY-RUN] would execute: pkexec %s", StageScriptPath)
		progressCh <- ProgressEvent{Type: EventMessage, Message: "[DRY-RUN] would run " + StageScriptPath}
		progressCh <- ProgressEvent{Type: EventComplete, Message: "Dry run complete"}
		close(progressCh)
		return nil
	}
	return runStageStreaming(ctx, progressCh, pkexecCommand, StageScriptPath)
}

// runStageStreaming runs a command, streaming stdout+stderr lines to
// progressCh. It closes progressCh before returning. Separated from
// StageUpdate so tests can run a local fake script without pkexec.
func runStageStreaming(ctx context.Context, progressCh chan<- ProgressEvent, name string, args ...string) error {
	defer close(progressCh)

	cmd := exec.CommandContext(ctx, name, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return &Error{Message: fmt.Sprintf("failed to create stdout pipe: %v", err)}
	}
	// Merge stderr into the same stream so podman/bootc messages surface.
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		if execErr, ok := err.(*exec.Error); ok && execErr.Err == exec.ErrNotFound {
			return &NotFoundError{Message: name + " not found"}
		}
		return &Error{Message: fmt.Sprintf("failed to start %s: %v", name, err)}
	}

	var lastLine string
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lastLine = line
		select {
		case progressCh <- ProgressEvent{Type: EventMessage, Message: line}:
		case <-ctx.Done():
			_ = cmd.Process.Kill()
			return ctx.Err()
		}
	}

	if err := cmd.Wait(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return &Error{Message: "Update staging timed out"}
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			msg := fmt.Sprintf("update staging failed (exit %d)", exitErr.ExitCode())
			if lastLine != "" {
				msg += ": " + lastLine
			}
			return &Error{Message: msg}
		}
		return &Error{Message: err.Error()}
	}

	progressCh <- ProgressEvent{Type: EventComplete, Message: "Staging complete"}
	return nil
}
```

Note: `cmd.Stderr = cmd.Stdout` after `StdoutPipe()` shares the pipe's write end — both streams arrive interleaved on the scanner. The `EventComplete` send after the scanner loop is safe without a ctx select because the loop already drained; keep it simple.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/bootc/ -v`
Expected: PASS (7 tests total in package).

- [ ] **Step 5: Commit**

```bash
git add internal/bootc/stage.go internal/bootc/stage_test.go
git commit -m "feat: add bootc update staging via snosi stage script with streaming output

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 3: System page — replace NBC status group with bootc status group

**Files:**
- Modify: `internal/views/system_page.go` (replace lines 42–59 gate+group, replace `loadNBCStatus` lines 139–250, delete `onSystemUpdateClicked` lines 252–308)

**Interfaces:**
- Consumes: `bootc.GetStatus`, `bootc.IsBootcBootedCached`, `bootc.DefaultContext`, `Deployment` accessors (Task 1).
- Produces: nothing new for later tasks. After this task `system_page.go` no longer imports `internal/nbc` (drop the `sync` import too — no longer needed).

The old flow gated on a cheap `os.Stat("/run/nbc-booted")` at build time. The bootc gate requires an exec, so the group is built **hidden** and shown asynchronously — same deferred-visibility pattern as `featuresGroup`.

- [ ] **Step 1: Replace the NBC group construction in `buildSystemPage`**

In `internal/views/system_page.go`, replace the block starting `// NBC Status group - only show if NBC is booted` (the `os.Stat("/run/nbc-booted")` gate and its body) with:

```go
	// bootc Status group - built hidden, shown asynchronously if this host
	// is booted from a bootc deployment (bootc status requires an exec, so
	// the gate must not run synchronously during page construction).
	if uh.config.IsGroupEnabled("system_page", "bootc_status_group") {
		group := adw.NewPreferencesGroup()
		group.SetTitle("System Image")
		group.SetDescription("bootc deployment status")
		group.SetVisible(false)

		bootcExpander := adw.NewExpanderRow()
		bootcExpander.SetTitle("Deployment Details")
		bootcExpander.SetSubtitle("Loading...")

		group.Add(&bootcExpander.Widget)
		page.Add(group)

		// Gate + load asynchronously
		go uh.loadBootcStatus(group, bootcExpander)
	}
```

- [ ] **Step 2: Replace `loadNBCStatus` with `loadBootcStatus` and delete `onSystemUpdateClicked`**

Delete the `loadNBCStatus` and `onSystemUpdateClicked` functions entirely and add:

```go
// loadBootcStatus checks the bootc boot gate and populates the status
// expander. Runs in a goroutine; shows the group only on bootc hosts.
func (uh *UserHome) loadBootcStatus(group *adw.PreferencesGroup, expander *adw.ExpanderRow) {
	if !bootc.IsBootcBootedCached() {
		return // group stays hidden on non-bootc hosts
	}

	ctx, cancel := bootc.DefaultContext()
	defer cancel()

	status, err := bootc.GetStatus(ctx)

	sgtk.RunOnMainThread(func() {
		group.SetVisible(true)

		if err != nil {
			expander.SetSubtitle(fmt.Sprintf("Error: %v", err))
			return
		}

		expander.SetSubtitle("Loaded")

		addRow := func(title, subtitle string) {
			row := adw.NewActionRow()
			row.SetTitle(title)
			row.SetSubtitle(subtitle)
			expander.AddRow(&row.Widget)
		}

		booted := status.Status.Booted
		if booted.ImageRef() != "" {
			addRow("Image", booted.ImageRef())
		}
		if booted.Version() != "" {
			addRow("Version", booted.Version())
		}
		if booted.Timestamp() != "" {
			addRow("Built", booted.Timestamp())
		}
		if digest := booted.Digest(); digest != "" {
			if len(digest) > 19 {
				digest = digest[:19] + "..."
			}
			addRow("Digest", digest)
		}

		if staged := status.Status.Staged; staged != nil {
			subtitle := "Restart to apply"
			if staged.Version() != "" {
				subtitle = fmt.Sprintf("%s — restart to apply", staged.Version())
			}
			addRow("Staged Update", subtitle)
		}

		if rollback := status.Status.Rollback; rollback != nil {
			subtitle := rollback.Version()
			if subtitle == "" {
				subtitle = "Available"
			}
			addRow("Rollback", subtitle)
		}
	})
}
```

- [ ] **Step 3: Fix imports**

In `system_page.go` imports: remove `"github.com/frostyard/chairlift/internal/nbc"` and `"sync"`; add `"github.com/frostyard/chairlift/internal/bootc"`.

- [ ] **Step 4: Build**

Run: `make build && go vet ./internal/views/`
Expected: success. (`internal/nbc` still exists — `updates_page.go` still uses it until Task 4.)

- [ ] **Step 5: Commit**

```bash
git add internal/views/system_page.go
git commit -m "feat: replace NBC status group with bootc deployment status on system page

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 4: Updates page — replace NBC updates group with bootc stage group

**Files:**
- Modify: `internal/views/updates_page.go` (replace NBC group in `buildUpdatesPage` lines 28–79; delete `onNBCCheckUpdateClicked`, `checkNBCUpdateAvailability`, `nbcOperationFunc`, `nbcOperationParams`, `runNBCOperation`, `onNBCUpdateClicked`, `onNBCDownloadClicked` — lines 305–566)
- Modify: `internal/views/views.go` (replace NBC field block lines 59–63; rename `nbcUpdateCount` line 76 and its use in `updateBadgeCount` line 111)

**Interfaces:**
- Consumes: `bootc.IsBootcBootedCached`, `bootc.StageScriptAvailable`, `bootc.StageUpdate`, `bootc.GetStatus`, `bootc.DefaultContext`, `bootc.ProgressEvent`, `bootc.EventMessage/EventError/EventComplete` (Tasks 1–2).
- Produces: `UserHome.bootcStageExpander *adw.ExpanderRow`, `UserHome.bootcStageBtn *gtk.Button`, `UserHome.bootcUpdateCount int` (views.go fields).

- [ ] **Step 1: Update `views.go` fields**

Replace the NBC field block:

```go
	// bootc update references
	bootcStageExpander *adw.ExpanderRow
	bootcStageBtn      *gtk.Button
```

and rename `nbcUpdateCount int` → `bootcUpdateCount int`; in `updateBadgeCount()` change `uh.nbcUpdateCount + ...` → `uh.bootcUpdateCount + ...`.

- [ ] **Step 2: Replace the NBC group in `buildUpdatesPage`**

Replace the whole `if _, err := os.Stat("/run/nbc-booted"); err == nil { ... }` block with:

```go
	// bootc System Updates group - built hidden, shown asynchronously on
	// bootc hosts that ship the update-stage script.
	if uh.config.IsGroupEnabled("updates_page", "bootc_updates_group") {
		group := adw.NewPreferencesGroup()
		group.SetTitle("System Updates")
		group.SetDescription("Download and stage system image updates; staged updates apply on restart")
		group.SetVisible(false)

		uh.bootcStageExpander = adw.NewExpanderRow()
		uh.bootcStageExpander.SetTitle("System Update")
		uh.bootcStageExpander.SetSubtitle("Checking status...")

		uh.bootcStageBtn = gtk.NewButtonWithLabel("Check for Updates")
		uh.bootcStageBtn.SetValign(gtk.AlignCenterValue)
		uh.bootcStageBtn.AddCssClass("suggested-action")
		stageClickedCb := func(btn gtk.Button) {
			uh.onBootcStageClicked()
		}
		uh.bootcStageBtn.ConnectClicked(&stageClickedCb)
		uh.bootcStageExpander.AddSuffix(&uh.bootcStageBtn.Widget)

		group.Add(&uh.bootcStageExpander.Widget)
		page.Add(group)

		go uh.loadBootcUpdateStatus(group)
	}
```

Remove the now-unused `"os"` import if nothing else in the file uses it (nothing does).

- [ ] **Step 3: Delete the seven NBC functions and add the bootc equivalents**

Delete `onNBCCheckUpdateClicked`, `checkNBCUpdateAvailability`, `nbcOperationFunc`, `nbcOperationParams`, `runNBCOperation`, `onNBCUpdateClicked`, `onNBCDownloadClicked`. Add:

```go
// loadBootcUpdateStatus gates the bootc updates group and reflects the
// current staged/booted state in the expander subtitle and update badge.
func (uh *UserHome) loadBootcUpdateStatus(group *adw.PreferencesGroup) {
	if !bootc.IsBootcBootedCached() || !bootc.StageScriptAvailable() {
		return // group stays hidden
	}

	ctx, cancel := bootc.DefaultContext()
	defer cancel()

	status, err := bootc.GetStatus(ctx)

	staged := err == nil && status.Status.Staged != nil
	uh.updateCountMu.Lock()
	if staged {
		uh.bootcUpdateCount = 1
	} else {
		uh.bootcUpdateCount = 0
	}
	uh.updateCountMu.Unlock()
	uh.updateBadgeCount()

	sgtk.RunOnMainThread(func() {
		group.SetVisible(true)
		if err != nil {
			uh.bootcStageExpander.SetSubtitle(fmt.Sprintf("Error: %v", err))
			return
		}
		if staged {
			version := status.Status.Staged.Version()
			if version != "" {
				uh.bootcStageExpander.SetSubtitle(fmt.Sprintf("Update %s staged — restart to apply", version))
			} else {
				uh.bootcStageExpander.SetSubtitle("Update staged — restart to apply")
			}
		} else {
			uh.bootcStageExpander.SetSubtitle("Check for and download the latest system image")
		}
	})
}

// onBootcStageClicked runs the stage script with streamed log output.
// The script checks, downloads, and stages in one idempotent operation.
func (uh *UserHome) onBootcStageClicked() {
	button := uh.bootcStageBtn
	expander := uh.bootcStageExpander

	button.SetSensitive(false)
	button.SetLabel("Working...")
	expander.SetExpanded(true)
	expander.SetSubtitle("Checking for updates...")

	// Activity row with a spinner (the stage script emits no percentages,
	// so progress is indeterminate).
	activityRow := adw.NewActionRow()
	activityRow.SetTitle("Progress")
	activityRow.SetSubtitle("Running...")
	spinner := gtk.NewSpinner()
	spinner.Start()
	activityRow.AddSuffix(&spinner.Widget)
	expander.AddRow(&activityRow.Widget)

	logExpander := adw.NewExpanderRow()
	logExpander.SetTitle("Details")
	logExpander.SetSubtitle("View output")
	expander.AddRow(&logExpander.Widget)

	go func() {
		ctx, cancel := bootc.DefaultContext()
		defer cancel()

		progressCh := make(chan bootc.ProgressEvent)

		var stageErr error
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			stageErr = bootc.StageUpdate(ctx, progressCh)
		}()

		var lastMessage string
		for event := range progressCh {
			evt := event
			if evt.Type == bootc.EventMessage {
				lastMessage = evt.Message
			}
			sgtk.RunOnMainThread(func() {
				switch evt.Type {
				case bootc.EventMessage:
					msgRow := adw.NewActionRow()
					msgRow.SetTitle(evt.Message)
					msgRow.SetSubtitle(time.Now().Format("15:04:05"))
					logExpander.AddRow(&msgRow.Widget)
					activityRow.SetSubtitle(evt.Message)
				case bootc.EventError:
					errRow := adw.NewActionRow()
					errRow.SetTitle(evt.Message)
					errRow.SetSubtitle("Error")
					errIcon := gtk.NewImageFromIconName("dialog-error-symbolic")
					errRow.AddPrefix(&errIcon.Widget)
					logExpander.AddRow(&errRow.Widget)
					logExpander.SetExpanded(true)
				case bootc.EventComplete:
					activityRow.SetSubtitle("Complete")
				}
			})
		}

		wg.Wait()

		// Re-read status so the subtitle and badge reflect reality
		// (staged vs already-current) rather than guessing from output.
		statusCtx, statusCancel := bootc.DefaultContext()
		status, statusErr := bootc.GetStatus(statusCtx)
		statusCancel()

		staged := statusErr == nil && status.Status.Staged != nil
		uh.updateCountMu.Lock()
		if staged {
			uh.bootcUpdateCount = 1
		} else {
			uh.bootcUpdateCount = 0
		}
		uh.updateCountMu.Unlock()
		uh.updateBadgeCount()

		sgtk.RunOnMainThread(func() {
			spinner.Stop()
			button.SetSensitive(true)
			button.SetLabel("Check for Updates")

			if stageErr != nil {
				expander.SetSubtitle(fmt.Sprintf("Update failed: %v", stageErr))
				uh.toastAdder.ShowErrorToast(fmt.Sprintf("Update failed: %v", stageErr))
				return
			}

			if staged {
				version := status.Status.Staged.Version()
				if version != "" {
					expander.SetSubtitle(fmt.Sprintf("Update %s staged — restart to apply", version))
				} else {
					expander.SetSubtitle("Update staged — restart to apply")
				}
				uh.toastAdder.ShowToast("System update staged. Restart to apply.")
			} else {
				subtitle := "System is up to date"
				if lastMessage != "" {
					subtitle = lastMessage
				}
				expander.SetSubtitle(subtitle)
				uh.toastAdder.ShowToast("System is up to date")
			}
		})
	}()
}
```

- [ ] **Step 4: Fix imports**

In `updates_page.go`: remove `"github.com/frostyard/chairlift/internal/nbc"`, `"context"`, and `"os"`; add `"github.com/frostyard/chairlift/internal/bootc"`. Keep `"sync"` and `"time"` (still used).

- [ ] **Step 5: Build and test**

Run: `make build && go test ./... && go vet ./internal/views/`
Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add internal/views/updates_page.go internal/views/views.go
git commit -m "feat: replace NBC update flow with bootc stage-script flow on updates page

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 5: Delete `internal/nbc`, drop the nbc dependency, rewire dry-run

**Files:**
- Delete: `internal/nbc/nbc.go` (entire directory)
- Modify: `internal/app/app.go` (line ~83: `nbc.SetDryRun(true)` → `bootc.SetDryRun(true)`, and the import)
- Modify: `go.mod` / `go.sum` (via `go mod tidy`)

**Interfaces:**
- Consumes: `bootc.SetDryRun` (Task 1).
- Produces: a repo with zero `internal/nbc` references; `github.com/frostyard/nbc` gone from go.mod.

- [ ] **Step 1: Rewire app.go**

In `internal/app/app.go`: replace import `"github.com/frostyard/chairlift/internal/nbc"` with `"github.com/frostyard/chairlift/internal/bootc"` and change `nbc.SetDryRun(true)` to `bootc.SetDryRun(true)`.

- [ ] **Step 2: Delete the package and tidy**

```bash
git rm -r internal/nbc
go mod tidy
```

- [ ] **Step 3: Verify no stragglers**

Run: `grep -rn "internal/nbc\|frostyard/nbc" --include="*.go" . ; go build ./... && go test ./...`
Expected: grep finds nothing; build and tests pass. Also confirm `grep -n "frostyard/nbc" go.mod` finds nothing.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "feat!: remove nbc integration (superseded by bootc)

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 6: PolicyKit, Makefile, goreleaser, and config renames

**Files:**
- Create: `data/org.frostyard.ChairLift.bootc.policy`, `data/org.frostyard.ChairLift.bootc.rules`
- Delete: `data/org.frostyard.ChairLift.nbc.policy`, `data/org.frostyard.ChairLift.nbc.rules`
- Modify: `Makefile` (lines 95–97, 111–112), `.goreleaser.yaml` (lines 109, 128–132), `config.yml`, `config.nbc-example.yml` → rename to `config.bootc-example.yml` semantics (see step 4), `CONFIG.md`

**Interfaces:**
- Consumes: `bootc.StageScriptPath` convention (`/usr/libexec/bootc-update-stage`, Task 2).
- Produces: polkit action id `org.frostyard.ChairLift.bootc.stage` (referenced in docs).

- [ ] **Step 1: Create the bootc polkit policy**

`data/org.frostyard.ChairLift.bootc.policy`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE policyconfig PUBLIC
 "-//freedesktop//DTD PolicyKit Policy Configuration 1.0//EN"
 "http://www.freedesktop.org/standards/PolicyKit/1/policyconfig.dtd">

<policyconfig>
  <vendor>Frostyard</vendor>
  <vendor_url>https://github.com/frostyard/chairlift</vendor_url>
  <icon_name>org.frostyard.ChairLift</icon_name>

  <action id="org.frostyard.ChairLift.bootc.stage">
    <description>Download and stage a system image update</description>
    <message>Authentication is required to stage a system update</message>
    <defaults>
      <allow_any>auth_admin</allow_any>
      <allow_inactive>auth_admin</allow_inactive>
      <allow_active>auth_admin_keep</allow_active>
    </defaults>
    <annotate key="org.freedesktop.policykit.exec.path">/usr/libexec/bootc-update-stage</annotate>
  </action>

</policyconfig>
```

`data/org.frostyard.ChairLift.bootc.rules`:

```js
// Allow users in the sudo group to stage bootc updates via ChairLift without authentication
polkit.addRule(function(action, subject) {
    if (action.id.startsWith("org.frostyard.ChairLift.bootc.") &&
        subject.active == true &&
        subject.local == true &&
        subject.isInGroup("sudo")) {
            return polkit.Result.YES;
    }
    return polkit.Result.NOT_HANDLED;
});
```

Then: `git rm data/org.frostyard.ChairLift.nbc.policy data/org.frostyard.ChairLift.nbc.rules`

- [ ] **Step 2: Update Makefile**

Replace the nbc install/uninstall lines with bootc equivalents (same install flags, new filenames):

```makefile
	# Install PolicyKit policy and rules for bootc
	install -Dm644 data/org.frostyard.ChairLift.bootc.policy $(DESTDIR)$(POLKITACTIONSDIR)/org.frostyard.ChairLift.bootc.policy
	install -Dm644 data/org.frostyard.ChairLift.bootc.rules $(DESTDIR)$(POLKITRULESDIR)/org.frostyard.ChairLift.bootc.rules
```

and in uninstall:

```makefile
	rm -f $(DESTDIR)$(POLKITACTIONSDIR)/org.frostyard.ChairLift.bootc.policy
	rm -f $(DESTDIR)$(POLKITRULESDIR)/org.frostyard.ChairLift.bootc.rules
```

- [ ] **Step 3: Update .goreleaser.yaml**

- Line ~109 description: `System management tool for nbc/bootc installations.` → `System management tool for bootc-based installations.`
- Replace the nbc policy/rules `contents` entries with the bootc filenames (same dst directories).

- [ ] **Step 4: Update configs**

- `config.yml`: rename key `nbc_status_group` → `bootc_status_group`; under `updates_page` add `bootc_updates_group: {enabled: true}`; **delete** the `updates_status_group` action `Check for System Updates: /usr/bin/bootc upgrade` (that upgrade path is broken on snow images and no builder consumes the group's actions).
- `config.nbc-example.yml`: delete it (`git rm config.nbc-example.yml`) — nbc is gone.
- `config.bootc-example.yml`: apply the same renames as config.yml so it stays a full example.
- `CONFIG.md`: update the group table/keys accordingly (`nbc_status_group` → `bootc_status_group`, add `bootc_updates_group`, drop nbc mentions).

- [ ] **Step 5: Verify and commit**

Run: `make build && grep -rn "nbc" Makefile .goreleaser.yaml config.yml config.bootc-example.yml data/ | grep -v bootc`
Expected: build OK; grep output empty.

```bash
git add -A
git commit -m "chore: swap nbc polkit/packaging/config for bootc stage script

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 7: `internal/homebrew` — untrusted tap detection

**Files:**
- Create: `internal/homebrew/trust.go`
- Test: `internal/homebrew/trust_test.go`

**Interfaces:**
- Consumes: `runBrewCommand` (existing, homebrew.go:81), `Error` type.
- Produces (used by Tasks 8–9):
  - `type UntrustedTap struct { Name string; Formulae []string; Casks []string }` (Formulae/Casks are fully qualified `user/tap/name`)
  - `func ListUntrustedTaps() ([]UntrustedTap, error)`
  - internal pure helpers: `parseUntrustedTapNames([]byte) ([]string, error)`, `installedFormulaeByTap(cellarDir string) map[string][]string`, `installedCasksByTap(caskroomDir string) map[string][]string`

- [ ] **Step 1: Write the failing test**

Create `internal/homebrew/trust_test.go`:

```go
package homebrew

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

const tapInfoJSON = `[
  {"name": "homebrew/core", "installed": true, "trusted": true},
  {"name": "multica-ai/tap", "installed": true, "trusted": false},
  {"name": "ublue-os/tap", "installed": true, "trusted": false},
  {"name": "charmbracelet/tap", "installed": true, "trusted": true}
]`

func TestParseUntrustedTapNames(t *testing.T) {
	names, err := parseUntrustedTapNames([]byte(tapInfoJSON))
	if err != nil {
		t.Fatalf("parseUntrustedTapNames: %v", err)
	}
	sort.Strings(names)
	want := []string{"multica-ai/tap", "ublue-os/tap"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("got %v, want %v", names, want)
	}
}

func TestParseUntrustedTapNamesMalformed(t *testing.T) {
	if _, err := parseUntrustedTapNames([]byte("nope")); err == nil {
		t.Error("want error on malformed JSON")
	}
}

// writeFile creates a file with parent dirs.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestInstalledFormulaeByTap(t *testing.T) {
	cellar := t.TempDir()
	writeFile(t, filepath.Join(cellar, "multica", "0.3.19", "INSTALL_RECEIPT.json"),
		`{"source": {"tap": "multica-ai/tap"}}`)
	writeFile(t, filepath.Join(cellar, "jq", "1.7", "INSTALL_RECEIPT.json"),
		`{"source": {"tap": "homebrew/core"}}`)
	// keg with no receipt is skipped silently
	if err := os.MkdirAll(filepath.Join(cellar, "broken", "1.0"), 0o755); err != nil {
		t.Fatal(err)
	}

	byTap := installedFormulaeByTap(cellar)
	if got := byTap["multica-ai/tap"]; !reflect.DeepEqual(got, []string{"multica-ai/tap/multica"}) {
		t.Errorf("multica-ai/tap = %v", got)
	}
	if got := byTap["homebrew/core"]; !reflect.DeepEqual(got, []string{"homebrew/core/jq"}) {
		t.Errorf("homebrew/core = %v", got)
	}
}

func TestInstalledCasksByTap(t *testing.T) {
	caskroom := t.TempDir()
	writeFile(t, filepath.Join(caskroom, "somecask", ".metadata", "1.0", "20260101", "Casks", "somecask.json"),
		`{"token": "somecask", "tap": "ublue-os/tap"}`)
	// API-installed cask without Casks/*.json metadata is skipped (trusted)
	writeFile(t, filepath.Join(caskroom, "codex", ".metadata", "INSTALL_RECEIPT.json"),
		`{"loaded_from_api": true}`)

	byTap := installedCasksByTap(caskroom)
	if got := byTap["ublue-os/tap"]; !reflect.DeepEqual(got, []string{"ublue-os/tap/somecask"}) {
		t.Errorf("ublue-os/tap = %v", got)
	}
	if _, ok := byTap["homebrew/cask"]; ok {
		t.Error("API cask should not be attributed to any tap")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/homebrew/ -v`
Expected: FAIL — undefined functions (compile error).

- [ ] **Step 3: Write the implementation**

Create `internal/homebrew/trust.go`:

```go
package homebrew

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// UntrustedTap describes an untrusted tap and the packages installed from it.
// Package names are fully qualified (user/tap/name), ready for `brew trust`.
type UntrustedTap struct {
	Name     string
	Formulae []string
	Casks    []string
}

// parseUntrustedTapNames extracts names of untrusted taps from
// `brew tap-info --installed --json` output (Homebrew 6 adds "trusted").
func parseUntrustedTapNames(data []byte) ([]string, error) {
	var taps []struct {
		Name    string `json:"name"`
		Trusted bool   `json:"trusted"`
	}
	if err := json.Unmarshal(data, &taps); err != nil {
		return nil, &Error{Message: fmt.Sprintf("failed to parse tap-info JSON: %v", err)}
	}
	var names []string
	for _, t := range taps {
		if !t.Trusted {
			names = append(names, t.Name)
		}
	}
	return names, nil
}

// installedFormulaeByTap maps tap name -> qualified installed formula names
// by reading Cellar keg INSTALL_RECEIPT.json files. This is the only
// reliable source: brew itself refuses to load (and therefore list)
// formulae from untrusted taps.
func installedFormulaeByTap(cellarDir string) map[string][]string {
	byTap := make(map[string][]string)
	kegs, err := os.ReadDir(cellarDir)
	if err != nil {
		return byTap
	}
	for _, keg := range kegs {
		if !keg.IsDir() {
			continue
		}
		versions, err := os.ReadDir(filepath.Join(cellarDir, keg.Name()))
		if err != nil {
			continue
		}
		for _, v := range versions {
			receiptPath := filepath.Join(cellarDir, keg.Name(), v.Name(), "INSTALL_RECEIPT.json")
			data, err := os.ReadFile(receiptPath)
			if err != nil {
				continue
			}
			var receipt struct {
				Source struct {
					Tap string `json:"tap"`
				} `json:"source"`
			}
			if err := json.Unmarshal(data, &receipt); err != nil || receipt.Source.Tap == "" {
				continue
			}
			byTap[receipt.Source.Tap] = append(byTap[receipt.Source.Tap], receipt.Source.Tap+"/"+keg.Name())
			break // one receipt per keg is enough
		}
	}
	return byTap
}

// installedCasksByTap maps tap name -> qualified installed cask tokens by
// reading Caskroom metadata. Casks installed from the Homebrew API have no
// Casks/<token>.json metadata and are skipped (they are homebrew/cask,
// which is always trusted).
func installedCasksByTap(caskroomDir string) map[string][]string {
	byTap := make(map[string][]string)
	entries, err := os.ReadDir(caskroomDir)
	if err != nil {
		return byTap
	}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		token := entry.Name()
		matches, err := filepath.Glob(filepath.Join(caskroomDir, token, ".metadata", "*", "*", "Casks", "*.json"))
		if err != nil || len(matches) == 0 {
			continue
		}
		data, err := os.ReadFile(matches[len(matches)-1]) // newest metadata last in sorted glob
		if err != nil {
			continue
		}
		var meta struct {
			Tap string `json:"tap"`
		}
		if err := json.Unmarshal(data, &meta); err != nil || meta.Tap == "" {
			continue
		}
		byTap[meta.Tap] = append(byTap[meta.Tap], meta.Tap+"/"+token)
	}
	return byTap
}

// brewPrefix returns Homebrew's installation prefix.
func brewPrefix() (string, error) {
	output, err := runBrewCommand("--prefix")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

// ListUntrustedTaps returns untrusted taps that have at least one package
// installed, with qualified package names ready for `brew trust`.
func ListUntrustedTaps() ([]UntrustedTap, error) {
	output, err := runBrewCommand("tap-info", "--installed", "--json")
	if err != nil {
		return nil, err
	}
	names, err := parseUntrustedTapNames([]byte(output))
	if err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return nil, nil
	}

	prefix, err := brewPrefix()
	if err != nil {
		return nil, err
	}
	formulaeByTap := installedFormulaeByTap(filepath.Join(prefix, "Cellar"))
	casksByTap := installedCasksByTap(filepath.Join(prefix, "Caskroom"))

	var result []UntrustedTap
	for _, name := range names {
		tap := UntrustedTap{
			Name:     name,
			Formulae: formulaeByTap[name],
			Casks:    casksByTap[name],
		}
		if len(tap.Formulae) == 0 && len(tap.Casks) == 0 {
			continue // nothing installed from this tap; not actionable
		}
		sort.Strings(tap.Formulae)
		sort.Strings(tap.Casks)
		result = append(result, tap)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/homebrew/ -v`
Expected: PASS (4 tests).

- [ ] **Step 5: Sanity-check against the real system**

Run: `brew tap-info --installed --json | python3 -c "import json,sys; taps=json.load(sys.stdin); print([t['name'] for t in taps if not t.get('trusted')])"`
Expected: a non-empty list of untrusted taps (at planning time: anomalyco/tap, multica-ai/tap, stacklok/tap, tta-lab/ttal, ublue-os/tap). This confirms the live JSON still carries the `trusted` field the parser depends on.

- [ ] **Step 6: Commit**

```bash
git add internal/homebrew/trust.go internal/homebrew/trust_test.go
git commit -m "feat: detect untrusted Homebrew taps with installed packages

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 8: `internal/homebrew` — TrustPackages and untrusted-tap error classification

**Files:**
- Modify: `internal/homebrew/trust.go` (add TrustPackages, UntrustedTapError)
- Modify: `internal/homebrew/homebrew.go` (add `"trust"` to `stateChangingCommands` map line 68; classify untrusted errors in `runBrewCommand` line 101)
- Test: `internal/homebrew/trust_test.go` (extend)

**Interfaces:**
- Consumes: `UntrustedTap` (Task 7), `runBrewCommand`, `stateChangingCommands`.
- Produces (used by Task 9):
  - `func TrustPackages(tap UntrustedTap) error`
  - `type UntrustedTapError struct{ Message string }` — returned by `runBrewCommand` when a failed command's stderr mentions untrusted taps; views detect via `errors.As`.
  - `func isUntrustedTapMessage(s string) bool` (internal)

- [ ] **Step 1: Write the failing tests (append to trust_test.go)**

```go
func TestIsUntrustedTapMessage(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"Error: Refusing to load formula opencode from untrusted tap anomalyco/tap.", true},
		{"Warning: The following taps are not trusted:\n  multica-ai/tap", true},
		{"Error: No such formula", false},
		{"", false},
	}
	for _, c := range cases {
		if got := isUntrustedTapMessage(c.in); got != c.want {
			t.Errorf("isUntrustedTapMessage(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestTrustPackagesDryRun(t *testing.T) {
	SetDryRun(true)
	defer SetDryRun(false)
	err := TrustPackages(UntrustedTap{
		Name:     "multica-ai/tap",
		Formulae: []string{"multica-ai/tap/multica"},
	})
	if err != nil {
		t.Fatalf("dry-run TrustPackages: %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/homebrew/ -v`
Expected: FAIL — undefined `isUntrustedTapMessage`, `TrustPackages`.

- [ ] **Step 3: Implement**

Append to `internal/homebrew/trust.go`:

```go
// UntrustedTapError indicates a brew command failed because packages come
// from untrusted taps (Homebrew 6 tap trust). Views should point users at
// the Untrusted Taps UI instead of dumping raw brew output.
type UntrustedTapError struct {
	Message string
}

func (e *UntrustedTapError) Error() string {
	return e.Message
}

// isUntrustedTapMessage reports whether brew output complains about
// untrusted taps.
func isUntrustedTapMessage(s string) bool {
	if s == "" {
		return false
	}
	return strings.Contains(s, "untrusted tap") || strings.Contains(s, "taps are not trusted")
}

// TrustPackages trusts every installed package from the given tap using
// `brew trust`. Trust is per-user (~/.homebrew/trust.json); no root needed.
func TrustPackages(tap UntrustedTap) error {
	if len(tap.Formulae) > 0 {
		args := append([]string{"trust", "--formula"}, tap.Formulae...)
		if _, err := runBrewCommand(args...); err != nil {
			return err
		}
	}
	if len(tap.Casks) > 0 {
		args := append([]string{"trust", "--cask"}, tap.Casks...)
		if _, err := runBrewCommand(args...); err != nil {
			return err
		}
	}
	return nil
}
```

In `internal/homebrew/homebrew.go`:

1. Add `"trust": true,` to the `stateChangingCommands` map (so dry-run skips it — this is what makes `TestTrustPackagesDryRun` pass).
2. In `runBrewCommand`, change the ExitError branch to classify untrusted-tap failures:

```go
		if _, ok := err.(*exec.ExitError); ok {
			if isUntrustedTapMessage(stderr.String()) {
				return "", &UntrustedTapError{Message: fmt.Sprintf("Brew command failed: %s", stderr.String())}
			}
			return "", &Error{Message: fmt.Sprintf("Brew command failed: %s", stderr.String())}
		}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/homebrew/ -v`
Expected: PASS (6 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/homebrew/
git commit -m "feat: add brew trust wrapper and untrusted-tap error classification

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 9: Updates page — Untrusted Taps UI group

**Files:**
- Modify: `internal/views/updates_page.go` (new group in `buildUpdatesPage` after the Homebrew Updates group; new functions; friendlier upgrade error; row-clearing fix in `loadOutdatedPackages`)
- Modify: `internal/views/views.go` (add fields `brewTrustGroup *adw.PreferencesGroup`, `brewTrustRows map[string]*adw.ActionRow`, `outdatedRows []*adw.ActionRow`)

**Interfaces:**
- Consumes: `homebrew.ListUntrustedTaps`, `homebrew.TrustPackages`, `homebrew.UntrustedTap`, `homebrew.UntrustedTapError` (Tasks 7–8); `adw.NewAlertDialog`/`AddResponse`/`SetResponseAppearance`/`ConnectResponse`/`Present` (verified in pinned puregotk).
- Produces: nothing consumed later.

- [ ] **Step 1: Add fields to views.go**

In the "References for dynamic updates" block add:

```go
	brewTrustGroup *adw.PreferencesGroup
	brewTrustRows  map[string]*adw.ActionRow
	outdatedRows   []*adw.ActionRow // Store references for cleanup
```

- [ ] **Step 2: Build the group in `buildUpdatesPage`**

After the Homebrew Updates group block, add:

```go
	// Untrusted Homebrew Taps group - hidden unless untrusted taps with
	// installed packages exist (Homebrew 6 tap trust).
	if uh.config.IsGroupEnabled("updates_page", "brew_trust_group") {
		uh.brewTrustGroup = adw.NewPreferencesGroup()
		uh.brewTrustGroup.SetTitle("Untrusted Homebrew Taps")
		uh.brewTrustGroup.SetDescription("Homebrew ignores packages from untrusted taps during upgrades. Trust a tap to resume updates for its packages.")
		uh.brewTrustGroup.SetVisible(false)
		page.Add(uh.brewTrustGroup)

		go uh.loadUntrustedTaps()
	}
```

- [ ] **Step 3: Add the loader and trust flow**

Add to `updates_page.go`:

```go
// loadUntrustedTaps populates the Untrusted Taps group. Runs in a
// goroutine; the group stays hidden when there is nothing actionable.
func (uh *UserHome) loadUntrustedTaps() {
	if !homebrew.IsInstalledCached() {
		return
	}

	taps, err := homebrew.ListUntrustedTaps()
	if err != nil {
		log.Printf("untrusted tap check failed: %v", err)
		return
	}
	if len(taps) == 0 {
		return
	}

	sgtk.RunOnMainThread(func() {
		uh.brewTrustRows = make(map[string]*adw.ActionRow)
		for _, tap := range taps {
			t := tap // capture
			row := adw.NewActionRow()
			row.SetTitle(t.Name)

			packages := append(append([]string{}, t.Formulae...), t.Casks...)
			// Show unqualified names in the subtitle for readability.
			var short []string
			for _, p := range packages {
				if i := strings.LastIndex(p, "/"); i >= 0 {
					short = append(short, p[i+1:])
				} else {
					short = append(short, p)
				}
			}
			row.SetSubtitle(fmt.Sprintf("%d installed: %s", len(short), strings.Join(short, ", ")))

			trustBtn := gtk.NewButtonWithLabel("Trust")
			trustBtn.SetValign(gtk.AlignCenterValue)
			btn := trustBtn
			clickedCb := func(_ gtk.Button) {
				uh.confirmTrustTap(t, btn)
			}
			trustBtn.ConnectClicked(&clickedCb)
			row.AddSuffix(&trustBtn.Widget)

			uh.brewTrustGroup.Add(&row.Widget)
			uh.brewTrustRows[t.Name] = row
		}
		uh.brewTrustGroup.SetVisible(true)
	})
}

// confirmTrustTap shows a confirmation dialog before trusting a tap's packages.
func (uh *UserHome) confirmTrustTap(tap homebrew.UntrustedTap, button *gtk.Button) {
	dialog := adw.NewAlertDialog(
		fmt.Sprintf("Trust packages from %s?", tap.Name),
		"Trusting allows this tap's package definitions to run code during installs and upgrades. Only trust taps you recognize.",
	)
	dialog.AddResponse("cancel", "Cancel")
	dialog.AddResponse("trust", "Trust")
	dialog.SetResponseAppearance("trust", adw.ResponseSuggestedValue)

	responseCb := func(_ adw.AlertDialog, response string) {
		if response != "trust" {
			return
		}
		button.SetSensitive(false)
		button.SetLabel("Trusting...")
		go uh.trustTap(tap, button)
	}
	dialog.ConnectResponse(&responseCb)
	dialog.Present(&uh.updatesPrefsPage.Widget)
}

// trustTap runs brew trust and updates the UI on completion.
func (uh *UserHome) trustTap(tap homebrew.UntrustedTap, button *gtk.Button) {
	err := homebrew.TrustPackages(tap)

	sgtk.RunOnMainThread(func() {
		if err != nil {
			button.SetSensitive(true)
			button.SetLabel("Trust")
			uh.toastAdder.ShowErrorToast(fmt.Sprintf("Failed to trust %s: %v", tap.Name, err))
			return
		}

		if row, ok := uh.brewTrustRows[tap.Name]; ok {
			uh.brewTrustGroup.Remove(&row.Widget)
			delete(uh.brewTrustRows, tap.Name)
		}
		if len(uh.brewTrustRows) == 0 {
			uh.brewTrustGroup.SetVisible(false)
		}
		uh.toastAdder.ShowToast(fmt.Sprintf("Trusted %s. Its packages can update again.", tap.Name))

		// Newly trusted packages may now appear as outdated.
		go uh.loadOutdatedPackages()
	})
}
```

- [ ] **Step 4: Make `loadOutdatedPackages` refresh-safe and error-aware**

In `loadOutdatedPackages`, inside the final `sgtk.RunOnMainThread`, clear old rows first (mirrors the flatpak pattern) and track new ones:

```go
	sgtk.RunOnMainThread(func() {
		for _, row := range uh.outdatedRows {
			uh.outdatedExpander.Remove(&row.Widget)
		}
		uh.outdatedRows = nil

		uh.outdatedExpander.SetSubtitle(fmt.Sprintf("%d packages available", len(packages)))
		for _, pkg := range packages {
			// ... existing row construction unchanged ...
			uh.outdatedExpander.AddRow(&row.Widget)
			uh.outdatedRows = append(uh.outdatedRows, row)
		}
	})
```

And in the per-package upgrade button callback, replace the raw error toast with an untrusted-aware one:

```go
					if err := homebrew.Upgrade(pkgName); err != nil {
						var trustErr *homebrew.UntrustedTapError
						msg := fmt.Sprintf("Upgrade failed: %v", err)
						if errors.As(err, &trustErr) {
							msg = fmt.Sprintf("%s comes from an untrusted tap — see Untrusted Homebrew Taps below", pkgName)
						}
						sgtk.RunOnMainThread(func() {
							uh.toastAdder.ShowErrorToast(msg)
						})
						return
					}
```

Add `"errors"` and `"strings"` to the imports of `updates_page.go`.

- [ ] **Step 5: Config key**

Add to `config.yml` and `config.bootc-example.yml` under `updates_page`:

```yaml
  brew_trust_group:
    enabled: true
```

- [ ] **Step 6: Build and test**

Run: `make build && go test ./... && go vet ./internal/views/`
Expected: pass.

- [ ] **Step 7: Commit**

```bash
git add internal/views/ config.yml config.bootc-example.yml
git commit -m "feat: add untrusted Homebrew taps group with per-tap trust flow

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 10: Bump updex to v1.2.3

**Files:**
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Bump**

```bash
go get github.com/frostyard/updex@v1.2.3
go mod tidy
```

- [ ] **Step 2: Verify**

Run: `make build && go test ./...`
Expected: builds clean (verified during planning: v1.2.3 compiles against existing code; transitive bumps of klauspost/compress and ini.v1 are expected).

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: bump frostyard/updex to v1.2.3

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 11: Startup timing instrumentation and sync-exec audit

**Files:**
- Modify: `cmd/chairlift/main.go`, `internal/window/window.go` (around `views.New` call, line ~103), `internal/views/views.go` (`New()`)

- [ ] **Step 1: Add timing logs**

In `internal/views/views.go` `New()`:

```go
func New(cfg *config.Config, toastAdder ToastAdder) *UserHome {
	start := time.Now()
	// ... existing body ...
	log.Printf("views: all pages built in %s", time.Since(start))
	return uh
}
```

(add `"log"` and `"time"` imports). In `internal/window/window.go` around the `w.views = views.New(...)` call and in `cmd/chairlift/main.go` before app creation, add equivalent `log.Printf("window: ...", time.Since(...))` marks so the log shows: process start → config loaded → views built → window presented.

- [ ] **Step 2: Audit for synchronous exec on the build path**

Run: `grep -n "exec.Command\|IsInstalled()\|GetStatus\|runBrewCommand\|ListFeatures" internal/views/*.go internal/window/*.go internal/app/*.go | grep -v "go \|go func"`

Manually confirm every hit is either (a) inside a function only called from a goroutine (`loadBootcStatus`, `loadUntrustedTaps`, `loadOutdatedPackages`, etc.) or (b) a click handler. The only synchronous IO allowed at build time: `config` file read and `/etc/os-release` read (local files). If any wrapper call is found on the synchronous path, move it into a goroutine using the deferred-visibility pattern (build hidden → `go load...()` → `RunOnMainThread` show).

- [ ] **Step 3: Manual timing run**

Run: `./build/chairlift --dry-run 2>&1 | head -30` on the dev host (GUI session). Confirm from the log timestamps that pages build in well under 1 second and the window presents without waiting on brew/bootc/updex checks. Close the window.
Expected: `views: all pages built in <N>ms` where N < 1000, ideally < 100.

- [ ] **Step 4: Commit**

```bash
git add cmd/chairlift/main.go internal/window/window.go internal/views/views.go
git commit -m "feat: add startup timing instrumentation and verify async page builds

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 12: Documentation

**Files:**
- Modify: `yeti/OVERVIEW.md`, `yeti/package-managers.md`, `CONFIG.md` (if anything left from Task 6), `README.md`, `CLAUDE.md` (only if it mentions nbc — check)

- [ ] **Step 1: yeti/OVERVIEW.md**

Rewrite every NBC reference for bootc. Specifically:
- Purpose paragraph: "NBC bootc system updates" → "bootc system updates (staged via the snow `bootc-update-stage` script)".
- Architecture tree: `internal/nbc/` line → `internal/bootc/   bootc wrapper (status reads, pkexec stage script, line streaming)`.
- Dependency flow list: swap nbc for bootc.
- "NBC boot gate" section → "bootc boot gate": gate is `bootc.IsBootcBootedCached()` checking `bootc status --format json` has non-null `status.booted`; explicitly note `/run/ostree-booted` is absent on composefs deployments and must not be used.
- "Streaming progress (NBC)" and "Shared NBC progress UI helper" sections → describe `bootc.StageUpdate` line-streaming (EventMessage/EventError/EventComplete) and `onBootcStageClicked`, including WHY the stage script is used instead of `bootc upgrade` (upstream registry-transport composefs bug; podman does the pull; single source of truth in snosi).
- Key config groups table: `nbc_status_group` → `bootc_status_group`, `nbc_updates_group` → `bootc_updates_group`, add `brew_trust_group`.
- Update badge section: `nbcUpdateCount` → `bootcUpdateCount` (1 when a deployment is staged).
- Privileged operations section: nbc paragraph → bootc stage script + polkit action id `org.frostyard.ChairLift.bootc.stage`.
- Key external Go dependencies table: remove `github.com/frostyard/nbc`; updex row version note.
- Runtime dependencies: replace NBC entry with `bootc` + `/usr/libexec/bootc-update-stage` (optional; UI gated on bootc-booted status).

- [ ] **Step 2: yeti/package-managers.md**

Replace the NBC wrapper section with an `internal/bootc` section covering: unprivileged `GetStatus`, the boot gate semantics, `StageUpdate` via pkexec + stage script, event types, dry-run behavior. Add a "Tap trust (Homebrew 6)" subsection to the Homebrew section: detection via tap-info JSON + Cellar receipts + Caskroom metadata (including WHY: untrusted formulae are invisible to `brew list`), `TrustPackages`, `UntrustedTapError`.

- [ ] **Step 3: README.md**

- Features: system update bullet now describes bootc staged updates; add tap-trust management bullet under Homebrew.
- Replace the stale Meson/Python build instructions with the Go build (`make build`, binaries in `build/`; runtime deps GTK4 + libadwaita; optional tools brew/flatpak/snap/bootc/updex).

- [ ] **Step 4: Sweep for leftovers**

Run: `grep -rni "nbc" README.md CONFIG.md CLAUDE.md yeti/ docs/ --include="*.md" | grep -v "docs/plans\|docs/superpowers\|README-go-port"`
Expected: no hits (historical plans/specs keep their nbc mentions; README-go-port.md is a historical document — leave it).

- [ ] **Step 5: Commit**

```bash
git add yeti/ README.md CONFIG.md
git commit -m "docs: update yeti, README, and CONFIG for bootc migration and brew trust

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 13: Final verification and PR

**Files:** none new.

- [ ] **Step 1: Full verification pass**

```bash
make build && go test ./... && make lint
grep -rn "internal/nbc\|frostyard/nbc" --include="*.go" .
git status --short   # no unexpected deletions/artifacts
```

Expected: build/test/lint green; grep empty; working tree clean apart from intended changes. If `make lint` fails on pre-existing issues unrelated to this branch, note them but do not fix in this PR.

- [ ] **Step 2: Manual smoke test on the dev host**

Run `./build/chairlift --dry-run` in the GUI session and verify:
- Window appears quickly (< ~1s to paint).
- System page: no bootc group (host is nbc-booted → gate correctly hides it), no errors.
- Updates page: Untrusted Taps group appears listing the host's untrusted taps (multica-ai/tap, stacklok/tap, tta-lab/ttal, ublue-os/tap at planning time); Flatpak/Homebrew groups load.
- Features page: features list loads with toggles (updex library path).
- No toasts with raw errors on startup.

- [ ] **Step 3: Push and open PR**

Target branch `main` (confirmed with user during brainstorming).

```bash
git push -u origin feat/modernize
gh pr create --base main --title "Modernize: bootc migration, Homebrew tap trust, updex bump, async startup" --body "$(cat <<'EOF'
## Summary
- Replace nbc integration with bootc: status via `bootc status --format json`, updates staged through the snow `bootc-update-stage` script (podman pull + `bootc switch --transport containers-storage`) because upstream `bootc upgrade` registry pulls fail on snow images
- Gate bootc UI on a non-null `status.booted` (NOT /run/ostree-booted, absent on composefs deployments)
- Add Untrusted Homebrew Taps group: Homebrew 6 tap trust detection (tap-info + Cellar receipts) with per-package `brew trust` behind a confirmation dialog
- Classify untrusted-tap upgrade failures into actionable messages
- Bump frostyard/updex v1.2.0 → v1.2.3
- Startup timing instrumentation; all tool checks remain async (deferred-visibility)
- First unit tests in the repo (bootc status parsing, stage streaming, tap trust detection)

## Breaking
- Removes nbc support entirely (config keys renamed nbc_* → bootc_*; nbc polkit files replaced)

## Post-merge follow-ups
- Cut v0.8.0 and upgrade hosts (installed v0.6.0 predates the updex library switch and async startup — that's what makes the Features tab and slow startup visible in the field)
- Validate staged-update flow on a bootc-booted snosi VM

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

Expected: PR created against `main`.

---

## Self-Review Notes (already applied)

- Spec coverage: bootc (Tasks 1–6), brew trust (7–9), updex (10), startup (11), docs (12), release follow-up (13 PR body). Config example rename handled in Task 6 (nbc example deleted, bootc example updated).
- Removed-code check: `onSystemUpdateClicked`'s Apply button has no bootc equivalent by design — staged deployments apply on reboot (spec: "no Apply button").
- Type consistency: `bootc.ProgressEvent{Type, Message}` used identically in Tasks 2 and 4; `UntrustedTap{Name, Formulae, Casks}` identical in Tasks 7–9; views fields declared in Task 4 Step 1 and Task 9 Step 1 before use.
- The nbc example config deletion (Task 6) means `git rm config.nbc-example.yml`; anyone with configs referencing `nbc_status_group` silently falls back to default-enabled `bootc_status_group`, which the gate hides on non-bootc hosts — degrades gracefully per spec.
