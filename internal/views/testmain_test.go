package views

import (
	"os"
	"testing"
)

// TestMain pins the icon theme: without it, every frostui.NewIcons asks
// gsettings for the desktop's theme, and tests must run no external
// command.
func TestMain(m *testing.M) {
	if err := os.Setenv("FROSTUI_ICON_THEME", "Adwaita"); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}
