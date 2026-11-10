package frostui

import (
	"errors"
	"sync"

	"github.com/ebitengine/purego"
)

var setAppIDOnce sync.Once

// SetAppID names the program for the desktop before MyGo opens the
// display: GTK takes the Wayland app ID and the X11 WM_CLASS from GLib's
// program name, which MyGo leaves to the executable's name. With the
// reverse-DNS ID of the app's .desktop file, the desktop shows the app's
// icon and name for its windows. displayName is the human name
// (g_set_application_name). Call it before mygo.App.Run; only the first
// call counts, as GLib allows.
func SetAppID(id, displayName string) error {
	var err error
	setAppIDOnce.Do(func() {
		var lib uintptr
		for _, name := range []string{"libglib-2.0.so.0", "libglib-2.0.so"} {
			if lib, err = purego.Dlopen(name, purego.RTLD_NOW|purego.RTLD_GLOBAL); err == nil {
				break
			}
		}
		if err != nil {
			return
		}
		var setPrgname, setApplicationName func(*byte)
		purego.RegisterLibFunc(&setPrgname, lib, "g_set_prgname")
		purego.RegisterLibFunc(&setApplicationName, lib, "g_set_application_name")
		if id == "" {
			err = errors.New("frostui: empty app ID")
			return
		}
		setPrgname(cString(id))
		if displayName != "" {
			setApplicationName(cString(displayName))
		}
	})
	return err
}

// cString returns a NUL-terminated copy of s. GLib copies the strings it
// is given, so the Go memory may go after the call.
func cString(s string) *byte {
	b := make([]byte, len(s)+1)
	copy(b, s)
	return &b[0]
}
