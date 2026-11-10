// Package window draws ChairLift's windows with MyGo: the command-center
// main window, which composes the frostui shell around the views' pages,
// and the first-run window (firstrun.go).
package window

import (
	_ "embed"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/chairlift/internal/navigation"
	"github.com/frostyard/chairlift/internal/sysinfo"
	"github.com/frostyard/chairlift/internal/version"
	"github.com/frostyard/chairlift/internal/views"
	"github.com/frostyard/chairlift/internal/views/pageview"
	"github.com/frostyard/frostui"
)

//go:embed assets/chairlift-symbolic.svg
var logoSVG []byte

// LogoSVG is ChairLift's symbolic icon, as data/icons ships it.
func LogoSVG() []byte { return logoSVG }

// Main is the main window's state: the shell, the pages and the
// navigation inventory. It belongs to the UI thread.
type Main struct {
	Env   *views.Env
	Store *views.Store
	Shell frostui.Shell
	Items []navigation.Item

	// Quit closes the app; the app sets it.
	Quit func()

	configToast bool
	// closeRefused is the title of the job a refused close waits for, until
	// the next frame shows why the window stayed.
	closeRefused string
	help         []pageview.HelpResource
}

// NewMain builds the main window's state from the configuration in env.
func NewMain(env *views.Env, store *views.Store) *Main {
	m := &Main{Env: env, Store: store}
	m.Items = navigation.VisibleItems(env.Enabled)
	m.Shell.Brand = frostui.Brand{Name: "ChairLift", Tagline: "Snow Linux command center", Logo: ui.MustParseSVG(logoSVG)}
	m.Shell.Selected = m.Items[0].Name
	m.Shell.Dialogs = env.Dialogs
	m.Shell.Console.Subtitle = "every command ChairLift runs, as it runs"
	env.SetNavigate(m.Navigate)
	if env.Enabled(navigation.HelpConfigPage, navigation.HelpGroup) {
		if g := env.Group(navigation.HelpConfigPage, navigation.HelpGroup); g != nil {
			m.help = pageview.HelpResources(g.Website, g.Issues, g.Chat)
		}
	}
	return m
}

// Navigate shows page when it is visible; the rail, the palette and the
// pages' links all come here.
func (m *Main) Navigate(page string) {
	if item, ok := navigation.Resolve(page, m.Items); ok {
		m.Shell.Selected = item.Name
	}
}

// CanClose reports whether closing the window now is safe: false while a
// job a cancel cannot stop runs, since the app would end with the work
// still going on, and a new ChairLift's empty queue could start a second
// change beside it. A stoppable job or one that only waits does not hold
// the window. The app vetoes the window's close, and so mygo.App.Quit and
// SIGTERM too, while it is false; a refusal shows a toast in the next frame.
// It runs on the UI thread.
func (m *Main) CanClose() bool {
	job := m.Env.Jobs.Current()
	if job == nil || !job.Unstoppable {
		return true
	}
	title := job.Title
	log.Printf("window: close refused while %q runs", title)
	// A close draws no frame of its own: the post asks for one.
	m.Env.Post(func() { m.closeRefused = title })
	return false
}

// View draws the window.
func (m *Main) View(c *ui.Context) {
	for _, t := range m.Env.TakeToasts() {
		c.AddToast(t)
	}
	if m.Env.ConfigErr != nil && !m.configToast {
		// The configuration error stays until the file is fixed and
		// ChairLift restarts (ADR-0003).
		m.configToast = true
		c.AddToast(ui.Toast{ID: "config-error", Title: "Configuration error", Description: configToastText(m.Env.ConfigErr), Type: "error", Timeout: -1})
	}
	if m.closeRefused != "" {
		// One ID: closing again replaces the toast instead of stacking it.
		c.AddToast(ui.Toast{ID: "close-refused", Title: closeRefusedText(m.closeRefused), Type: "info"})
		m.closeRefused = ""
	}
	m.bindShortcuts(c)
	if _, ok := navigation.Resolve(m.Shell.Selected, m.Items); !ok {
		m.Shell.Selected = m.Items[0].Name
	}
	page := m.Store.Page(m.Shell.Selected)
	m.Shell.View(c, frostui.Frame{
		Nav:     m.nav(),
		Actions: func() { m.actions(c) },
		Menu:    m.menu,
		RailFooter: func() {
			frostui.JobCard(c, m.Env.Jobs)
			m.hostCard(c)
		},
		Content: func() {
			if page != nil {
				page.View(c)
			}
		},
		Commands:    m.Commands(),
		Status:      m.statusStart(),
		StatusEnd:   m.statusEnd(),
		ConsoleData: m.Env.Console,
	})
}

