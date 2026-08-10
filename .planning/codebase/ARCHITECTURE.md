# Architecture

**Analysis Date:** 2026-08-09

## Pattern Overview

**Overall:** Layered GTK4/Libadwaita desktop application with dedicated
system-tool adapters.

**Key Characteristics:**
- Pure Go implementation using puregotk bindings (no CGO required)
- GObject-registered application and window types backed by snowkit
- Configuration-driven page and feature-group visibility
- Dedicated Homebrew, Flatpak, bootc, and Updex integrations
- Asynchronous external work with GTK main-thread UI updates
- Dry-run mode propagated across every state-changing path

## Runtime Package Layout

```text
cmd/chairlift/main.go
    -> internal/app
        -> internal/window
            -> internal/config
            -> internal/navigation
            -> internal/views
                -> internal/{bootc,flatpak,homebrew,updex}
                -> internal/views/{actionmsg,actionstate,badgestate,bundleview,
                                   featurestatus,flatpakstatus,rowset,trustmsg}
        -> internal/{bootc,flatpak,homebrew,updex,views} (dry-run setup)

cmd/chairlift-updex-helper/main.go
    -> internal/updexhelper
    -> github.com/frostyard/updex/updex
```

`internal/version` stores build metadata used by the main binary and window.
`internal/installcheck` is a test-only package that verifies documentation,
PolicyKit, Makefile, and release-package contracts.

## Layers

### Entry Points (`cmd`)

ChairLift builds two binaries:

- `cmd/chairlift` injects build metadata, creates `internal/app.Application`,
  and starts the GTK run loop.
- `cmd/chairlift-updex-helper` is the narrowly scoped privileged executable
  for Updex writes. It parses a fixed command grammar through
  `internal/updexhelper` before calling the Updex Go API.

### Application (`internal/app`)

`Application` is a registered `adw.Application` subtype. It owns the
application lifecycle, detects `--dry-run`, propagates dry-run state to the
integration and view packages, reuses the existing window on repeated
activation, and registers keyboard accelerators from the window's visible
navigation inventory.

### Window and Navigation (`internal/window`, `internal/navigation`)

`Window` is a registered `adw.ApplicationWindow` subtype. It loads
configuration, creates `views.UserHome`, and composes an
`adw.NavigationSplitView` from a sidebar and a `gtk.Stack`. It also implements
the views' `ToastAdder` interface for success/error messages and update badges.

`internal/navigation` is the single widget-free authority for page order,
titles, icons, configuration groups, shortcuts, and page-selection
transitions. `navigation.VisibleItems` removes functional pages whose groups
are all statically disabled, always retains Help, and compacts Alt+number
shortcuts. Both mouse and keyboard navigation call `Window.navigateToPage`,
which applies the transition returned by `navigation.Resolve` to the sidebar,
content stack, title, and collapsed split-view state.

### Views (`internal/views`)

`views.UserHome` coordinates six pages: Applications, Maintenance, Updates,
System, Features, and Help. Shared widget references and page lookup live in
`views.go`; each page's builders and handlers live in its own
`*_page.go` file.

View handlers start external work in goroutines and marshal widget changes
through `snowkit/gtk.RunOnMainThread`. Pure decision and bookkeeping logic is
kept in the GTK-free packages below `internal/views` so it can be unit tested
without loading native GTK libraries.

### Configuration (`internal/config`)

The configuration layer loads YAML, merges an authoritative file over built-in
defaults, validates the result, and answers page/group enablement queries.
The first existing candidate is authoritative. An unreadable, malformed, or
invalid file returns a fail-closed configuration with all groups disabled;
the window reports the retained `LoadError` through a persistent toast.

Static group configuration determines the page inventory. Some enabled groups
also apply runtime availability gates for optional tools, including the bootc
host and stage-script checks.

### System Integrations

- `internal/homebrew` wraps Homebrew listing, search, package actions,
  bundles, updates, and tap trust.
- `internal/flatpak` wraps installed applications, updates, uninstall, and
  cleanup operations.
- `internal/bootc` reads `bootc status --format json` and streams staged OS
  update progress from the distribution-provided stage helper.
- `internal/updex` uses the Updex Go API directly for reads and delegates
  writes to the installed ChairLift helper.
- `internal/updexhelper` validates the privileged helper's accepted argv and
  constructs the corresponding Updex options.

## Core Flows

### Application Startup

