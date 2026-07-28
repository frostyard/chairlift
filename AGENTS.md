# AGENTS

ChairLift is a GTK4/Libadwaita system-management GUI for
[Snow Linux](https://github.com/frostyard/snow), written in idiomatic Go using
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
- `make ci` — **runs every gate CI runs, in CI's order** (go.mod tidy check,
  `go vet`, gofmt check, lint, unit tests, race detector, build). The build
  step reproduces CI's `linux/amd64` + `linux/arm64` matrix into
  `build/ci-linux-<arch>/`, then rebuilds natively, so a cross-arch-only
  compile failure cannot pass locally and break CI. Run it before pushing;
  green locally means green in CI. The mill's deep gate calls this exact
  target.
- `make install`'s default `PREFIX` is `/usr` — the only prefix under which
  the installed PolicyKit policy/rules files land where `polkitd` reads them
  (`/usr/share/polkit-1/{actions,rules.d}`) and the updex helper's installed
  path matches its fixed `pkexec` exec-path annotation (see the privilege
  boundary invariant below).

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

There are no generated files and no codegen step; everything under version
control is hand-written Go, YAML, and data assets.

## Repository invariants

An agent must not break these:

- **Privilege boundary.** State-changing operations that require root go
  through `pkexec` (PolicyKit) with fixed, installed polkit policies and fixed
  helper binaries only: `pkexec /usr/libexec/bootc-update-stage` (action
  `org.frostyard.ChairLift.bootc.stage`) and `pkexec /usr/bin/chairlift-updex-helper`
  (`internal/updex.HelperPath`, action for updex writes) — always that fixed
  absolute path, matching the `org.freedesktop.policykit.exec.path` annotation
  in `data/org.frostyard.ChairLift.updex.policy`, never a bare/`$PATH`-resolved
  name. Homebrew tap trust (`brew trust`) is deliberately per-user and does
  **not** use pkexec. Do not add arbitrary privileged command execution,
  broaden what pkexec runs, or route new mutations around the fixed
  helper/policy pair.
- **GTK main-thread safety.** All external tool calls run in goroutines; every
  UI update marshals back to the GTK main thread via
  `snowkit`'s `sgtk.RunOnMainThread(...)`. Never touch a widget directly from a
  worker goroutine.
- **Navigation behavior has one authority.** Page order, titles, icons, and
  advertised/registered accelerators live in the pure
  `internal/navigation` package. Mouse activation and window navigation
  actions must both call `Window.navigateToPage`, which applies the complete
  `navigation.Resolve` transition (selected row, visible child, title, and
  collapsed-layout content reveal). Do not reintroduce a second page or
  shortcut inventory in `internal/window` or `internal/app`.
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
and updating relevant documentation.

**yeti/ directory** contains documentation written for AI consumption and
context enhancement, not primarily for humans. Read `yeti/OVERVIEW.md` and
`yeti/package-managers.md` for architecture, patterns, and decision rationale
before working. Write content there to be maximally useful to an AI agent
understanding the codebase — detailed architecture and rationale rather than
user-facing guides.

## Learned agent skills

**docs/agents/skills/** Read every file in `docs/agents/skills/` before
planning, implementing, or reviewing changes. Each file is a durable lesson
distilled from a previous automated run of
[the mill](https://github.com/frostyard/mill) (the spec→PR harness, configured
here via `.mill.toml`); they are binding guidance, not suggestions. New skills
are added by the mill's harvest step and reviewed like any other change in the
PR that carries them.