// closeRefusedText says why the window stayed open after a close.
func closeRefusedText(job string) string {
	return "ChairLift cannot close yet: " + job + " must finish first."
}

func configToastText(err error) string {
	type toaster interface{ ToastMessage() string }
	if t, ok := err.(toaster); ok {
		return t.ToastMessage()
	}
	return err.Error()
}

func (m *Main) nav() []frostui.NavSection {
	sec := frostui.NavSection{Title: "System"}
	for _, item := range m.Items {
		ni := frostui.NavItem{ID: item.Name, Title: item.Title, Icon: item.Icon, Shortcut: item.Display()}
		if p := m.Store.Page(item.Name); p != nil {
			ni.Badge, ni.BadgeTone = p.Badge()
		}
		sec.Items = append(sec.Items, ni)
	}
	return []frostui.NavSection{sec}
}

func (m *Main) actions(c *ui.Context) {
	check := frostui.IconButton(c, "view-refresh", "Check for updates (Ctrl R)").Disabled(m.Store.Checking())
	if check.Clicked() {
		m.Store.CheckAll()
	}
}

func (m *Main) menu(menu *ui.Menu) {
	if menu.Item("Command palette").Shortcut(ui.Cmd, ui.KeyK).Chosen() {
		m.Shell.Palette.Show()
	}
	if menu.Item("Keyboard shortcuts").Shortcut(ui.Cmd|ui.Shift, ui.KeySlash).Chosen() {
		m.showShortcuts()
	}
	if menu.Item("Help").Shortcut(0, ui.KeyF1).Chosen() {
		m.showHelp()
	}
	if menu.Item("About ChairLift").Chosen() {
		m.showAbout()
	}
	menu.Separator()
	if menu.Item("Quit").Shortcut(ui.Cmd, ui.KeyQ).Chosen() && m.Quit != nil {
		m.Quit()
	}
}

// bindShortcuts binds every shortcut navigation advertises, except the
// palette's and the console's, which the shell binds itself.
func (m *Main) bindShortcuts(c *ui.Context) {
	for _, s := range navigation.Shortcuts(m.Items) {
		if s.Action == navigation.ActionPalette || s.Action == navigation.ActionConsole {
			continue
		}
		mods, key, ok := keyOf(s.Keys)
		if !ok {
			log.Printf("window: shortcut %v has no key binding", s.Keys)
			continue
		}
		action := s.Action
		c.OnWindowShortcut(mods, key, func() { m.runAction(action) })
	}
}

func (m *Main) runAction(action string) {
	switch action {
	case navigation.ActionPalette:
		m.Shell.Palette.Show()
	case navigation.ActionConsole:
		m.Shell.Console.Open = !m.Shell.Console.Open
	case navigation.ActionCheck:
		m.Store.CheckAll()
	case navigation.ActionShortcuts:
		m.showShortcuts()
	case navigation.ActionHelp:
		m.showHelp()
	case navigation.ActionQuit:
		if m.Quit != nil {
			m.Quit()
		}
	default:
		m.Navigate(action)
	}
}

// keyOf maps navigation's key names to MyGo's.
func keyOf(keys []string) (ui.Modifiers, ui.Key, bool) {
	var mods ui.Modifiers
	for i, k := range keys {
		last := i == len(keys)-1
		switch {
		case k == "Ctrl" && !last:
			mods |= ui.Cmd
		case k == "Alt" && !last:
			mods |= ui.Alt
		case k == "Shift" && !last:
			mods |= ui.Shift
		case last:
			key, ok := keyNames[k]
			if k == "?" {
				mods |= ui.Shift
			}
			return mods, key, ok
		default:
			return 0, 0, false
		}
	}
	return 0, 0, false
}

