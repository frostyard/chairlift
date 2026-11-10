# Spec: Opt-in problem reporting (consent, sent log, HTTP request, CLI)

This contract governs what ChairLift sends to frostyard's problem intake
service, when it sends it, and what it records. It applies to phase 2 of plan
T857 ([ADR-0015](../adr/0015-problems-page-and-opt-in-reporting.md)).
Consumers: the package `internal/reporting` (the only sender), the Problems
page of the command center (`internal/views/problems_reporting.go`: the
consent, the sends and the Report dialog; `internal/views/problems.go`: the
first load of the consent and the sent log; `internal/views/problems_view.go`:
the reporting card), the first-run Reporting step
(`internal/window/firstrun_reporting.go`), the windowless mode
`chairlift --send-pending-reports`, and `internal/views/actionmsg`'s
`Report`/`ReportingConsent` decisions.

Nothing leaves the computer unless every rule below allows it.

## Interface

### Consent file (per user)

`$XDG_CONFIG_HOME/chairlift/reporting.json`, else
`$HOME/.config/chairlift/reporting.json` (only absolute values count;
`reporting.ConsentPath`):

```json
{
  "format": 1,
  "enabled": true,
  "mode": "auto",
  "decided_at": "2026-11-04T10:00:00Z",
  "schema": "frostyard.problem/1"
}
```

| Field | Type | Required | Constraints |
| --- | --- | --- | --- |
| `format` | integer | yes | `1`; any other value is an error, and reporting is off |
| `enabled` | boolean | yes | off is the default |
| `mode` | string | yes | `ask` or `auto`; any other value is an error, and reporting is off |
| `decided_at` | RFC 3339 | yes | when the user decided, UTC |
| `schema` | string | yes | the schema version the user agreed to |

A missing file is the zero consent (off) with no error. A file that cannot be
read, is larger than 64 KiB, or fails a rule above reads off plus an error.
`reporting.SaveConsent` writes a temporary file in the same folder (0600) and
renames it; the folder is made with 0700.

**The write is synchronous.** The consent file is about 200 bytes, so the page
(`Problems.applyReportingConsent`) and the first-run step
(`FirstRun.onReportingSwitched`) write it on the UI thread with
`reporting.SaveConsent`: a small local file write, not an external tool call,
so the UI-thread rule allows it. An asynchronous writer is rejected: it
brought a stale callback that could restore an older choice, a queued Off that
an exit lost, and 2 consent values that could disagree. 1 value in memory is
the only authority. Because the write is synchronous, an exit cannot lose an
Off, and no callback can restore an older value. While the page applies a
consent it sets a guard (`reportingApplying`), so nothing it redraws can start
a second save.

**Withdrawal before the save, grant after it.** A change that withdraws (Off,
or leaving automatic while automatic was in effect) sets the in-memory value
BEFORE `SaveConsent` runs, so a send stops at once even while the save is in
flight. A change that grants (On, or automatic) sets it only after a
successful save. A failed save of a withdrawal keeps the withdrawal in memory
(fail safe) and shows an error toast; a failed save of a grant keeps the old
value, which the controls go on showing.

**Reading it again.** `reporting.FileConsent(path)` returns a function that
reads the file on every call, and a read error is off. A live send uses it
before every request, because 1 ChairLift process can write the consent while
another sends (the first-run window and the main window can run at the same
time). The live consent is `reporting.LiveConsent(inMemory, fromFile)`: it
reads the file FIRST and memory second (a withdrawal made while the file read
blocks must count), then applies `reporting.Stricter`, which allows only when
both allow, and sends automatically only when both do, so a
local Off stops a send at once and another process can only make it stricter.
A dry run uses the in-memory value alone.

**The switch waits for the load.** The switch and the mode control are drawn
disabled until the first consent load published its value
(`reportingLoaded`, set only by that load); the handlers ignore a change
before that, so a slow load cannot overwrite a newer choice.

**An automatic run starts on a new automatic choice.** `reporting.StartsAutoRun(before, after)`
is true when `after` allows and sends automatically and that was not already
in effect. Both the live path and the dry-run path call `Run` then; under
`--dry-run` that only logs the previews.

The switch shows `Consent.Allows()`, not `Consent.Enabled`, and
`reporting.SwitchChanged(stored, active)` decides whether the user changed it.
A stale consent therefore shows off, and turning the switch on renews it.

**Data generations.** `dataGeneration` maps each schema version to a data
generation. `frostyard.problem/1` is generation 1. A later schema that adds a
kind or a field adds data, so it gets a new number: a consent for an older
generation stops sending until the user agrees again (`Consent.Stale`). A
later schema that only adds enum values keeps the generation, so it keeps
sending. `Consent.Allows` is `enabled` AND the agreed generation equals the
current one.

