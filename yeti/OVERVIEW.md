# ChairLift Overview

## Purpose

ChairLift is a GTK4/Libadwaita system management GUI for [Snow Linux](https://github.com/frostyard/snosi), written in Go using [puregotk](https://codeberg.org/puregotk/puregotk) bindings (no CGO). It provides a unified interface for managing Homebrew and Flatpak applications, bootc system updates (staged via the snow `bootc-update-stage` script), system features (via updex), and maintenance tasks. The UI is YAML-configuration-driven, making it portable to other Linux distributions by toggling feature groups on/off.

## Architecture

```
cmd/chairlift/main.go                 Entry point: version injection, app creation
cmd/chairlift-updex-helper/main.go    Privileged helper for updex write operations
        │
internal/app/app.go             GObject-registered Application (adw.Application subtype)
        │
internal/window/window.go       Main window: NavigationSplitView, sidebar, content stack
        │
internal/views/                 Page builders and event handlers (one file per page)
        │                       ├── internal/views/actionmsg/     ┐ puregotk-free leaf packages:
        │                       ├── internal/views/actionstate/   │ toast/decision text, async gates,
        │                       ├── internal/views/badgestate/    │ badge counts, bundle action state,
        │                       ├── internal/views/bundleview/    │ row bookkeeping, expander status,
        │                       ├── internal/views/trustmsg/      │ Flatpak and feature update-status
        │                       ├── internal/views/rowset/        │ text and decisions, all unit-tested
        │                       ├── internal/views/flatpakstatus/ │ headlessly
        │                       └── internal/views/featurestatus/ ┘
        │
        ├── internal/config/    YAML config loading, feature group enablement
        ├── internal/navigation/ Canonical pages, shortcuts, and pure navigation transitions
        ├── internal/homebrew/  Homebrew CLI wrapper (JSON output parsing)
        ├── internal/flatpak/   Flatpak CLI wrapper (tabular output parsing)
        ├── internal/bootc/     bootc wrapper (status reads, pkexec stage script, line streaming)
        ├── internal/updex/     Updex feature manager (Go library reads, helper binary writes)
        ├── internal/updexhelper/ Puregotk-free argv-parsing/Options-building for cmd/chairlift-updex-helper
        └── internal/version/   Build metadata (ldflags injection)
```

### Dependency flow

`cmd → app → window → views → {config, homebrew, flatpak, bootc, updex}`.
`app` and `window` also depend on the pure `navigation` package.

External shared library: `github.com/frostyard/snowkit` (published module, pinned in go.mod) provides:
- `gobj` — GObject type registration and instance registry
- `sgtk.RunOnMainThread()` — main-thread dispatch for GTK safety

### Views coordinator (`internal/views/views.go`)

The `views.go` file defines the central `UserHome` struct that holds references to all page widgets, config, and the `ToastAdder` interface. It provides:
- `New(cfg, toastAdder)` — constructor that initializes `UserHome`
- `ToastAdder` interface — `ShowToast(msg)`, `ShowErrorToast(msg)`, `SetUpdateBadge(count)` — implemented by Window

`internal/views` imports puregotk, so it can never hold a `_test.go` (see `docs/agents/skills/gtk-headless-tests.md`). Decidable logic is therefore pushed down into eight puregotk-free leaf packages beneath it — `internal/views/actionmsg` and `internal/views/trustmsg` (toast text and UI decisions, see [package-managers.md](./package-managers.md#view-layer-toast-and-decision-helpers-internalviewsactionmsg-internalviewstrustmsg)), `internal/views/actionstate` (Homebrew update command/refresh outcomes and repeated-click gates, see [package-managers.md](./package-managers.md#view-layer-update-action-state-internalviewsactionstate)), `internal/views/badgestate` (thread-safe per-provider update counts and totals, see [package-managers.md](./package-managers.md#view-layer-update-badge-state-internalviewsbadgestate)), `internal/views/bundleview` (Brew bundle empty/error/unavailable presentation and per-row install gating, see [package-managers.md](./package-managers.md#view-layer-brew-bundle-state-internalviewsbundleview)), `internal/views/rowset` (single-row removal plus clear-then-repopulate bookkeeping, see [package-managers.md](./package-managers.md#view-layer-row-bookkeeping-internalviewsrowset)), `internal/views/flatpakstatus` (the Flatpak updates expander's subtitle text and expandable decision, applied by `loadFlatpakUpdates` from both retained `ListUpdates` errors, see [package-managers.md](./package-managers.md#view-layer-flatpak-update-status-internalviewsflatpakstatus)) and `internal/views/featurestatus` (the Features page's per-feature update-status subtitle, the any-component update decision and the features group description for all three check outcomes — `GroupDescriptionCheckFailed` when the check itself failed, and `GroupDescription` when it completed with zero features updatable or with updates found — applied by `checkFeatureUpdates`, which composes no subtitle or description text of its own, see [package-managers.md](./package-managers.md#view-layer-feature-update-status-internalviewsfeaturestatus)) — each table- or scenario-tested headlessly.

### Pages

The UI defines six pages, each in its own file under `internal/views/`. Static
configuration may omit any functional page whose builder-backed groups are all
disabled; Help is always retained:

| Page | File | Purpose |
|------|------|---------|
| Applications | `applications_page.go` | Manage Homebrew formulae/casks and installed Flatpaks; launch an external manager for new Flatpak installs |
| Maintenance | `maintenance_page.go` | Homebrew/Flatpak cleanup, configurable maintenance scripts (executed via `exec.Command`/`pkexec`) |
| Updates | `updates_page.go` | bootc staged system updates, Flatpak updates, Homebrew outdated packages, untrusted-tap trust prompts |
| System | `system_page.go` | OS info (`/etc/os-release`), bootc deployment status, health monitor launch |
| Features | `features_page.go` | Toggle system features via `updex` tool |
| Help | `help_page.go` | Configurable links to website, issues, chat (opened via `xdg-open`) |

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

### Dry-run mode

The `--dry-run` / `-d` flag is propagated to wrapper packages via `SetDryRun(true)`, set once at startup in `app.New()` for homebrew, flatpak, bootc, updex, and `internal/views` itself (`internal/views/dryrun.go` — for configured custom maintenance scripts, which have no wrapper package of their own).

**The general rule, applied uniformly:** every state-changing view handler branches on the relevant wrapper's `IsDryRun()` (or `views.IsDryRun()` for custom scripts) to show an explicit preview toast instead of a completed/saved/installed message. Anywhere that same handler would *also* mutate a row, a group's visibility, or a switch on success, that mutation decision is pulled out of the view and expressed as a small struct or an `actionstate.Decision` — `ScriptDecision.Execute`, `BundleInstallDecision.Complete`, `TapTrustDecision.MutateUI`, `FeatureToggleDecision.Confirm`, `PackageInstall`, `PackageUninstall`, or `PackagePin`. The view computes `IsDryRun()` exactly once, builds the decision, and branches solely on it for both the mutation *and* the toast, so a table-driven test proves the mutation gate and the toast cannot drift from it (see [package-managers.md](./package-managers.md#view-layer-toast-and-decision-helpers-internalviewsactionmsg-internalviewstrustmsg) for the full function/type list). Sites with no second UI mutation to gate (package upgrade/update/self-update, Flatpak uninstall, cleanup, Brewfile dump, bootc stage, and feature-update toasts) get a plain string function instead.

**Intentional exception:** bootc staging's completion **toast** is dry-run-aware (`actionmsg.BootcStage`), but its expander **subtitle** deliberately is not. The subtitle is a persistent status readout of live `bootc.GetStatus()` — what deployment is actually staged/booted right now — not a per-click completion claim, so it stays accurate and unchanged in both dry-run and live mode. Only the toast, which inherently answers "what did this click just do," needed dry-run-specific wording; there is no mutation left to gate once the subtitle is deliberately excluded, which is why `BootcStage` is string-only rather than a decision struct.

Per-wrapper mechanics:

- **Homebrew/Flatpak**: state-changing commands are skipped entirely at the wrapper layer (return mock/empty results); ordinary package-action toasts use the plain `actionmsg` string functions (`Install`, `Uninstall`, `Pin`, `Upgrade`, `Update`, `SelfUpdate`, `BundleDump`, `Cleanup`). Homebrew search installs and installed-package uninstall/pin actions pair that text with `actionstate` decisions: live success completes the old row controls and refreshes the installed inventory, while failure or dry-run restores the controls. Brew bundle installation uses `BundleInstallDecision` with the same live-complete/dry-run-reset distinction.
- **Updex**: `EnableFeature`/`DisableFeature`/`UpdateFeatures` skip their `pkexec` call entirely under dry-run and return empty/nil results; the helper binary itself (`cmd/chairlift-updex-helper`, dispatch logic in `internal/updexhelper`) also honors `--dry-run` for `update`, matching `enable-feature`/`disable-feature`, as defense-in-depth even though it's unreachable from the wrapper today.
- **bootc**: `StageUpdate` short-circuits before invoking pkexec: it logs the would-be command, emits a synthetic `EventMessage` + `EventComplete` pair on the progress channel, and returns — the stage script is never actually run (see the exception above for the toast/subtitle split).
- **Homebrew tap trust**: `trustTap` (`internal/views/updates_page.go`) computes `decision := actionmsg.TapTrust(homebrew.IsDryRun(), tap.Name)` once, after a successful `homebrew.TrustPackages` call, and gates removing the tap's row, hiding the group, and refreshing outdated packages on `decision.MutateUI`.
- **views (custom maintenance scripts)**: `runMaintenanceAction` (`internal/views/maintenance_page.go`) calls `actionmsg.MaintenanceScript(IsDryRun(), title)` once, before spawning its goroutine, to get a `ScriptDecision{Execute, Toast}`: when `Execute` is false no `exec.Cmd` is ever constructed (no `pkexec`, no direct script exec) — only a `[DRY-RUN] Would execute: ...` log line.
- **Features page switch confirmation**: `onFeatureToggled` (`internal/views/features_page.go`) computes `decision := actionmsg.FeatureToggle(updex.IsDryRun(), enabled, name)` once, after a successful `updex.EnableFeature`/`DisableFeature` call, and branches solely on `decision.Confirm` to decide whether the switch confirms the flip (`toggle.SetActive(enabled)`) or reverts to its pre-click state (`toggle.SetActive(!enabled)`).

### Configuration-driven UI visibility

Each preference group on every page checks `config.IsGroupEnabled(pageName, groupName)` before building its widgets. Groups default to enabled if not specified in config. The `maintenance_cleanup_group` defaults to disabled in the default config.

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
denied opening the file — distinct from "file does not exist", which
`Load()` treats as absent-and-fall-back, not an error); `KindParseType`
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
  instead; a malformed second document preserves *that* document's parser
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
*immediate* `Alias` target is a `yaml.MappingNode`; or a `yaml.SequenceNode`
each of whose entries is itself one of the two prior shapes. Everything
else is rejected as `KindParseType` — a scalar operand, an alias to a
sequence or a scalar, a sequence containing a scalar or a nested sequence,
and a sequence containing an alias to a non-mapping all fail
(`isMergeOperandMapping` is the single-alias-hop predicate both the direct
and sequence-entry checks share). The immediate-target rule produces a
deliberate asymmetry: an alias-to-alias-to-mapping chain is perfectly valid
as an *ordinary* mapping value (the generic traversal above only requires a
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
identity is used only to decide *precedence* — which candidate wins, and
whether two explicit complex keys collide (see below) — never to compute
it: a *winning* complex key is still fully resolved and emitted
structurally, alias-free, by `emitEffectiveNode` like any other node
(charged against `maxEffectiveOutputNodes` like everything else it
emits), while a *losing* complex key (impossible for two candidates to
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
source-graph validation, and `doc == nil` returns `(nil, nil)` only *after*
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
*target's* metadata, not the alias node's own — this applies identically
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
comparing a per-call counter *before* allocating a `*yaml.Node` for that
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
unwraps a direct mapping, or an alias's *immediate* target, matching
`validateMergeOperand`'s already-accepted shapes), and each operand
mapping's own `effectiveEntries` computed recursively — so a merge nested
inside a merge operand is fully flattened before its candidates are
considered here. The combined candidate list is then deduplicated by
`effectiveKeyIdentity`, keeping the *first* candidate for each identity.
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
collects) for two whose `effectiveKeyIdentity` compare equal *after* alias
dereferencing — catching, for example, a literal scalar key and an alias
key targeting an anchored scalar with the same resolved tag and value,
which `checkDuplicateMappingKeys` cannot catch because the alias node's
own `Kind` and `Value` differ from its target's. Finding such a pair sets
`effectiveEmitState.collided` and records the *later* key (in source
`Content` order) as `collisionKey`, aborting the whole `resolveEffective`
call — exactly like a node-budget overflow aborts it — without
inventorying or emitting anything further; `resolveEffective` then returns
`effectiveIdentityCollisionError(path, doc, st.collisionKey)`, a
`KindParseType` `*LoadError` with a nil `Err` and a `Detail` reusing the
same `collectSourceInventory`/`attributeSourceLines` line-attribution pair
(own line, else nearest positive-line ancestor over all paths, else `1`)
as the output-limit error above. The rule applies only to two *explicit*
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
from it over *all* root-reachable paths, not merely the path the traversal
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
*boundary* node — one whose own count exceeds 128 but whose every direct
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
API since map iteration order is not deterministic). `rawConfig` is *not* a
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

| Effective input | Result |
|---|---|
| malformed YAML, second document, or source duplicate | `KindParseType` |
| cyclic, malformed, source-path-over-budget, or effective-output-over-budget graph | `KindParseType` |
| empty document or top-level null | valid no-op overlay |
| top-level scalar or sequence | `KindParseType` |
| unknown top-level page | `KindSchema` |
| known page with null | valid no-op for that page |
| known page with scalar or sequence | `KindParseType` |
| unknown group under a known page | `KindSchema` |
| known group with null | valid no-op for that group |
| known group with scalar or sequence | `KindParseType` |
| unknown group field | `KindSchema` |
| `actions` value that is not null or a sequence | `KindParseType` |
| action entry that is null or otherwise not a mapping | `KindParseType` |
| unknown action field | `KindSchema` |
| known field that cannot decode into its declared Go type | `KindParseType` |
| valid effective document | decoded `*rawConfig` |

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
(`errors.Is(err, fs.ErrNotExist)`) advances to the next search location. Any
other read failure, or any parse/type/schema failure in the first file that
exists, makes that file authoritative: lower-priority files are not read,
`Load` returns `disabledConfig()` plus the structured `*LoadError`, and every
canonical feature group is hidden while non-visibility defaults remain
available. If all candidates are absent, the ordinary built-in defaults and
a nil error are returned.

`resolveCandidatePath` makes diagnostic paths deterministic and absolute when
the operating system supplies a working directory: absolute candidates are
cleaned; a relative candidate prefers a file beside the executable and
otherwise resolves against the current working directory. Filesystem reads,
the executable path, and the working directory have narrow package-level
seams so precedence and permission failures are testable without relying on
host permissions.

On an authoritative failure, `LoadError.LogMessage` prefixes the full
path-and-cause diagnostic with `CONFIGURATION ERROR`, states that all groups
were disabled, and instructs the user to restart after fixing the file.
`window.New` retains the same `LoadError`, builds the toast overlay, then calls
`ShowErrorToast` with `LoadError.ToastMessage`. `ShowErrorToast` sets timeout
zero, so the startup error remains visible instead of expiring; construction
and the toast call both occur on the GTK main thread.

### Package manager wrapper pattern

Each wrapper in `internal/` follows a consistent shape:
- Module-level `dryRun` flag with `SetDryRun()`/`IsDryRun()`
- `IsInstalled()` to check tool availability, plus `IsInstalledCached()` (`sync.Once`) for use from views during async startup
- Homebrew, Flatpak, and Updex implement both `IsInstalled()` and `IsInstalledCached()`
- List/Search/Install/Uninstall/Update functions
- Context-based timeouts. Homebrew and Flatpak both use a two-class model selected per invocation by an unexported `commandTimeout(args)` helper: 30s for read-only commands, 30m for state-changing ones (the keys of each package's `stateChangingCommands` map). updex uses 5min and bootc 30min.
- Custom error types where needed

### Streaming progress (bootc stage)

`bootc.StageUpdate(ctx, progressCh)` runs `pkexec /usr/libexec/bootc-update-stage`, streaming combined stdout+stderr line-by-line to the caller's channel and closing it when done:
1. Caller creates a `chan bootc.ProgressEvent` and passes it to `StageUpdate`
2. Each non-empty output line becomes an `EventMessage`; the channel is closed after either an `EventComplete` (success) or the function returning an error
3. Event types: `EventMessage` and `EventComplete` — deliberately simpler than
   a step/percent model because the stage script's own output is unstructured
   log lines, not a structured progress protocol. Failures return as errors;
   they are not duplicated into the event stream.
4. The view goroutine (`internal/views/updates_page.go`, `onBootcStageClicked`) reads events and dispatches UI updates to the main thread via `sgtk.RunOnMainThread`

**Caller-visible outcomes.** Both context-taking bootc functions classify failures with `errors.Is` against the context sentinels (never `==` on `ctx.Err()`), and `bootc.Error` has an `Err error` field plus `Unwrap() error` so callers can tell them apart:

- `StageUpdate` — deadline: `*Error` "Update staging timed out" unwrapping to `context.DeadlineExceeded`; cancellation: `*Error` "Update staging was canceled" unwrapping to `context.Canceled` (or the bare `context.Canceled` when cancellation wins the race inside the streaming send, which classifies the same); non-zero exit: `*Error` "update staging failed (exit N): <last output line>" matching neither sentinel; missing `pkexec`: `*NotFoundError`.
- `GetStatus` — deadline: `*Error` "bootc status timed out" unwrapping to `context.DeadlineExceeded`; cancellation: `*Error` "bootc status was canceled" unwrapping to `context.Canceled`; non-zero exit: `*Error` "bootc status failed (exit N): <stderr>" matching neither sentinel; missing `bootc`: `*NotFoundError`. `GetStatus(ctx)` is `return getStatusFrom(ctx, bootcCommand)`; the unexported `getStatusFrom` seam exists so tests exercise all of these against a fake script without a real `bootc`.

The deadline and cancellation messages differ in both functions, and neither ever surfaces as `signal: killed`.

**bootc direct-kills; homebrew and flatpak kill the process group.** On cancellation `runStageStreaming` kills only the direct child (`cmd.Process.Kill()`) and sets no `Setpgid`, because the staging child runs under `pkexec` and is root-owned — an unprivileged process cannot signal it or its process group, so a group kill would fail by design. The unprivileged `internal/homebrew` and `internal/flatpak` runners do the opposite: they run in their own process group and kill the whole group so download helpers are not orphaned. The three wrappers therefore do *not* terminate processes the same way; making the privileged path group-killable is a privilege-model change.

**Why a stage script instead of `bootc upgrade`:** upstream `bootc upgrade`'s registry-transport pull currently fails on snow's composefs images. The snow-shipped `/usr/libexec/bootc-update-stage` script works around this: `podman pull` fetches the image into containers-storage (podman's pull path works where bootc's does not), then `bootc switch --transport containers-storage` stages the already-pulled image as the next boot deployment. This keeps snow's actual upgrade logic in one place (the snosi script) rather than duplicating pull/switch orchestration in ChairLift; ChairLift only invokes the script via pkexec and streams its output. The script is idempotent — it exits 0 without staging anything when the deployment is already current.

### bootc progress UI (updates page)

`onBootcStageClicked()` (`internal/views/updates_page.go`) drives the "System Update" expander: it disables the button, spawns `bootc.StageUpdate` in a goroutine, and processes the `ProgressEvent` channel on a second goroutine — `EventMessage` lines are appended to a log expander with timestamps, and `EventComplete` marks the activity complete. A returned `stageErr` drives the error subtitle and toast after the stream closes; there is no duplicate error event. After `wg.Wait()` returns, the handler re-reads live `bootc.GetStatus()` and calls `uh.updateCounts.Set(badgestate.Bootc, 0|1)` plus `uh.updateBadgeCount()` unconditionally in both dry-run and live mode (this is a plain read, not a mutation, so it always reflects reality); it then sets `expander`'s subtitle from that same live read unconditionally as well, but shows `actionmsg.BootcStage(bootc.IsDryRun(), staged)` for the completion toast — an explicit preview string under dry-run rather than one of the "staged"/"up to date" strings that read as a verified completion claim about a click that, under dry-run, checked and changed nothing. The system page has a separate, simpler bootc path: `loadBootcStatus` (gated on `IsBootcBootedCached()`) calls `bootc.GetStatus` to show the booted/staged/rollback deployment images, versions, and digests, with no staging controls of its own — staging happens on the Updates page.

### Update badge tracking

The updates page stores bootc, Flatpak, and Homebrew counts in the mutex-backed
`badgestate.Counts` value on `UserHome`. The bootc provider is 1 when
`bootc.GetStatus()` reports a staged deployment and 0 otherwise — a boolean
folded into the total, not a count of available images. Provider refreshes use
`Set`, so a repeated load replaces rather than accumulates; successful
row-level Homebrew upgrades use `Add(Homebrew, -1)`, which cannot go below
zero. `updateBadgeCount` reads the aggregate `Total` and pushes it through
`ToastAdder.SetUpdateBadge()`. Refresh requests receive an increasing
generation from `actionstate.RefreshGate`; only the newest request may apply
its result, so an older, slower query cannot overwrite newer metadata. Every
completion callback still runs when superseded so its action is restored. Only
a successful current query replaces rows/count. A command or refresh failure
preserves the last known count (and a refresh failure preserves the current
rows), while dry-run previews restore their controls without changing either.

### Privileged operations

bootc staging and updex require root for state-changing operations. They invoke commands through `pkexec` (PolicyKit). bootc runs `pkexec /usr/libexec/bootc-update-stage` directly (polkit action id `org.frostyard.ChairLift.bootc.stage`), while updex delegates to the fixed absolute path `internal/updex.HelperPath` (`/usr/bin/chairlift-updex-helper`) via `pkexec`. Polkit policy files are installed for both: `data/org.frostyard.ChairLift.bootc.policy` and `data/org.frostyard.ChairLift.updex.policy`. ChairLift deliberately ships no `.rules` files: the policies require normal administrator authentication (`auth_admin`, with `auth_admin_keep` for an active local session) rather than granting blanket passwordless access to a login group. Source installation removes the two legacy ChairLift `.rules` files so an older passwordless rule cannot survive an upgrade. Homebrew tap trust (`brew trust`) is explicitly per-user and does *not* go through pkexec — see [package-managers.md](./package-managers.md).

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
contains only `chairlift-updex-helper`, and its contents contain the two
policies plus `/usr/share/chairlift/config.yml`. The packages declare conflicts
because they intentionally own the same privileged files.

The integration package does **not** ship `bootc-update-stage`. That operation
is distro policy, so an image that enables `bootc_updates_group` must provide a
trusted implementation at the existing fixed
`/usr/libexec/bootc-update-stage` path. Keeping the path fixed preserves the
PolicyKit executable boundary; making it a user-writable config value would
allow the GUI configuration to redirect a root execution. The page already
gates the group on `bootc.StageScriptAvailable`, so an absent distro helper
hides the operation.

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

## Configuration

### Config file search order

1. `/etc/chairlift/config.yml` — system-wide (highest priority)
2. `/usr/share/chairlift/config.yml` — package-maintainer defaults installed
   by both source `make install` and nFPM packages
3. `config.yml` — beside the executable when present, otherwise relative to
   the current working directory (development fallback)

Only a missing candidate advances the search. The first existing candidate is
authoritative; a read, parse, type, or schema error disables every feature
group and produces both a high-signal log entry and a persistent toast. If no
file is found, all features default to enabled except
`maintenance_cleanup_group`, which defaults to disabled. See
[CONFIG.md](../CONFIG.md) for the full reference.

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
    app_id: "..."          # External app to launch
    actions:               # Custom scripts (updates/maintenance)
      - title: "..."
        script: "/path/to/script"
        sudo: true/false
    bundles_paths: [...]   # Homebrew bundle directories
    website: "..."         # Help page URLs
    issues: "..."
    chat: "..."
```

### Key config groups

| Page | Group | Controls |
|------|-------|----------|
| `system_page` | `system_info_group` | OS info from `/etc/os-release` |
| `system_page` | `bootc_status_group` | bootc deployment status display (gated on `bootc.IsBootcBootedCached()`) |
| `system_page` | `health_group` | System monitor launcher (configurable `app_id`, default: Mission Center) |
| `updates_page` | `bootc_updates_group` | bootc system updates — stage via `bootc-update-stage`, apply on restart (gated on `bootc.IsBootcBootedCached()` and stage script availability) |
| `updates_page` | `flatpak_updates_group` | Flatpak pending updates |
| `updates_page` | `brew_updates_group` | Homebrew outdated packages |
| `updates_page` | `brew_trust_group` | Untrusted Homebrew taps with installed packages (Homebrew 6 tap trust); hidden unless there is something to trust |
| `applications_page` | `flatpak_user_group` | User Flatpak applications with uninstall actions |
| `applications_page` | `flatpak_system_group` | System Flatpak applications with uninstall actions |
| `applications_page` | `brew_group` | Installed Homebrew formulae/casks with uninstall and formula pin/unpin actions |
| `applications_page` | `brew_search_group` | Typed Homebrew formula/cask search and confirmed install |
| `applications_page` | `brew_bundles_group` | Curated `*.Brewfile` bundles discovered from every configured `bundles_paths` directory, with guarded install actions |
| `applications_page` | `applications_installed_group` | External Flatpak-manager launcher for discovery/install (configurable `app_id`, default: Bazaar); ChairLift has no direct Flatpak-install UI |
| `maintenance_page` | `maintenance_cleanup_group` | Custom cleanup scripts (5min timeout, pkexec for sudo); **disabled by default** |
| `maintenance_page` | `maintenance_brew_group` | Homebrew cleanup (deferred visibility) |
| `maintenance_page` | `maintenance_flatpak_group` | Flatpak unused cleanup (deferred visibility) |
| `maintenance_page` | `maintenance_optimization_group` | System optimization (placeholder) |
| `features_page` | `features_group` | Updex feature toggles |
| `help_page` | `help_resources_group` | Configurable links (website, issues, chat) |

## Build and Release

- **Build**: `make build` builds two binaries: `build/chairlift` (main app) and `build/chairlift-updex-helper` (privileged helper), both with `CGO_ENABLED=0`
- **CI mirror**: `make ci` runs every host-independent gate from `.github/workflows/test.yml` in fail-fast order — go.mod tidy check, `go vet`, gofmt check, `golangci-lint`, unit tests (`./internal/...` under `-run "^Test[^I]" -skip "Integration"`), the race detector, and the build. Its build step reproduces CI's `linux/amd64` + `linux/arm64` matrix into `build/ci-linux-<arch>/` before rebuilding natively, so a compile failure on the non-host architecture cannot pass locally. The mill's deep gate (`.mill.toml`) calls this target. Codecov's remote project status additionally rejects coverage regressions greater than one percentage point, with no fixed project or patch target; it cannot be mirrored locally. The runtime-dependent E2E job is deliberately separate: `make e2e` builds both binaries, executes the application's `--help` path, boots the dry-run GTK window under a private D-Bus/Xvfb session, polls all three readiness markers for at most 30 seconds, requires one second of post-readiness stability, then terminates its private process group, stages the real `make install` layout under a temporary `DESTDIR`, and executes the staged `chairlift-updex-helper` rejection paths. Its Go test package lives at `test/e2e`, imports no puregotk package, and is enforced by that explicit target rather than the `./internal/...` unit-test filter.
- **Dev build**: `make dev` builds with `CGO_ENABLED=1` and `-race` flag for race detection
- **Version**: Set via ldflags by goreleaser (`buildVersion`, `buildCommit`, `buildDate`, `buildBy`)
- **Semantic versioning**: Uses [svu](https://github.com/caarlos0/svu) via `make bump`
- **CI**: GitHub Actions workflows for test, snapshot, and release (`.github/workflows/`); snapshot publishers use the `chairlift-dev-release` concurrency group without in-progress cancellation so uploads to the rolling `dev` release cannot overlap. GitHub retains only the newest pending run in a concurrency group, so rapid pushes can skip intermediate snapshots while preserving the active upload and latest queued snapshot. Every external `uses:` reference in every workflow is pinned to a full 40-character commit SHA (with its version or source ref retained as a comment); `internal/installcheck.TestWorkflowActionsUseImmutableCommitSHAs` inventories both `.yml` and `.yaml` workflow files and rejects mutable tags, branches, short SHAs, and expressions while allowing repository-local `./` actions.
- **Release**: GoReleaser config at `.goreleaser.yaml`. Its `metadata.homepage` is the single source of truth for the repository URL and is consumed by `release.footer`, whose "Full Changelog" link is templated from `{{ .Metadata.Homepage }}` rather than a hardcoded owner; two static tests guard that pairing — see the "Install-path consistency (`internal/installcheck`)" section of [package-managers.md](./package-managers.md#install-path-consistency-internalinstallcheck)
- **Other targets**: `make fmt` (gofmt), `make lint` (golangci-lint), `make install`/`make uninstall` (system install including polkit policies, icons, and wrapper script; default `PREFIX=/usr`, the only prefix that matches where polkit reads policy files and the updex helper's fixed pkexec exec-path annotation — see "Privileged operations" above), `make build-linux-amd64`/`make build-linux-arm64` (cross-compilation)

### Runtime dependencies

- GTK 4 and libadwaita 1 (shared libraries loaded at runtime by puregotk)
- Homebrew (optional)
- Flatpak (optional)
- `bootc` + `/usr/libexec/bootc-update-stage` (both optional; UI gated on `bootc.IsBootcBootedCached()`, i.e. `bootc status` reporting a non-null `booted` deployment — not on any sentinel file)
- Updex features configured on the system (optional; read via Go library, writes via `chairlift-updex-helper`)

### Key external Go dependencies

| Module | Purpose |
|--------|---------|
| `codeberg.org/puregotk/puregotk` | GTK4/Adwaita bindings (no CGO) |
| `github.com/frostyard/snowkit` | GObject registration, main-thread dispatch |
| `github.com/frostyard/updex` | Updex Go library for feature reads and helper binary (currently pinned to v1.3.0 in go.mod) |
| `gopkg.in/yaml.v3` | YAML config parsing |
| `golang.org/x/text` | Title-casing OS release info keys |

There is no separate Go client library dependency for bootc: status/stage types (`Status`, `Deployment`, `ProgressEvent`, etc.) are defined locally in `internal/bootc`, parsed directly from `bootc status --format json` and the stage script's line output.

## Subsystem Details

- [Package Manager Wrappers](./package-managers.md) — Homebrew (including tap trust), Flatpak, bootc, and Updex wrapper details