var keyNames = map[string]ui.Key{
	"1": ui.Key1, "2": ui.Key2, "3": ui.Key3, "4": ui.Key4, "5": ui.Key5,
	"6": ui.Key6, "7": ui.Key7, "8": ui.Key8, "9": ui.Key9,
	"K": ui.KeyK, "J": ui.KeyJ, "R": ui.KeyR, "Q": ui.KeyQ, "?": ui.KeySlash, "F1": ui.KeyF1,
}

// Commands are the palette's: every page's, then "Go to" each page, then
// the window's own.
func (m *Main) Commands() []frostui.Command {
	var cmds []frostui.Command
	for _, item := range m.Items {
		if p := m.Store.Page(item.Name); p != nil {
			cmds = append(cmds, p.Commands()...)
		}
	}
	for _, item := range m.Items {
		name := item.Name
		cmds = append(cmds, frostui.Command{
			ID: "go-" + name, Title: "Go to " + item.Title, Group: "Go to", Icon: item.Icon,
			Keys: item.Keys, Run: func() { m.Navigate(name) },
		})
	}
	cmds = append(cmds,
		frostui.Command{ID: "check", Title: "Check for updates", Group: "ChairLift", Icon: "view-refresh", Keys: []string{"Ctrl", "R"},
			Disabled: m.Store.Checking(), Reason: "checking", Run: m.Store.CheckAll},
		frostui.Command{ID: "stop-jobs", Title: "Stop the running jobs", Group: "ChairLift", Icon: "process-stop",
			Hint: "cancel queue", Disabled: !m.Env.Jobs.Busy(), Reason: "nothing runs", Run: m.Env.Jobs.CancelAll},
		frostui.Command{ID: "console", Title: "Show or hide the console", Group: "ChairLift", Icon: "utilities-terminal", Keys: []string{"Ctrl", "J"},
			Run: func() { m.Shell.Console.Open = !m.Shell.Console.Open }},
		frostui.Command{ID: "shortcuts", Title: "Keyboard shortcuts", Group: "ChairLift", Icon: "input-keyboard", Keys: []string{"Ctrl", "?"}, Run: m.showShortcuts},
		frostui.Command{ID: "help", Title: "Help", Group: "ChairLift", Icon: "help-browser", Keys: []string{"F1"}, Run: m.showHelp},
		frostui.Command{ID: "about", Title: "About ChairLift", Group: "ChairLift", Icon: "help-about", Run: m.showAbout},
	)
	return uniqueCommands(cmds)
}

// uniqueCommands keeps the first command of each ID: two pages may list
// the same shared command (Homebrew's on Apps and Updates), and the palette
// keys its rows by ID.
func uniqueCommands(cmds []frostui.Command) []frostui.Command {
	seen := make(map[string]bool, len(cmds))
	out := cmds[:0]
	for _, c := range cmds {
		if !seen[c.ID] {
			seen[c.ID] = true
			out = append(out, c)
		}
	}
	return out
}

func (m *Main) showShortcuts() {
	var entries []frostui.ShortcutEntry
	for _, s := range navigation.Shortcuts(m.Items) {
		entries = append(entries, frostui.ShortcutEntry{Group: s.Group, Title: s.Title, Keys: s.Keys})
	}
	m.Shell.Info.Show("Keyboard shortcuts", frostui.ShortcutsBody(entries))
}

func (m *Main) showHelp() {
	links := m.help
	m.Shell.Info.Show("Help", func(c *ui.Context) {
		frostui.Muted(c, "ChairLift keeps Snow Linux up to date and tidy. Everything it runs shows in the console (Ctrl J).").Wrap()
		if len(links) == 0 {
			frostui.Muted(c, "No help links are configured.")
			return
		}
		for i, l := range links {
			ui.Row(c.Key(i)).Gap(frostui.SpaceS).AlignItems(ui.Center).Children(func() {
				frostui.Icon(c, "web-browser")
				ui.Link(c, l.Title, l.URL)
			})
		}
	})
}

func (m *Main) showAbout() {
	m.Shell.Info.Show("About ChairLift", frostui.AboutBody(frostui.About{
		Name: "ChairLift", Version: version.Version, Summary: "The system manager of Snow Linux.",
		Logo: "org.frostyard.ChairLift-symbolic", Website: "https://github.com/frostyard/chairlift",
		Issues: "https://github.com/frostyard/chairlift/issues", License: "MIT",
		Copyright: "© 2025-2026 Brian Ketelsen", Developers: []string{"Brian Ketelsen", "ChairLift contributors"},
	}))
}

