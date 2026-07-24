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
