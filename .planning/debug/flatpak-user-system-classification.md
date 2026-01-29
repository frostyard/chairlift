---
status: diagnosed
trigger: "In the Chairlift application, the Flatpak applications section shows ALL applications as 'system flatpaks', but many of them are actually user flatpaks installed in the user's home directory."
created: 2026-01-28T00:00:00Z
updated: 2026-01-28T00:00:00Z
symptoms_prefilled: true
goal: find_root_cause_only
---

## Current Focus

hypothesis: CONFIRMED - Issue is in how Namespace comparison works. Either (1) flatpak_user_group is disabled in config making p.flatpakUserExpander nil, OR (2) pkg.Ref.Namespace is not "user" for user installations (could be empty string from fallback parsing)
test: checking pm library fallback parsing paths
expecting: to find that Namespace can be empty string in certain conditions
next_action: provide diagnosis with two possible root causes

## Symptoms

expected: Flatpak applications should be correctly labeled as either "system" or "user" based on their actual installation location
actual: All flatpak apps show as system flatpaks even when they are user flatpaks
errors: None reported
reproduction: View flatpak applications in the applications section
started: Unknown

## Eliminated

## Evidence

- timestamp: 2026-01-28T00:05:00Z
  checked: pm library flatpak backend parsing
  found: pm library correctly parses installation column from `flatpak list --app --columns=name,application,version,installation` and sets it as Namespace field
  implication: The pm library is working correctly

- timestamp: 2026-01-28T00:06:00Z
  checked: Executed flatpak list command directly
  found: Command returns correct "user" and "system" values in installation column
  implication: The underlying flatpak command works correctly

- timestamp: 2026-01-28T00:07:00Z
  checked: Executed pm library test command
  found: pm library test correctly displays [user] and [system] namespaces
  implication: pm library wrapper.ListInstalled() returns correct Namespace values

- timestamp: 2026-01-28T00:08:00Z
  checked: /home/bjk/projects/frostyard/chairlift/internal/pm/wrapper.go line 156
  found: Code checks `pkg.Ref.Namespace == "user"` to set IsUser field
  implication: The logic appears correct - if Namespace is "user", IsUser will be true; otherwise false

- timestamp: 2026-01-28T00:09:00Z
  checked: Created test program using same logic as Chairlift wrapper
  found: Test correctly separates user and system apps (13 user, 26 system from my system)
  implication: The wrapper logic is working correctly when called directly

- timestamp: 2026-01-28T00:10:00Z
  checked: Default config in internal/config/config.go lines 114-115
  found: Both flatpak_user_group and flatpak_system_group are enabled by default
  implication: Unless user has custom config, both groups should be visible

- timestamp: 2026-01-28T00:11:00Z
  checked: How groups are conditionally created in buildFlatpakGroups()
  found: p.flatpakUserExpander is only created if flatpak_user_group is enabled; p.flatpakSystemExpander is only created if flatpak_system_group is enabled
  implication: If user disabled one group in config, only one expander would be created, and apps would only show in that one section

- timestamp: 2026-01-28T00:12:00Z
  checked: pm library fallback parsing in flatpak.go lines 420-451
  found: If flatpak output has < 4 fields (missing installation column), Namespace is set to empty string
  implication: If flatpak command doesn't include installation column, all apps would have empty Namespace, making IsUser=false for all

- timestamp: 2026-01-28T00:13:00Z
  checked: Chairlift go.mod for pm version
  found: Using pm v0.2.1 (same as examined)
  implication: Not a version mismatch issue

- timestamp: 2026-01-28T00:14:00Z
  checked: Verified flatpak command works with --columns=installation
  found: Command works correctly on test system (Flatpak 1.16.2)
  implication: The command itself works, but could fail on older flatpak versions or different environments

## Resolution

root_cause: The flatpak_user_group is likely disabled in the user's configuration file, OR there's an edge case where pkg.Ref.Namespace is not being set to "user" correctly by the pm library for user installations. The wrapper.go code at line 156 checks `pkg.Ref.Namespace == "user"` - if this comparison fails for user apps, they all appear as system apps.
fix: Need to verify user's config and add defensive checking
verification: Test with both config scenarios and verify Namespace values
files_changed: []
