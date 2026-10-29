package actionmsg

import (
	"fmt"
	"strings"
	"testing"

	"github.com/frostyard/chairlift/internal/updex"
)

// TestBundleDump covers both dry-run states for the Brewfile-dump toast text.
func TestBundleDump(t *testing.T) {
	tests := []struct {
		name         string
		dryRun       bool
		path         string
		wantContains []string
		wantExact    string
	}{
		{
			name:      "live run reports the saved path",
			dryRun:    false,
			path:      "/home/user/Brewfile",
			wantExact: "Brewfile saved to /home/user/Brewfile",
		},
		{
			name:         "dry-run previews without claiming a save happened",
			dryRun:       true,
			path:         "/home/user/Brewfile",
			wantContains: []string{"[DRY-RUN]", "/home/user/Brewfile", "no changes made"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BundleDump(tt.dryRun, tt.path)

			if tt.wantExact != "" && got != tt.wantExact {
				t.Errorf("BundleDump(%v, %q) = %q, want %q", tt.dryRun, tt.path, got, tt.wantExact)
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("BundleDump(%v, %q) = %q, want it to contain %q", tt.dryRun, tt.path, got, want)
				}
			}
		})
	}
}

func TestBundleInstallDecision(t *testing.T) {
	tests := []struct {
		name         string
		dryRun       bool
		wantComplete bool
		wantExact    string
		wantContains []string
	}{
		{
			name:         "live install completes the row",
			wantComplete: true,
			wantExact:    "Brew bundle cli installed",
		},
		{
			name:         "dry-run resets the row after a preview",
			dryRun:       true,
			wantComplete: false,
			wantContains: []string{"[DRY-RUN]", "cli", "no changes made"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BundleInstall(tt.dryRun, "cli")
			if got.Complete != tt.wantComplete {
				t.Errorf("BundleInstall(%v, cli).Complete = %v, want %v", tt.dryRun, got.Complete, tt.wantComplete)
			}
			if tt.wantExact != "" && got.Toast != tt.wantExact {
				t.Errorf("BundleInstall(%v, cli).Toast = %q, want %q", tt.dryRun, got.Toast, tt.wantExact)
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(got.Toast, want) {
					t.Errorf("BundleInstall(%v, cli).Toast = %q, want it to contain %q", tt.dryRun, got.Toast, want)
				}
			}
		})
	}
}

// TestCleanup covers both dry-run states for the Homebrew/Flatpak cleanup
// toast text. This is the extraction of onBrewCleanupClicked's and
// onFlatpakCleanupClicked's already-correct message selection into a tested,
// pure function — behavior is unchanged, only now exercised headlessly.
func TestCleanup(t *testing.T) {
	tests := []struct {
		name   string
		dryRun bool
		tool   string
		output string
		want   string
	}{
		{
			name:   "live run reports fixed completion message",
			dryRun: false,
			tool:   "Homebrew",
			output: "some brew cleanup output",
			want:   "Homebrew cleanup completed",
		},
		{
			name:   "dry-run passes through the wrapper's mock output",
			dryRun: true,
			tool:   "Homebrew",
			output: "[DRY-RUN] Would execute: brew cleanup",
			want:   "[DRY-RUN] Would execute: brew cleanup",
		},
		{
			name:   "flatpak live run reports fixed completion message",
			dryRun: false,
			tool:   "Flatpak",
			output: "some flatpak output",
			want:   "Flatpak cleanup completed",
		},
		{
			name:   "flatpak dry-run passes through the wrapper's mock output",
			dryRun: true,
			tool:   "Flatpak",
			output: "[DRY-RUN] Would execute: flatpak uninstall --unused -y",
			want:   "[DRY-RUN] Would execute: flatpak uninstall --unused -y",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Cleanup(tt.dryRun, tt.tool, tt.output)
			if got != tt.want {
				t.Errorf("Cleanup(%v, %q, %q) = %q, want %q", tt.dryRun, tt.tool, tt.output, got, tt.want)
			}
		})
	}
}

