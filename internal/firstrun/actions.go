package firstrun

import (
	"github.com/frostyard/chairlift/internal/flatpak"
	"github.com/frostyard/chairlift/internal/i18n"
)

// ActionKind is the kind of a Progress action.
type ActionKind int

// The Progress action kinds.
const (
	// ActionAddFlathub runs `flatpak remote-add --user --if-not-exists
	// flathub https://dl.flathub.org/repo/flathub.flatpakrepo`.
	ActionAddFlathub ActionKind = iota
	// ActionInstall runs `flatpak install -y --user <id>`.
	ActionInstall
)

// Action is one action of the Progress step.
type Action struct {
	Kind ActionKind
	// App is the application of an ActionInstall.
	App App
}

// Title is the action's row title: "Setting up Flathub", or "Installing"
// and the app name, translated.
func (a Action) Title() string {
	if a.Kind == ActionAddFlathub {
		return i18n.T(i18n.MsgSettingUpFlathub)
	}
	return i18n.T(i18n.MsgInstalling) + " " + a.App.Name
}

// PlanActions returns the Progress actions for the selected apps: none when
// no app is selected, else the Flathub remote first, then one install per
// app in selection order.
func PlanActions(selected []App) []Action {
	if len(selected) == 0 {
		return nil
	}
	actions := []Action{{Kind: ActionAddFlathub}}
	for _, app := range selected {
		actions = append(actions, Action{Kind: ActionInstall, App: app})
	}
	return actions
}

// ActionState is the state of one Progress action.
type ActionState int

// The action states. Every action starts Pending.
const (
	ActionPending ActionState = iota
	ActionRunning
	ActionSucceeded
	ActionFailed
)

// Event reports a state change of the action at Index. Err is set for
// ActionFailed.
type Event struct {
	Index int
	State ActionState
	Err   error
}

// Runner runs one action.
type Runner func(Action) error

// FlatpakRunner is the production Runner. It uses internal/flatpak, so the
// flatpak dry-run flag skips remote-add and install (ADR-0009).
func FlatpakRunner(a Action) error {
	if a.Kind == ActionAddFlathub {
		return flatpak.AddRemote(flatpak.FlathubRemoteName, flatpak.FlathubRepoURL, true)
	}
	return flatpak.Install(a.App.ID, true)
}

// Report holds the error of each action by index; nil means it succeeded.
type Report struct {
	Errors []error
}

// Failed reports whether at least one action failed.
func (r Report) Failed() bool {
	for _, err := range r.Errors {
		if err != nil {
			return true
		}
	}
	return false
}

// Execute runs every action in order, also after one failed, as first-setup
// did. emit receives Running, then Succeeded or Failed, for each action.
// The window passes an emit that marshals to the GTK main thread.
func Execute(actions []Action, run Runner, emit func(Event)) Report {
	report := Report{Errors: make([]error, len(actions))}
	for i, action := range actions {
		emit(Event{Index: i, State: ActionRunning})
		if err := run(action); err != nil {
			report.Errors[i] = err
			emit(Event{Index: i, State: ActionFailed, Err: err})
			continue
		}
		emit(Event{Index: i, State: ActionSucceeded})
	}
	return report
}

// ErrorEntry is one entry of the Done step's error log.
type ErrorEntry struct {
	// Title is "Setup failed: " and the action title, translated.
	Title string
	// Message is the action's error text.
	Message string
}

// ErrorLog returns one entry per failed action, in action order. It is empty
// when every action succeeded, and the Done step then shows no error log.
func ErrorLog(actions []Action, report Report) []ErrorEntry {
	var entries []ErrorEntry
	for i, err := range report.Errors {
		if err == nil || i >= len(actions) {
			continue
		}
		entries = append(entries, ErrorEntry{
			Title:   i18n.T(i18n.MsgSetupFailed) + actions[i].Title(),
			Message: err.Error(),
		})
	}
	return entries
}
