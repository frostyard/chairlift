// Package views provides the shell that composes page packages.
// This file creates and manages the lifecycle of all pages in the application.
package views

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/frostyard/chairlift/internal/config"
	"github.com/frostyard/chairlift/internal/pages"
	"github.com/frostyard/chairlift/internal/pages/applications"
	"github.com/frostyard/chairlift/internal/pages/extensions"
	"github.com/frostyard/chairlift/internal/pages/help"
	"github.com/frostyard/chairlift/internal/pages/maintenance"
	"github.com/frostyard/chairlift/internal/pages/system"
	"github.com/frostyard/chairlift/internal/pages/updates"

	"github.com/jwijenbergh/puregotk/v4/adw"
)

// ToastAdder is an interface for adding toasts and notifying about updates
type ToastAdder interface {
	ShowToast(message string)
	ShowErrorToast(message string)
	SetUpdateBadge(count int)
}

// UserHome manages all content pages by composing page packages.
// It acts as a thin shell that creates and coordinates page lifecycle.
type UserHome struct {
	config     *config.Config
	toastAdder ToastAdder

	// Page packages (extracted pages with lifecycle management)
	systemPagePkg       *system.Page
	helpPagePkg         *help.Page
	maintenancePagePkg  *maintenance.Page
	extensionsPagePkg   *extensions.Page
	applicationsPagePkg *applications.Page
	updatesPagePkg      *updates.Page

	// Pages (ToolbarViews) - widgets returned by page packages
	systemPage       *adw.ToolbarView
	updatesPage      *adw.ToolbarView
	applicationsPage *adw.ToolbarView
	helpPage         *adw.ToolbarView
}

// New creates a new UserHome views manager
func New(cfg *config.Config, toastAdder ToastAdder) *UserHome {
	uh := &UserHome{
		config:     cfg,
		toastAdder: toastAdder,
	}

	// Create page packages (System and Help) with dependency injection
	deps := pages.Deps{
		Config:  cfg,
		Toaster: toastAdder,
	}

	// Create System page using extracted package
	uh.systemPagePkg = system.New(deps, uh.launchApp, uh.openURL)
	uh.systemPage = uh.systemPagePkg.Widget()

	// Create Help page using extracted package
	uh.helpPagePkg = help.New(deps, uh.openURL)
	uh.helpPage = uh.helpPagePkg.Widget()

	// Create Maintenance page using extracted package
	uh.maintenancePagePkg = maintenance.New(deps)

	// Create Extensions page using extracted package
	uh.extensionsPagePkg = extensions.New(deps)

	// Create Applications page using extracted package
	uh.applicationsPagePkg = applications.New(deps, uh.launchApp, uh.openURL)
	uh.applicationsPage = uh.applicationsPagePkg.Widget()

	// Create Updates page using extracted package
	uh.updatesPagePkg = updates.New(deps, uh.toastAdder.SetUpdateBadge)
	uh.updatesPage = uh.updatesPagePkg.Widget()

	return uh
}

// GetPage returns a page by name
func (uh *UserHome) GetPage(name string) *adw.ToolbarView {
	switch name {
	case "system":
		return uh.systemPage
	case "updates":
		return uh.updatesPage
	case "applications":
		return uh.applicationsPage
	case "maintenance":
		return uh.maintenancePagePkg.Widget()
	case "extensions":
		return uh.extensionsPagePkg.Widget()
	case "help":
		return uh.helpPage
	default:
		return nil
	}
}

// Destroy cleans up all page packages, cancelling their goroutines.
// Call this when the window is being destroyed.
func (uh *UserHome) Destroy() {
	if uh.systemPagePkg != nil {
		uh.systemPagePkg.Destroy()
	}
	if uh.helpPagePkg != nil {
		uh.helpPagePkg.Destroy()
	}
	if uh.maintenancePagePkg != nil {
		uh.maintenancePagePkg.Destroy()
	}
	if uh.extensionsPagePkg != nil {
		uh.extensionsPagePkg.Destroy()
	}
	if uh.updatesPagePkg != nil {
		uh.updatesPagePkg.Destroy()
	}
	if uh.applicationsPagePkg != nil {
		uh.applicationsPagePkg.Destroy()
	}
}

// Helper methods

func (uh *UserHome) launchApp(appID string) {
	log.Printf("Launching app: %s", appID)

	var cmd *exec.Cmd

	// Check if this looks like a flatpak app ID (reverse DNS with 3+ parts)
	// e.g., "io.missioncenter.MissionCenter" or "org.gnome.Calculator"
	parts := strings.Split(appID, ".")
	isFlatpakStyle := len(parts) >= 3

	if isFlatpakStyle {
		// Check if flatpak knows about this app
		checkCmd := exec.Command("flatpak", "info", appID)
		if err := checkCmd.Run(); err == nil {
			// It's a flatpak app - use flatpak run
			log.Printf("Detected flatpak app, using 'flatpak run': %s", appID)
			cmd = exec.Command("flatpak", "run", appID)
		}
	}

	// Fall back to gtk-launch for non-flatpak apps or if flatpak check failed
	if cmd == nil {
		cmd = exec.Command("gtk-launch", appID)
	}

	cmd.Env = os.Environ()

	if err := cmd.Start(); err != nil {
		log.Printf("Failed to launch app %s: %v", appID, err)
		uh.toastAdder.ShowErrorToast(fmt.Sprintf("Failed to launch %s", appID))
		return
	}

	log.Printf("App launch started successfully: %s (pid: %d)", appID, cmd.Process.Pid)

	// Don't wait for the command to finish - it's a GUI app
	go func() {
		if err := cmd.Wait(); err != nil {
			log.Printf("App %s exited with error: %v", appID, err)
		}
	}()
}

func (uh *UserHome) openURL(url string) {
	log.Printf("Opening URL: %s", url)

	// Try gio open first (handles flatpak/snap portals better)
	// Fall back to xdg-open if gio fails
	cmd := exec.Command("gio", "open", url)
	cmd.Env = os.Environ()

	if err := cmd.Start(); err != nil {
		// Fallback to xdg-open
		log.Printf("gio open failed, trying xdg-open: %v", err)
		cmd = exec.Command("xdg-open", url)
		cmd.Env = os.Environ()
		if err := cmd.Start(); err != nil {
			log.Printf("Failed to open URL %s: %v", url, err)
			uh.toastAdder.ShowErrorToast(fmt.Sprintf("Failed to open %s", url))
			return
		}
	}

	// Don't wait for the command to finish
	go func() {
		_ = cmd.Wait()
	}()
}
