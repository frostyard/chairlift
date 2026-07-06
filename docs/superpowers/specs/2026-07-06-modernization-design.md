# ChairLift Modernization Design

**Date:** 2026-07-06
**Branch:** `feat/modernize` (single PR → `main`)
**Status:** Approved

## Background

Four problems reported on a production Snow Linux host (running installed chairlift v0.6.0; repo HEAD is v0.7.0+28):

1. **nbc is deprecated** — Snow images are moving back to upstream bootc (composefs deployments built by `snosi`). The host's `/run/nbc-booted` gate and the entire `internal/nbc` integration target a dying tool.
2. **Homebrew 6 tap trust** — third-party taps are now untrusted by default. `brew upgrade` ignores formulae from untrusted taps and emits a "taps are not trusted" warning, which chairlift surfaces as a raw error.
3. **Features tab dead** — root-caused to the *installed v0.6.0 binary*, which parses `updex features list --json` CLI output; updex ≥1.2.x emits JSONL progress lines before the JSON array, breaking the parse. HEAD already uses the updex Go library, verified working on-host with v1.2.0 (both `Features()` and `CheckFeatures()`).
4. **10+ second startup** — also v0.6.0 vintage; HEAD already has the deferred-visibility async pattern (commit `3351d4c`). Needs verification and cleanup of any remaining synchronous exec on the startup path.

Decisions made during brainstorming:
- **Clean cut** to bootc: delete `internal/nbc` entirely, no dual support. This host loses system-update UI until reinstalled as bootc — accepted.
- **Trust button in the UI** for Homebrew taps (not messaging-only, not auto-trust).
- **One branch/PR** for all four workstreams.

## 1. bootc migration

### Constraint: the upstream upgrade path is broken

`bootc upgrade`'s registry-transport composefs pull fails on snosi images (known upstream bug). Snow images ship a workaround at `/usr/libexec/bootc-update-stage` (from the snosi repo), run hourly by `bootc-update-stage.timer`:

```
bootc status --format json          # read followed image from .spec.image.image
podman image prune -f
podman pull --quiet "$image"        # podman does the transfer; enforces containers-policy.json
# compare pulled digest vs booted/staged/rollback digests; exit 0 if current
bootc switch --quiet --transport containers-storage "$image"
```

The result is a **staged** deployment applied at the next natural reboot (no forced reboot).

**Design decision: chairlift wraps this script** (`pkexec /usr/libexec/bootc-update-stage`) rather than reimplementing podman-pull + digest-compare + switch in Go. One source of truth for the workaround; chairlift inherits its idempotency (script exits 0 when already current) and its policy enforcement. When the upstream bug is fixed and the script retires, chairlift can move to native `bootc upgrade --progress-fd` (verified present in bootc 1.16.3) for real progress UI.

### `internal/bootc` package

Replaces `internal/nbc` (which is deleted along with the `github.com/frostyard/nbc` go.mod dependency).

```go
// Read-only, unprivileged
GetStatus(ctx) (*Status, error)   // bootc status --format json
IsBootcBooted() bool              // status.booted != nil; plus IsBootcBootedCached() via sync.Once

// Privileged, via pkexec
StageUpdate(ctx, progressCh chan<- ProgressEvent) error  // pkexec /usr/libexec/bootc-update-stage
```