### Sent log

`$XDG_STATE_HOME/chairlift/reports.json`, else
`$HOME/.local/state/chairlift/reports.json` (`reporting.StatePath`):

```json
{
  "format": 1,
  "sent": [
    {
      "report_id": "3f2a1b4c-5d6e-4f70-8a9b-0c1d2e3f4a5b",
      "kind": "unit_failed",
      "problem_id": "27d5ac0722e592bf",
      "week": "2026-W45",
      "sent_at": "2026-11-04T10:00:00Z"
    }
  ],
  "count_week": "2026-W45",
  "retry_after": "2026-11-05T10:00:00Z"
}
```

`problem_id` is empty for a count. `retry_after` is omitted while no pause
is in force. `format` is 1. A missing file is an empty log. A file that
cannot be read or parsed is `reporting.ErrStateUnreadable`: the automatic
path then sends nothing, because it cannot enforce the caps or record a send.
The log keeps at most 500 entries and drops every entry older than 90 days
(oldest first). `State.Save` uses the same temporary-file rule as the consent.

### HTTP request

| Element | Value |
| --- | --- |
| Method, target | `POST <endpoint>`; the default endpoint is `https://report.frostyard.org/v1/report` (`reporting.DefaultEndpoint`) |
| `Content-Type` | `application/json` |
| `User-Agent` | none (the request sets it to the empty string) |
| Body | exactly the bytes `Report.Body`; at most 8191 bytes |
| Answer | 2xx is success; every other answer is `*reporting.StatusError{Code}`; a redirect is not followed (`http.ErrUseLastResponse`) |
| Endpoint | `HTTPSender.Endpoint`; an empty value is `ErrNoEndpoint`, and no request is made |
| Timeout | 15 s per request |

The intake API answers `202 {"status":"accepted"}` and fails with 503 (kill
switch), 429 (rate limit), 415, 413, 400, 502 or 500. A 429 means at most 10
requests in 60 s per IP: the pace between 2 requests is 7 s in production
(`sendPace`), 0 in tests.

### Endpoint configuration

