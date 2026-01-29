# Chairlift

## What This Is

A GTK4/Libadwaita desktop application for managing bootc/nbc immutable Linux systems. Chairlift provides a graphical interface for users who find CLI tools intimidating, allowing them to manage packages (Flatpak, Homebrew, Snap), system updates (via nbc), and systemd-sysext extensions (via updex) without touching the terminal.

## Core Value

Less technical users can confidently manage their immutable Linux desktop without needing to understand the underlying CLI tools or immutable filesystem concepts.

## Requirements

### Validated

- GUI wrapper for Flatpak package management — existing
- GUI wrapper for Homebrew package management — existing
- GUI wrapper for Snap package management — existing
- NBC system update integration — existing
- Basic updex/sysext display — existing
- YAML-driven feature enablement — existing
- Dry-run mode for safe testing — existing
- PolicyKit integration for privileged operations — existing

### Active

- [ ] Refactor 2500-line userhome.go monolith into feature-based views
- [ ] Extract reusable UI components (progress bars, list items, action rows)
- [ ] Unified async operation framework (progress, cancellation, error handling)
- [ ] Full updex library integration (replace CLI wrapper approach)
- [ ] Eliminate duplicate code paths throughout codebase
- [ ] Comprehensive test coverage for confident refactoring
- [ ] Extract reusable GTK4/Go patterns into separate library
- [ ] Production-ready polish (bulletproof, accessible, documented)

### Out of Scope

- Mobile/responsive layouts — desktop-only GTK4 application
- Web interface — native desktop app only
- Windows/macOS support — Linux-only, relies on GTK4/Libadwaita
- Custom package manager backends — uses existing frostyard/pm library

## Context

**Technical environment:**
- Pure Go with puregotk bindings (no CGO)
- GTK4/Libadwaita for modern GNOME UX
- Wraps external CLI tools: nbc, updex, instex, flatpak, snap, brew
- Uses frostyard/pm library for package manager abstraction
- frostyard/pm/progress exists but isn't consistently integrated

**Current state:**
- Single 2500-line userhome.go contains all view code
- Duplicate patterns across package manager handling, UI components, async operations
- Progress system is bolted on rather than a core pattern
- updex was refactored from CLI tool to library; chairlift still uses CLI approach
- No tests; refactoring is high-risk

**Target users:**
- Less technical users who need GUI because CLI is intimidating
- Expect clear feedback, understandable errors, polished UX

## Constraints

- **Tech stack**: Pure Go, GTK4/Libadwaita, no CGO — existing architecture, don't change
- **Compatibility**: Must continue working with existing frostyard ecosystem (nbc, pm, updex libraries)
- **Quality bar**: Production ready — bulletproof, accessible, documented

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Extract reusable GTK4/Go library during refactor | Avoid doing the work twice; patterns will emerge during refactoring | — Pending |
| Library location TBD | Need to see what's extractable before deciding same-repo vs separate | — Pending |
| Progress/async as core infrastructure | Every async operation needs consistent progress, cancellation, error handling | — Pending |

---
*Last updated: 2026-01-26 after initialization*
