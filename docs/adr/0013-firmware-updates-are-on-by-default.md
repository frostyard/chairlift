# 0013 — Firmware updates are on by default

- **Status:** Proposed
- **Date:** 2026-11-03

<!--
Proposed until Brian's test on real hardware passes (PR body of the change
that adds this ADR). On acceptance, set Status to Accepted, record the date
and the hardware in Context, and set ADR-0012's Status to "Accepted; its
default-off paragraph is superseded by 0013".
-->

## Context

[ADR-0012](0012-firmware-updates-use-fwupds-own-polkit.md) added
`firmware_updates_group` on the Updates page and kept it off by default, in
the built-in defaults and in the shipped configuration files, "until a real
hardware test passes". The reason: a firmware flash is the least forgiving
operation in the application.

The rest of ADR-0012 does not change: the unprivileged
`fwupdmgr update --no-reboot-check <DeviceId>`, fwupd's own PolicyKit actions,
the device id check, one device at a time, Install only for a device that fwupd
will flash now, and the device-state verification after each install.

The group is already safe on a host without fwupd: it is built hidden and shown
only when `fwupd.IsInstalledCached()` is true.

## Decision

`firmware_updates_group` is enabled by default. `defaultConfig()` sets
`Enabled: true`, and `config.yml` and `config.bootc-example.yml` set
`enabled: true`. The only group that stays off by default is
`maintenance_cleanup_group`.

This decision takes effect only after a test on real hardware passes. The test
is a list of devices, a dry-run confirmation, and, at the tester's choice, one
install on a device whose vendor ships firmware through the Linux Vendor
Firmware Service.

This ADR supersedes the last paragraph of ADR-0012's Decision ("The group
`firmware_updates_group` is off by default …") and nothing else.

## Consequences

Easier: hosts with fwupd see firmware updates without a configuration change,
and the badge counts installable firmware with the other providers.

Harder: every host with fwupd now runs `fwupdmgr get-updates --json` at
startup. The call is a read with the 60 s read timeout and changes nothing.

An administrator who does not want the group must set
`firmware_updates_group: enabled: false` in `/etc/chairlift/config.yml`. A
host whose existing configuration file sets `enabled: false` explicitly keeps
the group off: an explicit value always wins over the default.

All obligations of ADR-0012 stay.

## Alternatives considered

- **Keep the group off by default:** rejected after the hardware test. Users
  who never edit their configuration would never see firmware updates.
- **Turn it on only in `config.yml`, not in `defaultConfig()`:** rejected. The
  built-in default and the shipped file must agree, so that a host without a
  file and a host with the packaged file show the same groups.

## References

- Shapes: [design/overview.md](../design/overview.md),
  [reference.md](../reference.md), [CONFIG.md](../../CONFIG.md)
- Builds on: [ADR-0012](0012-firmware-updates-use-fwupds-own-polkit.md)
  (firmware updates use fwupd's own PolicyKit actions),
  [ADR-0003](0003-two-tier-config-with-fail-closed-semantics.md) (two-tier
  configuration)