`problems_reporting_group.endpoint` in a configuration file overrides the
default endpoint. Only `/etc/chairlift/config.yml` and
`/usr/share/chairlift/config.yml` are administrator-owned; `Config.ReportEndpoint`
returns the value only for those 2 files and ignores it, with 1 log line, from
the user-writable development fallback. `reporting.ParseEndpoint` accepts an
absolute `http` or `https` URL with a host, no user information and no
fragment; an empty value is `DefaultEndpoint`. Only `ParseEndpoint` chooses
the default: a sender with an empty endpoint sends nothing, so a value that
fails to parse can never fall back to the public service. The page then
disables every send path (no automatic run, no Report… button, "Send
pending reports" disabled, and `pageview.ReportingEndpointInvalid` as the reporting
card's description), and `Run`
with a nil `Sender` returns `Skipped` "no endpoint".

### Reports

`reporting.ForItem(item, client)` returns the report of a reportable item: a
new `report_id` (`problems.NewReportID`), the v1 document of
`problems.BuildPayload`, indented with 2 spaces when that stays under 8192
bytes, and else compact. Both builders encode with `SetEscapeHTML(false)`, so
the count's `"<1w"` and `">=6m"` stay literal in the dialog and in the
request. `Report.Text()` is `string(Report.Body)`: the dialog
shows exactly the bytes that are sent. `reporting.NewCountReport` builds the
weekly count the same way, with `reporting.BuildCount`.

The count detail holds 2 fields only: `install_age` (`<1w`, `<1m`, `<6m`,
`>=6m`, from the modification time of `/etc/machine-id`) and `channel`
(`latest` when the bootc image reference's tag is `latest`, else `other`).
The week of a report is the ISO week of the send time in UTC
(`problems.ISOWeek`). Only the modification time of `/etc/machine-id` is
read, never its content.

## Rules

1. **Kind `crash` is never sent.** `ForItem` refuses it with
   `reporting.ErrLocalOnly`, `HTTPSender.Send` refuses it before any network
   call, and the page offers no Report button for it.
2. **The order of 1 send is reserve, check, send, settle.** Both paths
   reserve: they put the entry (or the count week) in the sent log **and** a
   conservative pause (`retry_after = now + 24 h`) in the same document, and
   save it **before** the request. If that save fails, nothing is sent and the
   error is returned. Then the consent is read once more, immediately before
   the request; a withdrawal rolls the reservation back (entry, count week and
   pause), saves, and stops without a failure and without a pause
   (`SendReport` returns `ErrNoConsent`). After the request the log settles: a
   success restores the earlier `retry_after` and saves; a failure forgets the
   entry, keeps the pause, and saves. A failed settle save leaves the entry and
   the pause on disk, which only under-sends: a later run then sends nothing
   for 24 hours, and no report is sent twice. A process that exits during a
   request leaves the same reservation behind.
3. **The automatic path (`reporting.Run`), in this order.** The group is off
   → nothing, `Skipped` "group off". The consent does not `Allows` → "consent".
   The mode is not `auto` → "ask mode". No release client string → "not a
   release build". The send lock is held → "busy". The sent log is unreadable
   → "state unreadable". `retry_after` is in the future → "paused until …".
   Otherwise it sends the pending reports in item order: `Reportable`, kind
   not `crash`, not `WasSent(problemID, week)`, at most `Remaining` (20 minus
   the entries of the last 24 hours). Then the count, when `CountDue` (this
   ISO week was not counted), the envelope is valid, and the install info is
   known. Before each request it reads the consent again and stops when it no
   longer allows sending.
4. **The first failed send pauses everything for 24 hours.** `State.Fail`
   sets `retry_after` to now + 24 h, the log is saved, the run stops, and
   `RunResult.Err` carries the failure. Every success is recorded and saved at
   once.
5. **The manual path (`reporting.SendReport`) needs the group, a consent that
   `Allows` (either mode), a release client, a kind that is not `crash`, and
   `Remaining > 0` (else `ErrDailyLimit`), and takes the lock. It ignores a
   pause: the user asked for this report. An unreadable sent log stops it too
   (`ErrStateUnreadable`): the cap cannot be checked and the log must not be
   overwritten. A failure sets `retry_after` too. It never runs under
   `--dry-run` (`ErrDryRun`); the caller decides that with `actionmsg.Report`.
6. **1 lock per host.** `reporting.Lock` takes a non-blocking exclusive
   `flock` on `<state folder>/reports.lock`. A second holder gets
   `reporting.ErrBusy`: the GUI and `chairlift --send-pending-reports` never
   send at the same time.
7. **`--dry-run` changes nothing** (ADR-0009). `Run` with `DryRun` builds
   every report, logs `[DRY-RUN] would send report <kind>` for each, returns
   them in `RunResult.Previews`, calls no sender, reads and writes no state and
   takes no lock. `actionmsg.Report(true, kind)` has `Send=false` and
   `MarkSent=false`; `actionmsg.ReportingConsent(true, …)` has `Persist=false`.
8. **The windowless mode.** `chairlift --send-pending-reports` runs before
   `app.Run`, so no window exists and no GTK library is loaded (MyGo loads
   GTK only in `App.Run`). A configuration error prints 1 line and
   exits 1. The group off, a consent that does not `Allows` (including a stale
   schema), ask mode, or a version that is not a release prints 1 line and
   exits 0: nothing is sent. It loads the user's dismiss store
   (`problems.SeenPath`, `problems.LoadSeen`) and passes it to
   `problems.Load`, so it sends no problem the user dismissed; a store that
   cannot be read sends nothing and says why. Otherwise it runs `Run` and
   exits 1 when a send failed, else 0. It reads the consent file again before
   each request, so a withdrawal stops the run; a read error counts as off.
   `--dry-run` prints each preview's `Text()` to stdout and sends nothing.
9. `internal/reporting` is the only package whose production code imports
   `net/http` and names `report.frostyard.org`. `internal/problems` imports
   neither, and does not import `internal/reporting`.
10. A report body is under 8192 bytes and validates against the vendored
   `frostyard.problem/1` schema (`internal/problems/schemacheck`).

## Derived artifacts

| Artifact | Derivation |
| --- | --- |
| The enums of the count detail | `internal/reporting/envelope.go`, compared with the vendored `v1.json` by the package's tests |
| The vendored schema copy | `internal/problems/testdata/schema/v1.json` (sha256 pinned there); `internal/problems/schemacheck` validates bodies of both packages |

## References

- Rationale: [ADR-0015](../adr/0015-problems-page-and-opt-in-reporting.md),
  [ADR-0009](../adr/0009-dry-run-output-convention-and-single-decision-structs.md)
- Contract of the values: [problems-page.md](problems-page.md),
  [core ADR-0069](https://github.com/frostyard/core/blob/main/docs/adr/0069-report-problems-local-first-opt-in-with-one-sender-per-host.md)
- Context: [design/overview.md — Problems page](../design/overview.md#problems-page),
  [CONFIG.md](../../CONFIG.md)
