# ChairLift Overview

Living design document (formerly `yeti/OVERVIEW.md`; folded into `docs/` per
[frostyard/core ADR-0025](https://github.com/frostyard/core/blob/main/docs/adr/0025-consolidate-repository-docs-into-docs.md)).

## Purpose

ChairLift is a GTK4/Libadwaita system management GUI for [Snow Linux](https://github.com/frostyard/snosi), written in Go using [puregotk](https://codeberg.org/puregotk/puregotk) bindings (no CGO). It provides a unified interface for managing Homebrew and Flatpak applications, OS system updates (staged via the snow `bootc-update-stage` script on bootc installs, or the snow `snosi-sysupdate-stage` script on native A/B installs), system features (via updex), and maintenance tasks. The UI is YAML-configuration-driven, making it portable to other Linux distributions by toggling feature groups on/off. A separate first-run mode (`chairlift --first-run`, started by the pure-Go `chairlift-first-run` gate) replaces SNOW First Setup on images that ship its trigger; see [First-run mode](#first-run-mode). The Problems page shows the problems that the system checks, the user's crashes and frostyard apps recorded on this computer, and sends nothing until the user opts in to reporting; see [Problems page](#problems-page).

## Architecture

```
cmd/chairlift/main.go                 Entry point: version injection, app creation
cmd/chairlift-updex-helper/main.go    Privileged helper for updex write operations
cmd/chairlift-first-run/main.go       Pure-Go first-run gate: exits or execs `chairlift --first-run`
        │
internal/app/app.go             GObject-registered Application (adw.Application subtype)
        │
internal/window/window.go       Main window: NavigationSplitView, sidebar, content stack
internal/window/firstrun.go     First-run window: header Back/Next, a stack of the first-run steps
        │
internal/views/                 Page builders and event handlers (one file per page)
        │                       ├── internal/views/actionmsg/     ┐ puregotk-free leaf packages:
        │                       ├── internal/views/actionstate/   │ toast/decision text, async gates,
        │                       ├── internal/views/badgestate/    │ badge counts, bundle action state,
        │                       ├── internal/views/bundleview/    │ row bookkeeping, expander status,
        │                       ├── internal/views/trustmsg/      │ Flatpak and feature update-status
        │                       ├── internal/views/rowset/        │ text and decisions, all unit-tested
        │                       ├── internal/views/flatpakstatus/ │ headlessly
        │                       ├── internal/views/featurestatus/ │
        │                       └── internal/views/pageview/      ┘
        │
        ├── internal/config/    YAML config loading, feature group enablement
        ├── internal/navigation/ Canonical pages, shortcuts, and pure navigation transitions
        ├── internal/homebrew/  Homebrew CLI wrapper (JSON output parsing)
        ├── internal/flatpak/   Flatpak CLI wrapper (tabular output parsing)
        ├── internal/fwupd/     fwupd CLI wrapper (JSON update list, unprivileged install)
        ├── internal/bootc/     bootc wrapper (status reads, fixed stage adapter)
        ├── internal/sysupdate/ native A/B detection, status, rollback, fixed stage adapter
        ├── internal/stageexec/ Pure-Go shared OS staging stream/event executor
        ├── internal/updex/     Updex feature manager (Go library reads, helper binary writes)
        ├── internal/updexhelper/ Puregotk-free argv-parsing/Options-building for cmd/chairlift-updex-helper
        ├── internal/firstrun/  Puregotk-free first-run logic: gate, markers, steps, apps list, actions
        ├── internal/i18n/      Puregotk-free .po reader and the embedded first-run translations
        ├── internal/problems/  Puregotk-free, network-free Problems page data: sources, v1 validation, hints, dismiss store
        ├── internal/selfreport/ ChairLift's own problem recorder (std problems) and its error codes
        └── internal/version/   Build metadata (ldflags injection)
```

### Dependency flow

`cmd → app → window → views → {config, homebrew, flatpak, fwupd, bootc, sysupdate, updex}`.
`app` and `window` also depend on the pure `navigation` package.
The first-run mode adds `cmd/chairlift-first-run → firstrun → {config, flatpak, i18n}`
(no GTK package, guarded by `internal/installcheck`) and `app → window (firstrun.go) → {firstrun, i18n}`.
The Problems page adds `views (problems_page.go) → {problems, selfreport}` and
`problems → {selfreport, github.com/frostyard/std/problems}`; `cmd/chairlift`,
`window` and `updex` call `selfreport` to record ChairLift's own problems.

External shared library: `github.com/frostyard/snowkit` (published module, pinned in go.mod) provides:

- `gobj` — GObject type registration and instance registry
- `sgtk.RunOnMainThread()` — main-thread dispatch for GTK safety

### Views coordinator (`internal/views/views.go`)

The `views.go` file defines the central `UserHome` struct that holds references to all page widgets, config, and the `ToastAdder` interface. It provides:

- `New(cfg, toastAdder)` — constructor that initializes `UserHome`
- `ToastAdder` interface — `ShowToast(msg)`, `ShowErrorToast(msg)`, `SetUpdateBadge(count)` — implemented by Window

`internal/views` imports puregotk, so it can never hold a `_test.go` (see `docs/agents/skills/gtk-headless-tests.md`). Decidable logic is therefore pushed down into nine puregotk-free leaf packages beneath it — `internal/views/actionmsg` and `internal/views/trustmsg` (toast text and UI decisions, see [package-managers.md](./package-managers.md#view-layer-toast-and-decision-helpers-internalviewsactionmsg-internalviewstrustmsg)), `internal/views/actionstate` (Homebrew update command/refresh outcomes and repeated-click gates, see [package-managers.md](./package-managers.md#view-layer-update-action-state-internalviewsactionstate)), `internal/views/badgestate` (thread-safe per-provider update counts and totals, see [package-managers.md](./package-managers.md#view-layer-update-badge-state-internalviewsbadgestate)), `internal/views/bundleview` (Brew bundle empty/error/unavailable presentation and per-row install gating, see [package-managers.md](./package-managers.md#view-layer-brew-bundle-state-internalviewsbundleview)), `internal/views/rowset` (single-row removal plus clear-then-repopulate bookkeeping, see [package-managers.md](./package-managers.md#view-layer-row-bookkeeping-internalviewsrowset)), `internal/views/flatpakstatus` (the Flatpak updates expander's subtitle text and expandable decision, applied by `loadFlatpakUpdates` from both retained `ListUpdates` errors, see [package-managers.md](./package-managers.md#view-layer-flatpak-update-status-internalviewsflatpakstatus)), `internal/views/featurestatus` (the Features page's per-feature update-status subtitle, which covers an update, a feature that waits for a newer OS image, a feature whose new version applies after a reboot, and an up-to-date feature, the any-component update decision, in which the waiting branch takes precedence over every update branch and the update over the pending-reboot branch, the whole check summarised at once by `Summarize` — one `Status` per row plus the update and waiting feature counts, and a base subtitle the page restores for every name the check did not answer for — and the features group description for all five check outcomes — `GroupDescriptionCheckFailed` when the check itself failed, and `GroupDescription` when it completed with zero features updatable, with updates found, with features waiting for a newer OS image, or with both updates and waiting features — applied by `checkFeatureUpdates`, which composes no subtitle or description text of its own and runs again after an update through `recheckFeatureUpdates`, see [package-managers.md](./package-managers.md#view-layer-feature-update-status-internalviewsfeaturestatus)), and `internal/views/pageview` (the row text, page status, os-release parsing, Help resource ordering, and maintenance-command selection shared by all seven page builders, plus the Problems page's row, status and detail text, see [package-managers.md](./package-managers.md#view-layer-page-presentation-internalviewspageview)) — each table- or scenario-tested headlessly. This layout is decision record
[ADR-0007](../adr/0007-pure-leaf-packages-route-around-untestable-gtk.md).

### Pages

The UI defines seven pages, each in its own file under `internal/views/`. The
first-run window is not a page: it lives in `internal/window/firstrun.go`, and
`internal/navigation` does not list it. Static
configuration may omit any functional page whose builder-backed groups are all
disabled; Help is always retained:

| Page         | File                   | Purpose                                                                                                                                 |
| ------------ | ---------------------- | --------------------------------------------------------------------------------------------------------------------------------------- |
| Applications | `applications_page.go` | Manage Homebrew formulae/casks and installed Flatpaks; launch an external manager for new Flatpak installs                              |
| Maintenance  | `maintenance_page.go`  | Homebrew/Flatpak cleanup, configurable maintenance scripts (executed via `exec.Command`/`pkexec`)                                       |
| Updates      | `updates_page.go`      | bootc or native A/B (systemd-sysupdate) staged system updates, Flatpak updates, Homebrew outdated packages, untrusted-tap trust prompts |
| System       | `system_page.go`       | OS info (`/etc/os-release`), bootc deployment status, health monitor launch                                                             |
| Features     | `features_page.go`     | Toggle system features via `updex` tool                                                                                                 |
| Problems     | `problems_page.go`     | Local problem records: snosi-health's system checks, the user's crashes and frostyard app records; Copy details and Dismiss ([ADR-0015](../adr/0015-problems-page-and-opt-in-reporting.md)) |
| Help         | `help_page.go`         | Configurable links to website, issues, chat (opened via `xdg-open`)                                                                     |

## Key Patterns

### GObject registration via snowkit

Application and Window are registered as GObject subtypes using `gobj.RegisterType()`. This returns a `gobject.Type` and an `*gobj.InstanceRegistry`. The pattern:

1. `init()` registers the type with `ClassInit` callback
2. `ClassInit` overrides `Constructed` to create the Go struct and pin it in the registry
3. Constructor (`New()`) calls `gobject.NewObject()` then retrieves the Go instance from the registry

See `internal/app/app.go` and `internal/window/window.go`.

### Async operations with main-thread dispatch

All external tool calls run in goroutines. UI updates are marshaled back via `sgtk.RunOnMainThread()`:

```go
go func() {
    result, err := homebrew.ListInstalledFormulae()
    sgtk.RunOnMainThread(func() {
        // update widgets here
    })
}()
```

### Deferred visibility (async startup)

To avoid blocking startup on slow tool-availability checks, groups that depend on optional tools (Homebrew, Flatpak, Updex) are built immediately with placeholder descriptions and then shown or hidden asynchronously. The pattern:

1. Build the UI group unconditionally (if config-enabled), with a placeholder description
2. Store a reference to the group on `UserHome` (e.g., `maintenanceBrewGroup`)
3. Spawn a goroutine that calls `IsInstalledCached()` (see below)
4. On the main thread, either hide the group (`SetVisible(false)`) or update its description

This applies to: `maintenanceBrewGroup`, `maintenanceFlatpakGroup`, `featuresGroup`/`featuresUnavailableGroup`. The Features page uses a dual-group approach — one for available features, one for "not available" — toggling visibility between them.

### bootc boot gate

bootc-related UI groups (system page's `bootc_status_group` and updates page's `bootc_updates_group`) are gated on `bootc.IsBootcBootedCached()`, which runs `bootc status --format json` once (via `sync.Once`) and reports true only when the parsed `status.booted` field is non-null. This is deliberately not a sentinel-file check: `/run/ostree-booted` is absent on snow's composefs-based deployments, so relying on it would hide the groups on every snow bootc host. `bootc status` itself exits 0 with a null `booted` entry on non-bootc hosts, so the gate must inspect the JSON body rather than the exit code.

### Native A/B gate

The updates page's `sysupdate_updates_group` is gated on `sysupdate.IsNativeABCached()` (an `os.Stat` of `/usr/lib/snosi/native-ab`, cached via `sync.Once`) plus `sysupdate.StageScriptAvailable()` (`/usr/libexec/snosi-sysupdate-stage` exists). Unlike the bootc case, a marker-file check is correct here: the marker is snosi's own published contract for "this host updates via systemd-sysupdate" — every snosi unit and script gates on it (`ConditionPathExists=/usr/lib/snosi/native-ab`) — whereas `/run/ostree-booted` was a foreign sentinel that snow's bootc deployments never wrote. The `bootc` binary is absent on native A/B images and `os-release`'s `IMAGE_ID` is identical across both variants, so neither is usable for this gate. The two OS-update gates are mutually exclusive at runtime: at most one "System Updates" group ever becomes visible even though both may be config-enabled and built.

### Dry-run mode

Decision record: [ADR-0009](../adr/0009-dry-run-output-convention-and-single-decision-structs.md)
(the `[DRY-RUN]` output convention and single decision structs).

The `--dry-run` / `-d` flag is propagated to wrapper packages via `SetDryRun(true)`, set once at startup in `app.New()` for homebrew, flatpak, fwupd, bootc, sysupdate, updex, `internal/firstrun` (the first run's color-scheme write and completion markers), and `internal/views` itself (`internal/views/dryrun.go` — for configured custom maintenance scripts, which have no wrapper package of their own).

**The general rule, applied uniformly:** every state-changing view handler branches on the relevant wrapper's `IsDryRun()` (or `views.IsDryRun()` for custom scripts) to show an explicit preview toast instead of a completed/saved/installed message. Anywhere that same handler would _also_ mutate a row, a group's visibility, or a switch on success, that mutation decision is pulled out of the view and expressed as a small struct or an `actionstate.Decision` — `ScriptDecision.Execute`, `BundleInstallDecision.Complete`, `TapTrustDecision.MutateUI`, `FeatureToggleDecision.Confirm`, `FeatureUpdateDecision.Persistent`, `FirmwareInstallDecision.PublishList`, `ProblemDismissDecision.Hide`, `PackageInstall`, `PackageUninstall`, or `PackagePin`. The view computes `IsDryRun()` exactly once, builds the decision, and branches solely on it for both the mutation _and_ the toast, so a table-driven test proves the mutation gate and the toast cannot drift from it (see [package-managers.md](./package-managers.md#view-layer-toast-and-decision-helpers-internalviewsactionmsg-internalviewstrustmsg) for the full function/type list). `FeatureUpdateDecision.Persistent` is the same pattern for the Features page's Update toast: it decides whether the toast stays on screen as an error toast, and the same struct's `Recheck` decides whether the update check runs again, so the rows show the state after the update instead of the startup check's state. Sites with no second UI decision to gate (package upgrade/update/self-update, Flatpak uninstall, cleanup, Brewfile dump, and bootc stage toasts) get a plain string function instead.

**Intentional exception:** bootc staging's completion **toast** is dry-run-aware (`actionmsg.BootcStage`), but its expander **subtitle** deliberately is not. The subtitle is a persistent status readout of live `bootc.GetStatus()` — what deployment is actually staged/booted right now — not a per-click completion claim, so it stays accurate and unchanged in both dry-run and live mode. Only the toast, which inherently answers "what did this click just do," needed dry-run-specific wording; there is no mutation left to gate once the subtitle is deliberately excluded, which is why `BootcStage` is string-only rather than a decision struct. Native A/B staging follows the identical split: `actionmsg.SysupdateStage` is the dry-run-aware toast, while the subtitle and rollback row re-read the real `/run/snosi` state files and partition labels in both modes.

Per-wrapper mechanics:

- **Homebrew/Flatpak**: state-changing commands are skipped entirely at the wrapper layer (return mock/empty results); ordinary package-action toasts use the plain `actionmsg` string functions (`Install`, `Uninstall`, `Pin`, `Upgrade`, `Update`, `SelfUpdate`, `BundleDump`, `Cleanup`). Homebrew search installs and installed-package uninstall/pin actions pair that text with `actionstate` decisions: live success completes the old row controls and refreshes the installed inventory, while failure or dry-run restores the controls. Brew bundle installation uses `BundleInstallDecision` with the same live-complete/dry-run-reset distinction.
- **Updex**: `EnableFeature`/`DisableFeature`/`UpdateFeatures` skip their `pkexec` call entirely under dry-run and return empty/nil results (`UpdateFeatures` returns nil results and a nil error, and `actionmsg.FeatureUpdate` shows the preview toast); the helper binary itself (`cmd/chairlift-updex-helper`, dispatch logic in `internal/updexhelper`) also honors `--dry-run` for `update`, matching `enable-feature`/`disable-feature`, as defense-in-depth even though it's unreachable from the wrapper today.
- **bootc**: `StageUpdate` short-circuits before invoking pkexec: it logs the would-be command, emits a synthetic `EventMessage` + `EventComplete` pair on the progress channel, and returns — the stage script is never actually run (see the exception above for the toast/subtitle split).
- **sysupdate**: `StageUpdate` has the same shape as bootc's — under dry-run it logs the would-be `pkexec /usr/libexec/snosi-sysupdate-stage` command, emits the synthetic `EventMessage` + `EventComplete` pair, and never constructs an `exec.Cmd`. Status reads (`GetStatus`, `RollbackVersion`) are not dry-run-gated: they are unprivileged, side-effect-free reads of real state.
- **Homebrew tap trust**: `trustTap` (`internal/views/updates_page.go`) computes `decision := actionmsg.TapTrust(homebrew.IsDryRun(), tap.Name)` once, after a successful `homebrew.TrustPackages` call, and gates removing the tap's row, hiding the group, and refreshing outdated packages on `decision.MutateUI`.
- **views (custom maintenance scripts)**: `runMaintenanceAction` (`internal/views/maintenance_page.go`) calls `actionmsg.MaintenanceScript(IsDryRun(), title)` once, before spawning its goroutine, to get a `ScriptDecision{Execute, Toast}`: when `Execute` is false no `exec.Cmd` is ever constructed (no `pkexec`, no direct script exec) — only a `[DRY-RUN] Would execute: ...` log line.
- **Features page switch confirmation**: `onFeatureToggled` (`internal/views/features_page.go`) computes `decision := actionmsg.FeatureToggle(updex.IsDryRun(), enabled, name)` once, after a successful `updex.EnableFeature`/`DisableFeature` call, and branches solely on `decision.Confirm` to decide whether the switch confirms the flip (`toggle.SetActive(enabled)`) or reverts to its pre-click state (`toggle.SetActive(!enabled)`).
- **First run**: the first-run window reads `firstrun.IsDryRun()` once when the Progress step starts and passes it to `firstrun.Complete`, whose `firstrun.Completion` decision (`WriteMarkers`, `Log`, `DoneDescription`) decides both whether the markers are written and the Done step's text (`[DRY-RUN] Preview: first run finished — no changes made`). `firstrun.ApplyColorScheme` logs `[DRY-RUN] would execute: gsettings ...`, and the Flatpak wrapper skips `remote-add` and `install`.
- **Firmware install**: `installFirmwareUpdate` (`internal/views/updates_page.go`) calls `fwupd.InstallAndVerify`, then builds `decision := actionmsg.FirmwareInstall(fwupd.IsDryRun(), installErr, report, …)` once. `decision.PublishList` decides whether the verified device list is published, `decision.RestoreControl` whether the Install button comes back, and `decision.IsError` whether the toast stays on screen. `fwupdmgr` can exit 0 without flashing a device, so the view never reads the exit status as success on its own.
- **Problems page Dismiss**: the Dismiss button (`internal/views/problems_page.go`) computes `decision := actionmsg.ProblemDismiss(IsDryRun(), item.ID, title)` once. Under dry-run `decision.Persist` and `decision.Hide` are false: nothing is written to the dismiss store, the row stays, the toast is `[DRY-RUN] Preview: dismiss <title> — no changes made`, and the log line is `[DRY-RUN] would dismiss problem <id>`. Live, the page writes the dismissal and removes the row only after the write succeeds; a failed write keeps the row and shows `actionmsg.ProblemDismissFailed`. Recording a journal problem record (`internal/selfreport`) is logging, not a system change, so it also runs under dry-run.

### Configuration-driven UI visibility

Each preference group on every page checks `config.IsGroupEnabled(pageName, groupName)` before building its widgets. Groups default to enabled if not specified in config. `maintenance_cleanup_group` defaults to disabled in the default config.

`internal/config/config.go` builds the effective `*Config` by overlaying a
parsed file onto `defaultConfig()` field by field, not by replacing it
wholesale — `mergeConfig` walks each page, `mergePage` walks each group within
a page, and `mergeGroup` walks each field within a group. The overlay is
driven by `rawConfig`/`rawPageConfig`/`rawGroupConfig`, a pointer-typed mirror
of `Config`/`PageConfig`/`GroupConfig` used only for YAML decoding: a `nil`
pointer means the file omitted (or explicitly nulled) that key, so
`defaultConfig()`'s value survives; a non-nil pointer means the file set that
key — including to an empty string or empty slice — so it replaces the
default outright. This is why `maintenance_cleanup_group` stays disabled
overall (and keeps its default `actions` entry) when a config file mentions
the group only to flip an unrelated field. When every search candidate is
absent, `Load()` returns `defaultConfig()`. An authoritative file that cannot
be read or validated instead returns `disabledConfig()`: the same defaults
for non-visibility fields, with every canonical group's `Enabled` field
forced to false.

**Structured load-error vocabulary (`internal/config/loaderror.go`).** A
stable `ErrorKind` enumerates why loading/validating a config file could
fail: `KindRead` ("read") for a filesystem/read failure (e.g. permission
denied opening the file, or a dangling symlink — distinct from "file does
not exist", which `Load()` treats as absent-and-fall-back, not an error); `KindParseType`
("parse/type") for a YAML syntax error, a value that decodes to the wrong
Go type, or a malformed source graph detected by pure shape/graph
inspection after the YAML parsed successfully (an unsupported node shape,
an alias with no target, an alias cycle, etc.) — like `KindSchema` below,
that shape-inspection case may legitimately carry a nil `Err`, since there
is no underlying parser error to wrap; and `KindSchema` ("schema") for a
validator-detected shape failure
found only after the document parsed successfully — e.g. an unknown key or a
value of the right YAML kind but the wrong shape — which may legitimately
carry a nil `Err` since shape inspection alone can detect the problem with
no underlying cause to wrap. `LoadError.Error()` renders `Path`, the exact
`Kind` string, `Detail`, and the cause (in that order, each only when
non-empty/non-nil), so the three `Kind` literals above always appear
verbatim in the message. `LoadError.Unwrap()` returns `Err` unchanged
(nil when there is none), which is what lets `errors.Is`/`errors.As` see
through a `*LoadError` to a wrapped sentinel or recover the original value
from a `fmt.Errorf("%w", ...)` wrapper.

**Single-document YAML parsing (`internal/config/source.go`).** The
unexported `parseYAMLDocument(path string, data []byte) (*yaml.Node,
*LoadError)` is the first thing in this package that actually constructs a
`LoadError`. It decodes `data` with `yaml.NewDecoder` and accepts exactly one
YAML document, with a fixed cardinality outcome per input shape:

- Empty input, and whitespace-only input, are both the valid empty result:
  `(nil, nil)` — no document, no error. This matches `Load()`'s existing
  "absent config" fallback semantics elsewhere in the package.
- A single well-formed document returns its `*yaml.Node` (`Kind ==
yaml.DocumentNode`, one child under `Content`) and a nil `*LoadError`.
- A malformed first document returns `(nil, err)` with `err.Kind ==
KindParseType`, `err.Path` copied verbatim, `err.Err` set to yaml.v3's own
  parser error, and `err.Detail` set to that same error's message — which
  yaml.v3 always renders as `"yaml: line N: ..."`, so the reported line is
  visible in `Detail` without a second look at `Err`.
- A trailing bare `---` is a second document, not a harmless end-of-stream
  marker: yaml.v3 decodes it as a second, null document, and
  `parseYAMLDocument` rejects it exactly like any other second document
  (`KindParseType`, `Detail` naming its line) rather than accepting the
  input as a single document.
- Any other second document — well-formed or malformed — is rejected the
  same way: a well-formed second document has no parser error to wrap, so
  `Err` is left nil and `Detail` names that document's own starting line
  instead; a malformed second document preserves _that_ document's parser
  error and line in `Err`/`Detail`, just like a malformed first document
  would.

These capabilities are now the runtime loading path:
`Load`/`loadFromPath` → `loadResolvedPath` → `parseAndValidate` →
`parseYAMLDocument`/`resolveEffective`/`validateSourceGraph`. Read failures
produce `KindRead`; malformed or wrong-type YAML produces `KindParseType`;
unknown schema names and invalid shapes produce `KindSchema`. No runtime
load uses the old permissive `yaml.Unmarshal` path.

**Exact merge-key recognition and tag normalization
(`internal/config/sourcegraph.go`).** `isMergeKey(n *yaml.Node) bool` and
`shortYAMLTag(tag string) string` are deliberate behaviorally exact reproductions
of two unexported predicates from `gopkg.in/yaml.v3` v3.0.1 itself —
`decode.go:isMerge` and `resolve.go:shortTag` — rather than reimplementations
from a description of YAML merge-key semantics, so this package's own
merge-key walk (`effectiveEntries`, described below) recognizes exactly the
same nodes yaml.v3's own decoder would treat as a merge key, node for node.
`isMergeKey` reports
true only for a `yaml.ScalarNode` whose `Value` is exactly `"<<"` and whose
`Tag` is one of: absent (`""`, the implicit/unresolved tag), the bare
non-specific tag (`"!"`), the short merge tag (`"!!merge"`), or its canonical
long form (`"tag:yaml.org,2002:merge"`) — the last two both recognized via
`shortYAMLTag`. A quoted `"<<"` (explicitly tagged `"!!str"`) is therefore an
ordinary key, not a merge key, and so is a merge-tagged scalar whose `Value`
isn't literally `"<<"`; non-scalar nodes (mapping, sequence, alias) and a nil
`*yaml.Node` are never merge keys either — `isMergeKey` is nil-safe rather
than panicking. `shortYAMLTag` rewrites a canonical `"tag:yaml.org,2002:xxx"`
tag to yaml.v3's short `"!!xxx"` form and returns any tag without that prefix
unchanged (including the empty tag, `"!"`, an already-short `"!!xxx"` tag, a
custom `"!xxx"` tag, and a long tag under a different authority such as
`"tag:example.com,2020:merge"`). These are the only two unexported helpers in
this package's source-graph slice that the spec singles out for a direct
unit test (`TestMergeKeyRecognition`, `TestShortYAMLTagNormalization` in
`sourcegraph_test.go`) rather than only exercising them through an exported
entry point, per
`docs/agents/skills/helper-functions-need-direct-test-calls.md`; neither
helper is called directly by runtime loading, but both are reached indirectly
through `resolveEffective` in the strict validator pipeline.

**Reachable source-graph shape validation
(`internal/config/sourcegraph.go`).** `validateSourceGraph(path string, doc
*yaml.Node) *LoadError` walks every node and content edge reachable from
`doc` — through document content, mapping keys, mapping values, sequence
entries, and alias targets, in each node's `Content` slice order — and
rejects a malformed source graph as `KindParseType` rather than panicking,
even against a synthetic `*yaml.Node` tree unreachable from real YAML text.
`doc == nil` succeeds unconditionally: it is `parseYAMLDocument`'s valid
empty-input result. Otherwise the checks are, in order:

- the root must be a `yaml.DocumentNode` with exactly one non-nil child;
- every reachable mapping must have an even number of content entries, and
  neither a key nor a value in any key/value pair may be nil;
- every reachable sequence must have no nil entries;
- every reachable scalar must have no content children;
- every reachable alias must have no content children and a non-nil
  `Alias` target; and
- every reachable node's `Kind` must be one of yaml.v3's five supported
  kinds (an all-zero `Kind` or any other unsupported value is rejected).

Node identity — the `*yaml.Node` pointer itself, not any decoded value —
drives an unseen/visiting/done state map keyed on that pointer, so a node
reached through several parents (a shared anchor aliased more than once, a
scalar reused as several mapping values) is validated exactly once, and
re-encountering a node still in the `visiting` state on the active
depth-first path is rejected as an alias cycle (covering both a self cycle,
where an anchored mapping's own value aliases back to itself, and a mutual
cycle between two anchored mappings). Because pure shape/graph rejections
have no underlying Go error to preserve, every `*LoadError` `validateSourceGraph`
returns carries a nil `Err`; `LoadError.Error()` still renders the full
`"config parse/type error: <path>: <detail>"` wording from `Path` and
`Detail` alone. A self merge (`&a {<<: *a}`) and a mutual merge cycle
between two anchored mappings are both rejected too, but as a byproduct of
the same generic alias-cycle rule above rather than merge-specific code: the
merge operand's alias is still in the `visiting` state by the time any
merge-shape check would run, so the cycle rule fires first. Every reachable
mapping's explicit keys must also be pairwise unique (see the duplicate-key
paragraph below); the memoized 64-consecutive-alias-hop bound and the
memoized 128-source-node-path-visit bound, both described below, now run
as a second pass once these checks prove the graph acyclic.
Runtime loading reaches `validateSourceGraph` through
`parseAndValidate` → `resolveEffective`.

**Merge-operand shape validation (`internal/config/sourcegraph.go`).**
Layered onto the traversal above: every mapping entry whose key
`isMergeKey` reports true additionally has its value checked as a merge
operand by `validateMergeOperand`, reproducing `gopkg.in/yaml.v3` v3.0.1
`decode.go`'s `merge`/`failWantMap` rule exactly rather than accepting any
value ordinary YAML content would allow. A merge operand is accepted only
as one of: a `yaml.MappingNode` literal; a `yaml.AliasNode` whose
_immediate_ `Alias` target is a `yaml.MappingNode`; or a `yaml.SequenceNode`
each of whose entries is itself one of the two prior shapes. Everything
else is rejected as `KindParseType` — a scalar operand, an alias to a
sequence or a scalar, a sequence containing a scalar or a nested sequence,
and a sequence containing an alias to a non-mapping all fail
(`isMergeOperandMapping` is the single-alias-hop predicate both the direct
and sequence-entry checks share). The immediate-target rule produces a
deliberate asymmetry: an alias-to-alias-to-mapping chain is perfectly valid
as an _ordinary_ mapping value (the generic traversal above only requires a
non-nil `Alias` target of any kind) but is rejected as a merge operand,
because yaml.v3 itself only unwraps one alias hop before deciding a merge
value is not a mapping. All four of `isMergeKey`'s recognized tag forms —
implicit, `"!"`, `"!!merge"`, and the canonical long tag — trigger this
check identically; a quoted `"<<"` key (tagged `"!!str"`) does not, so it
may hold any otherwise-valid value, including a bare scalar. Merge operands
are validated wherever the traversal reaches them — inside a complex
mapping key's own subtree (complex keys are traversed like any other node
but are not otherwise classified in this slice), and in a mapping entry a
later, still-unimplemented merge-precedence pass would go on to discard in
favor of a later `<<` entry — because this function proves every reachable
node well-formed, not just the nodes an eventual effective-merge result
would keep
(`docs/agents/skills/discarded-merge-branches-still-need-validation.md`).

**Duplicate explicit-key detection (`internal/config/sourcegraph.go`).**
Every reachable mapping's explicit keys must be pairwise unique under
`sourceKeyID{Kind, Value}` identity — a repeated key rejects the whole
graph as `KindParseType`, naming the duplicated key's value and, when the
parser recorded a positive line for it, that line. The identity
deliberately compares only `Kind` and `Value`, reproducing `gopkg.in/yaml.v3`
v3.0.1 `decode.go`'s own `uniqueKeys` predicate (inside `decoder.mapping`)
exactly rather than the tag-aware key identity
`docs/agents/skills/yaml-scalar-key-identity-needs-tag-not-just-value.md`
requires elsewhere in this package for merge-precedence purposes — that rule
is about a different concern (`effectiveKeyIdentity`, described just below)
and does not apply here, because yaml.v3's own duplicate-key guard never
looks at `Tag` either. Two surprising consequences follow directly: two
`<<` merge-key entries in the same mapping collide as duplicates regardless
of which of `isMergeKey`'s four accepted tag forms each carries (an
implicit `<<` and one explicitly tagged `!!merge` are still "the same key"),
and a bare `1` (yaml.v3 resolves it to `!!int`) collides with an explicitly
quoted `"1"` (`!!str`) even though their `Tag`s differ, because neither
`Kind` nor `Value` distinguishes them. Detection is per-mapping, not
global — the same key value recurring in a sibling mapping, or in a nested
mapping reachable through it, is never a duplicate — and it runs on every
reachable mapping regardless of how it is reached: directly, as a merge
operand's value, through an alias target, or inside a complex mapping key's
own subtree. The implementation is one `map[sourceKeyID]struct{}` per
mapping, sized from `len(n.Content)/2`, filled by a single linear pass over
key indices — deliberately not yaml.v3's own nested-loop `uniqueKeys`
comparison, which is pairwise (`O(k^2)` per mapping). This package's version
is `O(k)` per mapping and `O(V+E)` over the whole graph, so a mapping with
tens of thousands of keys does not make the validator's own running time
blow up the way the pairwise loop would.

**Effective (merge-precedence) key identity
(`internal/config/effectivekeys.go`).** A separate, standalone helper,
`effectiveKeyIdentity`, computes a comparable `effectiveKeyID` for a mapping
key node — the identity the merge-precedence resolver `effectiveEntries`
(`internal/config/effectivemerge.go`, described below) uses to decide
whether two keys from different merge sources are "the same key" and must
therefore resolve to one effective value rather than two. It is
deliberately a different type and a different comparison from
`sourceKeyID` above: `effectiveKeyIdentity` first follows `n`'s
`yaml.AliasNode` chain (any number of hops, guarded by a local
node-pointer seen-set so a synthetic self-referential alias handed to it
directly terminates instead of hanging) to its non-alias target, then
branches on that target's `Kind`. A `yaml.ScalarNode` target yields `{kind:
yaml.ScalarNode, tag: target.ShortTag(), value: target.Value}` —
`ShortTag()` is used rather than the raw `Tag` field because
`gopkg.in/yaml.v3` v3.0.1's `yaml.go` implements it to resolve an unset or
`"!"` tag from the node's own value (`resolve("", n.Value)` for scalars),
so this rule is exactly the tag-aware identity
`docs/agents/skills/yaml-scalar-key-identity-needs-tag-not-just-value.md`
requires: a bare `1` and an explicitly `!!int`-tagged `1` are the same
key, but a bare `1` and an explicitly `!!str`-tagged (quoted) `"1"` are not,
and `01` and `1` are not (different `Value`). A `yaml.MappingNode` or
`yaml.SequenceNode` target instead yields `{complex: target}` — pointer
identity on the target node alone, with no structural expansion of the
complex key's contents: two distinct nodes that look alike are different
keys, two aliases sharing one complex target are the same key, and a
complex identity is never equal to any scalar identity (the zero
`*yaml.Node` complex field only appears alongside the zero scalar fields
for a nil or dead-ended-alias input, and a real complex node's pointer is
never nil). This is the tag-blind `sourceKeyID` above's direct
counterpart, not a reuse of it: `sourceKeyID` intentionally omits `Tag` to
reproduce yaml.v3's own duplicate-key guard exactly, while
`effectiveKeyIdentity` intentionally includes it because merge-precedence
resolution must not let a `!!str "1"` entry silently discard or be
discarded by an `!!int 1` entry from another merge source. A complex key's
identity is used only to decide _precedence_ — which candidate wins, and
whether two explicit complex keys collide (see below) — never to compute
it: a _winning_ complex key is still fully resolved and emitted
structurally, alias-free, by `emitEffectiveNode` like any other node
(charged against `maxEffectiveOutputNodes` like everything else it
emits), while a _losing_ complex key (impossible for two candidates to
tie on, since distinct complex-key nodes always have distinct pointers,
but reachable when the complex key belongs to a losing merge candidate's
value) is never dereferenced, resolved, or emitted at all.

**Validate-first, alias-resolving effective emitter
(`internal/config/effective.go`).** `resolveEffective(path string, doc
*yaml.Node) (*yaml.Node, *LoadError)` is the slice's entry point toward an
"effective" (post-merge) configuration tree, and imports only
`gopkg.in/yaml.v3` plus stdlib. It always calls `validateSourceGraph(path,
doc)` first and returns that error unchanged on failure — Detail and Path
byte-identical to what `validateSourceGraph` itself would return for the
same input — so a caller cannot use `resolveEffective` to bypass
source-graph validation, and `doc == nil` returns `(nil, nil)` only _after_
that successful validation, matching `parseYAMLDocument`'s valid
empty-input result.

On success `resolveEffective` emits a fresh copy of `doc` via the
unexported `emitEffectiveNode`: the result is rooted at a `yaml.DocumentNode`
copy with exactly one resolved child, and every emitted node — document,
mappings, sequences, keys, and values — copies exactly `Kind`, `Style`,
`Tag`, `Value`, `Line`, `Column`, `HeadComment`, `LineComment`, and
`FootComment` from the source node it corresponds to, via the unexported
`emitNodeMetadata`, leaving `Anchor` as `""` and `Alias` as `nil` on every
copy and allocating a distinct `*yaml.Node` each time (mutating the result
never mutates `doc`). A `yaml.AliasNode` is never copied as itself: the
unexported `dereferenceAliasTarget` follows its (possibly multi-hop) `Alias`
chain to the non-alias target first, and the emitted node carries that
_target's_ metadata, not the alias node's own — this applies identically
whether the alias appears in mapping value position or as a mapping key.
Because the output must stay alias-free, a single source node reached
through more than one alias becomes more than one independent fresh copy in
the result, and each such copy is charged separately against the
`maxEffectiveOutputNodes` budget described just below.

For a mapping node specifically, `emitEffectiveNode` does not copy the
target's raw `Content` — it emits `effectiveEntries(target)`'s winning
entries instead (`internal/config/effectivemerge.go`, described in the next
section), so only a winning key and its winning value are ever resolved and
charged against the budget; a losing merge candidate's value is never
cloned. A recognized `<<` merge key is therefore never itself emitted as a
key, and the result contains no recognized merge directive and no
`yaml.AliasNode` anywhere, at any depth — including inside a retained
mapping key's own subtree or a retained value's own nested merge.

`resolveEffective`'s emit path is bounded by the unexported `const
maxEffectiveOutputNodes = 100000`: every retained document, mapping,
sequence, key and value node counts exactly once toward it, mirroring
`sourcebounds.go`'s `pathVisitCount` accounting rule that "key", "value" and
"alias target" only name which child is traversed next and add no separate
charge, and each independent copy an expanded alias produces is charged
separately (an alias node itself is never charged, since it is dereferenced
to its target before the budget check runs). The check happens in
`emitEffectiveNode`'s first statements for each node — incrementing and
comparing a per-call counter _before_ allocating a `*yaml.Node` for that
node or recursing into its `Content` — so the 100,001st attempted emission
fails before its own allocation and before any of its descendants are ever
visited, which is what makes an exponential alias expansion (a compact,
small source graph whose alias-free output would otherwise need on the
order of `2^n` nodes) abort at the boundary rather than run to completion.
Once the budget is exceeded on any call, every other pending call in the
same `resolveEffective` invocation also returns immediately without further
allocation or recursion. Exactly `maxEffectiveOutputNodes` emitted nodes
succeeds; `maxEffectiveOutputNodes`+1 fails. On overflow `resolveEffective`
returns a nil tree and a `KindParseType` `*LoadError` with `Path` copied
from the `path` argument, a nil `Err`, and a `Detail` naming the
100,000-node bound and a source line for the node whose emission would have
crossed it. That line comes from reusing `sourcebounds.go`'s existing
`collectSourceInventory` and `attributeSourceLines` pair on the validated
source document — the same all-paths line attribution
(`checkSourceGraphBounds` uses it too) rather than a second, separate line-
attribution implementation — computed once when constructing the resolver
error: a node's own line when positive, otherwise its nearest positive-line
ancestor over all root-reachable paths, otherwise `1` for a wholly synthetic
graph with no line metadata anywhere, so the reported line is always positive.

**Merge precedence, memoized candidate inventory, merge-free output
(`internal/config/effectivemerge.go`).** `effectiveEntries(m *yaml.Node)
[]effectiveEntry` computes a mapping node's complete, ordered, deduplicated
inventory of winning entries — the real, yaml.v3-compatible merge-precedence
result `emitEffectiveNode`'s mapping branch emits from, replacing the
earlier chunk's interim "retain the `<<` key as an ordinary entry" behavior.
An `effectiveEntry` is an `{id effectiveKeyID, key, value *yaml.Node}`
triple: `id` is the candidate's `effectiveKeyIdentity` (`effectivekeys.go`),
used to decide whether two candidates from different sources are "the same
key"; `key` and `value` are the source nodes for that candidate, not yet
copied.

The inventory is built in two passes, then deduplicated once: first, every
retained explicit entry, in `m`'s source `Content` order, skipping any
recognized merge directive (`isMergeKey`); second, every inherited
candidate, discovered by merge directives in source `Content` order, each
directive's sequence operands left to right (`mergeOperandMappings`
unwraps a direct mapping, or an alias's _immediate_ target, matching
`validateMergeOperand`'s already-accepted shapes), and each operand
mapping's own `effectiveEntries` computed recursively — so a merge nested
inside a merge operand is fully flattened before its candidates are
considered here. The combined candidate list is then deduplicated by
`effectiveKeyIdentity`, keeping the _first_ candidate for each identity.
Because explicit entries are always listed first, this yields
explicit-over-merged precedence; because merge candidates are discovered in
directive-then-sequence-then-recursive order, it yields
earlier-sequence-operand-over-later precedence; and because the explicit
pass runs unconditionally before the merge pass regardless of where the
`<<` directive appears in `m`'s `Content`, an explicit key suppresses a
matching inherited candidate whether the directive comes before or after
that explicit key in the source. This matches `gopkg.in/yaml.v3` v3.0.1
`decode.go`'s `decoder.mapping`/`decoder.merge` behavior exactly:
`mapping` decodes explicit entries first and skips `isMerge` keys, then
`merge` seeds its `mergedFields` set from every explicit parent key before
consuming operands in `Content` order and marking each merged key as
consumed — with nested merges inheriting that accumulated set rather than
resetting it, which is why `effectiveEntries`' own recursive calls flatten
rather than re-seed.

The emitted mapping's key order is therefore fully deterministic: retained
explicit entries in source `Content` order, then inherited winners in
merge-candidate discovery order (directives in `Content` order, sequence
operands first to last, each operand's own effective entry order) —
running the same fixture through `resolveEffective` twice yields identical
order both times.

`effectiveEntries` delegates to the unexported `effectiveEntriesWithMemo`,
threading an `effectiveMergeMemo` (`map[*yaml.Node][]effectiveEntry`) keyed
by mapping-node pointer identity through the recursion described above. The
memo lives on `effectiveEmitState` for one complete `resolveEffective` call:
a mapping node reached as a merge operand through more than one emitted
parent, or through more than one alias to the same anchor, has its candidate
inventory computed once and reused rather than recomputed once per parent —
`TestResolveEffectiveSharedOperandInventoriedOnce`'s 40-level doubling merge
chain (mirroring `buildSourceSharingDAG`'s construction, but merging via
`<<: [*prev, *prev]` at each level instead of plain aliasing) would need on
the order of `2^40` recursive computations without this memo and completes
in well under a bounded timeout with it.
`TestResolveEffectiveSharedOperandMemoSpansEmittedMappings` separately pins
the memo lifetime: thousands of distinct retained parent mappings merge one
wide shared operand, which would rebuild that inventory for every parent if
the memo were scoped only to a top-level `effectiveEntries` call. A losing
candidate's value is never resolved by this pass either — `effectiveEntries`
only ever returns the source nodes for winning entries, so
`emitEffectiveNode` never clones, and never charges against
`maxEffectiveOutputNodes`, anything a merge discarded.

The post-alias key-identity-collision rule is layered on top of, not a
replacement for, `checkDuplicateMappingKeys`'s tag-blind `Kind`+`Value`
duplicate-key validation above, which `validateSourceGraph` still runs
first, unchanged: `effectiveEntriesWithMemo` additionally scans each
mapping's own retained explicit entries (the same entries its first pass
collects) for two whose `effectiveKeyIdentity` compare equal _after_ alias
dereferencing — catching, for example, a literal scalar key and an alias
key targeting an anchored scalar with the same resolved tag and value,
which `checkDuplicateMappingKeys` cannot catch because the alias node's
own `Kind` and `Value` differ from its target's. Finding such a pair sets
`effectiveEmitState.collided` and records the _later_ key (in source
`Content` order) as `collisionKey`, aborting the whole `resolveEffective`
call — exactly like a node-budget overflow aborts it — without
inventorying or emitting anything further; `resolveEffective` then returns
`effectiveIdentityCollisionError(path, doc, st.collisionKey)`, a
`KindParseType` `*LoadError` with a nil `Err` and a `Detail` reusing the
same `collectSourceInventory`/`attributeSourceLines` line-attribution pair
(own line, else nearest positive-line ancestor over all paths, else `1`)
as the output-limit error above. The rule applies only to two _explicit_
keys of one source mapping: an inherited merge candidate colliding with an
explicit key of the same identity is ordinary suppression (the existing
first-candidate dedup pass), not this error, since only an explicit
key can ever be the "other side" of a genuine ambiguity about which of two
literally-written keys in the same mapping was meant.

`emitEffectiveNode`, `dereferenceAliasTarget`/`emitNodeMetadata`, the
`effectiveEmitState` counter/collision-tracking type,
`effectiveOutputLimitError`, `effectiveIdentityCollisionError`,
`effectiveEntry`, `effectiveMergeMemo`, `effectiveEntriesWithMemo`,
`explicitKeyCollision`, and `mergeOperandMappings`/`mergeOperandMapping`
are all unexported helpers reachable only from `resolveEffective`; per the
frozen `directCallAllowlist` in `sourcesurface_test.go`, no `_test.go` file
names any of them directly — `effective_test.go`, `effectivebound_test.go`,
`effectivemerge_test.go`, and `effectiveidentity_test.go` exercise them
only through `resolveEffective` itself, alongside `resolveEffective` and
`effectiveKeyIdentity`, the two additions that allowlist authorizes.
Runtime loading reaches `resolveEffective` only through
`parseAndValidate`; its lower-level helpers remain encapsulated behind that
validator entry point.

**Reachable inventory, all-paths line attribution, and the 64
consecutive-alias-hop and 128-source-node-path-visit bounds
(`internal/config/sourcebounds.go`).** Once
`walkSourceNode`'s traversal and every check above have proven a source
graph acyclic and well-formed, `validateSourceGraph`'s single call to
`checkSourceGraphBounds` runs a second, purely additive pass — it never
runs on a graph that failed an earlier check, so a cyclic or otherwise
malformed graph still surfaces its original error (an alias cycle, a
malformed merge operand, a duplicate key) rather than a bounds error.
`collectSourceInventory` walks the already-proven-acyclic graph exactly
once per unique node, in `Content` slice order, recording every reachable
node in discovery order, every parent→child content edge reachable in the
graph — including a second edge into a node reached through more than one
parent, not just the edge the walk happened to follow first — and each
node's first-encounter active nearest positive-line ancestor.

`attributeSourceLines` gives every reachable node a source line to report
in a bounds-limit error: a node's own `Line` when positive; otherwise the
line of a positive-line ancestor at the minimum number of content edges
from it over _all_ root-reachable paths, not merely the path the traversal
happened to discover it by first; otherwise `1`, the deterministic
fallback for a wholly synthetic graph with no line metadata anywhere. This
is a multi-source breadth-first search in the parent→child direction,
seeded with every reachable positive-line node at distance zero attributing
its own line and relaxing each edge `parent→child` with
`(dist(parent)+1, line(parent))`; because it walks `collectSourceInventory`'s
already-deduplicated node and edge lists, it visits each at most once and
is `O(V+E)`, so it never re-expands a shared subgraph once per path — the
same discipline that keeps a compact alias-sharing DAG linear rather than
exponential. When two positive-line ancestors tie at the same minimum
distance to a node, the node's first-encounter active ancestor wins if it
is one of the tied candidates; otherwise the candidate reached through the
parent with the lower discovery index wins. An alias edge participates in
this distance exactly like an ordinary content edge, so a node reachable
one alias hop below a positive-line ancestor can out-distance — and so
out-rank — a farther positive-line ancestor reached only through ordinary
content.

The alias-hop bound itself is a memoized dynamic program over node
identity: `hops(n) = 1 + hops(n.Alias)` for a `yaml.AliasNode`, and `0` for
every other kind, so descending from any non-alias node into ordinary
content always resets the count for that child path — which is why more
than 64 independent, shallow sibling aliases to the same anchored target
all succeed even though there are far more than 64 of them in the graph.
`aliasHopCount` memoizes by node pointer, so a node reachable through many
parents is computed once rather than once per path. `checkAliasHopLimit`
rejects the first node (in discovery order) whose `aliasHopCount` exceeds
64 as `KindParseType`, naming the 64-hop limit and reporting
`attributeSourceLines`' line for that node; exactly 64 consecutive hops
succeeds and 65 fails.

The 128-source-node-path-visit bound is a second, separate memoized
dynamic program over node identity: `pathVisitCount(n) = 1 +
max(pathVisitCount(child))` over every content edge reachable directly
from `n` — a document's, mapping's, or sequence's `Content` entries, or an
alias node's single `Alias` target — and `1` for a node with no such
children (an ordinary scalar). Every encountered node, including a
mapping's own key nodes, contributes exactly one visit; "key", "value",
and "alias target" only name which child is traversed next and add no
separate charge, so a scalar used as a mapping key counts once, not once
for being a scalar plus once for being a key, and an alias node plus its
target contribute two visits together, not one. `pathVisitCount` memoizes
by node pointer exactly like `aliasHopCount`, so wide siblings that all
share one deep target are each checked once and do not accumulate a
global count, and a compact alias-sharing DAG is evaluated in time linear
in its node and edge count rather than once per exponentially-many
root-to-leaf paths.

Because `pathVisitCount` is monotonically non-decreasing from any node
toward the root, once one node's count exceeds 128 every one of its
ancestors up to the graph's root does too; `checkSourcePathVisitLimit`
therefore does not report the first over-limit node found in discovery
order (which would almost always be the root itself, carrying no useful
attribution) but instead walks `inv.nodes` in discovery order for the
_boundary_ node — one whose own count exceeds 128 but whose every direct
child does not, i.e. the exact point along an offending path where the
count first crosses the limit — and reports `attributeSourceLines`' line
for that node, naming the 128-visit limit; this stays deterministic even
when more than one branch independently crosses the boundary, since
discovery order breaks the tie. Exactly 128 source-node visits on a
root-to-leaf path succeeds and 129 fails. When a graph violates both
bounds, `checkSourceGraphBounds` checks the alias-hop bound first, so the
alias-hop error is reported and the path-visit pass never runs. Every
error either pass returns has a nil `Err`, like every other
`validateSourceGraph` rejection.

**Canonical page/group inventory (`internal/config/schema.go`).** `SchemaPages()`
and `SchemaGroups(page)` derive the authoritative list of page names and,
per page, group names by reflection — never by a second hand-maintained
list — so the inventory cannot drift from the types it describes. Page names
come from the `yaml` struct tags on `Config`'s exported fields (via the
unexported `yamlFieldNames` helper, in struct declaration order); group names
for a page come from that page's map key set in `defaultConfig()` (via the
unexported `schemaPageGroups` helper, sorted lexicographically for a stable
API since map iteration order is not deterministic). `rawConfig` is _not_ a
schema authority here — it is `Config`'s pointer-typed YAML-decoding mirror
(see above), and a dedicated reflection test
(`TestRawConfigMatchesConfigFields`) proves its exported fields, field order,
and yaml tag names match `Config`'s exactly, so it stays a provably-in-sync
mirror rather than a second source of truth per
`docs/agents/skills/derive-schema-from-canonical-struct-not-shadow-representation.md`.
Both exported functions return a freshly allocated slice (and `SchemaGroups`
an error for a page name outside `SchemaPages()`) on every call, so a caller
mutating a returned slice cannot affect a later call. An empty or duplicate
page/group name is reported as a returned `error`, never a panic, `log.Fatal`,
or `os.Exit`, so failures stay deterministic and assertable from tests.

`SchemaGroupFields()` and `SchemaActionFields()` extend the same inventory to
field names, reusing `yamlFieldNames` — no second tag-parsing rule.
`SchemaGroupFields()` reads `rawGroupConfig`'s yaml tags, in struct
declaration order, because `rawGroupConfig` (not `GroupConfig`) is what
yaml.v3 actually decodes a group into; `GroupConfig` remains the semantic
authority, and a dedicated reflection test
(`TestRawGroupConfigMatchesGroupConfigFields`) holds the two to each other —
same field count, same names and yaml tags per index, and each
`rawGroupConfig` field type equal to `GroupConfig`'s or exactly a pointer to
it (ignoring only that pointer-vs-value difference and `omitempty`) — so
`rawGroupConfig` cannot silently drift from `GroupConfig` per
`docs/agents/skills/derive-schema-from-canonical-struct-not-shadow-representation.md`.
`SchemaActionFields()` reads `ActionConfig`'s yaml tags directly, since
`ActionConfig` has no raw/pointer mirror. Both return a freshly allocated
slice on every call and report an empty or duplicate field name as an error,
never a panic, `log.Fatal`, or `os.Exit`. `parseAndValidate` consumes this
inventory in production, so unknown pages, groups, and fields are rejected by
the runtime loader rather than silently ignored.

**The four-stage validator entry point (`internal/config/validate.go`).**
`parseAndValidate(path string, data []byte) (*rawConfig, *LoadError)` is the
unexported entry point that ties together every validation capability
described above into a single call: stage 1 is `parseYAMLDocument`
(source.go); stage 2 is `resolveEffective` (effective.go), which itself runs
`validateSourceGraph` (sourcegraph.go, including its `checkSourceGraphBounds`
bounds pass, sourcebounds.go) before emitting an alias-free, merge-resolved
effective document; stage 3 is this file's own document-level shape check;
and stage 4 decodes an accepted mapping document into a `*rawConfig`. A
stage 1 or stage 2 `*LoadError` is returned unchanged — `parseAndValidate`
can never bypass parser, source-graph, or bounds validation by continuing
past their errors.

The full pipeline, in order, is: `parseAndValidate` calls `parseYAMLDocument`
(stage 1) to parse `data` into a single `*yaml.Node` document, rejecting a
malformed document or a second document; then `resolveEffective` (stage 2),
which validates the reachable source-branch graph via `validateSourceGraph`
(including its node/path/alias-hop bounds) before resolving `<<` merges and
aliases into one alias-free effective tree; then this file's own ChairLift
schema/shape/declared-type validation (stage 3) walks that effective tree
page by page, group by group, group-field by group-field, and (for
`actions`) action-entry by action-field; and finally, only once every prior
stage and every entry at every level has passed, stage 4 decodes the
already-validated effective document into a `*rawConfig` via
`yaml.Node.Decode`. A failure at any earlier stage returns immediately
without running any later stage.

Every reachable outcome of that pipeline classifies to exactly one of three
results — `KindParseType`, `KindSchema`, or a valid decoded/no-op result —
per this decision table:

| Effective input                                                                   | Result                     |
| --------------------------------------------------------------------------------- | -------------------------- |
| malformed YAML, second document, or source duplicate                              | `KindParseType`            |
| cyclic, malformed, source-path-over-budget, or effective-output-over-budget graph | `KindParseType`            |
| empty document or top-level null                                                  | valid no-op overlay        |
| top-level scalar or sequence                                                      | `KindParseType`            |
| unknown top-level page                                                            | `KindSchema`               |
| known page with null                                                              | valid no-op for that page  |
| known page with scalar or sequence                                                | `KindParseType`            |
| unknown group under a known page                                                  | `KindSchema`               |
| known group with null                                                             | valid no-op for that group |
| known group with scalar or sequence                                               | `KindParseType`            |
| unknown group field                                                               | `KindSchema`               |
| `actions` value that is not null or a sequence                                    | `KindParseType`            |
| action entry that is null or otherwise not a mapping                              | `KindParseType`            |
| unknown action field                                                              | `KindSchema`               |
| known field that cannot decode into its declared Go type                          | `KindParseType`            |
| valid effective document                                                          | decoded `*rawConfig`       |

The first four rows are produced by stages 1-2 (`parseYAMLDocument`,
`validateSourceGraph`, `resolveEffective`) and always take precedence: a
parser, graph, source-duplicate, or resolver-bound failure is returned
before stage 3 ever inspects a single mapping entry. Every remaining row is
produced by stage 3's per-level entry walk or stage 4's final `Decode`.

At every one of the four name levels (page, group, group field, action
field), each mapping entry is classified in this fixed order, and later
steps never run once an earlier one has already classified the entry:

1. **Key shape.** A mapping key counts as a name only when its effective
   node is a scalar whose `ShortTag()` is exactly `!!str`. An integer,
   boolean, or null scalar key; a custom-tagged scalar; a sequence or
   mapping key; or an alias to any of those, is rejected as `KindParseType`
   via `validatorKeyShapeError` before its name or value is inspected at
   all. `schemaKeyName` implements this rule; the quoted string key `"<<"`
   resolves to tag `!!str` and is therefore an ordinary name (rejected as
   `KindSchema` if absent from the canonical inventory, like any other
   unrecognized name) — it is only the bare, unquoted `<<` merge key that
   carries yaml.v3's own `!!merge` tag, and that key is consumed by
   `resolveEffective` during stage 2, long before `schemaKeyName` ever runs,
   so it never reaches this classification at all.
2. **Name membership**, checked without descending into that entry's value.
   A well-formed string key that is not present in the canonical inventory
   for that level is rejected as `KindSchema`, naming the literal offending
   key and a positive line — regardless of whether the associated value is
   null, a scalar, a sequence, or a mapping. An unknown name's value is
   never inspected, decoded, or recursed into.
3. **Value inspection**, only for a name that passed step 2. A known page's
   or group's non-null scalar/sequence value is `KindParseType`; a null
   value is a no-op; a mapping value recurses one schema level down. A known
   group field's or action field's value is decoded into a fresh instance of
   that field's declared Go type, with any `*yaml.TypeError` (or other
   decode failure) preserved in `Err` and classified `KindParseType`.

Page, group, group-field, and action-field name inventories are never a
hand-maintained list in `validate.go`. `SchemaPages()`, `SchemaGroupFields()`,
and `SchemaActionFields()` (schema.go) reflect directly on the canonical
`Config`, `rawGroupConfig`, and `ActionConfig` structs, so adding, renaming,
or removing a field on one of those structs changes the accepted schema
automatically, with no parallel edit required in the validator. `SchemaGroups(page)`
is derived differently: it reads the group names present as map keys in that
page's entry in `defaultConfig()`, not struct fields of `Config` — a group
added only to `defaultConfig()`'s map (with no corresponding struct field)
still changes the accepted schema, and a parity test keeps that map's keys
from drifting out of sync with the fields the validator otherwise expects.

**Interpretation I8: runtime validation precedes `mergePage`'s tolerant
mechanics.** `mergePage` (config.go) mechanically
tolerates a group name absent from `defaultConfig()` for a page: it
synthesizes a zero `GroupConfig{Enabled: true}` as the merge base and
proceeds, matching `IsGroupEnabled`'s "missing group -> enabled" fallback.
`parseAndValidate`'s `validateGroupEntries`, by contrast, rejects that same
unknown group name outright as `KindSchema`. Runtime loading always calls
`parseAndValidate` before `mergeConfig`, so unrecognized groups can never
reach `mergePage`; its tolerance remains useful only to package-internal
callers operating on already trusted `rawConfig` values.

Once both prior stages succeed, stage 3 classifies the effective document by
its top-level node shape into exactly one of these outcomes:

- **Nil effective document** (the shared "empty or whitespace-only input"
  result `parseYAMLDocument`/`resolveEffective` already return for that
  case) — a non-nil, zero-valued `*rawConfig` (every page map nil/empty) and
  no error. This is a usable no-op overlay, matching the "absent config"
  semantics elsewhere in this package. Stage 4's `Decode` call never runs
  for this outcome.
- **Top-level null scalar** (YAML's `null`/`~`, or an absent value, which
  yaml.v3 resolves to tag `!!null`) — treated identically to the nil-document
  outcome above: a non-nil, zero-valued `*rawConfig`, no error, and no
  `Decode` call.
- **Top-level scalar that is not null, or a top-level sequence** — rejected
  as `KindParseType` via the unexported `validatorShapeError`, which names
  the actual shape found and a positive source line from the unexported
  `effectiveNodeLine` (`n.Line`, clamped to `1` when yaml.v3 left it
  non-positive, mirroring `effectiveOutputLimitError`'s own clamp). No
  `Decode` call runs for this outcome either, since there is no mapping to
  decode.
- **Top-level mapping** — accepted structurally, and its entries are
  classified one at a time, in effective `Content` order, by
  `validatePageEntries` (interpretation I3's per-entry order: key shape,
  then name membership, then value shape):
  - A mapping key whose effective node is not a scalar with `ShortTag() ==
"!!str"` — an integer, boolean, or null scalar; a custom-tagged scalar;
    a sequence; a mapping; or an alias to any of those (already
    dereferenced into a copy of its target by `resolveEffective`) — is
    rejected as `KindParseType` via the unexported `validatorKeyShapeError`
    before its name or value is ever inspected. The unexported
    `schemaKeyName(key *yaml.Node) (string, bool)` implements this rule; a
    quoted `"<<"` key resolves to tag `!!str` (an ordinary name), while a
    bare `<<` merge key carries `!!merge` and is consumed by
    `resolveEffective` long before this walk runs, so it never reaches
    `schemaKeyName` at all.
  - A well-formed string key that is not one of `SchemaPages()`'s canonical
    page names (schema.go, reflected off `Config`'s yaml tags — never a
    literal list in validate.go) is rejected as `KindSchema` via the
    unexported `validatorSchemaError`, naming the literal offending key and
    a positive line, without descending into that entry's value at all.
    (`SchemaPages()` returning an error — only possible for a malformed
    struct tag on `Config` itself, which cannot happen for that canonical
    struct — is still surfaced defensively as a `KindSchema` `*LoadError`
    via the unexported `validatorSchemaPagesError`, wrapping the reflect
    error, rather than ignored or panicked on.)
  - A known page's **null** value is accepted as a no-op for that page.
  - A known page's **non-null scalar or sequence** value is rejected as
    `KindParseType` via the unexported `validatorPageValueShapeError`,
    naming the page, the actual shape found, and a positive line.
  - A known page's **mapping** value has its own entries — groups — classified
    the same way, one schema level down, by `validateGroupEntries`:
    - A non-`!!str` mapping key is rejected as `KindParseType` via
      `validatorKeyShapeError`, exactly as at page level.
    - A well-formed string key that is not one of `SchemaGroups(page)`'s
      canonical group names for that page (schema.go, reflected off
      `defaultConfig()` — never a literal list in validate.go) is rejected as
      `KindSchema` via `validatorSchemaError`, naming the offending key and a
      positive line, without descending into its value. (`SchemaGroups`
      erroring — only possible for a page unknown to it, which cannot happen
      once `page` has passed `SchemaPages()` — is still surfaced defensively
      as `KindSchema` via the unexported `validatorSchemaGroupsError`.)
    - A known group's **null** value is a no-op for that group; its
      **non-null scalar or sequence** value is `KindParseType` via the
      unexported `validatorGroupValueShapeError`, naming the group, shape,
      and line.
    - A known group's **mapping** value has its own entries — group fields —
      classified by `validateGroupFieldEntries`, sourced from the unexported
      `groupFieldTypes()` (reflection over `rawGroupConfig`'s yaml tags and
      declared Go field types — never a literal list in validate.go, aside
      from the literal name `"actions"` itself, needed for the special case
      below):
      - A non-`!!str` mapping key is rejected as `KindParseType`, exactly as
        at page and group level; an unrecognized field name is rejected as
        `KindSchema`, exactly as an unrecognized group name is.
      - The special-cased `actions` field is recognized as a known name and
        validated structurally by the unexported `validateActionsEntries`,
        instead of by a generic decode into `*[]ActionConfig` (interpretation
        I5, because yaml.v3 silently ignores an unknown struct field on
        decode and silently decodes a null sequence entry into a zero
        `ActionConfig`):
        - Its value must be **null** (a no-op — no actions configured) or a
          **sequence**; any other shape (a non-null scalar or a mapping) is
          rejected as `KindParseType` via the unexported
          `validatorActionsValueShapeError`, naming the actual shape found
          and a positive line.
        - Every sequence entry must be a YAML **mapping**; a **null** entry
          is explicitly not accepted as a zero action, and a scalar or
          sequence entry is likewise rejected — all as `KindParseType` via
          the unexported `validatorActionEntryShapeError`.
        - Each mapping entry's own fields are classified by the unexported
          `validateActionFieldEntries`, one schema level down from a group's
          fields and sourced from the unexported `actionFieldTypes()`
          (reflection over `ActionConfig`'s yaml tags and declared Go field
          types — never a literal list in validate.go): a non-`!!str`
          mapping key is `KindParseType`, exactly as at page/group/group-field
          level; an unrecognized action-field name (e.g. `SchemaActionFields()`
          not listing it) is `KindSchema` via `validatorSchemaError`, naming
          the offending key and a positive line, without descending into its
          value; and a known field's effective value node is decoded into a
          fresh value of its declared Go type
          (`reflect.New(fieldType).Interface()`), a decode failure
          (yaml.v3's own `*yaml.TypeError`, e.g. `sudo: {a: 1}` into `bool`)
          classified `KindParseType` via `validatorDecodeError`, with the
          yaml.v3 error's message in `Detail` and the error itself preserved
          in `Err`.
      - Every other known field's effective value node is decoded into a
        fresh value of that field's declared Go type,
        `reflect.New(fieldType).Interface()` (interpretation I4) — e.g.
        `enabled`'s `*bool`, `bundles_paths`'s `*[]string`. A decode failure
        (yaml.v3's own `*yaml.TypeError`, e.g. `enabled: [1, 2]` into
        `*bool`) is classified `KindParseType` via the unexported
        `validatorDecodeError`, with the yaml.v3 error's message in `Detail`
        and the error itself preserved in `Err` — the same builder stage 4's
        final `Decode` call uses.

  Once every entry passes, stage 4 calls the effective document's
  `yaml.Node.Decode` into a `*rawConfig`. A `Decode` failure here is
  classified `KindParseType` via the unexported `validatorDecodeError`,
  with the yaml.v3 error's message in `Detail` and the error itself
  preserved in `Err` — a defensive final step, since
  `validateSourceGraph`/`resolveEffective` and the page-entry walk above
  already prove the effective document is well-formed by the time `Decode`
  runs, but any residual decode failure is still classified rather than
  left unhandled. Once every level's fields pass, an explicitly set zero
  value survives the decode as set, not as omission: `enabled: false`,
  `app_id: ""`, and `bundles_paths: []` all decode to non-nil pointers
  (`*bool`, `*string`, `*[]string`) carrying `false`, `""`, and an empty
  non-nil slice respectively — `rawGroupConfig`'s pointer fields exist
  precisely so a merge onto `defaultConfig()` can tell "explicitly set to
  the zero value" apart from "key absent."

Per the guarded `directCallAllowlist` in `sourcesurface_test.go`,
`parseAndValidate` and `resolveCandidatePath` are the runtime entry helpers
authorized for direct test calls;
`validatorShapeError`, `validatorDecodeError`, `effectiveNodeLine`,
`validatePageEntries`, `schemaKeyName`, `validatorSchemaError`,
`validatorSchemaPagesError`, `validatorKeyShapeError`,
`validatorPageValueShapeError`, `describeNodeShape`,
`validateGroupEntries`, `validatorSchemaGroupsError`,
`validatorGroupValueShapeError`, `validateGroupFieldEntries`,
`groupFieldTypes`, `validatorGroupFieldTypesError`,
`validateActionsEntries`, `validatorActionsValueShapeError`,
`validatorActionEntryShapeError`, `validateActionFieldEntries`,
`actionFieldTypes`, and `validatorActionFieldTypesError` are all exercised
only indirectly, through `parseAndValidate`, in `validate_test.go`.

**Runtime loading, precedence, and diagnostics
(`internal/config/config.go`, `paths.go`, `diagnostic.go`).** `Load` resolves
each configured candidate to the exact path it will read, then calls
`loadResolvedPath`, which reads bytes, runs `parseAndValidate`, and only then
merges the validated overlay onto `defaultConfig()`. A missing candidate
(`errors.Is(err, fs.ErrNotExist)`) advances to the next search location
unless `danglingSymlink(path)` reports that the directory entry is a symlink
whose target is missing: `os.ReadFile` reports ENOENT for such a link exactly
as for an absent path, so `Load` uses `os.Lstat` to tell them apart and treats
the dangling link as a present, unreadable authoritative file (`KindRead`).
Otherwise an administrator's broken `/etc/chairlift/config.yml` link would
silently fall back to package defaults. Any
other read failure, or any parse/type/schema failure in the first file that
exists, makes that file authoritative: lower-priority files are not read,
`Load` returns `disabledConfig()` plus the structured `*LoadError`, and every
canonical feature group is hidden while non-visibility defaults remain
available. If all candidates are absent, the ordinary built-in defaults and
a nil error are returned.

`resolveCandidatePath` makes diagnostic paths deterministic and absolute when
the operating system supplies a working directory: absolute candidates are
cleaned; a relative candidate prefers a file beside the executable and
otherwise resolves against the current working directory. The
executable-relative probe uses `os.Lstat`, so a dangling symlink beside the
executable stays the selected candidate and fails closed in `Load` instead of
falling through to the working directory. Filesystem reads,
the executable path, and the working directory have narrow package-level
seams so precedence and permission failures are testable without relying on
host permissions.

On an authoritative failure, `LoadError.LogMessage` prefixes the full
path-and-cause diagnostic with `CONFIGURATION ERROR`, states that all groups
were disabled, and instructs the user to restart after fixing the file.
`window.New` retains the same `LoadError`, builds the toast overlay, then calls
`ShowErrorToast` with `LoadError.ToastMessage`. `ShowErrorToast` sets timeout
zero, so the startup error remains visible instead of expiring. Both
`ShowToast` and `ShowErrorToast` disable Pango markup, because AdwToast parses
markup by default and a path or tool error containing `<`, `>` or `&` would
otherwise render as a blank toast. `internal/installcheck`'s
`TestToastsRenderPlainText` parses every production Go file and fails unless
each `adw.NewToast` result is assigned to a variable and the next statement
in the same block is `<name>.SetUseMarkup(false)`. Any other `SetUseMarkup`
reference in production code (on any receiver, deferred, in a goroutine, as a
method value or through a helper) also fails it, so a widget that needs markup
must first change that test. It also fails when it finds no toast constructor
at all; construction
and the toast call both occur on the GTK main thread.

### Package manager wrapper pattern

Each wrapper in `internal/` follows a consistent shape:

- Module-level `dryRun` flag with `SetDryRun()`/`IsDryRun()`
- `IsInstalled()` to check tool availability, plus `IsInstalledCached()` (`sync.Once`) for use from views during async startup
- Homebrew, Flatpak, fwupd, and Updex implement both `IsInstalled()` and `IsInstalledCached()`
- List/Search/Install/Uninstall/Update functions
- Context-based timeouts. Homebrew and Flatpak both use a two-class model selected per invocation by an unexported `commandTimeout(args)` helper: 30s for read-only commands, 30m for state-changing ones (the keys of each package's `stateChangingCommands` map). fwupd selects its timeout per call instead, because it has exactly one read command and one state-changing command: 60s for the update query and 30m for an install. updex uses 5min and bootc 30min.
- Custom error types where needed

### Shared OS staging progress (`internal/stageexec`)

`bootc.StageUpdate` and `sysupdate.StageUpdate` retain their provider APIs and
fixed commands — `pkexec /usr/libexec/bootc-update-stage` and `pkexec
/usr/libexec/snosi-sysupdate-stage`, respectively — but both delegate execution
to the pure-Go `internal/stageexec` leaf package:

1. The caller creates the provider's `ProgressEvent` channel; both provider
   types are aliases of `stageexec.ProgressEvent`.
2. Each non-empty output line becomes an `EventMessage`; the channel is closed after either an `EventComplete` (success) or the function returning an error
3. Event types: `EventMessage` and `EventComplete` — deliberately simpler than
   a step/percent model because the stage script's own output is unstructured
   log lines, not a structured progress protocol. Failures return as errors;
   they are not duplicated into the event stream.
4. `stageexec.Run` owns merged stdout/stderr, non-zero exits with the last output
   line, deadline/cancellation classification, missing bare-name or absolute-path
   executables, direct-child kill/reap, the single success completion, and
   channel closure. `stageexec.DryRun` owns the synthetic preview/completion and
   closure without constructing an `exec.Cmd`.
5. The bootc and sysupdate view goroutines read the same event contract and
   dispatch UI updates to the main thread via `sgtk.RunOnMainThread`.

**Caller-visible outcomes.** Provider adapters preserve `bootc.Error` /
`sysupdate.Error` and `NotFoundError` while carrying the shared executor's
message and cause. Both context-taking bootc functions classify failures with
`errors.Is` against the context sentinels, and `bootc.Error` has an `Err error`
field plus `Unwrap() error` so callers can tell them apart:

- Either provider's `StageUpdate` — deadline: provider `*Error` "Update staging timed out" unwrapping to `context.DeadlineExceeded`; cancellation: provider `*Error` "Update staging was canceled" unwrapping to `context.Canceled`; non-zero exit: provider `*Error` "update staging failed (exit N): <last output line>" matching neither sentinel; missing `pkexec`: provider `*NotFoundError`.
- `GetStatus` — deadline: `*Error` "bootc status timed out" unwrapping to `context.DeadlineExceeded`; cancellation: `*Error` "bootc status was canceled" unwrapping to `context.Canceled`; non-zero exit: `*Error` "bootc status failed (exit N): <stderr>" matching neither sentinel; missing `bootc`: `*NotFoundError`. `GetStatus(ctx)` is `return getStatusFrom(ctx, bootcCommand)`; the unexported `getStatusFrom` seam exists so tests exercise all of these against a fake script without a real `bootc`.

The deadline and cancellation messages differ in both functions, and neither ever surfaces as `signal: killed`.

**OS staging direct-kills; homebrew and flatpak kill the process group.** On
cancellation `stageexec.Run` kills only the direct child (`cmd.Process.Kill()`)
and sets no `Setpgid`, because both staging adapters run under `pkexec` and the
privileged child cannot be signaled as an unprivileged process group. The
unprivileged Homebrew and Flatpak runners instead kill their whole process
groups so download helpers are not orphaned. Making either privileged staging
path group-killable is a privilege-model change.

**Why a stage script instead of `bootc upgrade`:** upstream `bootc upgrade`'s registry-transport pull currently fails on snow's composefs images. The snow-shipped `/usr/libexec/bootc-update-stage` script works around this: `podman pull` fetches the image into containers-storage (podman's pull path works where bootc's does not), then `bootc switch --transport containers-storage` stages the already-pulled image as the next boot deployment. This keeps snow's actual upgrade logic in one place (the snosi script) rather than duplicating pull/switch orchestration in ChairLift; ChairLift only invokes the script via pkexec and streams its output. The script is idempotent — it exits 0 without staging anything when the deployment is already current.

### bootc progress UI (updates page)

`onBootcStageClicked()` (`internal/views/updates_page.go`) drives the "System Update" expander: it disables the button, spawns `bootc.StageUpdate` in a goroutine, and processes the `ProgressEvent` channel on a second goroutine — `EventMessage` lines are appended to a log expander with timestamps, and `EventComplete` marks the activity complete. A returned `stageErr` drives the error subtitle and toast after the stream closes; there is no duplicate error event. After `wg.Wait()` returns, the handler re-reads live `bootc.GetStatus()` and calls `uh.updateCounts.Set(badgestate.Bootc, 0|1)` plus `uh.updateBadgeCount()` unconditionally in both dry-run and live mode (this is a plain read, not a mutation, so it always reflects reality); it then sets `expander`'s subtitle from that same live read unconditionally as well, but shows `actionmsg.BootcStage(bootc.IsDryRun(), staged)` for the completion toast — an explicit preview string under dry-run rather than one of the "staged"/"up to date" strings that read as a verified completion claim about a click that, under dry-run, checked and changed nothing. The system page has a separate, simpler bootc path: `loadBootcStatus` (gated on `IsBootcBootedCached()`) calls `bootc.GetStatus` to show the booted/staged/rollback deployment images, versions, and digests, with no staging controls of its own — staging happens on the Updates page.

### Firmware updates (updates page)

`internal/fwupd` wraps the unprivileged `fwupdmgr` client of the Linux Vendor
Firmware Service, and `buildUpdatesPage` builds `firmware_updates_group` hidden
and checks availability asynchronously. The group stays hidden when
`fwupd.IsInstalledCached()` is false, so a host without fwupd never sees it. On
a host with fwupd, `loadFirmwareUpdates` runs
`fwupdmgr get-updates --json` on a worker goroutine, shows the group, and hands
the list to `publishFirmwareList`. That function is the single publisher of
firmware rows: it replaces every row, sets the badge to the installable count,
and sets the subtitle from the full list. The load is generation-guarded by
`firmwareRefresh`, so a stale query cannot replace a newer list. A query that
exits 2 ("no updatable devices") is an empty list, not an error; any other
failure shows `pageview.FirmwareUpdatesSubtitle`'s error text, keeps the last
known count, and passes `uh.firmwareRows.Len()` so rows from an earlier load
stay expandable.

The expander subtitle counts every listed update, while the badge stores only
the installable ones (`installableCount`, in `badgestate.Firmware`). A device
that fwupd will not flash now — no `updatable` flag (a closed lid, a host on
battery), a non-empty `Problems` array, or a non-empty `UpdateError` — keeps its
row and its reason but gets no `Install` button, and its subtitle comes from
`pageview.FirmwareBlockedUpdate`, which appends the reason to the normal row
text. Without that rule a zero exit from a skipped device would read as a
successful flash.

One `firmwareInstallGate` (`actionstate.Gate`) on `UserHome` covers the whole
page, so only one firmware install runs at a time: a click that finds the gate
busy shows the toast `Another firmware update is installing` and does nothing
else.

The Install button opens `adw.NewAlertDialog` with
`pageview.FirmwareInstallConfirmation`: the body always warns not to turn off or
unplug the computer and to keep the power connected, then states whether the
device needs a restart, a shutdown, or nothing, and that ChairLift will not
restart the computer itself. The install response is `adw.ResponseDestructiveValue`,
and closing the dialog is the `cancel` response, so the page gate is released.

On confirm the handler calls `fwupd.InstallAndVerify`. `fwupdmgr update` can
exit 0 without flashing a device, and neither a missing update offer nor a
different offered version proves a flash, so the install is followed by
`fwupdmgr get-devices --json` — the device's own report. `fwupd.VerifyInstall`
classifies it, first match wins: a failed state query is `InstallUnverified`
with its error text; a device that is not listed is `InstallUnverified` because
it can be restarting; `UpdateState` of `needs-reboot` or `pending` is
`InstallStaged`; `UpdateState` of `failed` or `failed-transient` is
`InstallNotApplied`, even with a matching version (fail closed); the new
version is `InstallFlashed`; the old version is
`InstallNotApplied`; anything else is `InstallUnverified` and names the version
the device reports. `InstallUnverified` is the zero value of `InstallResult`, so
an unknown outcome cannot read as a success. The `get-updates` list is read
afterwards only to rebuild the rows.

`actionmsg.FirmwareInstall` turns the report into one decision:
`decision.PublishList` publishes `report.Updates` as the new list,
`decision.RestoreControl` restores the Install button, `decision.IsError`
selects the error toast, and `decision.CompletedLabel` records a finished
install (`Installed` or `Restart required`) on the old button when the list
could not be read, which then also shows the list error in the subtitle. The
view removes no row by identity and never decrements the badge: the published
list is the whole list, so a device that still has an update stays visible and
the badge is the new installable count. Taking a fresh `firmwareRefresh`
generation before publishing makes any load still in flight stale. The page gate
resets last, in every case. Every call is unprivileged — fwupd's own PolicyKit
actions authorize the daemon — which is decision record
[ADR-0012](../adr/0012-firmware-updates-use-fwupds-own-polkit.md).

### Update badge tracking

The updates page stores bootc, sysupdate, firmware, Flatpak, and Homebrew
counts in the mutex-backed `badgestate.Counts` value on `UserHome`. The bootc provider is 1
when `bootc.GetStatus()` reports a staged deployment and 0 otherwise — a
boolean folded into the total, not a count of available images. The sysupdate
provider follows the same boolean rule: 1 when `sysupdate.GetStatus().IsStaged()`
(the `/run/snosi/update-staged` semaphore exists, or the last check recorded
`outcome=staged`) and 0 otherwise, including after a failed check — there is
no persistent "available but unstaged" state because the snosi stager checks
and stages in one run. Provider refreshes use
`Set`, so a repeated load replaces rather than accumulates; successful
row-level Homebrew upgrades use `Add(Homebrew, -1)`, which cannot go below
zero. Firmware uses `Set` only and never `Add`: a verified install publishes the
whole checked list, with the badge set to that list's installable count, so
decrementing would double-count the change. The firmware provider counts the
devices fwupd lists as updatable; it is set by a completed firmware query and
never lowered by a failed one.
`updateBadgeCount` reads the aggregate `Total` and pushes it through
`ToastAdder.SetUpdateBadge()`. Refresh requests receive an increasing
generation from `actionstate.RefreshGate`; only the newest request may apply
its result, so an older, slower query cannot overwrite newer metadata. Every
completion callback still runs when superseded so its action is restored. Only
a successful current query replaces rows/count. A command or refresh failure
preserves the last known count (and a refresh failure preserves the current
rows), while dry-run previews restore their controls without changing either.

### Privileged operations

Decision records: [ADR-0001](../adr/0001-fixed-path-pkexec-privilege-boundary.md)
(the fixed-path pkexec boundary and helper argv re-validation),
[ADR-0002](../adr/0002-usr-prefix-is-the-only-supported-install-prefix.md)
(`PREFIX=/usr`), and
[ADR-0006](../adr/0006-split-system-integration-package-with-mutual-conflicts.md)
(the system-integration package split).

bootc staging, native A/B staging, and updex require root for state-changing operations. They invoke commands through `pkexec` (PolicyKit). bootc runs `pkexec /usr/libexec/bootc-update-stage` directly (polkit action id `org.frostyard.ChairLift.bootc.stage`), native A/B staging runs `pkexec /usr/libexec/snosi-sysupdate-stage` directly (`internal/sysupdate.StageScriptPath`, action id `org.frostyard.ChairLift.sysupdate.stage`), and updex delegates to the fixed absolute path `internal/updex.HelperPath` (`/usr/bin/chairlift-updex-helper`) via `pkexec`. Polkit policy files are installed for all three: `data/org.frostyard.ChairLift.bootc.policy`, `data/org.frostyard.ChairLift.sysupdate.policy`, and `data/org.frostyard.ChairLift.updex.policy`. ChairLift deliberately ships no `.rules` files: the policies require normal administrator authentication (`auth_admin`, with `auth_admin_keep` for an active local session) rather than granting blanket passwordless access to a login group. Source installation removes the two legacy ChairLift `.rules` files so an older passwordless rule cannot survive an upgrade. Homebrew tap trust (`brew trust`) is explicitly per-user and does _not_ go through pkexec — see [package-managers.md](./package-managers.md). Firmware installs are the other unprivileged mutation: `internal/fwupd` calls `fwupdmgr update --no-reboot-check <DeviceId>` directly and the fwupd daemon authorizes it with fwupd's own PolicyKit actions, so ChairLift contributes no policy file and no helper to that path ([ADR-0012](../adr/0012-firmware-updates-use-fwupds-own-polkit.md)).

**Why the helper path must be absolute, and why `PREFIX=/usr`:** `pkexec`
resolves the program it's asked to run to an absolute path and compares it
textually against the `org.freedesktop.policykit.exec.path` annotation on
each action in `data/org.frostyard.ChairLift.updex.policy` (all three actions
annotate `/usr/bin/chairlift-updex-helper`). The policy also uses
`org.freedesktop.policykit.exec.argv1` to select the corresponding
`enable-feature`, `disable-feature`, or `update` action from the first helper
argument. PolicyKit does not validate the remainder of argv: the privileged
helper's pure `internal/updexhelper.ParseInvocation` boundary accepts only
`enable-feature <name> [--dry-run]`, `disable-feature <name> [--dry-run]`, and
`update [--dry-run]`, rejecting extra, misplaced, or unknown arguments before
calling updex. A bare, `$PATH`-resolved command name can resolve to a different
absolute path depending on the invoking process's `$PATH`, which makes the
path comparison miss and falls `pkexec` back to the generic, more restrictive
action. `internal/updex/updex.go`'s `runHelper` therefore always invokes
`HelperPath` (never a bare name).

**Helper output and exit-code contract.** On success the helper prints its
result as JSON on stdout and exits 0; for `update` that is a JSON array of
`updex.UpdateFeaturesResult`. On failure it prints the error on stderr and
exits non-zero: `updexhelper.ExitFailure` (1) for any failure, and
`updexhelper.ExitActivationIncomplete` (3) when the error wraps
`sysext.ErrActivationIncomplete` (the extensions merged, but activation did
not finish; a reboot finishes it). pkexec passes the code through. The
`update` command also prints its results array on stdout when it fails,
because updex returns the per-feature results together with the error
(`updexhelper.WriteUpdateResult`); `enable-feature` and `disable-feature`
print nothing on failure, and ChairLift never sets `Now`, so they never
refresh. The GUI and the helper can come from different ChairLift versions
(the system-integration package ships the helper alone), so the contract is
additive: success output is unchanged; an older GUI ignores stdout and
treats every non-zero exit as failure, which stays safe for code 3; a newer
GUI with an older helper sees exit 1 with empty stdout and shows the plain
"Update failed: ..." toast.

The helper keeps stdout for the JSON result alone. The updex SDK runs
`systemd-sysext`, `systemd-sysusers` and `systemd-tmpfiles` with
`cmd.Stdout = os.Stdout`, and their text would come before the JSON. So
`main` keeps the real stdout for the result (all three commands write to it)
and sets `os.Stdout = os.Stderr` before the SDK runs: child-process output goes
to stderr. The GUI still reads defensively, for helpers from older ChairLift
versions: `updex.UpdateFeatures` decodes the whole trimmed stdout as a JSON
array, then falls back to the last non-empty line (the JSON ends with a
newline). When the helper exits 0 and no array can be read (empty stdout, bad
JSON, `null`), the outcome is unknown: the result is nil results with
`*updex.ResultsUnreadableError`, logged, and the Features toast says "Feature
update finished, but ChairLift could not read the results. See the log for
details." as a persistent error toast. It never claims success. When the helper
exits non-zero, the helper error wins (`*ActivationIncompleteError` or
`*Error`) and any results that did parse come with it.

Separately, `polkitd` reads application policies from the fixed directory
`/usr/share/polkit-1/actions` — not `$XDG_DATA_DIRS`, not any
`$PREFIX`-derived path — so the Makefile's `install`/`uninstall` targets
require `PREFIX=/usr` (the default since issue #59) for a source install's
polkit assets to land somewhere polkit actually looks. These constraints are
system facts, not values ChairLift decides; the Makefile and
`internal/updex.HelperPath` exist to conform to them, matching the layout
`.goreleaser.yaml`'s nFPM packages already use.

**System-integration delivery:** GoReleaser publishes two mutually exclusive
package shapes. `frostyard-chairlift` is the existing self-contained package
with both application binaries, desktop assets, maintainer config, and
policies. `frostyard-chairlift-system-integration` is the root-owned companion
for a user-scoped GUI delivery such as the Homebrew cask: its build filter
contains only `chairlift-updex-helper`, and its contents contain all three
policies plus `/usr/share/chairlift/config.yml`. The packages declare conflicts
because they intentionally own the same privileged files.

The integration package does **not** ship `bootc-update-stage` or
`snosi-sysupdate-stage`. Those operations are distro policy, so an image that
enables `bootc_updates_group` must provide a trusted implementation at the
existing fixed `/usr/libexec/bootc-update-stage` path, and native A/B images
ship `/usr/libexec/snosi-sysupdate-stage` (plus the `/usr/lib/snosi/native-ab`
marker) themselves. Keeping the paths fixed preserves the PolicyKit executable
boundary; making either a user-writable config value would allow the GUI
configuration to redirect a root execution. The page already gates each group
on its script-availability check (`bootc.StageScriptAvailable`,
`sysupdate.StageScriptAvailable`), so an absent distro helper hides the
operation.

### Maintenance action execution

Configurable maintenance scripts (from `config.yml` `actions` entries) are executed via `runMaintenanceAction()` in `internal/views/maintenance_page.go`. The pattern:

1. `decision := actionmsg.MaintenanceScript(IsDryRun(), title)` is computed once, before the goroutine, from the views-level dry-run flag (see "Dry-run mode" above)
2. Button is disabled and label set to "Running..."
3. A goroutine checks `decision.Execute`: when true it spawns the script via `exec.CommandContext` (5-minute timeout), using `pkexec` wrapper if `sudo: true`, exactly as before; when false (dry-run) it constructs no `exec.Cmd` at all and just logs `[DRY-RUN] Would execute: ...`
4. On completion, the main thread re-enables the button and shows `decision.Toast` (dry-run) or a success/error toast for the real run

### Keyboard shortcuts

`internal/navigation` is the single puregotk-free authority for sidebar page
order, titles, icons, advertised shortcuts, registered accelerators, and the
complete page-selection transition. Its item metadata also maps every page to
the groups that actually build content. `navigation.VisibleItems` filters that
inventory using `Config.IsGroupEnabled`, always retains Help, and assigns
compacted Alt+number keys. `internal/window` uses the result for its sidebar,
stack, actions, initial selection, and shortcuts dialog. After construction,
`internal/app` registers `navigation.Bindings(window.NavigationItems())`, so
the registered and advertised keys use the exact same visible inventory.

Binding an accelerator does not create the action it targets. `internal/window`
registers the `win.*` actions (`show-shortcuts`, `show-about`, and one
`navigate-<page>` per visible page) and `internal/app` registers `app.quit`
(`registerQuitAction`) — GTK supplies no default quit action. Because neither
package can host a test binary, `navigation`'s
`TestEveryAdvertisedActionIsRegistered` reads both source files and fails when
any advertised action has no `gio.NewSimpleAction` registration. For
`app.quit` it also requires the `registerQuitAction` call in `onActivate`, the
`ConnectActivate` and `AddAction` calls, and the `a.Quit()` handler.

The accelerators are:

- `Ctrl+Q` → quit
- `Ctrl+?` → show shortcuts dialog
- `Alt+1` through `Alt+N` → navigate to the first through Nth visible page in
  canonical order, with omitted pages leaving no gaps
- `F1` → navigate to Help (the same `win.navigate-help` action as Help's
  current compacted Alt+number binding)

Mouse row activation and keyboard navigation actions both call
`Window.navigateToPage`. That method calls `navigation.Resolve` against the
visible inventory, rejects an omitted or unconstructed page, then applies all
four successful outcomes: select the compacted sidebar row, set the stack's
visible child, update the content-page title, and set
`NavigationSplitView.show-content` true so a collapsed layout reveals the
destination. `internal/navigation` tests every functional page with all of its
groups disabled, each builder-backed group individually enabled, the Help-only
fallback, compacted indices/accelerators, unavailable and unknown rejection,
the complete advertised-to-registered shortcut inventory, the F1 Help
binding, and static app/window wiring. No `_test.go` is added to the
puregotk-importing `internal/window` or `internal/app` packages.

Note: `GtkShortcutsWindow` is not available in puregotk, so a custom `adw.Window` with `adw.PreferencesGroup` rows is used for the shortcuts dialog.

### URL opening

Help page links are opened via `xdg-open` using `exec.Command`. The process is started asynchronously and its exit is waited on in a goroutine to avoid zombie processes.

## Problems page

Decision record: [ADR-0015](../adr/0015-problems-page-and-opt-in-reporting.md);
the exact contracts are [specs/problems-page.md](../specs/problems-page.md) and
[specs/problem-reporting.md](../specs/problem-reporting.md). The page reads
records on this computer and sends nothing until the user turns reporting on
(phase 2 of plan T857); `internal/reporting` is the only sender.

### Groups and visibility

`internal/navigation` lists `problems` (title "Problems", icon
`dialog-warning-symbolic`, config page `problems_page`) directly before Help,
so with every page visible Help is Alt+7. Its groups are
`problems_system_group`, `problems_apps_group` and
`problems_reporting_group`, all enabled by default. The page is omitted when
all 3 are off, also after a configuration error (ADR-0003).
`buildProblemsPage` builds the enabled groups with "Loading…" descriptions,
then `loadProblems` runs `problems.Load` on a worker goroutine and fills the
groups through `sgtk.RunOnMainThread`. The groups are nil when disabled, and
the main-thread code nil-guards both. See "Reporting" below for the
reporting group's rows. After the groups are filled the page logs
`problems.BuiltMarker`: `problems: page built system=<n> apps=<n>
source=<outcome>` (`source=disabled` without the system group).

### Sources (`internal/problems`)

`internal/problems` imports no puregotk package and no network package
(`TestNoNetworkImports`, `TestNoNetworkDependencies`). `problems.Load` reads:

- `/var/lib/snosi-health/problems.json` (`ReadSystem`, at most 1 MiB) for
  both data groups, because its envelope (`image.id`, `image.version`,
  `secure`) is the image identity of the user items too. Its outcome is 1 of
  4: `SourceMissing` (no file; 1 row, "System checks are not available on
  this system"), `SourceUnreadable` (open or read failed) and
  `SourceMalformed` (bad JSON, `format` not 1, wrong `schema`, no problems
  list, over 1 MiB; both show "System checks could not be read", and
  `Malformed` also records `problems.source_invalid`), and `SourceOK`. The
  system group's text for each outcome comes from
  `pageview.ProblemsSystemStatus`: the 3 failure rows, "No problems found"
  with "Last checked <when>" for an empty `SourceOK`, and only the
  description with "Last checked <when>" when items are shown.
- `/var/lib/snosi-health/details.json` (at most 4 MiB) only for system
  items, and only when readable: EACCES (mode 0640 root:sudo) is normal.
- The user's crashes: `coredumpctl --json=short --no-pager --since=-90d`
  with a 10-second timeout; ChairLift stops reading at 4 MiB of output,
  kills coredumpctl and logs an error. Only the current UID's entries are
  kept; "No coredumps found" (exit 1) and a missing coredumpctl are an empty
  list.
- The S8 app records of the user journal: ChairLift runs journalctl itself
  with std `problems.Read`'s arguments (`problems.JournalArgs`: `--user`,
  the last 90 days, at most 500 entries) and a 10-second timeout, and gives
  the output to std's `problems.ParseJSON`. It does not call std's `Read`,
  which keeps all output in memory: a forged entry with a huge field could
  exhaust memory, because 500 entries do not bound bytes. `readJournal`
  reads at most 8 MiB (`MaxJournalOutput`) and 4 KiB of standard error;
  at the cap it kills journalctl, parses only the complete lines before it
  and logs the truncation. It passes no `--output-fields`: std's `Validate`
  drops a record with an unregistered `FROSTYARD_*` field, and a field list
  would hide that field from it. std drops forged records, and the count
  goes to 1 log line only.

Every detail value of every item is checked again against the v1 rules of
its kind (`validateDetail`); a number or boolean must be that JSON token
type, so `"retries": "1"` fails. A value that fails is shown, marked not valid,
and the item is not reportable; `Reportable` also needs the file's own flag,
a valid `week` and a valid envelope. An unknown kind is shown as "An unknown
problem was recorded" and is never reportable. A system crash that a user
crash repeats (same program and signal, within the system record's time
±1 minute) shows once, in the Apps group (`MergeCrashes`). Each list holds
at most 200 items.

### Presentation and actions

Each item is an `adw.ExpanderRow`. `pageview.Problem` gives the title
(`problems.Title`, in plain words, such as "An update could not be prepared"
or "Frostmail stopped unexpectedly") and the subtitle (when, the count, and
the unit, feature, version or code); `pageview.ProblemDetails` gives the
rows inside: "What it means" (`problems.Hint`: a hint for each kind, each
value of the hinted enums and each known code; an unknown code gets its
class's hint), "Recorded values", and "On this computer" (details.json
values, units, messages, the program path, and an app record's `MESSAGE`,
`_COMM` and unit). Titles and subtitles go through `glib.MarkupEscapeText`.
The view composes no text of its own.

- **Copy details** puts `problems.DetailsText(item)` on the GDK clipboard
  (`SetText`). It is local text: it can hold names and paths that a report
  never holds.
- **Dismiss** writes the id to
  `$XDG_STATE_HOME/chairlift/problems-seen.json` (temp file and rename,
  0600, at most 1000 ids) through `problems.Seen.Dismiss`, then removes the
  row; see "Dry-run mode" for `actionmsg.ProblemDismiss`. A dismissed item
  shows again when its last time is after the dismissal.

`problems.BuildPayload` builds the `frostyard.problem/1` document of a
reportable item from validated schema fields only; only
`internal/reporting` calls it, and only after the user opted in. Its tests
validate every payload of the fixtures against a vendored byte copy of core's
`v1.json` (sha256 pinned) with a JSON-schema-subset validator, which lives in
`internal/problems/schemacheck` so the sender's tests use the same one.

`CHAIRLIFT_PROBLEMS_FIXTURES=<absolute folder>` replaces every source, only
with `--dry-run` (`problems.DecideFixtures`); `make e2e` uses it.

### Reporting (`internal/reporting`)

Full contract: [specs/problem-reporting.md](../specs/problem-reporting.md).
`internal/reporting` is a pure leaf package (no puregotk) and the only
production importer of `net/http`; `internal/installcheck/reporting_test.go`
fails when another package imports it, or when another non-test Go file names
`report.frostyard.org`.

- **Consent** (`consent.go`, `writer.go`):
  `$XDG_CONFIG_HOME/chairlift/reporting.json` (else
  `$HOME/.config/chairlift/reporting.json`, 0600) records `enabled`, `mode`
  (`ask`/`auto`), `decided_at` and the agreed `schema`. It is off until the
  user turns it on, and every failure reads as off. A schema that adds a kind
  or a field is a new data generation (`dataGeneration`), so sending stops
  until the user agrees again; an enum-only schema keeps it on.
  `ConsentWriter` writes the choices one at a time and the latest wins: the
  wanted value changes at once, a result for an older generation is ignored,
  and a failed latest write restores the value on disk.
- **Sent log** (`state.go`): `$XDG_STATE_HOME/chairlift/reports.json` (else
  `$HOME/.local/state/chairlift/reports.json`) holds the sent entries, the
  week of the last count and `retry_after`; at most 500 entries, none older
  than 90 days. `LoadState`'s error stops the automatic path: it could not
  enforce the caps.
- **Envelope** (`envelope.go`): `ForItem` builds a report with a new
  `report_id`, and `Prepare` indents the body for the dialog: `Report.Text()`
  is `string(Report.Body)`, so the dialog shows exactly what is sent. Kind
  `crash` is refused with `ErrLocalOnly`. `BuildCount`/`NewCountReport` build
  the weekly count: `install_age` (from `/etc/machine-id`'s modification
  time) and `channel` (`latest`/`other` from the bootc image reference).
- **Sender** (`sender.go`): `POST <endpoint>` with `Content-Type:
  application/json`, no `User-Agent`, a 15 s timeout, at most 8191 bytes of
  body, no redirect followed, 2xx is success and every other answer is a
  `*StatusError`. An empty endpoint is `ErrNoEndpoint` and makes no request:
  only `ParseEndpoint` chooses the default, so an invalid administrator
  setting disables sending instead of reaching the public service.
- **Run** (`run.go`): the one automatic path, in order: group off, consent,
  ask mode, release client, the lock, the sent log, the pause; then the
  pending reports (reportable, not `crash`, not sent this week, at most 20 in
  24 hours), then the count when this ISO week was not counted. It records
  and saves each entry BEFORE its request, so a process that exits during a
  request can only under-send, and a failed request removes the entry. It
  reads the consent again after the pace and just before every request (a
  withdrawal is a normal stop), and the first failure sets `retry_after` to
  now + 24 h and stops. A dry run previews and writes
  nothing. `manual.go`'s `SendReport` is the dialog's send: it ignores the
  pause (the user asked), refuses an unreadable sent log, and shares the caps
  and the lock. Both builders encode without HTML escaping, so enum text such
  as `<1w` stays readable.
- **Endpoint** (`endpoint.go`): `https://report.frostyard.org/v1/report` by
  default; `problems_reporting_group.endpoint` counts only from an
  administrator-owned file (`config.Config.ReportEndpoint`).
- **The windowless mode** (`cli.go`): `chairlift --send-pending-reports`
  runs before `app.New()`, so it builds no GTK; it exits 1 for a
  configuration error or a failed send, and 0 with 1 line when there is
  nothing to do. It loads the dismiss store the page uses, so both paths
  send the same items.
- **The page** (`internal/views/problems_page.go`): the switch, mode,
  "Sent reports", "What is sent" and "Weekly count" rows, a Report… button
  per offered problem, and `reporting.Run` in Auto mode. Consent changes go
  through the page's own `ConsentWriter`, and the consent and sent log load
  also when both data groups are off, so a consent can always be withdrawn. The decisions come
  from `actionmsg.Report` and `actionmsg.ReportingConsent` (ADR-0009).
- **The first run** shows the optional Reporting step between Style and Apps
  when the reporting group is on and `/var/lib/snosi-health/problems.json`
  exists (`firstrun.ReportingAvailable`); ADR-0014's `Steps`, markers and
  `--mark-done` do not change (see "First-run mode").

### ChairLift's own problems (`internal/selfreport`)

`cmd/chairlift/main.go` sets `version.Version` from the GoReleaser-stamped
`main.buildVersion`, then calls `selfreport.Init(version.Version)` (std
`problems.New("org.frostyard.ChairLift", version, ComponentGUI)` and
`CrashOutput`) and defers `RecoverPanic` directly in `main`. A dev or
snapshot version is not a release semver, so the recorder is inert and
`Init` logs that once. 4 coded errors, constants in `internal/selfreport`
only, are recorded at their paths: `config.load_failed` (`internal/window`
`window.go` and `firstrun.go`, on a configuration error),
`problems.source_invalid` (`problems_page.go`, a malformed problems.json),
`firstrun.complete_failed` (`internal/window/firstrun.go`, the completion
markers could not be written) and `updex.results_unreadable`
(`internal/updex`, the helper exited 0 with unreadable results).
`TestCodesAreRecordedAtTheirPaths` reads the source to prove each site.
The records stay in the user's journal and show in the Apps group.

## First-run mode

Decision record: [ADR-0014](../adr/0014-first-run-mode.md). The first-run
mode replaces SNOW First Setup (frostyard/first-setup) on images that ship its
trigger. ChairLift itself ships no autostart entry, so on any other
distribution nothing starts on its own.

### Binaries and trigger

- `cmd/chairlift-first-run` (`/usr/bin/chairlift-first-run`, GUI package
  `frostyard-chairlift` only) is pure Go: `internal/installcheck`'s
  `TestFirstRunGateImportsNoGTK` fails when `go list -deps` of it,
  `internal/firstrun` or `internal/i18n` contains a puregotk or snowkit
  package. puregotk dlopens GTK in package `init`, so a gate inside
  `cmd/chairlift` could not exit before GTK loads. `main.go` only calls
  `firstrun.Gate`, prints its output and `syscall.Exec`s.
- `firstrun.Gate` outcomes, in order: an unknown argument prints the usage to
  stderr and exits 2; `--help`/`-h` prints the usage to stdout and exits 0;
  UID 0 exits 0 with `first-run: skipped for root` (also for `--mark-done`);
  no config home (neither `XDG_CONFIG_HOME` nor `HOME`) exits 1;
  `--mark-done` calls `firstrun.Complete` and exits 0, or 1 when the write
  fails; an existing marker exits 0 with `first-run: already done`; otherwise
  it execs the fixed path `/usr/bin/chairlift` with argv `chairlift
  --first-run` (plus `--dry-run`).
- snosi ships the trigger on Snow and Snowfield:
  `/etc/xdg/autostart/org.frostyard.ChairLift.FirstRun.desktop` with
  `Exec=/usr/bin/chairlift-first-run`, and the list
  `/usr/share/snow/first-run/apps.json`.

### Application and window

`internal/app` checks `os.Args` for `--first-run` before it creates the
GObject (as it does for `--dry-run`), and then uses the application ID
`org.frostyard.ChairLift.FirstRun`; with the main ID a running ChairLift would
receive the activation and the window would never show. `first-run` is a
registered main option, so `chairlift --help` lists it. On activate the app
builds `window.NewFirstRun` instead of the main window and presents it;
`FirstRunWindow.Present` logs `first-run: window presented`, the readiness
marker that `make e2e` and the lab read (ADR-0008 style). The window is an
`adw.ApplicationWindow` with a header bar (Back, Next) and a `gtk.Stack` of
`adw.StatusPage` steps, inside an `adw.ToastOverlay`. All its text goes
through `i18n.T`; worker results return through `sgtk.RunOnMainThread`.

When `config.Load` returns an error, the window shows
`LoadError.ToastMessage()` as a persistent toast (`SetUseMarkup(false)`,
`SetTimeout(0)`), as the main window does; the Apps step is hidden and the
user can still finish. `internal/installcheck.TestToastsRenderPlainText`
scans this toast too.

**Close guard.** `Flow.CanClose` is false only on the Progress step before
`FinishProgress`. The window connects `close-request` and returns
`!Flow.CanClose()`, so a close is refused while the Flatpak actions and the
marker write run. `startProgress` also calls `Hold` on the GApplication before
the worker starts, and `finishProgress` calls `Release`; after Progress ends,
close works normally.

### Steps and flow

`firstrun.Flow` owns the order Welcome, Connectivity, Style, Apps, Progress,
Done and every transition:

- **Welcome** — `Hello {}!` with `firstrun.WelcomeName` (the GECOS full name's
  first comma field, else the login name, else `$USER`/`$LOGNAME`), Start, and
  Accessibility (`gnome-control-center universal-access`).
- **Connectivity** — status only: `firstrun.CheckConnectivity` dials
  `dl.flathub.org:443` over TCP with a 5 s timeout (`ConnChecking`,
  `ConnOnline`, `ConnOffline`). Next waits while the check runs, because the
  result decides whether Apps shows. Offline shows Check Again and Open
  Settings (`gnome-control-center wifi`); the user can go on.
- **Style** — Light/Dark toggles. Each toggle calls `Request` on the window's
  one `firstrun.SchemeWriter` from the main thread. The writer has one worker
  goroutine and a mutex-protected pending value: a request during a write
  replaces the pending value, so the latest choice wins and at most the
  running write and the latest one run. `SchemeWriter.Wait` blocks until no
  write is running or pending; the Progress sequence calls it before the
  completion. Each write is
  `firstrun.ApplyColorScheme`: `gsettings set org.gnome.desktop.interface
  color-scheme prefer-light|prefer-dark`, never `gtk-theme`. The window
  follows the choice through its style manager.
- **Apps** — visible only when the list loaded, `first_run_apps_group` is
  enabled, and the check said online (`Flow.Visible`). One `adw.ExpanderRow`
  with an enable switch per category, one `adw.SwitchRow` per app, no icons
  and no network fetch.
- **Progress** — Back is not possible; Next waits until every action ended.
  `firstrun.PlanActions` gives no action when no app is selected, else
  `flatpak remote-add --user --if-not-exists flathub
  https://dl.flathub.org/repo/flathub.flatpakrepo` (`flatpak.AddRemote`) and
  then `flatpak install -y --user <id>` per selected app
  (`flatpak.Install(id, true)`). `firstrun.Execute` runs every action, also
  after a failure, and emits Running, then Succeeded or Failed per row.
- **Done** — `Finished!`, the completion text, Exit, and an `Error log`
  expander. `firstrun.DoneState` composes it from the failed actions'
  entries (`firstrun.ErrorLog`) and the completion outcome: the warning icon
  and the expander show when an action failed or the completion could not be
  recorded. No Back, no Next.

The settings buttons show only when `gnome-control-center` is on `PATH`
(`firstrun.SettingsAvailable`); they are launches, not state changes, so they
also run in dry run, and they have no config keys (GNOME-only).

### Application list

`firstrun.LoadConfiguredApps` returns `ErrAppsDisabled` when
`first_run_page`/`first_run_apps_group` is disabled — also after a
configuration error, because `disabledConfig()` turns every group off — and
otherwise `firstrun.LoadApps(apps_path)`. `LoadApps`/`ParseApps` accept
first-setup's shape, `{"<category>": [{"name", "id", "active"}]}` with an
absent `active` meaning true, and reject a read error, a file over 1 MiB,
invalid JSON or trailing data, unknown fields, an empty object or category,
an empty name, an ID that is not a valid Flatpak application ID
(`firstrun.ValidFlatpakID`: at least 3 elements, `-` only in the last, at
most 255 bytes) and a duplicate ID. Categories show in the order browsers,
utilities, office, then the other keys sorted; known keys get first-setup's
translated title and subtitle, an unknown key shows the key. Any error hides
Apps and is logged (`first-run: Apps step hidden: ...`). `firstrun.Selection`
holds the switches: a category starts on when at least one of its apps is
active; `Selected` returns the apps whose category and own switch are on, in
list order.

### Markers and completion

The config home is `$XDG_CONFIG_HOME` when it is not empty, else
`$HOME/.config`. `firstrun.IsDone` is true when
`<config home>/chairlift/first-run.done` or first-setup's
`<config home>/snow-first-setup.done` exists. `firstrun.MarkDone` is the only
writer: it creates `<config home>/chairlift` (0700), writes both markers as
empty 0644 files (an existing file is kept), and removes the legacy per-user
`<config home>/autostart/org.frostyard.FirstSetup.autostart.desktop`.
`firstrun.Complete(dryRun, paths)` builds the `firstrun.Completion` decision
and calls `MarkDone` only when `WriteMarkers` is true;
`chairlift-first-run --mark-done` reaches it through `Gate`. The window calls
`firstrun.CompleteAt(dryRun, paths, pathsErr)` after the Progress actions end
(also when some failed) and before Done shows. The outcomes:

- success — `DoneDescription` "Your user account is ready to use", no
  warning;
- dry run — no write, `[DRY-RUN] would write first-run markers: ...`, the
  dry-run preview text (a missing config home is no failure here);
- a live run with no config home, or a `MarkDone` error — `Err` is set,
  `ShowWarning` is true, the `Log` line is `first-run: could not record
  completion: ...`, and `Completion.ErrorLog` gives one entry, "Setup failed:
  Could not record that the first run is finished" with the error text. There
  is no retry button.

`firstrun.DoneState(actionEntries, completion, gate)` composes the Done step.
`gate` is the gate state after the completion, from
`firstrun.GateStateAt(paths, pathsErr)`, one of 3 values:

- `GateRunsAgain` — the paths are known and no marker exists: the gate
  execs ChairLift at the next login. Only this state gets the description
  "The first run starts again at your next login" after a failure.
- `GateDone` — a marker exists. `MarkDone` can fail after it wrote the
  markers, for example when the legacy autostart path is a non-empty
  directory; the gate then skips the next login, and the description is
  "Setup finished with errors".
- `GateUnknown` — the window has no config home (`pathsErr`). The gate in
  that environment exits 1 and starts nothing, so the description is also
  "Setup finished with errors", with no promise of another run.
  `GateUnknown` is the zero value, so an unset state never promises a run.

The warning icon and the error log show after every completion failure.

`firstrun.RunProgress(actions, steps)` is the Progress worker's whole
sequence, in this order: run every action (`Execute`), drain the
color-scheme writer (`Drain`, the window's `SchemeWriter.Wait`), record the
completion (`Complete`, `CompleteAt`), then read the gate state (`Gate`,
`GateStateAt`) and compose `DoneState`. The window runs it off the GTK thread and
calls `finishProgress` (which releases the application hold) only after it
returns, so the last style choice is written before Exit can end the
process; Back is not possible from Progress, so no newer choice can follow
the drain. Wiring tests
in `internal/firstrun` prove both paths reach the one writer, that only
`MarkDone` writes files, and that only `internal/firstrun/markers.go` spells
the marker names.

**Legacy marker end rule.** Reading `snow-first-setup.done` never ends.
Writing it (`writeLegacyMarker` in `internal/firstrun/markers.go`) ends no
earlier than 90 days after the first snosi main image without first-setup
publishes. The change that removes the write records that date and the
ChairLift release that drops it here. Status: the write is active; the end
date is not set yet.

### Translations

`internal/i18n` embeds `internal/i18n/po/*.po` (ported from first-setup commit
`b5f7de3`; see `internal/i18n/po/README.md`) and reads them with its own
parser (`ParsePO`: msgid/msgstr, multi-line strings, C escapes; fuzzy and
empty entries skipped; msgctxt and plurals are errors). `Candidates` applies
gettext's locale choice: the first non-empty of `LC_ALL`, `LC_MESSAGES`,
`LANG`; English for an empty, `C` or `POSIX` locale (then `LANGUAGE` is
ignored); else each `LANGUAGE` entry, then the locale, each as `ll_CC` then
`ll` after the codeset and modifier are removed, with `zh_CN`/`zh_SG` mapped to
`zh_Hans` and `zh_TW`/`zh_HK`/`zh_MO` to `zh_Hant`. `T` returns the first
catalog's translation, else the msgid; `Format` fills `{}` placeholders. Every
first-run msgid is a constant in `internal/i18n/msgids.go`, and a test fails
when an embedded file holds a msgid outside that list.

## Configuration

Decision records: [ADR-0003](../adr/0003-two-tier-config-with-fail-closed-semantics.md)
(two-tier search order and fail-closed semantics),
[ADR-0004](../adr/0004-configuration-error-diagnostic-vocabulary.md)
(the `CONFIGURATION ERROR` diagnostic vocabulary), and
[ADR-0005](../adr/0005-config-schema-reflected-from-canonical-struct.md)
(the reflected schema and overlay semantics).

### Config file search order

1. `/etc/chairlift/config.yml` — system-wide (highest priority)
2. `/usr/share/chairlift/config.yml` — package-maintainer defaults installed
   by both source `make install` and nFPM packages
3. `config.yml` — beside the executable when present, otherwise relative to
   the current working directory (development fallback)

Only a missing candidate advances the search; a dangling symlink is present,
not missing. The first existing candidate is
authoritative; a read, parse, type, or schema error disables every feature
group and produces both a high-signal log entry and a persistent toast. If no
file is found, all features default to enabled except
`maintenance_cleanup_group`, which defaults to disabled. See
[CONFIG.md](../../CONFIG.md) for the full reference.

Both packaging paths own only the `/usr/share` candidate and may replace it on
upgrade. Neither writes `/etc/chairlift/config.yml`; that higher-precedence
path remains administrator-owned, so local policy is never overwritten by a
ChairLift install or package update.

### Config structure

```yaml
page_name:
  group_name:
    enabled: true/false
    # Optional per-group fields:
    app_id: "..." # External app to launch
    actions: # Custom scripts (updates/maintenance)
      - title: "..."
        script: "/path/to/script"
        sudo: true/false
    bundles_paths: [...] # Homebrew bundle directories
    apps_path: "..." # First-run application list (first_run_page only)
    website: "..." # Help page URLs
    issues: "..."
    chat: "..."
```

### Key config groups

| Page                | Group                            | Controls                                                                                                                                                                                                |
| ------------------- | -------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `system_page`       | `system_info_group`              | OS info from `/etc/os-release`                                                                                                                                                                          |
| `system_page`       | `bootc_status_group`             | bootc deployment status display (gated on `bootc.IsBootcBootedCached()`)                                                                                                                                |
| `system_page`       | `health_group`                   | System monitor launcher (configurable `app_id`, default: Mission Center)                                                                                                                                |
| `updates_page`      | `bootc_updates_group`            | bootc system updates — stage via `bootc-update-stage`, apply on restart (gated on `bootc.IsBootcBootedCached()` and stage script availability)                                                          |
| `updates_page`      | `sysupdate_updates_group`        | native A/B system updates — stage via `snosi-sysupdate-stage`, apply on restart, with a read-only previous-version rollback row (gated on `sysupdate.IsNativeABCached()` and stage script availability) |
| `updates_page`      | `firmware_updates_group`         | device firmware updates through fwupd, one device at a time with a confirmation dialog (gated on `fwupd.IsInstalledCached()`); enabled by default since [ADR-0013](../adr/0013-firmware-updates-are-on-by-default.md) |
| `updates_page`      | `flatpak_updates_group`          | Flatpak pending updates                                                                                                                                                                                 |
| `updates_page`      | `brew_updates_group`             | Homebrew outdated packages                                                                                                                                                                              |
| `updates_page`      | `brew_trust_group`               | Untrusted Homebrew taps with installed packages (Homebrew 6 tap trust); hidden unless there is something to trust                                                                                       |
| `applications_page` | `flatpak_user_group`             | User Flatpak applications with uninstall actions                                                                                                                                                        |
| `applications_page` | `flatpak_system_group`           | System Flatpak applications with uninstall actions                                                                                                                                                      |
| `applications_page` | `brew_group`                     | Installed Homebrew formulae/casks with uninstall and formula pin/unpin actions                                                                                                                          |
| `applications_page` | `brew_search_group`              | Typed Homebrew formula/cask search and confirmed install                                                                                                                                                |
| `applications_page` | `brew_bundles_group`             | Curated `*.Brewfile` bundles discovered from every configured `bundles_paths` directory, with guarded install actions                                                                                   |
| `applications_page` | `applications_installed_group`   | External Flatpak-manager launcher for discovery/install (configurable `app_id`, default: Bazaar); ChairLift has no direct Flatpak-install UI                                                            |
| `maintenance_page`  | `maintenance_cleanup_group`      | Custom cleanup scripts (5min timeout, pkexec for sudo); **disabled by default**                                                                                                                         |
| `maintenance_page`  | `maintenance_brew_group`         | Homebrew cleanup (deferred visibility)                                                                                                                                                                  |
| `maintenance_page`  | `maintenance_flatpak_group`      | Flatpak unused cleanup (deferred visibility)                                                                                                                                                            |
| `maintenance_page`  | `maintenance_optimization_group` | System optimization (placeholder)                                                                                                                                                                       |
| `features_page`     | `features_group`                 | Updex feature toggles                                                                                                                                                                                   |
| `help_page`         | `help_resources_group`           | Configurable links (website, issues, chat)                                                                                                                                                              |
| `problems_page`     | `problems_system_group`          | Problems page: snosi-health's system problems; "System checks are not available on this system" without `/var/lib/snosi-health/problems.json` ([ADR-0015](../adr/0015-problems-page-and-opt-in-reporting.md)) |
| `problems_page`     | `problems_apps_group`            | Problems page: the user's crashes (coredumpctl) and frostyard app problem records (user journal) |
| `problems_page`     | `problems_reporting_group`       | Problems page: opt-in reporting (switch, Ask/Auto mode, sent reports, weekly count); `endpoint` counts only from an administrator-owned file ([spec](../specs/problem-reporting.md)) |
| `first_run_page`    | `first_run_apps_group`           | Not a navigation page: the first run's Apps step and its `apps_path` list (default `/usr/share/snow/first-run/apps.json`); disabled, a configuration error, or a missing/invalid list hides the step ([ADR-0014](../adr/0014-first-run-mode.md)) |

## Build and Release

- **Build**: `make build` builds three binaries: `build/chairlift` (main app), `build/chairlift-updex-helper` (privileged helper), and `build/chairlift-first-run` (pure-Go first-run gate), all with `CGO_ENABLED=0`
- **CI mirror**: `make ci` runs every host-independent gate from `.github/workflows/test.yml` in fail-fast order — go.mod tidy check, `go vet`, gofmt check, `golangci-lint`, unit tests (`./internal/...` under `-run "^Test[^I]" -skip "Integration"`), the race detector, and the build. Its build step reproduces CI's `linux/amd64` + `linux/arm64` matrix into `build/ci-linux-<arch>/` before rebuilding natively, so a compile failure on the non-host architecture cannot pass locally. The mill's deep gate (`.mill.toml`) calls this target. Codecov's remote project status additionally rejects coverage regressions greater than one percentage point, with no fixed project or patch target; it cannot be mirrored locally. The runtime-dependent E2E job is deliberately separate: `make e2e` builds the three binaries, executes the application's `--help` path (which must list `--first-run`), boots the dry-run GTK window under a private D-Bus/Xvfb session with `CHAIRLIFT_PROBLEMS_FIXTURES` set to the fixtures of `test/e2e/testdata/problems/`, polls all four readiness markers (the fourth is `problems: page built system=16 apps=7 source=ok`) for at most 30 seconds, requires one second of post-readiness stability, then terminates its private process group, stages the real `make install` layout under a temporary `DESTDIR` (which must hold an executable `usr/bin/chairlift-first-run` and no autostart path), and executes the staged `chairlift-updex-helper` rejection paths. For the first run it also checks `chairlift-first-run --help`, runs `--mark-done` and the gate against temporary homes (as `nobody` when the test runs as root), and boots `chairlift --first-run --dry-run` until it logs `first-run: window presented`, then checks that no marker exists. Its Go test package lives at `test/e2e`, imports no puregotk package, and is enforced by that explicit target rather than the `./internal/...` unit-test filter. The readiness markers are a log-line contract — decision record [ADR-0008](../adr/0008-e2e-readiness-is-a-log-marker-contract.md).
- **Dev build**: `make dev` builds with `CGO_ENABLED=1` and `-race` flag for race detection
- **Version**: Set via ldflags by goreleaser (`buildVersion`, `buildCommit`, `buildDate`, `buildBy`)
- **Semantic versioning**: Uses [svu](https://github.com/caarlos0/svu) via `make bump`
- **CI**: GitHub Actions workflows for test, snapshot, and release (`.github/workflows/`); per [ADR-0034](../org-adrs.md), snapshot publishers use the repository-scoped `goreleaser-nightly` concurrency group with in-progress cancellation so only the newest tested `main` commit publishes to the rolling `dev` release and concurrent GoReleaser uploads cannot collide. Every external `uses:` reference in every workflow is pinned to a full 40-character commit SHA (with its version or source ref retained as a comment); `internal/installcheck.TestWorkflowActionsUseImmutableCommitSHAs` inventories both `.yml` and `.yaml` workflow files and rejects mutable tags, branches, short SHAs, and expressions while allowing repository-local `./` actions.
- **Release**: GoReleaser config at `.goreleaser.yaml`. Its `metadata.homepage` is the single source of truth for the repository URL and is consumed by `release.footer`, whose "Full Changelog" link is templated from `{{ .Metadata.Homepage }}` rather than a hardcoded owner; two static tests guard that pairing — see the "Install-path consistency (`internal/installcheck`)" section of [package-managers.md](./package-managers.md#install-path-consistency-internalinstallcheck)
- **Debian publication**: a tag push runs `.github/workflows/release.yml`. GoReleaser creates the GitHub release with the archives, `checksums.txt` and the deb/rpm/apk packages. `actions/attest-build-provenance` then attests all of them, and the final step sends a `publish-deb` `repository_dispatch` with the repository and tag to `frostyard/apt-publisher`, authenticated by `APT_PUBLISH_TOKEN`. apt-publisher is the only writer of `https://repository.frostyard.org/debian/` ([frostyard/core ADR-0055](../org-adrs.md)). It downloads every `.deb` asset of the tag, verifies its provenance against `refs/tags/<tag>`, and publishes it to the codenames registered for chairlift in its `config/producers.tsv`. After apt can download the new packages through the public URL, it dispatches `build` to the image repositories registered there (ADR-0056). The request is not `continue-on-error`, so a failed request fails the release. The rolling `dev` prerelease from `snapshot.yml` is never published to the APT repository and dispatches nothing. `internal/installcheck/release_workflow_test.go` pins the exact release and snapshot step sequences, the request's inputs, the workflow permissions and attestation subjects, and that the only `.deb` packages GoReleaser builds are the two names that must be registered with apt-publisher.
- **Other targets**: `make fmt` (gofmt), `make lint` (golangci-lint), `make install`/`make uninstall` (system install including the three binaries, polkit policies, icons, and wrapper script, and no autostart entry; default `PREFIX=/usr`, the only prefix that matches where polkit reads policy files and the updex helper's fixed pkexec exec-path annotation — see "Privileged operations" above), `make build-linux-amd64`/`make build-linux-arm64` (cross-compilation)

### Runtime dependencies

- GTK 4 and libadwaita 1 (shared libraries loaded at runtime by puregotk)
- Homebrew (optional)
- Flatpak (optional)
- fwupd / `fwupdmgr` (optional; the firmware-updates group is enabled by default and hidden unless `fwupdmgr` is present)
- `bootc` + `/usr/libexec/bootc-update-stage` (both optional; UI gated on `bootc.IsBootcBootedCached()`, i.e. `bootc status` reporting a non-null `booted` deployment — not on any sentinel file)
- `/usr/lib/snosi/native-ab` marker + `/usr/libexec/snosi-sysupdate-stage` (both optional, shipped by native A/B OS images; UI gated on `sysupdate.IsNativeABCached()` and `sysupdate.StageScriptAvailable()`)
- Updex features configured on the system (optional; read via Go library, writes via `chairlift-updex-helper`)
- Problems page: `/var/lib/snosi-health/problems.json` and `details.json` (optional, written by snosi-health), `coredumpctl` (optional), and `journalctl` (the user journal)
- First run only: `gsettings` (the Style step), `flatpak` (the Apps step), and `gnome-control-center` (optional; the settings buttons show only when it is on `PATH`)

### Key external Go dependencies

| Module                           | Purpose                                                                                     |
| -------------------------------- | ------------------------------------------------------------------------------------------- |
| `codeberg.org/puregotk/puregotk` | GTK4/Adwaita bindings (no CGO)                                                              |
| `github.com/frostyard/snowkit`   | GObject registration, main-thread dispatch                                                  |
| `github.com/frostyard/updex/v2`  | Updex Go library for feature reads and helper binary (currently pinned to v2.1.3 in go.mod) |
| `gopkg.in/yaml.v3`               | YAML config parsing                                                                         |
| `golang.org/x/text`              | Title-casing OS release info keys                                                           |
| `github.com/frostyard/std`       | `problems`: ChairLift's own problem records and the journal reader of the Problems page    |

There is no separate Go client library dependency for bootc: status/stage types (`Status`, `Deployment`, `ProgressEvent`, etc.) are defined locally in `internal/bootc`, parsed directly from `bootc status --format json` and the stage script's line output.

## Subsystem Details

- [Package Manager Wrappers](./package-managers.md) — Homebrew (including tap trust), Flatpak, bootc, and Updex wrapper details
