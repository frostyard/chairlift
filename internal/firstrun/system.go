package firstrun

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os/exec"
	"os/user"
	"strings"
	"time"
)

// ColorScheme is a value of org.gnome.desktop.interface color-scheme that
// the Style step writes (first-setup's values).
type ColorScheme string

// The color schemes of the Style step.
const (
	SchemeLight ColorScheme = "prefer-light"
	SchemeDark  ColorScheme = "prefer-dark"
)

// gsettingsTimeout bounds one gsettings call.
const gsettingsTimeout = 10 * time.Second

// ColorSchemeArgs returns the gsettings arguments that apply a scheme. The
// Style step writes color-scheme only; it does not write the legacy
// gtk-theme key (ADR-0014).
func ColorSchemeArgs(scheme ColorScheme) []string {
	return []string{"set", "org.gnome.desktop.interface", "color-scheme", string(scheme)}
}

// gsettingsRunner runs gsettings with args. Tests replace it.
var gsettingsRunner = func(args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), gsettingsTimeout)
	defer cancel()
	output, err := exec.CommandContext(ctx, "gsettings", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("gsettings %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

// ApplyColorScheme writes the color scheme with gsettings. It rejects any
// other value. In dry run it logs "[DRY-RUN] would execute: gsettings ..."
// and runs nothing.
func ApplyColorScheme(scheme ColorScheme) error {
	if scheme != SchemeLight && scheme != SchemeDark {
		return fmt.Errorf("unknown color scheme %q", scheme)
	}
	args := ColorSchemeArgs(scheme)
	if dryRun {
		log.Printf("[DRY-RUN] would execute: gsettings %s", strings.Join(args, " "))
		return nil
	}
	return gsettingsRunner(args...)
}

const (
	// ConnectivityAddress is the host the connectivity check dials: the
	// Flathub download server the Apps step needs.
	ConnectivityAddress = "dl.flathub.org:443"
	// ConnectivityTimeout bounds the connectivity check.
	ConnectivityTimeout = 5 * time.Second
)

// DialFunc opens a network connection (net.Dialer.DialContext in
// production).
type DialFunc func(ctx context.Context, network, address string) (net.Conn, error)

// DefaultDial is the production DialFunc.
var DefaultDial DialFunc = (&net.Dialer{}).DialContext

// CheckConnectivity dials ConnectivityAddress over TCP within
// ConnectivityTimeout: ConnOnline when the dial succeeds, else ConnOffline.
// It never returns ConnChecking.
func CheckConnectivity(ctx context.Context, dial DialFunc) Connectivity {
	ctx, cancel := context.WithTimeout(ctx, ConnectivityTimeout)
	defer cancel()
	conn, err := dial(ctx, "tcp", ConnectivityAddress)
	if err != nil {
		return ConnOffline
	}
	_ = conn.Close()
	return ConnOnline
}

// WelcomeName returns the name the Welcome step greets: the first
// comma-separated field of the GECOS full name, else the login name, else
// $USER or $LOGNAME. lookupErr is the error of user.Current.
func WelcomeName(u *user.User, lookupErr error, getenv func(string) string) string {
	if lookupErr == nil && u != nil {
		full, _, _ := strings.Cut(u.Name, ",")
		if full = strings.TrimSpace(full); full != "" {
			return full
		}
		if u.Username != "" {
			return u.Username
		}
	}
	for _, name := range []string{"USER", "LOGNAME"} {
		if value := getenv(name); value != "" {
			return value
		}
	}
	return ""
}

// SettingsPanel is a GNOME Settings panel the first run opens.
type SettingsPanel string

// The panels: the Welcome step's Accessibility button and the connectivity
// step's Open Settings button.
const (
	PanelAccessibility SettingsPanel = "universal-access"
	PanelWifi          SettingsPanel = "wifi"
)

// SettingsCommand is GNOME Settings. The first run runs only on GNOME
// images, so the command is fixed and has no config key (ADR-0014).
const SettingsCommand = "gnome-control-center"

// SettingsArgv returns the command line that opens a panel.
func SettingsArgv(panel SettingsPanel) []string {
	return []string{SettingsCommand, string(panel)}
}

// SettingsAvailable reports whether gnome-control-center is on PATH. The
// window shows the settings buttons only when it is.
func SettingsAvailable(lookPath func(string) (string, error)) bool {
	_, err := lookPath(SettingsCommand)
	return err == nil
}

// startProcess starts argv and reaps it in the background. Tests replace it.
var startProcess = func(argv []string) error {
	cmd := exec.Command(argv[0], argv[1:]...)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// LaunchSettings opens a GNOME Settings panel without waiting for it. It is a
// launch, not a state change, so it also runs in dry run.
func LaunchSettings(panel SettingsPanel) error {
	if panel != PanelAccessibility && panel != PanelWifi {
		return errors.New("unknown settings panel")
	}
	return startProcess(SettingsArgv(panel))
}
