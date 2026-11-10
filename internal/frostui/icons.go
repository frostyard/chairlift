package frostui

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"
)

// Icons finds freedesktop icons in the desktop's own icon theme, so an app
// shows Adwaita's symbolic icons on GNOME and Breeze's on KDE. A theme
// without an icon falls back to Adwaita, then hicolor. Lookups are lazy and
// cached; an icon no theme has is nil, which Icon draws as an empty box of
// the same size.
type Icons struct {
	Theme string

	roots    []string
	themes   []string
	mu       sync.Mutex
	cache    map[string]*ui.SVG
	embedded map[string][]byte
}

// NewIcons returns the icons of the desktop's theme. embedded holds icons
// the app ships itself, by name, which win over the theme's.
func NewIcons(embedded map[string][]byte) *Icons {
	home, _ := os.UserHomeDir()
	theme := DesktopIconTheme()
	roots := []string{filepath.Join(home, ".local/share/icons"), filepath.Join(home, ".icons")}
	for _, dir := range filepath.SplitList(os.Getenv("XDG_DATA_DIRS")) {
		if dir != "" {
			roots = append(roots, filepath.Join(dir, "icons"))
		}
	}
	roots = append(roots, "/usr/share/icons")
	themes := []string{theme}
	for _, fallback := range []string{"Adwaita", "breeze", "hicolor"} {
		if !strings.EqualFold(fallback, theme) {
			themes = append(themes, fallback)
		}
	}
	return &Icons{Theme: theme, roots: dedupe(roots), themes: themes, cache: map[string]*ui.SVG{}, embedded: embedded}
}

// DesktopIconTheme is the icon theme the desktop uses: KDE's from
// kdeglobals, GNOME's from gsettings, and Adwaita when neither answers.
func DesktopIconTheme() string {
	if env := os.Getenv("FROSTUI_ICON_THEME"); env != "" {
		return env
	}
	if strings.Contains(strings.ToUpper(os.Getenv("XDG_CURRENT_DESKTOP")), "KDE") {
		home, _ := os.UserHomeDir()
		if name := kdeIconTheme(filepath.Join(home, ".config", "kdeglobals")); name != "" {
			return name
		}
		return "breeze"
	}
	if out, err := exec.Command("gsettings", "get", "org.gnome.desktop.interface", "icon-theme").Output(); err == nil {
		if name := strings.Trim(strings.TrimSpace(string(out)), "'"); name != "" {
			return name
		}
	}
	return "Adwaita"
}

func kdeIconTheme(path string) string {
	f, err := os.Open(path) //nolint:gosec // the user's own kdeglobals
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	section := ""
	for s := bufio.NewScanner(f); s.Scan(); {
		line := strings.TrimSpace(s.Text())
		if strings.HasPrefix(line, "[") {
			section = line
		} else if section == "[Icons]" && strings.HasPrefix(line, "Theme=") {
			return strings.TrimPrefix(line, "Theme=")
		}
	}
	return ""
}

// Get returns the named icon, preferring its symbolic form, or nil.
func (ic *Icons) Get(name string) *ui.SVG {
	if ic == nil || name == "" {
		return nil
	}
	ic.mu.Lock()
	defer ic.mu.Unlock()
	if svg, ok := ic.cache[name]; ok {
		return svg
	}
	var svg *ui.SVG
	base := strings.TrimSuffix(name, "-symbolic")
	for _, n := range []string{base + "-symbolic", base} {
		if data, ok := ic.embedded[n]; ok {
			if s, err := ui.ParseSVG(data); err == nil {
				svg = s
				break
			}
		}
	}
	if svg == nil {
		if path := ic.find(name); path != "" {
			if data, err := os.ReadFile(path); err == nil { //nolint:gosec // icon theme files
				if s, err := ui.ParseSVG(data); err == nil {
					svg = s
				}
			}
		}
	}
	ic.cache[name] = svg
	return svg
}

// find looks for name in the themes in order, symbolic first, in the
// directory layouts of Adwaita (symbolic/<context>, scalable/<context>),
// Breeze (<context>/16, <context>/symbolic) and hicolor.
func (ic *Icons) find(name string) string {
	base := strings.TrimSuffix(name, "-symbolic")
	names := []string{base + "-symbolic", base}
	layouts := []string{"symbolic/*", "scalable/*", "16x16/*", "*/16", "*/symbolic", "*/scalable", "*/22", "*/24", "*/32", "48x48/*"}
	for _, theme := range ic.themes {
		for _, n := range names {
			for _, root := range ic.roots {
				for _, layout := range layouts {
					matches, _ := filepath.Glob(filepath.Join(root, theme, layout, n+".svg"))
					if len(matches) > 0 {
						return matches[0]
					}
				}
			}
		}
	}
	return ""
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := in[:0]
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// icons is the kit's icon source, set by SetIcons.
var icons *Icons

// SetIcons makes ic the icons the kit's components draw.
func SetIcons(ic *Icons) { icons = ic }

// IconSVG returns the named icon from the kit's icons, or nil.
func IconSVG(name string) *ui.SVG { return icons.Get(name) }

// Icon draws the named icon at the text size, in the text color, which
// TextColor changes. A missing icon is an empty box of the text size, as a
// drawn icon is, so layouts do not move.
func Icon(c *ui.Context, name string) ui.Element {
	if svg := icons.Get(name); svg != nil {
		return ui.Icon(c, svg)
	}
	size := c.Theme().FontSize
	if size <= 0 {
		size = 14
	}
	return ui.Box(c).Size(size, size)
}
