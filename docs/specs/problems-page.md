# Spec: Problems page sources, validation, dismiss store and log marker

This contract governs what ChairLift's Problems page (ADR-0015) reads, how
it validates each value, what it shows, what it writes, and the log line it
prints when it is built. Consumers: the page builder
`internal/views/problems_page.go`, the pure package `internal/problems`, the
presentation in `internal/views/pageview/problems.go`, the dismiss decision
in `internal/views/actionmsg/problems.go`, the e2e test, and the opt-in
sender (`internal/reporting`), which calls `problems.BuildPayload`. Nothing is
sent until the user turns reporting on; what is sent is
[problem-reporting.md](problem-reporting.md).

## Interface

### Sources

| Source | Read by | Limit | Absent or failing |
| --- | --- | --- | --- |
| `/var/lib/snosi-health/problems.json` (format 1, snosi ADR-0029) | `problems.ReadSystem` | 1 MiB (`MaxProblemsBytes`) | see the outcomes below |
| `/var/lib/snosi-health/details.json` (format 1, mode 0640 root:sudo) | `readDetails`, only for system items | 4 MiB (`MaxDetailsBytes`) | missing or EACCES: no details, no error; other failure: a log line, no details |
| `coredumpctl --json=short --no-pager --since=-90d` | `problems.RunCoredumpctl`, `ParseCoredumps` | 10 s (`CommandTimeout`); 4 MiB of output, then killed; 4 KiB of stderr | not installed, or "No coredumps found" (exit 1): an empty list; output at the cap or another failure: a log line |
| User journal, S8 records | `journalctl --no-pager -q -o json --all --user --since=@<now − 90 days> -n 500 MESSAGE_ID=8915bdd215c84cbab0c31c05a1a33e7e` (`problems.JournalArgs`, std `problems.Read`'s arguments), run by `readJournal`; the output goes to std `problems.ParseJSON` | 10 s; 8 MiB of output (`MaxJournalOutput`), then killed; 4 KiB of stderr | failure: a log line; at the cap: the complete lines before it are parsed and 1 log line says so |

Each command runs in its own process group (`execStream`). At the output
cap, and when the 10-second timeout ends, the whole group gets SIGKILL, so a
descendant that inherited stdout or stderr dies too; ChairLift closes its end
of stdout before it waits, waits exactly once, and `WaitDelay` (2 s) bounds
the wait for a descendant that left the group.

ChairLift MUST NOT call std's `problems.Read` (v0.4.0): it keeps all of
journalctl's output in memory, and 500 entries do not bound bytes. No
`--output-fields` is passed: std's `Validate` must see every `FROSTYARD_*`
field to drop a record with an unregistered one.

The two paths are constants (`problems.ProblemsPath`,
`problems.DetailsPath`); no configuration key changes them.

### problems.json outcomes (`problems.SourceOutcome`)

| Outcome | When | System group shows |
| --- | --- | --- |
| `SourceMissing` (the zero value) | the file does not exist | 1 row: "System checks are not available on this system" |
| `SourceUnreadable` | opening or reading fails (permission, I/O, a directory) | 1 row: "System checks could not be read", "See the log for details." |
| `SourceMalformed` | not 1 JSON object, `format` is not 1, `schema` is not `frostyard.problem/1`, `problems` is not a list, or larger than 1 MiB | the same row as `SourceUnreadable`; ChairLift records the coded error `problems.source_invalid` |
| `SourceOK` | the file was parsed | its items; with none, "No problems found" and "Last checked <when>" from `generated` |

Unknown top-level keys are ignored. `generated` that is not RFC 3339 gives
"Last check time unknown". `image.id`, `image.version` and `client` may be
`null`.

### Items (`problems.Item`)

| Origin | ID | Group |
| --- | --- | --- |
| `OriginSystem` | the file's `id` (`^[0-9a-f]{16}$`) | System |
| `OriginUserCrash` | `crash-` + 16 hex digits of sha256(exe path, signal) | Apps |
| `OriginAppRecord` | `app-` + 16 hex digits of sha256(the grouping key) | Apps |

Each list is sorted newest last time first and holds at most 200 items
(`MaxItems`).

### Dismiss store

`$XDG_STATE_HOME/chairlift/problems-seen.json`, else
`$HOME/.local/state/chairlift/problems-seen.json` (only absolute values
count):

```json
{"format": 1, "dismissed": {"27d5ac0722e592bf": "2026-11-05T04:00:00Z"}}
```

### Dry-run fixture hook

`CHAIRLIFT_PROBLEMS_FIXTURES=<absolute folder>` with `--dry-run`: the page
reads `problems.json`, `details.json`, `coredumps.json` (coredumpctl's JSON
output) and `journal.json` (journalctl `-o json` output, through std's
`problems.ParseJSON`) from the folder and runs no command. A missing fixture
file is an empty source.

### Log marker

After the groups are filled, on the GTK main thread, the page logs exactly
1 line from `problems.BuiltMarker`:

```text
problems: page built system=<shown system items> apps=<shown app items> source=<missing|unreadable|malformed|ok|disabled>
```

`source=disabled` when the system group is off. With both data groups off
the line is logged at once with `system=0 apps=0`.

## Rules

1. A problems.json entry is dropped and counted (`SystemFile.Dropped`, a log
   line) when it is not an object, its `id` fails `^[0-9a-f]{16}$`, its
   `kind` fails `^[a-z][a-z0-9_]{0,63}$`, or `first_seen` or `last_seen` is
   not RFC 3339.
2. A `kind` that passes rule 1 but is not 1 of the 9 v1 kinds is shown as
   "An unknown problem was recorded" with the kind name and is never
   reportable.
3. Every detail value is checked against the v1 rules of its kind: type
   (the JSON token type: a number or boolean given as JSON text, such as
   `"retries": "1"`, fails; so do a textual `format`, `occurrences`,
   `reportable` or `secure`, and a textual coredumpctl `time`, `uid` or
   `sig`),
   enum, pattern, `shipped` exactly `true`, `retries` 0 to 10, frames 1 to
   32 of at most 160 characters, `location` at most 128 characters, and the
   S8 rules (class `error` needs `code` and has no frames or location; class
   `panic` has no `code`; runtime `go` has no location; runtime `rust` has
   no frames). A field that fails is shown with its text, marked not valid
   with a reason, and has no value. A field that no rule names is shown and
   is not valid. A missing required field is listed as "not recorded".
4. `Reportable` is true only when the file's `reportable` is `true`, every
   field passed rule 3, no required field is missing, `week` matches the v1
   pattern, and the envelope is valid (`image.id` in the closed enum,
   `image.version` `^[0-9]{14}$`, `secure` a boolean). User items take the
   problems.json envelope; without a valid one they are not reportable.
5. A user crash is reportable only when its path starts with `/usr/`, is
   clean, and its basename matches the v1 `exe` pattern; the payload holds
   the basename. Signal numbers map 4 `SIGILL`, 5 `SIGTRAP`, 6 `SIGABRT`,
   7 `SIGBUS`, 8 `SIGFPE`, 11 `SIGSEGV`, 31 `SIGSYS`, anything else `other`.
   Entries of another UID are dropped. Equal (exe path, signal) entries are
   1 item with a count, the first and the last time.
6. App records are the records that std's reader accepted (it drops forged
   ones; the count goes to a log line only). They are grouped by app ID,
   version, class, code, and the first 3 frames or the location; the newest
   record gives the detail, `MESSAGE`, `_COMM` and the unit. Each item is
   checked by rule 3 again.