// TestPackageInstallMessage covers both dry-run states for the Homebrew
// package-install toast text.
func TestPackageInstallMessage(t *testing.T) {
	tests := []struct {
		name         string
		dryRun       bool
		pkgName      string
		wantExact    string
		wantContains []string
	}{
		{
			name:      "live run reports the package as installed",
			dryRun:    false,
			pkgName:   "ripgrep",
			wantExact: "ripgrep installed",
		},
		{
			name:         "dry-run previews without claiming an install happened",
			dryRun:       true,
			pkgName:      "ripgrep",
			wantContains: []string{"[DRY-RUN]", "ripgrep", "no changes made"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Install(tt.dryRun, tt.pkgName)

			if tt.wantExact != "" && got != tt.wantExact {
				t.Errorf("Install(%v, %q) = %q, want %q", tt.dryRun, tt.pkgName, got, tt.wantExact)
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("Install(%v, %q) = %q, want it to contain %q", tt.dryRun, tt.pkgName, got, want)
				}
			}
		})
	}
}

// TestUninstall covers both dry-run states for shared Homebrew/Flatpak
// uninstall toast text.
func TestUninstall(t *testing.T) {
	tests := []struct {
		name         string
		dryRun       bool
		appID        string
		wantExact    string
		wantContains []string
	}{
		{
			name:      "live run reports the app as uninstalled",
			dryRun:    false,
			appID:     "org.mozilla.firefox",
			wantExact: "org.mozilla.firefox uninstalled",
		},
		{
			name:         "dry-run previews without claiming an uninstall happened",
			dryRun:       true,
			appID:        "org.mozilla.firefox",
			wantContains: []string{"[DRY-RUN]", "org.mozilla.firefox", "no changes made"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Uninstall(tt.dryRun, tt.appID)

			if tt.wantExact != "" && got != tt.wantExact {
				t.Errorf("Uninstall(%v, %q) = %q, want %q", tt.dryRun, tt.appID, got, tt.wantExact)
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("Uninstall(%v, %q) = %q, want it to contain %q", tt.dryRun, tt.appID, got, want)
				}
			}
		})
	}
}

func TestPin(t *testing.T) {
	tests := []struct {
		name      string
		dryRun    bool
		pin       bool
		wantExact string
	}{
		{
			name:      "live pin",
			pin:       true,
			wantExact: "ripgrep pinned",
		},
		{
			name:      "live unpin",
			wantExact: "ripgrep unpinned",
		},
		{
			name:      "dry-run pin",
			dryRun:    true,
			pin:       true,
			wantExact: "[DRY-RUN] Preview: ripgrep would be pinned — no changes made",
		},
		{
			name:      "dry-run unpin",
			dryRun:    true,
			wantExact: "[DRY-RUN] Preview: ripgrep would be unpinned — no changes made",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Pin(tt.dryRun, "ripgrep", tt.pin); got != tt.wantExact {
				t.Errorf("Pin(%v, %q, %v) = %q, want %q", tt.dryRun, "ripgrep", tt.pin, got, tt.wantExact)
			}
		})
	}
}

// TestUpgrade covers both dry-run states for the per-package Homebrew
// upgrade toast text.
func TestUpgrade(t *testing.T) {
	tests := []struct {
		name         string
		dryRun       bool
		pkgName      string
		wantExact    string
		wantContains []string
	}{
		{
			name:      "live run reports the package as upgraded",
			dryRun:    false,
			pkgName:   "ripgrep",
			wantExact: "ripgrep upgraded",
		},
		{
			name:         "dry-run previews without claiming an upgrade happened",
			dryRun:       true,
			pkgName:      "ripgrep",
			wantContains: []string{"[DRY-RUN]", "ripgrep", "no changes made"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Upgrade(tt.dryRun, tt.pkgName)

			if tt.wantExact != "" && got != tt.wantExact {
				t.Errorf("Upgrade(%v, %q) = %q, want %q", tt.dryRun, tt.pkgName, got, tt.wantExact)
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("Upgrade(%v, %q) = %q, want it to contain %q", tt.dryRun, tt.pkgName, got, want)
				}
			}
		})
	}
}

