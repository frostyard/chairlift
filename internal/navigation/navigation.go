// Package navigation owns ChairLift's canonical page and keyboard-shortcut
// inventory plus the widget-free state transition for selecting a page.
package navigation

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
	{Name: "applications", Title: "Applications", Icon: "application-x-executable-symbolic", Accelerator: "<Alt>1", Display: "Alt+1"},
	{Name: "maintenance", Title: "Maintenance", Icon: "emblem-system-symbolic", Accelerator: "<Alt>2", Display: "Alt+2"},
	{Name: "updates", Title: "Updates", Icon: "software-update-available-symbolic", Accelerator: "<Alt>3", Display: "Alt+3"},
	{Name: "system", Title: "System", Icon: "computer-symbolic", Accelerator: "<Alt>4", Display: "Alt+4"},
	{Name: "features", Title: "Features", Icon: "application-x-addon-symbolic", Accelerator: "<Alt>5", Display: "Alt+5"},
	{Name: "help", Title: "Help", Icon: "help-browser-symbolic", Accelerator: "<Alt>6", Display: "Alt+6"},
}

var generalShortcuts = []Shortcut{
	{Action: "win.show-shortcuts", Accelerator: "<Primary>question", Display: "Ctrl+?", Title: "Keyboard Shortcuts", Group: GroupGeneral},
	{Action: "app.quit", Accelerator: "<Primary>q", Display: "Ctrl+Q", Title: "Quit", Group: GroupGeneral},
	{Action: "win.navigate-help", Accelerator: "F1", Display: "F1", Title: "Help", Group: GroupGeneral},
}

// Items returns a copy of the canonical sidebar inventory.
func Items() []Item {
	return append([]Item(nil), items...)
}

// Shortcuts returns a copy of the canonical advertised shortcut inventory.
func Shortcuts() []Shortcut {
	result := make([]Shortcut, 0, len(items)+len(generalShortcuts))
	for _, item := range items {
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

// Bindings groups the canonical shortcuts by action for GTK registration.
func Bindings() []Binding {
	shortcuts := Shortcuts()
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
func Resolve(pageName string, available func(string) bool) (Transition, bool) {
	if available == nil || !available(pageName) {
		return Transition{}, false
	}
	for index, item := range items {
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
