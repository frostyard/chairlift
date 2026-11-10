package frostui

import (
	"os"
	"os/exec"
)

// LaunchDesktopApp starts the desktop app whose desktop file ID is id
// ("org.mozilla.firefox", without ".desktop") with gtk-launch, which finds
// the desktop file (Flatpak exports one per app) and starts the app as
// the desktop would. It does not wait: the app is a GUI app, and its end is
// reaped on a goroutine. It runs no shell; pass IDs from the app's
// configuration or a package manager's list, not typed input.
func LaunchDesktopApp(id string) error {
	cmd := exec.Command("gtk-launch", id) //nolint:gosec // see the comment above
	cmd.Env = os.Environ()
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
