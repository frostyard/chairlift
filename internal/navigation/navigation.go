// Package navigation owns ChairLift's canonical page and keyboard-shortcut
// inventory plus the widget-free state transition for selecting a page.
package navigation

import "strconv"

// Shortcut groups used by the shortcuts dialog.
const (
	GroupNavigation = "navigation"
	GroupGeneral    = "general"
)

// Item describes one page in sidebar order.
type Item struct {
	Name        string
	Title       string
	Icon        string
	Accelerator string
	Display     string
	ConfigPage  string
	Groups      []string
	AlwaysShow  bool
}

// Shortcut describes one advertised and registered keyboard shortcut.
type Shortcut struct {
	Action      string
	Accelerator string
	Display     string
	Title       string
	Group       string
}

// Binding groups every accelerator registered for one application action.
type Binding struct {
	Action       string
	Accelerators []string
}

// Transition is the complete UI state change for navigating to one page.
type Transition struct {
	SelectedIndex int
	VisibleChild  string
	Title         string
	ShowContent   bool
}

var items = []Item{
	{
		Name:       "applications",
		Title:      "Applications",
		Icon:       "application-x-executable-symbolic",
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
		Name:       "maintenance",
		Title:      "Maintenance",
		Icon:       "emblem-system-symbolic",
		ConfigPage: "maintenance_page",
		Groups: []string{
			"maintenance_cleanup_group",
			"maintenance_brew_group",
			"maintenance_flatpak_group",
			"maintenance_optimization_group",
		},
	},
	{
		Name:       "updates",
		Title:      "Updates",
		Icon:       "software-update-available-symbolic",
		ConfigPage: "updates_page",
		Groups: []string{
			"bootc_updates_group",
			"sysupdate_updates_group",
			"flatpak_updates_group",
			"brew_updates_group",
			"brew_trust_group",
		},
	},
	{
		Name:       "system",
		Title:      "System",
		Icon:       "computer-symbolic",
		ConfigPage: "system_page",
		Groups: []string{
			"system_info_group",
			"bootc_status_group",
			"health_group",
		},
	},
	{
		Name:       "features",
		Title:      "Features",
		Icon:       "application-x-addon-symbolic",
		ConfigPage: "features_page",
		Groups:     []string{"features_group"},
	},
	{
		Name:       "help",
		Title:      "Help",
		Icon:       "help-browser-symbolic",
		ConfigPage: "help_page",
		Groups:     []string{"help_resources_group"},
		AlwaysShow: true,
	},
}

var generalShortcuts = []Shortcut{
	{Action: "win.show-shortcuts", Accelerator: "<Primary>question", Display: "Ctrl+?", Title: "Keyboard Shortcuts", Group: GroupGeneral},
	{Action: "app.quit", Accelerator: "<Primary>q", Display: "Ctrl+Q", Title: "Quit", Group: GroupGeneral},
	{Action: "win.navigate-help", Accelerator: "F1", Display: "F1", Title: "Help", Group: GroupGeneral},
}

// Items returns a copy of the complete canonical sidebar inventory, with
// accelerators assigned as though every page were visible.
func Items() []Item {
	return compact(items)
}

// VisibleItems filters the canonical inventory using static group
// configuration and compacts Alt+number accelerators over the result. Help is
// always retained so the window always has a valid destination.
func VisibleItems(enabled func(page, group string) bool) []Item {
	visible := make([]Item, 0, len(items))
	for _, item := range items {
		if item.AlwaysShow || anyGroupEnabled(item, enabled) {
			visible = append(visible, item)
		}
	}
	return compact(visible)
}

// Shortcuts returns the advertised shortcuts for the supplied visible pages
// plus the canonical general shortcuts.
func Shortcuts(visible []Item) []Shortcut {
	result := make([]Shortcut, 0, len(visible)+len(generalShortcuts))
	for _, item := range visible {
		result = append(result, Shortcut{
			Action:      "win.navigate-" + item.Name,
			Accelerator: item.Accelerator,
			Display:     item.Display,
			Title:       "Go to " + item.Title,
			Group:       GroupNavigation,
		})
	}
	return append(result, generalShortcuts...)
}

// Bindings groups the shortcuts for the supplied visible pages by action for
// GTK registration.
func Bindings(visible []Item) []Binding {
	shortcuts := Shortcuts(visible)
	indexes := make(map[string]int, len(shortcuts))
	bindings := make([]Binding, 0, len(shortcuts))
	for _, shortcut := range shortcuts {
		index, ok := indexes[shortcut.Action]
		if !ok {
			index = len(bindings)
			indexes[shortcut.Action] = index
			bindings = append(bindings, Binding{Action: shortcut.Action})
		}
		bindings[index].Accelerators = append(
			bindings[index].Accelerators,
			shortcut.Accelerator,
		)
	}
	return bindings
}

// Resolve derives every state mutation needed to navigate to pageName.
// It rejects unknown pages and pages the caller did not construct.
func Resolve(pageName string, visible []Item, available func(string) bool) (Transition, bool) {
	if available == nil || !available(pageName) {
		return Transition{}, false
	}
	for index, item := range visible {
		if item.Name == pageName {
			return Transition{
				SelectedIndex: index,
				VisibleChild:  item.Name,
				Title:         item.Title,
				ShowContent:   true,
			}, true
		}
	}
	return Transition{}, false
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
		number := strconv.Itoa(index + 1)
		item.Groups = append([]string(nil), item.Groups...)
		item.Accelerator = "<Alt>" + number
		item.Display = "Alt+" + number
		result[index] = item
	}
	return result
}