// TestUpdate covers both dry-run states for the per-app Flatpak update toast
// text.
func TestUpdate(t *testing.T) {
	tests := []struct {
		name         string
		dryRun       bool
		appID        string
		wantExact    string
		wantContains []string
	}{
		{
			name:      "live run reports the app as updated",
			dryRun:    false,
			appID:     "org.mozilla.firefox",
			wantExact: "org.mozilla.firefox updated",
		},
		{
			name:         "dry-run previews without claiming an update happened",
			dryRun:       true,
			appID:        "org.mozilla.firefox",
			wantContains: []string{"[DRY-RUN]", "org.mozilla.firefox", "no changes made"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Update(tt.dryRun, tt.appID)

			if tt.wantExact != "" && got != tt.wantExact {
				t.Errorf("Update(%v, %q) = %q, want %q", tt.dryRun, tt.appID, got, tt.wantExact)
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("Update(%v, %q) = %q, want it to contain %q", tt.dryRun, tt.appID, got, want)
				}
			}
		})
	}
}

// TestSelfUpdate covers both dry-run states for a package manager
// self-update toast text (e.g. Homebrew's own `brew update`).
func TestSelfUpdate(t *testing.T) {
	tests := []struct {
		name         string
		dryRun       bool
		tool         string
		wantExact    string
		wantContains []string
	}{
		{
			name:      "live run reports fixed completion message",
			dryRun:    false,
			tool:      "Homebrew",
			wantExact: "Homebrew updated successfully",
		},
		{
			name:         "dry-run previews without claiming an update happened",
			dryRun:       true,
			tool:         "Homebrew",
			wantContains: []string{"[DRY-RUN]", "Homebrew", "no changes made"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SelfUpdate(tt.dryRun, tt.tool)

			if tt.wantExact != "" && got != tt.wantExact {
				t.Errorf("SelfUpdate(%v, %q) = %q, want %q", tt.dryRun, tt.tool, got, tt.wantExact)
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("SelfUpdate(%v, %q) = %q, want it to contain %q", tt.dryRun, tt.tool, got, want)
				}
			}
		})
	}
}

// TestBootcStage covers all four (dryRun, staged) combinations for the
// Updates page's bootc stage-button completion toast. Under dry-run,
// bootc.StageUpdate never re-checks or re-stages anything for real (it
// short-circuits before pkexec), so the toast must read as an unambiguous
// preview regardless of what a live bootc.GetStatus() re-read happens to
// report as staged — the dryRun=true cases must NOT contain either
// non-dry-run completion string verbatim.
func TestBootcStage(t *testing.T) {
	const stagedMsg = "System update staged. Restart to apply."
	const upToDateMsg = "System is up to date"

	tests := []struct {
		name         string
		dryRun       bool
		staged       bool
		wantExact    string
		wantContains []string
	}{
		{
			name:      "live run, staged reports the fixed staged message",
			dryRun:    false,
			staged:    true,
			wantExact: stagedMsg,
		},
		{
			name:      "live run, not staged reports the fixed up-to-date message",
			dryRun:    false,
			staged:    false,
			wantExact: upToDateMsg,
		},
		{
			name:         "dry-run, staged previews without claiming completion",
			dryRun:       true,
			staged:       true,
			wantContains: []string{"no changes made"},
		},
		{
			name:         "dry-run, not staged previews without claiming completion",
			dryRun:       true,
			staged:       false,
			wantContains: []string{"no changes made"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BootcStage(tt.dryRun, tt.staged)

			if tt.wantExact != "" && got != tt.wantExact {
				t.Errorf("BootcStage(%v, %v) = %q, want %q", tt.dryRun, tt.staged, got, tt.wantExact)
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("BootcStage(%v, %v) = %q, want it to contain %q", tt.dryRun, tt.staged, got, want)
				}
			}
			if tt.dryRun {
				if !strings.Contains(got, "Preview") && !strings.Contains(got, "[DRY-RUN]") {
					t.Errorf("BootcStage(%v, %v) = %q, want it to contain %q or %q", tt.dryRun, tt.staged, got, "Preview", "[DRY-RUN]")
				}
				if strings.Contains(got, stagedMsg) {
					t.Errorf("BootcStage(%v, %v) = %q, must not contain the non-dry-run staged completion string %q", tt.dryRun, tt.staged, got, stagedMsg)
				}
				if strings.Contains(got, upToDateMsg) {
					t.Errorf("BootcStage(%v, %v) = %q, must not contain the non-dry-run up-to-date completion string %q", tt.dryRun, tt.staged, got, upToDateMsg)
				}
			}
		})
	}
}

