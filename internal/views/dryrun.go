package views

import "log"

// dryRun is the views-level counterpart of the SetDryRun/IsDryRun flag each
// wrapper package (homebrew, flatpak, bootc, updex) carries. It exists
// because configured custom maintenance scripts (config.yml's `actions`
// entries, run by runMaintenanceAction in maintenance_page.go) are not a
// wrapper around any particular CLI tool — they have no package of their own
// to hold a dry-run flag, so it lives here instead.
var dryRun = false

// SetDryRun sets dry-run mode for view-level, config-driven actions —
// currently just custom maintenance scripts — that have no dedicated
// wrapper package of their own to carry this flag.
func SetDryRun(mode bool) {
	dryRun = mode
	log.Printf("Views dry-run mode: %v", mode)
}

// IsDryRun returns whether dry-run mode is enabled for view-level actions.
func IsDryRun() bool {
	return dryRun
}