- `Status` is a minimal struct mirroring only the fields the UI needs from the `org.containers.bootc/v1` `BootcHost` document: `Spec.Image.Image`, and for each of `Status.Booted/Staged/Rollback`: image reference, version, timestamp, digest.
- **Gate:** `IsBootcBooted()` parses `bootc status` and checks `status.booted != null`. NOT `/run/ostree-booted` (absent on snow's composefs deployments) and NOT binary presence (`bootc status` succeeds with all-null status on non-bootc hosts — verified on the nbc host).
- `StageUpdate` streams the script's stdout/stderr lines to `progressCh` as message-type events. A reduced `ProgressEvent` type lives in `internal/bootc` (the six-type nbc event taxonomy shrinks to `Message`, `Error`, `Complete` — the script emits plain log lines, no structured progress).
- Standard wrapper conventions: module-level `dryRun` + `SetDryRun()` (dry-run skips the pkexec call and emits mock events), `DefaultContext()` with 30-minute timeout, `Error`/`NotFoundError` types.

### Views

| Before | After |
|---|---|
| System page `nbc_status_group` | `bootc_status_group`: booted image ref, version, timestamp rows; staged row ("restart to apply") when present; rollback row when present |
| Updates page `nbc_updates_group` | `bootc_updates_group`: one **"Check & Stage Update"** button + log expander + **indeterminate** progress bar (script gives no percentages); on completion, refresh status and show staged state |
| `runNBCOperation` helper | `runBootcStage` helper, simplified to the reduced event set |
| Badge: `nbcUpdateCount` | `bootcUpdateCount` — 1 when a staged deployment exists or staging just completed, else 0 |

The Updates-page button is deliberately singular: the script is check-and-download in one, exiting quickly with an "already current" message when there is nothing to do. No separate Check button, no Apply button — staged deployments apply on the next reboot, and the UI says so rather than offering to reboot the machine.

If the host is bootc-booted but `/usr/libexec/bootc-update-stage` is missing (non-snow bootc system), the updates group hides and the status group still works.

### Polkit and packaging

- Delete `data/org.frostyard.ChairLift.nbc.policy`; add `data/org.frostyard.ChairLift.bootc.policy` authorizing `/usr/libexec/bootc-update-stage` (auth_admin_keep, active sessions).
- `make install` / goreleaser packaging updated accordingly.

### Config

`system_page.nbc_status_group` → `system_page.bootc_status_group`; `updates_page.nbc_updates_group` → `updates_page.bootc_updates_group`. Update `config.yml`, both example configs (currently identical copies — they stay identical apart from the rename), and `CONFIG.md`. No back-compat aliasing for the old keys: groups default to enabled when unspecified, so stale configs degrade gracefully (the group simply follows the gate).

## 2. Homebrew trust UI

### Detection

New in `internal/homebrew`:

```go
type UntrustedTap struct {
    Name     string   // e.g. "ublue-os/tap"
    Packages []string // installed formulae/casks fully qualified, e.g. "ublue-os/tap/foo"
}
ListUntrustedTaps() ([]UntrustedTap, error)  // brew tap-info --installed --json → "trusted": false
TrustPackages(tap UntrustedTap) error        // brew trust --formula <user/tap/name> (or --cask) per installed package
```

CLI verified against brew 6.0.8 on-host: `brew trust` supports `--tap`, `--formula`, `--cask` targets and writes a per-user trust store (`~/.homebrew/trust.json`) — no root or pkexec involved. Chairlift trusts per-package rather than per-tap, matching Homebrew's own guidance ("Prefer trusting only the specific formulae, casks or commands you need"). Installed packages per tap come from cross-referencing the `tap` field in the existing `brew info --installed` data against untrusted tap names.

Only taps that both are untrusted *and* have installed packages appear — an untrusted tap with nothing installed is noise.

### UI

Updates page gains an **"Untrusted Taps"** preference group, built with the deferred-visibility pattern (hidden until the async `ListUntrustedTaps()` completes; hidden entirely when the list is empty). Each tap is an `ActionRow`: title = tap name, subtitle = its installed packages, suffix = **Trust** button. The button opens an `adw.AlertDialog` stating that trusting allows the tap's code to run during installs/upgrades, with Cancel/Trust (destructive-appearance) responses. On confirm: run the trust command(s) in a goroutine, toast the result, remove the row, and refresh the outdated-packages list.

### Error handling on existing paths

`Upgrade()`/`Update()` output containing "taps are not trusted" produces a specific error the views translate to: *"Some packages come from untrusted taps — see the Untrusted Taps section below."* instead of the raw brew dump.

## 3. updex dependency bump

`github.com/frostyard/updex` v1.2.0 → v1.2.3 (pulls in the deterministic-version-selection fix and the public package restructure). Verified: chairlift HEAD compiles against v1.2.3 unchanged, and the v1.2.x library API works on-host. `chairlift-updex-helper` is rebuilt against the same version. No UI changes.

## 4. Startup verification

- Add `log.Printf` timing marks (behind existing logging, no new flag) at: app start, config loaded, window constructed, each page built, window presented.
- Audit the construction path (`app.New` → `window.New` → `views.New` → `build*Page`) for any synchronous `exec.Command`, file IO on network mounts, or blocking library calls. Known-good pattern (goroutine + `IsInstalledCached` + `RunOnMainThread`) already covers snap/brew/flatpak/updex groups; the bootc rewrite uses it from day one (`GetStatus` runs async after the window shows).
- **Acceptance:** no subprocess executes synchronously before the window is presented; window paint is not gated on any tool check.

## Testing

- **Unit:** `internal/bootc` status parsing against JSON fixtures (booted+staged, booted-only, all-null non-bootc, malformed); stage-script event streaming with a fake script; `ListUntrustedTaps` parsing against `brew tap-info --json` fixtures (trusted, untrusted-with-packages, untrusted-empty); untrusted-tap warning detection in upgrade output.
- **Manual on this (nbc, non-bootc) host:** app builds and starts fast; bootc groups absent without errors; Untrusted Taps group lists real taps and Trust works; Features tab loads and toggles.
- **Not covered here:** live bootc staging needs a bootc-booted VM — deferred to post-merge validation on a snosi image.
- `make build`, `make lint`, `go test ./...` green.

## Documentation

Update for the nbc→bootc swap and new brew trust flow: `yeti/OVERVIEW.md`, `yeti/package-managers.md`, `CONFIG.md`, `README.md` (also fix its stale Meson/Python build section while touching it), example configs.

## Out of scope / follow-ups

- **Cut release (v0.8.0) and upgrade hosts after merge.** Two of the four reported symptoms (Features tab, slow startup) are fixed at HEAD but invisible until the new binary ships — this follow-up is what actually resolves them on the host.
- Native `bootc upgrade --progress-fd` progress once the upstream composefs pull bug is fixed.
- Any nbc removal in sibling repos (snosi already migrated; frostyard/nbc archival is not chairlift's concern).