// TestSysupdateStage mirrors TestBootcStage: the native A/B stage toast has
// the identical dry-run contract (a preview string that never claims a
// completed or verified state, because staged reflects real system state, not
// this click's work).
func TestSysupdateStage(t *testing.T) {
	const stagedMsg = "System update staged. Restart to apply."
	const upToDateMsg = "System is up to date"

	tests := []struct {
		name         string
		dryRun       bool
		staged       bool
		wantExact    string
		wantContains []string
	}{
		{
			name:      "live run, staged reports the fixed staged message",
			dryRun:    false,
			staged:    true,
			wantExact: stagedMsg,
		},
		{
			name:      "live run, not staged reports the fixed up-to-date message",
			dryRun:    false,
			staged:    false,
			wantExact: upToDateMsg,
		},
		{
			name:         "dry-run, staged previews without claiming completion",
			dryRun:       true,
			staged:       true,
			wantContains: []string{"no changes made"},
		},
		{
			name:         "dry-run, not staged previews without claiming completion",
			dryRun:       true,
			staged:       false,
			wantContains: []string{"no changes made"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SysupdateStage(tt.dryRun, tt.staged)

			if tt.wantExact != "" && got != tt.wantExact {
				t.Errorf("SysupdateStage(%v, %v) = %q, want %q", tt.dryRun, tt.staged, got, tt.wantExact)
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("SysupdateStage(%v, %v) = %q, want it to contain %q", tt.dryRun, tt.staged, got, want)
				}
			}
			if tt.dryRun {
				if !strings.Contains(got, "Preview") && !strings.Contains(got, "[DRY-RUN]") {
					t.Errorf("SysupdateStage(%v, %v) = %q, want it to contain %q or %q", tt.dryRun, tt.staged, got, "Preview", "[DRY-RUN]")
				}
				if strings.Contains(got, stagedMsg) {
					t.Errorf("SysupdateStage(%v, %v) = %q, must not contain the non-dry-run staged completion string %q", tt.dryRun, tt.staged, got, stagedMsg)
				}
				if strings.Contains(got, upToDateMsg) {
					t.Errorf("SysupdateStage(%v, %v) = %q, must not contain the non-dry-run up-to-date completion string %q", tt.dryRun, tt.staged, got, upToDateMsg)
				}
			}
		})
	}
}

