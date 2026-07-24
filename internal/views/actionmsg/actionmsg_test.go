package actionmsg

import (
	"strings"
	"testing"
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

// TestInstall covers both dry-run states for the Homebrew package-install
// toast text.
func TestInstall(t *testing.T) {
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

// TestUninstall covers both dry-run states for the Flatpak
// application-uninstall toast text.
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