7. Merge: a system `crash` item and a user crash item with the same exe
   basename and signal, where a time of the user item lies in
   [`first_seen` − 1 minute, `last_seen` + 1 minute], show once: the user
   item in the Apps group.
8. details.json, `MESSAGE`, paths and trusted journald fields are local
   display text. They appear in the expanded row and in "Copy details"
   (`problems.DetailsText`), never in a payload. Each message is cut to 1024
   characters, at most 5 messages; display text has control characters
   replaced.
9. `BuildPayload(item, reportID, client)` returns `ErrNotReportable` for an
   item that is not reportable, an error for a report ID that is not a
   lowercase UUIDv4 or a client that fails the v1 client pattern, and else a
   `frostyard.problem/1` document under 8 KB with only validated schema
   fields. `NewReportID` uses `crypto/rand`.
10. Dismiss writes by a temporary file and a rename, file mode 0600, folder
    mode 0700, at most 1000 ids (the oldest dismissals go first). An item is
    hidden while its last time is not after its dismiss time. The dismiss
    time is the later of now and the item's last time. A failed write keeps
    the row and shows an error toast.
11. Under `--dry-run` Dismiss writes nothing and hides nothing; the toast is
    `[DRY-RUN] Preview: dismiss <title> — no changes made` and the log line
    `[DRY-RUN] would dismiss problem <id>`.