1. `cmd/chairlift/main.go` sets `internal/version` fields and calls
   `app.New()`.
2. `app.New()` registers command-line options and propagates dry-run mode.
3. GTK activation calls `window.New()`. The window loads configuration and
   builds the views.
4. `navigation.VisibleItems` derives the sidebar and stack inventory from
   static group configuration.
5. The window builds sidebar rows, adds the corresponding view pages to the
   content stack, selects the first visible page, and exposes the inventory to
   the application.
6. The application registers accelerators from the exact visible navigation
   inventory and presents the window.

### Ordinary User Action

1. A per-page handler disables or marks the initiating widget as busy.
2. The handler invokes the appropriate integration from a goroutine.
3. Results are dispatched to the GTK main thread.
4. The view updates rows and badges and reports success or failure through
   `ToastAdder`.

State-changing handlers derive dry-run-aware text and UI mutation decisions
from the GTK-free view subpackages. The integration packages also enforce
dry-run behavior so preview mode does not execute the underlying mutation.

### bootc Status and Staged Updates

bootc status reads are unprivileged. `internal/bootc.GetStatus` executes
`bootc status --format json` and parses booted, staged, and rollback
deployments. `IsBootcBootedCached` gates bootc UI on a non-null booted
deployment rather than a sentinel file or command exit status.

The System page only displays deployment status. The Updates page owns
staging:

1. The group is shown only when the host is bootc-booted and
   `/usr/libexec/bootc-update-stage` exists.
2. The view calls `bootc.StageUpdate` and consumes `ProgressEvent` messages.
3. `StageUpdate` runs `pkexec /usr/libexec/bootc-update-stage`; it does not run
   `bootc upgrade` or implement image pull/switch policy inside ChairLift.
4. The distribution-owned helper checks, downloads, and stages the next image.
   The staged deployment is applied on restart.
5. After completion, the view re-reads bootc status so its subtitle and update
   badge reflect the actual deployment state.

The progress channel carries message and completion events. Failures are
returned as errors rather than duplicated as error events.

## Privileged Boundary

ChairLift has two fixed privileged update paths:

| Operation | Unprivileged caller | PolicyKit executable |
|-----------|---------------------|----------------------|
| Stage a bootc OS image | `internal/bootc` | `/usr/libexec/bootc-update-stage` |
| Enable, disable, or update Updex features | `internal/updex` | `/usr/bin/chairlift-updex-helper` |

The executable paths are fixed in code and must exactly match the annotations
in `data/org.frostyard.ChairLift.bootc.policy` and
`data/org.frostyard.ChairLift.updex.policy`. The Updex policy additionally
selects the permitted operation by the helper's first argument, while
`internal/updexhelper` rejects unsupported or extra arguments before any
write.

ChairLift does not ship the bootc stage helper: staging mechanics are
distribution policy, and an image enabling `bootc_updates_group` must provide
the trusted helper at the fixed path. ChairLift does ship its Updex helper,
because that executable is the application's validation boundary around
Updex writes.

Configured maintenance actions may separately request `pkexec` with
`sudo: true`; they are explicit administrator-provided scripts rather than
part of either update integration.

## State and Concurrency

- Widget and page state is owned by `Window` and `views.UserHome`; there is no
  application-wide state store.
- Integration dry-run flags and cached availability checks are package-level.
- Update counts are held in the mutex-backed
  `internal/views/badgestate.Counts`.
- Refresh-generation gates prevent older asynchronous Homebrew queries from
  overwriting newer results.
- GTK objects are changed only on the main thread.

## Error Handling

- Integration packages return errors, including custom typed errors where the
  caller needs to distinguish missing tools, command failures, timeouts, or
  cancellation.
- View goroutines convert failures into error toasts and restore affected UI
  controls on the main thread.
- Configuration failures retain the source path and cause, disable all
  configurable groups, and produce both a high-signal log message and a
  persistent startup toast.
- External command output is surfaced where useful, but errors are not
  swallowed or represented as successful completion.

## External Dependencies

- `codeberg.org/puregotk/puregotk`: GTK4 and Libadwaita bindings
- `github.com/frostyard/snowkit`: GObject registration and GTK main-thread
  dispatch
- `github.com/frostyard/updex`: feature reads and privileged helper operations
- `gopkg.in/yaml.v3`: configuration parsing

---

*Architecture analysis: 2026-08-09*
