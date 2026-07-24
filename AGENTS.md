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
  `go vet`, gofmt check, lint, unit tests, race detector, build). Run it before
  pushing; green locally means green in CI. The mill's deep gate calls this
  exact target.

CI (`.github/workflows/test.yml`) filters tests with `-run "^Test[^I]"
-skip "Integration"`. Names beginning `TestI…` or containing `Integration` are
the escape hatch for tests that need a real environment and are **excluded from
the standard run** — a skipped test protects nothing, so keep regression tests
inside the standard filter (see the GTK-headless skill below).

There are no generated files and no codegen step; everything under version
control is hand-written Go, YAML, and data assets.

## Repository invariants

An agent must not break these:

- **Privilege boundary.** State-changing operations that require root go
  through `pkexec` (PolicyKit) with fixed, installed polkit policies and fixed
  helper binaries only: `pkexec /usr/libexec/bootc-update-stage` (action
  `org.frostyard.ChairLift.bootc.stage`) and the `chairlift-updex-helper` binary
  (action for updex writes). Homebrew tap trust (`brew trust`) is deliberately
  per-user and does **not** use pkexec. Do not add arbitrary privileged command
  execution, broaden what pkexec runs, or route new mutations around the fixed
  helper/policy pair.
- **GTK main-thread safety.** All external tool calls run in goroutines; every
  UI update marshals back to the GTK main thread via
  `snowkit`'s `sgtk.RunOnMainThread(...)`. Never touch a widget directly from a
  worker goroutine.
- **Config-driven visibility is real.** Any group can be disabled in config
  (`config.IsGroupEnabled(page, group)`), so its widgets may never be
  constructed. Code that runs after an async action must not assume a widget
  from another group exists — nil-guard cross-group widget access.

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
