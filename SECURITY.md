# Security policy

ChairLift is a system-management application (a MyGo window on GTK 3). It runs with
the rights of the user who started it. It reaches root only through `pkexec`
and the PolicyKit policies in `data/`, with administrator authentication:

- the updex helper at the fixed path `/usr/bin/chairlift-updex-helper`
  (actions `org.frostyard.ChairLift.updex.*`), which ChairLift ships;
- the OS staging scripts `/usr/libexec/bootc-update-stage` and
  `/usr/libexec/snosi-sysupdate-stage` (actions
  `org.frostyard.ChairLift.bootc.stage` and
  `org.frostyard.ChairLift.sysupdate.stage`), which the OS image ships;
- a maintenance action marked `sudo: true` in the administrator's
  configuration, under PolicyKit's default `pkexec` action.

Firmware updates use fwupd's own PolicyKit actions. Problem reports leave the
host only after the user opts in.

## Supported versions

Fixes go to the `main` branch and ship in the next `v0.x.y` tag. Only the
latest tag is supported; there are no backports to older tags. The rolling
`dev` prerelease is a snapshot of `main` for testing and gets no separate
fixes.

## Reporting a vulnerability

Report a vulnerability privately through GitHub private vulnerability
reporting: open <https://github.com/frostyard/chairlift/security> and choose
**Report a vulnerability**. Do not open a public issue for a vulnerability.

Include the affected tag or commit, the impact, and the steps to reproduce.
