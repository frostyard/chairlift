// Package navigation owns ChairLift's canonical page and keyboard-shortcut
// inventory and the widget-free decision of which pages show. The window,
// the command palette and the shortcuts dialog all read it; none keeps an
// inventory of its own.
package navigation

import "strconv"

// Shortcut groups used by the shortcuts dialog.
const (
	GroupNavigation = "Navigation"
	GroupGeneral    = "General"
)

// Page IDs. The window's page switch and the palette's "Go to" commands use
// them.
const (
	Overview    = "overview"
	Updates     = "updates"
	Apps        = "apps"
	Features    = "features"
	Problems    = "problems"
	Maintenance = "maintenance"
)

// Actions of the general shortcuts, which the window binds.
const (
	ActionPalette   = "palette"
	ActionConsole   = "console"
	ActionCheck     = "check"
	ActionShortcuts = "shortcuts"
	ActionHelp      = "help"
	ActionQuit      = "quit"
)

// Item describes one page in rail order.
type Item struct {
	Name  string
	Title string
	Icon  string
	// Keys is the page's shortcut, as {"Alt", "1"}, assigned over the
	// visible pages.
	Keys       []string
	ConfigPage string
	// Groups are the configuration groups whose content the page shows.
	// The page shows when any of them is enabled, or always with
	// AlwaysShow.
	Groups     []string
	AlwaysShow bool
}

// Display joins the item's keys as the shortcuts dialog shows them.
func (i Item) Display() string { return join(i.Keys) }

// Shortcut describes one advertised and bound keyboard shortcut.
type Shortcut struct {
	// Action is a page Name for a navigation shortcut, else one of the
	// Action constants.
	Action string
	Keys   []string
	Title  string
	Group  string
}

// Display joins the shortcut's keys as the shortcuts dialog shows them.
func (s Shortcut) Display() string { return join(s.Keys) }

var items = []Item{
	{
		Name:       Overview,
		Title:      "Overview",
		Icon:       "view-grid",
		ConfigPage: "system_page",
		Groups:     []string{"system_info_group", "bootc_status_group", "health_group"},
		// The overview sums up every other page, so it always shows: the
		// window always has a page to open.
		AlwaysShow: true,
	},
	{
		Name:       Updates,
		Title:      "Updates",
		Icon:       "software-update-available",
		ConfigPage: "updates_page",
		Groups: []string{
			"bootc_updates_group",
			"sysupdate_updates_group",
			"firmware_updates_group",
			"flatpak_updates_group",
			"brew_updates_group",
			"brew_trust_group",
		},
	},
	{
		Name:       Apps,
		Title:      "Apps",
		Icon:       "package-x-generic",
		ConfigPage: "applications_page",
		Groups: []string{
			"applications_installed_group",
			"flatpak_user_group",
			"flatpak_system_group",
			"brew_group",
			"brew_search_group",
			"brew_bundles_group",
		},
	},
	{
		Name:       Features,
		Title:      "Features",
		Icon:       "application-x-addon",
		ConfigPage: "features_page",
		Groups:     []string{"features_group"},
	},
	{
		Name:       Problems,
		Title:      "Problems",
		Icon:       "dialog-warning",
		ConfigPage: "problems_page",
		Groups: []string{
			"problems_system_group",
			"problems_apps_group",
			"problems_reporting_group",
		},
	},
	{
		Name:       Maintenance,
		Title:      "Maintenance",
		Icon:       "preferences-other",
		ConfigPage: "maintenance_page",
		Groups: []string{
			"maintenance_cleanup_group",
			"maintenance_brew_group",
			"maintenance_flatpak_group",
		},
	},
}

var generalShortcuts = []Shortcut{
	{Action: ActionPalette, Keys: []string{"Ctrl", "K"}, Title: "Search or run a command", Group: GroupGeneral},
	{Action: ActionConsole, Keys: []string{"Ctrl", "J"}, Title: "Show or hide the console", Group: GroupGeneral},
	{Action: ActionCheck, Keys: []string{"Ctrl", "R"}, Title: "Check for updates", Group: GroupGeneral},
	{Action: ActionShortcuts, Keys: []string{"Ctrl", "?"}, Title: "Keyboard shortcuts", Group: GroupGeneral},
	{Action: ActionHelp, Keys: []string{"F1"}, Title: "Help", Group: GroupGeneral},
	{Action: ActionQuit, Keys: []string{"Ctrl", "Q"}, Title: "Quit", Group: GroupGeneral},
}

// HelpConfigPage and HelpGroup configure the Help dialog's links (F1 and
// the main menu). Help is not a page: it is always reachable.
const (
	HelpConfigPage = "help_page"
	HelpGroup      = "help_resources_group"
)

// Items returns a copy of the complete canonical rail inventory, with
// shortcuts assigned as though every page were visible.
func Items() []Item {
	return compact(items)
}

// VisibleItems filters the canonical inventory using static group
// configuration and compacts the Alt+number shortcuts over the result. The
// Overview always shows, so the window always has a page.
func VisibleItems(enabled func(page, group string) bool) []Item {
	visible := make([]Item, 0, len(items))
	for _, item := range items {
		if item.AlwaysShow || anyGroupEnabled(item, enabled) {
			visible = append(visible, item)
		}
	}
	return compact(visible)
}

// Shortcuts returns the advertised shortcuts for the visible pages plus the
// general shortcuts.
func Shortcuts(visible []Item) []Shortcut {
	result := make([]Shortcut, 0, len(visible)+len(generalShortcuts))
	for _, item := range visible {
		result = append(result, Shortcut{
			Action: item.Name,
			Keys:   append([]string(nil), item.Keys...),
			Title:  "Go to " + item.Title,
			Group:  GroupNavigation,
		})
	}
	for _, s := range generalShortcuts {
		s.Keys = append([]string(nil), s.Keys...)
		result = append(result, s)
	}
	return result
}

// Resolve returns the page to show for pageName: pageName when it is
// visible, else false and the window keeps its page.
func Resolve(pageName string, visible []Item) (Item, bool) {
	for _, item := range visible {
		if item.Name == pageName {
			return item, true
		}
	}
	return Item{}, false
}

// GroupEnabled reports whether the page holding group shows it: the page
// must be visible and the group enabled. Cards use it for cross-page
// content, as the Updates page's Features source.
func GroupEnabled(enabled func(page, group string) bool, page, group string) bool {
	return enabled != nil && enabled(page, group)
}

func anyGroupEnabled(item Item, enabled func(page, group string) bool) bool {
	if enabled == nil {
		return false
	}
	for _, group := range item.Groups {
		if enabled(item.ConfigPage, group) {
			return true
		}
	}
	return false
}

func compact(source []Item) []Item {
	result := make([]Item, len(source))
	for index, item := range source {
		item.Groups = append([]string(nil), item.Groups...)
		item.Keys = []string{"Alt", strconv.Itoa(index + 1)}
		result[index] = item
	}
	return result
}

func join(keys []string) string {
	out := ""
	for i, k := range keys {
		if i > 0 {
			out += "+"
		}
		out += k
	}
	return out
}
