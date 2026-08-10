# AGENTS

ChairLift is a GTK4/Libadwaita system-management GUI for
[Snow Linux](https://github.com/frostyard/snosi), written in idiomatic Go using
[puregotk](https://codeberg.org/puregotk/puregotk) bindings — **no CGO**. GTK,
Libadwaita, and GLib shared libraries are loaded at runtime via `dlopen`. The UI
is YAML-configuration-driven; feature groups toggle on and off per host.

## Build, test, lint

The app builds pure-Go (`CGO_ENABLED=0`); the race detector needs CGO.

- `make build` — builds `build/chairlift` and `build/chairlift-updex-helper`
  (both `CGO_ENABLED=0`).
- `make test` — `go test ./...`.
- `make fmt` — `gofmt -s -w .`.
- `make lint` — `golangci-lint run`.
- `make ci` — runs every **host-independent** CI gate, in CI's order (go.mod
  tidy check, `go vet`, gofmt check, lint, unit tests, race detector, build).
  The build step reproduces CI's `linux/amd64` + `linux/arm64` matrix into
  `build/ci-linux-<arch>/`, then rebuilds natively, so a cross-arch-only
  compile failure cannot pass locally and break CI. Run it before pushing;
  the mill's deep gate calls this exact target. Codecov's remote project status
  additionally rejects coverage regressions greater than one percentage point;
  it has no fixed coverage target and cannot be mirrored locally.
- `make e2e` — builds both executables, checks the application's real
  `--help` surface, starts the dry-run GTK window under a private D-Bus/Xvfb
  session, stages `make install`, and executes the installed privileged
  helper's rejection paths. It requires GTK4, Libadwaita,
  `dbus-run-session`, GNU `timeout`, and `xvfb-run`; the hosted E2E job
  installs those runtime dependencies explicitly because ordinary unit-test
  hosts intentionally do not carry them.
- `make install`'s default `PREFIX` is `/usr` — the only prefix under which
  the installed PolicyKit policy files land where `polkitd` reads them
  (`/usr/share/polkit-1/actions`) and the updex helper's installed
  path matches its fixed `pkexec` exec-path annotation (see the privilege
  boundary invariant below). It installs maintainer defaults at
  `/usr/share/chairlift/config.yml` and must never install or overwrite the
  administrator-owned `/etc/chairlift/config.yml`. GoReleaser publishes both
  the self-contained `frostyard-chairlift` package and the mutually exclusive
  `frostyard-chairlift-system-integration` companion for user-scoped GUI
  installs; every nFPM entry carrying policies must retain the same fixed
  paths.

CI (`.github/workflows/test.yml`) filters tests with `-run "^Test[^I]"
-skip "Integration"`. That filter excludes *any* test whose name begins `TestI`
— not only `TestIntegration` — or contains `Integration` anywhere. Those names
are reserved for tests that require a real environment (a live `brew`,
`flatpak`, `bootc`, or GTK display). Ordinary unit tests must not use the
`TestI` prefix: a test that trips the filter is never executed by `make ci` or
by CI and therefore protects nothing. The accident is easy to make, because
plain unit-test names such as `TestIsValid`, `TestInitConfig`, or `TestIndexOf`
all start with `TestI` and would be silently skipped; name them so the first
letter after `Test` is not `I` (see the GTK-headless skill below).

The separately invoked tests under `test/e2e/` are outside the
`./internal/...` unit-test scope by design. They are enforced by the E2E
workflow's explicit `make e2e` step; do not assume adding a test outside
`internal/` is enough without that dedicated gate.

There are no generated files and no codegen step; everything under version
control is hand-written Go, YAML, and data assets.

## Repository invariants

An agent must not break these:

- **Privilege boundary.** State-changing operations that require root go
  through `pkexec` (PolicyKit) with fixed, installed polkit policies and fixed
  helper binaries only: `pkexec /usr/libexec/bootc-update-stage` (action
  `org.frostyard.ChairLift.bootc.stage`) and `pkexec
  /usr/bin/chairlift-updex-helper` (`internal/updex.HelperPath`, actions
  `org.frostyard.ChairLift.updex.{enable-feature,disable-feature,update}`) —
  always that fixed absolute path, matching the
  `org.freedesktop.policykit.exec.path` annotation, with the updex subcommand
  matching `org.freedesktop.policykit.exec.argv1`. The helper must strictly
  reject unsupported argv because PolicyKit does not validate arguments after
  action selection. ChairLift ships no passwordless PolicyKit rules; normal
  administrator authentication applies. Homebrew tap trust (`brew trust`) is
  deliberately per-user and does **not** use pkexec. Do not add arbitrary
  privileged command execution, broaden what pkexec runs, or route new
  mutations around the fixed helper/policy pair.
- **System-integration split.** The
  `frostyard-chairlift-system-integration` nFPM package contains the fixed-path
  updex helper, both PolicyKit policies, and package-maintainer config, but not
  the GUI or a bootc staging implementation. Distributions pairing it with a
  user-scoped ChairLift install must provide their trusted stage helper at
  `/usr/libexec/bootc-update-stage` before enabling `bootc_updates_group`.
  Do not make the privileged path configurable from ChairLift's user-writable
  configuration.
- **GTK main-thread safety.** All external tool calls run in goroutines; every
  UI update marshals back to the GTK main thread via
  `snowkit`'s `sgtk.RunOnMainThread(...)`. Never touch a widget directly from a
  worker goroutine.
- **Navigation behavior has one authority.** Page order, titles, icons, and
  advertised/registered accelerators live in the pure
  `internal/navigation` package. It also decides page visibility from static
  group configuration: omit a functional page when all of its builder-backed
  groups are disabled, always retain Help, and compact Alt+number over visible
  pages. Mouse activation and window navigation actions must both call
  `Window.navigateToPage`, which applies the complete `navigation.Resolve`
  transition (visible-row index, visible child, title, and collapsed-layout
  content reveal). The app and shortcuts dialog must use the window's same
  visible inventory. Do not reintroduce a second page or shortcut inventory in
  `internal/window` or `internal/app`.
- **Homebrew update actions preserve known state.** Per-package upgrades and
  the top-level metadata update use `internal/views/actionstate` gates before
  spawning work. Failures and dry-run previews restore their controls without
  changing rows or counts. A live package success removes its row, decrements
  the count/badge, and refreshes; a failed refresh preserves that last known
  row/count state instead of replacing it with an invented zero.
- **Homebrew application actions are typed and refresh-safe.** Search queries
  both formula and cask namespaces and carries the result kind into
  `brew install [--cask]`. Search and installed-package refreshes use separate
  `actionstate.RefreshGate` generations; stale workers must not replace newer
  rows. Confirmed installs use an `actionstate.Gate`, restore controls on
  failure/dry-run, and refresh installed rows only after a live success.
  Installed formula/cask rows likewise confirm uninstall, formula rows confirm
  pin/unpin, and every row shares one gate across its mutation controls so
  actions cannot overlap. A live success completes the old controls and starts
  a generation-guarded inventory refresh; failure or dry-run restores them.
- **Update badge counts have one state owner.** Bootc, Flatpak, and Homebrew
  counts live in the pure `internal/views/badgestate` package. Refreshes
  replace a provider's count, successful row removals decrement without going
  negative, and the displayed total is always the sum of all three providers.
  Do not restore independent integer fields in `UserHome`.
- **Config-driven visibility is real.** Any group can be disabled in config
  (`config.IsGroupEnabled(page, group)`), so its widgets may never be
  constructed. Code that runs after an async action must not assume a widget
  from another group exists — nil-guard cross-group widget access. In
  particular, `brew_bundles_group` is independent of `brew_group`; bundle
  discovery and installs must not assume the formulae/casks expanders exist.
- **Configuration precedence fails closed.** Only a missing candidate advances
  to the next configuration search path. The first file that exists is
  authoritative: read, YAML, or schema errors must disable every configurable
  group, emit the `CONFIGURATION ERROR` diagnostic, and remain visible in the
  UI as a persistent toast until the file is fixed and ChairLift is restarted.

## Documentation

After any change to source code, update relevant documentation in `AGENTS.md`,
`README.md`, and the `yeti/` folder. A task is not complete without reviewing
and updating relevant documentation. For behavior, configuration, dependency,
or install-layout changes, also follow
`docs/documentation-consistency.md`; current-state claims must be checked
against source/config/go.mod rather than copied from historical plans.

**yeti/ directory** contains documentation written for AI consumption and
context enhancement, not primarily for humans. Read `yeti/OVERVIEW.md` and
`yeti/package-managers.md` for architecture, patterns, and decision rationale
before working. Write content there to be maximally useful to an AI agent
understanding the codebase — detailed architecture and rationale rather than
user-facing guides.

**.knowledge/ directory** is the repository's cross-session knowledge index.
Read `.knowledge/README.md` before working so prior corrections, handoffs,
durable lessons, and architecture guidance are discovered from their canonical
locations instead of duplicated into competing stores.

**.memory/ directory** is the repository's committed correction store for AI
agents. Read `.memory/README.md` and any learning artifacts in that directory
before working. Record verified corrections there when a session establishes
that a prior belief about ChairLift was wrong, and promote stable rules into
this file, `docs/agents/skills/`, or `yeti/` as appropriate. Never record
secrets or personal data because the directory is version-controlled.

## Learned agent skills

**docs/agents/skills/** Read every file in `docs/agents/skills/` before
planning, implementing, or reviewing changes. Each file is a durable lesson
distilled from a previous automated run of
[the mill](https://github.com/frostyard/mill) (the spec→PR harness, configured
here via `.mill.toml`); they are binding guidance, not suggestions. New skills
are added by the mill's harvest step and reviewed like any other change in the
PR that carries them.