// TestTapTrust covers both dry-run states for Homebrew tap trust, asserting
// both the UI-mutation gate (MutateUI) and the Toast text. MutateUI is the
// criterion that directly proves the Untrusted Homebrew Taps UI does not
// imply a tap was trusted (row removed, group hidden, refresh triggered)
// after a dry-run click, since homebrew.TrustPackages's underlying `brew
// trust` never actually runs under dry-run.
func TestTapTrust(t *testing.T) {
	tests := []struct {
		name             string
		dryRun           bool
		tapName          string
		wantMutateUI     bool
		wantToast        string
		wantToastContain []string
	}{
		{
			name:         "live run trusts the tap and mutates the UI",
			dryRun:       false,
			tapName:      "some/tap",
			wantMutateUI: true,
			wantToast:    "Trusted some/tap. Its packages can update again.",
		},
		{
			name:             "dry-run previews without mutating the UI",
			dryRun:           true,
			tapName:          "some/tap",
			wantMutateUI:     false,
			wantToastContain: []string{"some/tap", "no changes made"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TapTrust(tt.dryRun, tt.tapName)

			if got.MutateUI != !tt.dryRun {
				t.Errorf("TapTrust(%v, %q).MutateUI = %v, want %v", tt.dryRun, tt.tapName, got.MutateUI, !tt.dryRun)
			}
			if got.MutateUI != tt.wantMutateUI {
				t.Errorf("TapTrust(%v, %q).MutateUI = %v, want %v", tt.dryRun, tt.tapName, got.MutateUI, tt.wantMutateUI)
			}
			if tt.wantToast != "" && got.Toast != tt.wantToast {
				t.Errorf("TapTrust(%v, %q).Toast = %q, want %q", tt.dryRun, tt.tapName, got.Toast, tt.wantToast)
			}
			if tt.dryRun && !strings.Contains(got.Toast, "Preview") && !strings.Contains(got.Toast, "[DRY-RUN]") {
				t.Errorf("TapTrust(%v, %q).Toast = %q, want it to contain %q or %q", tt.dryRun, tt.tapName, got.Toast, "Preview", "[DRY-RUN]")
			}
			for _, want := range tt.wantToastContain {
				if !strings.Contains(got.Toast, want) {
					t.Errorf("TapTrust(%v, %q).Toast = %q, want it to contain %q", tt.dryRun, tt.tapName, got.Toast, want)
				}
			}
		})
	}
}

// TestMaintenanceScript covers both dry-run states for configured custom
// maintenance scripts, asserting both the execution gate (Execute) and the
// Toast text. Execute is the criterion that directly proves no
// state-changing path runs in dry-run for custom scripts, which have no
// wrapper package of their own to gate this the way homebrew/flatpak/bootc/
// updex do.
func TestMaintenanceScript(t *testing.T) {
	tests := []struct {
		name         string
		dryRun       bool
		title        string
		wantExecute  bool
		wantToast    string
		wantContains []string
	}{
		{
			name:        "live run executes and reports completion",
			dryRun:      false,
			title:       "Clear tmp",
			wantExecute: true,
			wantToast:   "Clear tmp completed",
		},
		{
			name:         "dry-run never executes and previews instead",
			dryRun:       true,
			title:        "Clear tmp",
			wantExecute:  false,
			wantContains: []string{"[DRY-RUN]", "Clear tmp", "no changes made"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MaintenanceScript(tt.dryRun, tt.title)

			if got.Execute != !tt.dryRun {
				t.Errorf("MaintenanceScript(%v, %q).Execute = %v, want %v", tt.dryRun, tt.title, got.Execute, !tt.dryRun)
			}
			if got.Execute != tt.wantExecute {
				t.Errorf("MaintenanceScript(%v, %q).Execute = %v, want %v", tt.dryRun, tt.title, got.Execute, tt.wantExecute)
			}
			if tt.wantToast != "" && got.Toast != tt.wantToast {
				t.Errorf("MaintenanceScript(%v, %q).Toast = %q, want %q", tt.dryRun, tt.title, got.Toast, tt.wantToast)
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(got.Toast, want) {
					t.Errorf("MaintenanceScript(%v, %q).Toast = %q, want it to contain %q", tt.dryRun, tt.title, got.Toast, want)
				}
			}
		})
	}
}