12. The fixture hook is ignored, with 1 log line, without `--dry-run` or
    with a path that is not absolute (`problems.DecideFixtures`).
13. `internal/problems` imports no network package (`net`, `net/...`,
    `crypto/tls`), directly or through a dependency.
14. ChairLift records its own coded errors only with the codes of
    `internal/selfreport` (`config.load_failed`, `problems.source_invalid`,
    `firstrun.complete_failed`, `updex.results_unreadable`), each at 1 real
    error path, and its panics with `RecoverPanic` deferred in `main`. A
    version that is not a release semver writes nothing.

### Reporting group

The group shows reporting off until the consent file was read off the main
thread. This table is the phase-2 replacement of phase 1's single promise row
("Problem reports stay on this computer"), which must not come back:

| Row | Text (`internal/views/pageview`) | Behaviour |
| --- | --- | --- |
| "Send problem reports" | `ReportingSwitchTitle`, with the group description `ReportingDescription(consent)`: "Off: nothing leaves this computer.", "On: Ask each time." / "On: Send automatically.", or the stale-format text "The report format changed. Turn reporting on to agree again." | An `adw.SwitchRow`; a change writes the consent through `reporting.SaveConsent` off the main thread, and a failed write puts the switch back and shows an error toast |
| "When a problem is found" | `ReportingMode`: "Ask each time" or "Send automatically" | An `adw.ComboRow`, sensitive only while reporting is on |
| "Sent reports" | `SentRows(state, loc)`: the send date and the kind's title, the `report_id` below; newest first, at most 50; "No reports have been sent." when empty | An `adw.ExpanderRow` |
| "What is sent" | `ReportingWhat`: `https://frostyard.org/privacy/problem-reports/` | Opens the privacy page with `xdg-open` |
| "Weekly count" | `ReportingCountRow`: "Weekly count" with `Review…` | Shows in Ask mode while this ISO week's count was not sent; `Review…` opens the same dialog for the count |

A problem row has a **Report…** button when
`pageview.ReportOffered(item, allowed, state)` is true: reporting is allowed,
the item is reportable, its kind is not `crash`, and its week was not sent.
The button builds the report off the main thread (`reporting.ForItem`) and
opens an `adw.AlertDialog` whose body is the report's `Text()` in a
monospace, selectable, scrollable label, with Cancel and Send. Send takes
`actionmsg.Report(IsDryRun(), kind)`: a dry run logs
`[DRY-RUN] would send report <kind>` and toasts a preview; a live send calls
`reporting.SendReport` with the same `Report` value, marks the row "Sent" on
success, and shows an error toast on failure.

In Auto mode, after the groups are filled and when the consent allows it, the
page calls `reporting.Run` off the main thread (pace 7 s) and refreshes the
sent list on the main thread. Under `--dry-run` `Run` logs its previews.

## Derived artifacts

| Artifact | Derivation |
| --- | --- |
| `internal/problems/testdata/schema/v1.json` and `examples/` | Byte copies of core `docs/specs/schemas/problem-report/` at commit `f1b62e0692adc1da6050c6f4733814f06b7476c6`; the sha256 is pinned |
| The enums, patterns and allowlist in `internal/problems/schema.go` | Compared with the vendored `v1.json` by `TestEnumsMatchSchema` and `TestPatternsMatchSchema` |
| The hints in `internal/problems/hints.go` | 1 hint for every value of the hinted enums of `v1.json` and every known code (`TestEveryEnumValueHasAHint`, `TestEveryKnownCodeHasAHint`) |
| `test/e2e/testdata/problems/` | The snosi-health fixture and the user fixtures of `internal/problems/testdata/` |

## References

- Rationale: [ADR-0015](../adr/0015-problems-page-and-opt-in-reporting.md),
  [core ADR-0069](https://github.com/frostyard/core/blob/main/docs/adr/0069-report-problems-local-first-opt-in-with-one-sender-per-host.md)
- Contract of the values: [core spec problem-report.md](https://github.com/frostyard/core/blob/main/docs/specs/problem-report.md)
- Context: [design/overview.md — Problems page](../design/overview.md#problems-page)
