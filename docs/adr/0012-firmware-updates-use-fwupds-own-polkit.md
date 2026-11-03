# 0012 — Firmware updates use fwupd's own PolicyKit actions

- **Status:** Accepted
- **Date:** 2026-11-03

## Context

The Updates page lists device firmware updates. The Linux Vendor Firmware
Service (fwupd) owns this work: `fwupdmgr` is an unprivileged client that talks
to the `fwupd` daemon over the system bus, and the daemon performs the flash.

Writing firmware needs root, so the option to route the install through
ChairLift's own privileged path looks natural. It would be wrong here:

- The daemon already holds the privilege. A second privileged layer would do
  the same job twice.
- fwupd ships its own PolicyKit actions (`org.freedesktop.fwupd.update-internal`,
  `org.freedesktop.fwupd.update-hotplug`, and others), and the daemon selects
  the action for the device it is about to flash. ChairLift cannot make that
  choice: only fwupd knows whether a device is internal or hotplug.
- A `pkexec` helper of our own would have to reimplement device selection and
  would widen the privilege boundary in
  [ADR-0001](0001-fixed-path-pkexec-privilege-boundary.md) for no gain.

`fwupdmgr update` can write into the ESP (UEFI capsule updates) and can require
a restart or a shutdown. ChairLift has no terminal, so fwupdmgr cannot ask its
own questions. After the flash fwupdmgr asks about a restart, and an answer that
assumes yes would restart the computer at once.

fwupdmgr also lists devices that it will not flash now: a closed laptop lid, a
host running on battery, or a device with a recorded update error. For such a
device `fwupdmgr update` skips the work and still exits 0.

## Decision

Firmware installs run the unprivileged client:

```
fwupdmgr update --no-reboot-check <DeviceId>
```

Authorization comes from fwupd's own PolicyKit actions. The `fwupd` daemon asks
the session's PolicyKit agent for authentication, exactly as it does for a
command typed in a terminal. ChairLift ships no PolicyKit policy file and no
helper binary for firmware, adds no pkexec invocation, and never learns a root
path.

Five constraints make that safe and unambiguous:

1. **The device id is validated before it reaches argv.** `internal/fwupd`
   accepts only `^[0-9a-f]{40}$`, the id format fwupd itself enforces.
   PolicyKit does not inspect the arguments of the daemon's actions, so an
   option such as `--force` must be rejected by ChairLift rather than passed
   on. Any other value is an error and no command runs.
2. **`--no-reboot-check` is always passed.** fwupdmgr must not ask about a
   restart and must not start one. The view asks the user first, in a dialog
   that warns not to turn off or unplug the computer and that says ChairLift
   will not restart it.
3. **One device at a time.** The list offers the install for a single device;
   mass updates and unattended installs are not part of the UI. A page-wide
   `actionstate.Gate` also lets only one install run at any moment, so two
   confirmations cannot start two flashes together.
4. **Install is offered only for a device fwupd will flash now.** A device is
   installable only when its flags contain `updatable`, its `Problems` array is
   empty, and its `UpdateError` is empty. Any other device stays in the list
   with no install control and a visible reason, because fwupdmgr would skip it
   and exit 0. ChairLift must not turn that exit into a success message.
5. **Every install is verified before ChairLift reports it.** The device state
   can change while the confirmation dialog is open, so after `fwupdmgr update`
   ChairLift runs the update query again and classifies the result
   (`fwupd.VerifyInstall`): a device that no longer has the update, or that now
   has a different version, is verified; the same release still offered is not
   applied, with the device's own reason when fwupd reports one; a failed query
   is unverified. Only a verified result is reported as an install. A published
   list is always replaced whole, never patched by removing one row, so a device
   that still has an update remains visible and the badge is the new count.

The group `firmware_updates_group` is off by default. It is built hidden and
shown only when fwupdmgr is present. It stays disabled in the shipped
configuration until a real hardware test passes, because a firmware flash is
the least forgiving operation in the application.

## Consequences

Easier: no new privileged artifact to package, review, or attest. ChairLift
inherits fwupd's authorization policy, its device support, and its own
progress handling, and the firmware path cannot drift from the OS vendor's.

Harder: ChairLift cannot report authentication failures differently from other
fwupdmgr failures; both arrive as a non-zero exit with stderr. The dialog's
restart warning is the only protection against a device that needs a restart
after the flash, so it must stay in the confirmation body.

Harder: a zero exit says only that fwupdmgr finished, not that a device was
flashed. fwupdmgr can exit 0 without a flash, so ChairLift offers Install only
for installable devices and verifies each install with a second query. The
published list is the whole verified list, so a device that still has an update
appears again, and a failed verification is reported as an error instead of a
success.

New obligations: the device id validator must not be relaxed,
`--no-reboot-check` must stay in the argument list, the verification query must
stay after every install, and the installable test (flag, problems, update
error) must stay in step with fwupd. A future change
that adds a privileged firmware path must first supersede this ADR.

## Alternatives considered

- **A `pkexec` helper for `fwupdmgr update`:** rejected. It duplicates the
  daemon's privilege, must reimplement device selection, and broadens the
  fixed-path boundary of ADR-0001.
- **A ChairLift PolicyKit policy that allows `fwupdmgr` without
  authentication:** rejected. ChairLift ships no passwordless rules; firmware
  installation must authenticate like any other administrative action.
- **`fwupdmgr update` without `--no-reboot-check`:** rejected. After the flash
  fwupdmgr asks about a restart, and with an assume-yes flag it would restart
  the computer at once. ChairLift has no terminal to answer that question, and
  the decision to restart belongs to the user.

## References

- Shapes: [design/package-managers.md](../design/package-managers.md),
  [design/overview.md](../design/overview.md), [reference.md](../reference.md)
- Builds on: [ADR-0001](0001-fixed-path-pkexec-privilege-boundary.md) (fixed
  path pkexec boundary), [ADR-0009](0009-dry-run-output-convention-and-single-decision-structs.md)
  (dry-run convention and single decision structs)