// TestFeatureToggle covers both dry-run states and both enable/disable for
// a feature switch toggle, asserting both the switch-confirmation gate
// (Confirm) and the Toast text. Confirm is the criterion that directly
// proves the switch does not visually imply a state change after a
// dry-run preview, since updex.EnableFeature/DisableFeature's underlying
// pkexec call never actually runs under dry-run.
func TestFeatureToggle(t *testing.T) {
	tests := []struct {
		name         string
		dryRun       bool
		enable       bool
		featName     string
		wantConfirm  bool
		wantToast    string
		wantContains []string
	}{
		{
			name:        "live enable confirms the switch",
			dryRun:      false,
			enable:      true,
			featName:    "docker",
			wantConfirm: true,
			wantToast:   "docker enabled. Update to download, reboot to apply.",
		},
		{
			name:        "live disable confirms the switch",
			dryRun:      false,
			enable:      false,
			featName:    "docker",
			wantConfirm: true,
			wantToast:   "docker disabled. Update to apply, reboot to complete.",
		},
		{
			name:         "dry-run enable previews without confirming the switch",
			dryRun:       true,
			enable:       true,
			featName:     "docker",
			wantConfirm:  false,
			wantContains: []string{"docker", "enabled", "no changes made"},
		},
		{
			name:         "dry-run disable previews without confirming the switch",
			dryRun:       true,
			enable:       false,
			featName:     "docker",
			wantConfirm:  false,
			wantContains: []string{"docker", "disabled", "no changes made"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FeatureToggle(tt.dryRun, tt.enable, tt.featName)

			if got.Confirm != !tt.dryRun {
				t.Errorf("FeatureToggle(%v, %v, %q).Confirm = %v, want %v", tt.dryRun, tt.enable, tt.featName, got.Confirm, !tt.dryRun)
			}
			if got.Confirm != tt.wantConfirm {
				t.Errorf("FeatureToggle(%v, %v, %q).Confirm = %v, want %v", tt.dryRun, tt.enable, tt.featName, got.Confirm, tt.wantConfirm)
			}
			if tt.wantToast != "" && got.Toast != tt.wantToast {
				t.Errorf("FeatureToggle(%v, %v, %q).Toast = %q, want %q", tt.dryRun, tt.enable, tt.featName, got.Toast, tt.wantToast)
			}
			if tt.dryRun && !strings.Contains(got.Toast, "Preview") && !strings.Contains(got.Toast, "[DRY-RUN]") {
				t.Errorf("FeatureToggle(%v, %v, %q).Toast = %q, want it to contain %q or %q", tt.dryRun, tt.enable, tt.featName, got.Toast, "Preview", "[DRY-RUN]")
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(got.Toast, want) {
					t.Errorf("FeatureToggle(%v, %v, %q).Toast = %q, want it to contain %q", tt.dryRun, tt.enable, tt.featName, got.Toast, want)
				}
			}
		})
	}
}