func (m *Main) hostCard(c *ui.Context) {
	info := m.Store.System.Info()
	host := info.Hostname
	if host == "" {
		return
	}
	t := c.Theme()
	ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Avatar(c, host, nil).Size(32, 32)
		ui.Column(c).Grow(1).Children(func() {
			ui.Text(c, host).Bold().FontSize(13).SingleLine()
			sub := info.Model
			if sub == "" && info.OSVersion != "" {
				sub = strings.TrimSpace(info.OSName + " " + info.OSVersion)
			}
			ui.Text(c, sub).FontSize(11).TextColor(t.TextMuted).SingleLine()
		})
	})
}

func (m *Main) statusStart() []frostui.StatusItem {
	info := m.Store.System.Info()
	var items []frostui.StatusItem
	if info.Booted.Present {
		name := info.OSName
		if name == "" {
			name = "Booted"
		}
		items = append(items, frostui.StatusItem{Icon: "security-high", Text: name + " " + info.Booted.Version, Tooltip: "The running system image", Progress: -1})
	}
	if info.Channel != "" {
		items = append(items, frostui.StatusItem{Icon: "media-playlist-shuffle", Text: info.Channel, Tooltip: "Update channel", Progress: -1})
	}
	if info.Uptime != "" {
		items = append(items, frostui.StatusItem{Icon: "preferences-system-time", Text: info.Uptime, Progress: -1})
	}
	if info.Staged.Present {
		items = append(items, frostui.StatusItem{Icon: "system-reboot", Text: info.Staged.Version + " staged", Tone: frostui.Accent,
			Tooltip: "Restart to use it", Progress: -1, OnClick: func() { m.Navigate(navigation.Overview) }})
	}
	if m.Env.DryRun {
		items = append(items, frostui.StatusItem{Icon: "dialog-information", Text: "Dry run: nothing changes", Tone: frostui.Warning, Progress: -1})
	}
	return items
}

func (m *Main) statusEnd() []frostui.StatusItem {
	var items []frostui.StatusItem
	if cur := m.Env.Jobs.Current(); cur != nil {
		items = append(items, frostui.StatusItem{Text: cur.Title, Progress: max(m.Env.Jobs.BatchFraction(), 0.01),
			OnClick: func() { m.Shell.Console.Open = true }, Tooltip: "Show the console"})
	}
	if _, ok := navigation.Resolve(navigation.Updates, m.Items); ok {
		n, _ := m.Store.UpdateCount()
		items = append(items, frostui.StatusItem{Icon: "software-update-available", Text: plural(n, "update", "updates"), Progress: -1,
			Tone:    map[bool]frostui.Tone{true: frostui.Accent, false: frostui.Neutral}[n > 0],
			OnClick: func() { m.Navigate(navigation.Updates) }, Tooltip: "Open Updates"})
	}
	if _, ok := navigation.Resolve(navigation.Problems, m.Items); ok {
		if p := m.Store.Problems.Summary(); p.Known {
			tone := frostui.Neutral
			if p.Attention > 0 {
				tone = frostui.Warning
			}
			items = append(items, frostui.StatusItem{Icon: "dialog-warning", Text: plural(p.Count, "problem", "problems"), Tone: tone, Progress: -1,
				OnClick: func() { m.Navigate(navigation.Problems) }, Tooltip: "Open Problems"})
		}
	}
	if d := m.Store.Live.Latest().Disk; d.Total > 0 {
		items = append(items, frostui.StatusItem{Icon: "drive-harddisk", Text: sysinfo.FormatBytes(d.Free) + " free", Progress: -1,
			Tooltip: "Free space on " + d.Path})
	}
	return items
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}

// Ready logs the startup marker the end-to-end test waits for (ADR-0008).
func Ready(since time.Time) {
	log.Printf("app: window presented in %s (since activate)", time.Since(since))
}

// Describe is a one-line description of the window for logs.
func (m *Main) Describe() string {
	names := make([]string, len(m.Items))
	for i, it := range m.Items {
		names[i] = it.Name
	}
	return fmt.Sprintf("pages=%s", strings.Join(names, ","))
}
