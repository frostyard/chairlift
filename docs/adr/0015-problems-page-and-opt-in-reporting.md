# 0015 — Show local problems on a Problems page; report only after opt-in, later

- **Status:** Accepted
- **Date:** 2026-11-05

## Context

Problems on installed snosi hosts are invisible to the user and to the
project: a failed image unit, an update that never stages, a TPM unlock that
needs retries, a failed sysext update, a composefs or verity error, and the
crashes of programs and frostyard apps show only in a journal that nobody
reads. frostyard/core
[ADR-0069](https://github.com/frostyard/core/blob/main/docs/adr/0069-report-problems-local-first-opt-in-with-one-sender-per-host.md)
and its spec
[problem-report.md](https://github.com/frostyard/core/blob/main/docs/specs/problem-report.md)
fix the schema `frostyard.problem/1` and the rule: local first, opt-in, one
sender per host. Plan T857 gives ChairLift the desktop side.

Facts that constrain it:

- snosi's collector `snosi-health` (snosi ADR-0029) writes
  `/var/lib/snosi-health/problems.json` (mode 0644, only schema values) and
  `details.json` (mode 0640 root:sudo, local journal text). It leaves out
  crashes and app records of users from UID 1000 up: ChairLift reads those
  from the user's own journal (one sender for each record).
- Anyone can write `FROSTYARD_*` journal fields with `systemd-cat`, and
  anyone who can write a file can put any value in it (core spec rule 4).
- Most ChairLift hosts are not snosi hosts; they have no snosi-health file.
- `internal/views` cannot host a test (ADR-0007); `--dry-run` must change
  nothing (ADR-0009); configuration errors fail closed (ADR-0003).
- frostyard/std v0.4.0 ships `problems`, the Go writer and reader of the
  journal records (std ADR-0002).

## Decision

1. **A 7th page, "Problems".** `internal/navigation` lists `problems`
   (icon `dialog-warning-symbolic`, config page `problems_page`) directly
   before Help, with 3 groups: `problems_system_group`,
   `problems_apps_group`, `problems_reporting_group`, each enabled by
   default and each able to be turned off. The existing rule omits the page
   when all 3 are off; a configuration error turns them off (ADR-0003).
   Help moves from Alt+6 to Alt+7 when every page is visible.
2. **Local until opt-in.** The page shows records on this computer, and it
   sends nothing until the user turns reporting on. Phase 1 showed 1 row,
   "Problem reports stay on this computer"; phase 2 (T857 t19) replaces it
   with the opt-in sender of decision 10. `internal/problems` still imports
   no network package: a test lists its dependencies and fails on `net`,
   `net/...` or `crypto/tls`. `internal/reporting` is the only sender.
3. **One pure package owns the data.** `internal/problems` (no puregotk)
   reads problems.json (format 1, at most 1 MiB) and details.json (at most
   4 MiB, read only when readable) at fixed paths that no user setting can
   change; runs `coredumpctl --json=short --no-pager --since=-90d` and keeps
   the user's own UID; reads the S8 records of the user journal by running
   journalctl itself (std `problems.Read`'s arguments) and validating them
   with std's `problems.ParseJSON`. std's `Read` keeps all of journalctl's
   output in memory, and the 500-entry limit does not bound bytes, so
   ChairLift does not call it: it reads at most 8 MiB of journalctl output
   and 4 MiB of coredumpctl output, kills the command at the cap, and keeps
   at most 4 KiB of standard error. Commands run with a 10-second timeout.
   Each list holds at most 200 items. The outcome of problems.json
   is 1 of 4 (`SourceMissing`, `SourceUnreadable`, `SourceMalformed`,
   `SourceOK`); a missing file shows 1 row, "System checks are not available
   on this system".
4. **ChairLift does not trust its inputs.** Every detail value is checked
   against the v1 rules again. A value that fails is shown, marked not
   valid, and the item is not reportable. A number or boolean given as JSON
   text (`"retries": "1"`) is the wrong type and fails too. `Reportable` is
   the file's flag
   AND ChairLift's own check AND a valid envelope (`image.id`,
   `image.version`). An unknown kind is shown as "An unknown problem was
   recorded" and is never reportable. details.json, `MESSAGE` and paths are
   local display text only and never enter a payload. `BuildPayload`
   copies only validated schema fields; phase 1 does not call it.
5. **One crash, one sender.** A system crash and a user crash with the same
   program and signal within the system record's time window (±1 minute)
   show once, in the apps group.
6. **Dismiss is per user and comes back.** Dismissed ids live in
   `$XDG_STATE_HOME/chairlift/problems-seen.json` (mode 0600, at most 1000);
   a dismissed problem shows again when it happens after the dismissal.
   Under `--dry-run` Dismiss writes and hides nothing (ADR-0009,
   `actionmsg.ProblemDismiss`).
7. **ChairLift records its own problems.** `cmd/chairlift` calls
   `selfreport.Init(version.Version)` (std `problems.New` for
   `org.frostyard.ChairLift`, component `gui`, plus `CrashOutput`) and
   defers `RecoverPanic` in `main`. A dev or snapshot version is not a
   release semver, so its recorder is inert. 4 coded internal errors are
   recorded at their real paths: `config.load_failed`,
   `problems.source_invalid`, `firstrun.complete_failed`,
   `updex.results_unreadable`. A journal record is logging, so it is also
   written under `--dry-run`.
8. **English strings.** The page's text is English, like the other 6 pages.
   `internal/i18n` stays the first-run catalogue only (ADR-0014).
9. **A dry-run fixture hook.** `CHAIRLIFT_PROBLEMS_FIXTURES=<absolute
   folder>` replaces every source, only with `--dry-run`, so `make e2e`
   builds the page from fixtures and waits for the log line
   `problems: page built system=<n> apps=<n> source=<outcome>` (ADR-0008).

10. **Opt-in reporting (phase 2).** `internal/reporting` is the only sender
    (the only production importer of `net/http`); its contract is
    [specs/problem-reporting.md](../specs/problem-reporting.md).
    - **Per-user consent.** `$XDG_CONFIG_HOME/chairlift/reporting.json`
      (0600) records `enabled`, `mode` (`ask` or `auto`), the decision time
      and the schema version the user agreed to. It is off until the user
      turns it on, and a broken file reads as off. A schema that adds a kind
      or a field is a new data generation: sending stops until the user
      agrees again (a schema that only adds enum values keeps sending).
    - **The dialog shows exactly what is sent.** The Report dialog shows
      `Report.Text()`, and Send posts the same `Report.Body` bytes: one
      buffer, so no second rendering can differ. The body is under 8192
      bytes and validates against the vendored `frostyard.problem/1` schema.
    - **Ask each time, or send automatically.** In Ask mode only the dialog
      sends. In Auto mode the page and `chairlift --send-pending-reports`
      send the pending reports, at most 20 in 24 hours, paced at 7 seconds
      apart (the intake API allows 10 a minute), and each problem once per
      week. The weekly count (install age bucket and update channel only)
      goes at most once per ISO week. These are 2 separate limits: the count
      does not use the 20-report budget.
    - **A small file, written at once, and a send in 4 steps.** The consent
      file is about 200 bytes, so the page and the first-run step write it
      synchronously on the GTK main thread, with 1 in-memory authority: a
      failed write keeps the value and shows an error, and no queued write can
      restore an older choice. It is read again from the file before every
      live request, because another ChairLift process may have turned
      reporting off. 1 send reserves the entry (or the count week) together
      with a conservative pause, saves, reads the consent once more, and then
      sends; a withdrawal rolls the reservation back and stops without a
      failure, and the log settles after the request. A process that exits
      during a request therefore under-sends, never sends twice.
    - **A failed send waits 24 hours.** The first failure pauses every
      automatic send for 24 hours, and the next run says why. An explicit
      Report dialog send ignores the pause: the user asked for it, and the
      dialog refuses a report this week's log already holds.
    - **One sender per host.** A non-blocking lock (`reports.lock`) keeps the
      window and the command line apart.
    - **Kind `crash` is never sent in phase 2.** The sender refuses it before
      any network call, and the page offers no Report button for it.
    - **Admin-only endpoint, and no fallback.** `problems_reporting_group.endpoint`
      counts only from `/etc/chairlift/config.yml` or
      `/usr/share/chairlift/config.yml`; the user-writable development
      fallback is ignored with 1 log line. Only `reporting.ParseEndpoint`
      chooses the default: an endpoint that fails to parse disables every
      send path and never falls back to the public service.
    - **A windowless mode and an optional first-run step.** `chairlift
      --send-pending-reports` runs the automatic send and exits, with no
      window and no GTK; `--dry-run` prints the reports and sends nothing.
      The first run shows the optional Reporting step between Style and Apps
      when the reporting group is on and the system problem file exists
      (ADR-0014's steps and markers do not change).
    - **Dry run changes nothing** (ADR-0009): `actionmsg.Report` and
      `actionmsg.ReportingConsent` gate the toast and the write together.

## Consequences

- A snosi user sees system and app problems with a plain-words title, a
  hint, and local details to copy, without a terminal.
- The page reads a root-written file and the journal: forged or broken
  values are a normal input, and every one of them is tested.
- The schema is a vendored byte copy with a pinned sha256; a new schema
  version needs a new copy, new enums and new hints, and the hint test
  fails until every enum value and known code has a hint.
- A user who turns reporting on decides between a per-report dialog and an
  automatic send, sees every sent report with its `report_id`, and stops
  sending by turning the switch off; nothing is sent before that.
- A schema change that adds data needs a new agreement, so an upgrade never
  widens what a user already agreed to.
- The intake service is a network dependency of 2 code paths (the page and
  the command line); both share `reporting.Run`, so they cannot drift.
- Kind `crash` stays local until a later task agrees its own consent text.
- `github.com/frostyard/std` becomes a direct dependency (v0.4.0).
- The 7th page moves Help's Alt+number; docs that name Alt+6 for Help are
  wrong.

## Alternatives considered

- **Trust problems.json's `reportable` flag:** rejected — the file is the
  input that must be checked; a second check costs little and keeps one
  rule in the sender.
- **Show the dropped forged journal records:** rejected — a user who can
  write to their journal can fill the page; the count goes to the log.
- **A configurable path for problems.json:** rejected — a user setting must
  not choose the file whose values a later sender may report.
- **Send reports in phase 1:** rejected — consent, the sender and the intake
  service were later tasks; phase 1 had to be safe on its own. Phase 2 adds
  them behind the opt-in of decision 10.
- **Render the report twice (once for the dialog, once for the request):**
  rejected — 2 renderings can differ, and then the dialog lies. One buffer is
  shown and posted.
- **A machine-wide or image-level consent:** rejected — each user decides,
  and the consent file is per user (core ADR-0069).
- **Translate the page through `internal/i18n`:** rejected for now — the
  main window has no catalogue; translating one page alone would mix
  languages.

## References

- Shapes: [specs/problems-page.md](../specs/problems-page.md),
  [specs/problem-reporting.md](../specs/problem-reporting.md),
  [design/overview.md — Problems page](../design/overview.md#problems-page),
  [CONFIG.md](../../CONFIG.md)
- Builds on: [ADR-0003](0003-two-tier-config-with-fail-closed-semantics.md),
  [ADR-0007](0007-pure-leaf-packages-route-around-untestable-gtk.md),
  [ADR-0008](0008-e2e-readiness-is-a-log-marker-contract.md),
  [ADR-0009](0009-dry-run-output-convention-and-single-decision-structs.md),
  [ADR-0014](0014-first-run-mode.md),
  [core ADR-0069](https://github.com/frostyard/core/blob/main/docs/adr/0069-report-problems-local-first-opt-in-with-one-sender-per-host.md)
- Enforced by: `internal/problems` tests (schema pin and subset validator,
  forged values, hints, merge, dismiss, no network imports),
  `internal/selfreport` tests, `internal/views/pageview/problems_test.go`,
  `internal/views/pageview/reporting_test.go`,
  `internal/views/pageview/wiring_test.go`,
  `internal/views/actionmsg` `TestProblemDismiss`,
  `internal/views/actionmsg/reporting_test.go`, `internal/navigation` tests,
  `internal/config/problems_config_test.go`,
  `internal/config/reporting_config_test.go`,
  `internal/installcheck/selfreport_test.go`,
  `internal/installcheck/reporting_test.go` (only `internal/reporting`
  imports `net/http` or names the intake host),
  `internal/firstrun/reporting_test.go`,
  `internal/reporting` tests (0 requests without consent, the 20-a-day
  budget, 1 count a week, the schema generation, the 24-hour pause, the
  never-sent crash kind, the exact bytes, no User-Agent, no redirect, the
  lock), and `TestApplicationStartsInDryRun` with
  `TestSendPendingReportsDryRunNeedsNoWindow` in `test/e2e/e2e_test.go`
- Plan: T857 (problem reports), tasks t6 and t19; snosi ADR-0029 (the
  collector)