// TestFeatureUpdate is the decision table of FeatureUpdate: one row per rule,
// with exact strings.
func TestFeatureUpdate(t *testing.T) {
	transfer := func(mods ...func(*updex.TransferUpdate)) updex.TransferUpdate {
		u := updex.TransferUpdate{Component: "comp"}
		for _, m := range mods {
			m(&u)
		}
		return u
	}
	updated := func(name string) updex.FeatureUpdate {
		return updex.FeatureUpdate{Feature: name, Results: []updex.TransferUpdate{
			transfer(func(u *updex.TransferUpdate) { u.Downloaded = true; u.Linked = true; u.Installed = true }),
		}}
	}
	current := func(name string) updex.FeatureUpdate {
		return updex.FeatureUpdate{Feature: name, Results: []updex.TransferUpdate{
			transfer(func(u *updex.TransferUpdate) { u.Installed = true }),
		}}
	}
	waiting := func(name, comp, image string) updex.FeatureUpdate {
		return updex.FeatureUpdate{Feature: name, Results: []updex.TransferUpdate{
			transfer(func(u *updex.TransferUpdate) { u.Component = comp; u.NeedsImage = image }),
		}}
	}
	held := func(name, msg string) updex.FeatureUpdate {
		return updex.FeatureUpdate{Feature: name, Results: []updex.TransferUpdate{
			transfer(func(u *updex.TransferUpdate) { u.Held = true; u.NextActionMessage = msg }),
		}}
	}
	failed := func(name, msg string) updex.FeatureUpdate {
		return updex.FeatureUpdate{Feature: name, Results: []updex.TransferUpdate{
			transfer(func(u *updex.TransferUpdate) { u.Error = msg }),
		}}
	}
	activation := &updex.ActivationIncompleteError{Message: "command failed (exit 3): merged"}
	wrapped := &updex.Error{Message: "command failed (exit 1): boom"}

	tests := []struct {
		name           string
		dryRun         bool
		results        []updex.FeatureUpdate
		err            error
		wantToast      string
		wantPersistent bool
		// wantRecheck is the expected FeatureUpdateDecision.Recheck: false for
		// the dry-run rows, true for every row that ran the update.
		wantRecheck bool
	}{
		{
			name:        "dry-run previews and is not persistent",
			dryRun:      true,
			wantToast:   "[DRY-RUN] Preview: features would be updated — no changes made",
			wantRecheck: false,
		},
		{
			name:        "dry-run ignores results and error",
			dryRun:      true,
			results:     []updex.FeatureUpdate{failed("f", "boom")},
			err:         wrapped,
			wantToast:   "[DRY-RUN] Preview: features would be updated — no changes made",
			wantRecheck: false,
		},
		{
			name:           "activation incomplete without results",
			err:            activation,
			wantToast:      "Features merged, but activation did not finish. Reboot to finish.",
			wantPersistent: true,
			wantRecheck:    true,
		},
		{
			name:           "activation incomplete with empty results",
			results:        []updex.FeatureUpdate{},
			err:            fmt.Errorf("wrapped: %w", activation),
			wantToast:      "Features merged, but activation did not finish. Reboot to finish.",
			wantPersistent: true,
			wantRecheck:    true,
		},
		{
			name:           "other error without results (old helper)",
			err:            wrapped,
			wantToast:      "Update failed: command failed (exit 1): boom",
			wantPersistent: true,
			wantRecheck:    true,
		},
		{
			name:           "nil results and nil error do not claim success",
			wantToast:      "Feature update finished, but ChairLift could not read the results. See the log for details.",
			wantPersistent: true,
			wantRecheck:    true,
		},
		{
			name:           "unreadable results error",
			err:            &updex.ResultsUnreadableError{Message: "cannot read update results: boom"},
			wantToast:      "Feature update finished, but ChairLift could not read the results. See the log for details.",
			wantPersistent: true,
			wantRecheck:    true,
		},
		{
			name:           "wrapped unreadable results error with empty results",
			results:        []updex.FeatureUpdate{},
			err:            fmt.Errorf("wrapped: %w", &updex.ResultsUnreadableError{Message: "x"}),
			wantToast:      "Feature update finished, but ChairLift could not read the results. See the log for details.",
			wantPersistent: true,
			wantRecheck:    true,
		},
		{
			name:        "empty results report no features",
			results:     []updex.FeatureUpdate{},
			wantToast:   "No enabled features to update.",
			wantRecheck: true,
		},
		{
			name:        "all updated keeps the old text exactly",
			results:     []updex.FeatureUpdate{updated("a"), updated("b")},
			wantToast:   "Features updated. Changes apply after reboot.",
			wantRecheck: true,
		},
		{
			name:        "updated and current keep the old text exactly",
			results:     []updex.FeatureUpdate{updated("a"), current("b")},
			wantToast:   "Features updated. Changes apply after reboot.",
			wantRecheck: true,
		},
		{
			name:        "only current",
			results:     []updex.FeatureUpdate{current("a"), current("b")},
			wantToast:   "Features are already up to date.",
			wantRecheck: true,
		},
		{
			name:           "waiting for image only",
			results:        []updex.FeatureUpdate{waiting("c", "app", "needs image >= 5"), current("a")},
			wantToast:      "Waiting for a newer OS image: c (app: needs image >= 5).",
			wantPersistent: true,
			wantRecheck:    true,
		},
		{
			name:           "held only",
			results:        []updex.FeatureUpdate{held("e", "held at 1 by a rollback of 2")},
			wantToast:      "Held by a rollback: e (held at 1 by a rollback of 2).",
			wantPersistent: true,
			wantRecheck:    true,
		},
		{
			name:           "failed only with the aggregate error already covered by the reasons",
			results:        []updex.FeatureUpdate{failed("f", "download failed"), failed("g", "no space")},
			err:            wrapped,
			wantToast:      "Failed: f (download failed); g (no space).",
			wantPersistent: true,
			wantRecheck:    true,
		},
		{
			name: "every sentence in order",
			results: []updex.FeatureUpdate{
				updated("a"), updated("b"),
				waiting("c", "app", "needs image >= 5"), waiting("d", "lib", "needs image >= 6"),
				held("e", "held"),
				failed("f", "x"), failed("g", "y"),
				current("h"),
			},
			err: wrapped,
			wantToast: "Updated: a, b. " +
				"Waiting for a newer OS image: c (app: needs image >= 5); d (lib: needs image >= 6). " +
				"Held by a rollback: e (held). " +
				"Failed: f (x); g (y). " +
				"Changes apply after reboot.",
			wantPersistent: true,
			wantRecheck:    true,
		},
		{
			name:           "updated and waiting with nil error",
			results:        []updex.FeatureUpdate{updated("a"), waiting("c", "app", "needs image >= 5")},
			wantToast:      "Updated: a. Waiting for a newer OS image: c (app: needs image >= 5). Changes apply after reboot.",
			wantPersistent: true,
			wantRecheck:    true,
		},
		{
			name:           "updated with activation incomplete drops the reboot sentence",
			results:        []updex.FeatureUpdate{updated("a")},
			err:            activation,
			wantToast:      "Updated: a. Extensions merged, but activation did not finish. Reboot to finish.",
			wantPersistent: true,
			wantRecheck:    true,
		},
		{
			name:           "failed with activation incomplete",
			results:        []updex.FeatureUpdate{updated("a"), failed("f", "x")},
			err:            fmt.Errorf("joined: %w", activation),
			wantToast:      "Updated: a. Failed: f (x). Extensions merged, but activation did not finish. Reboot to finish.",
			wantPersistent: true,
			wantRecheck:    true,
		},
		{
			name:           "other error with no failed feature appends the error",
			results:        []updex.FeatureUpdate{updated("a")},
			err:            wrapped,
			wantToast:      "Updated: a. Changes apply after reboot. Update failed: command failed (exit 1): boom",
			wantPersistent: true,
			wantRecheck:    true,
		},
		{
			name:           "other error with only current features",
			results:        []updex.FeatureUpdate{current("a")},
			err:            wrapped,
			wantToast:      "Update failed: command failed (exit 1): boom",
			wantPersistent: true,
			wantRecheck:    true,
		},
		{
			name:           "activation incomplete with only current features",
			results:        []updex.FeatureUpdate{current("a")},
			err:            activation,
			wantToast:      "Extensions merged, but activation did not finish. Reboot to finish.",
			wantPersistent: true,
			wantRecheck:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FeatureUpdate(tt.dryRun, tt.results, tt.err)
			if got.Toast != tt.wantToast {
				t.Errorf("Toast = %q, want %q", got.Toast, tt.wantToast)
			}
			if got.Persistent != tt.wantPersistent {
				t.Errorf("Persistent = %v, want %v", got.Persistent, tt.wantPersistent)
			}
			if got.Recheck != tt.wantRecheck {
				t.Errorf("Recheck = %v, want %v", got.Recheck, tt.wantRecheck)
			}
		})
	}
}
