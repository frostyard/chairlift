// Package maintenance implements the Maintenance page for system cleanup and optimization.
package maintenance

import (
	"context"
	"os/exec"

	"github.com/frostyard/chairlift/internal/config"
)

// Action represents a maintenance action to display/execute.
type Action struct {
	Title       string
	Description string
	Script      string
	Sudo        bool
}

// ScriptExecutor abstracts script execution for testability.
type ScriptExecutor interface {
	Execute(ctx context.Context, script string, sudo bool) error
}

// DefaultExecutor implements ScriptExecutor using os/exec.
// For sudo scripts, it uses pkexec for PolicyKit integration.
type DefaultExecutor struct{}

// Execute runs the script, using pkexec for sudo scripts.
func (e *DefaultExecutor) Execute(ctx context.Context, script string, sudo bool) error {
	var cmd *exec.Cmd
	if sudo {
		cmd = exec.CommandContext(ctx, "pkexec", script)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", script)
	}
	return cmd.Run()
}

// ParseActions extracts actions from a GroupConfig.
// Returns nil if cfg is nil or has no actions.
func ParseActions(cfg *config.GroupConfig) []Action {
	if cfg == nil {
		return nil
	}

	var actions []Action
	for _, a := range cfg.Actions {
		actions = append(actions, Action{
			Title:       a.Title,
			Description: a.Script, // Description same as Script for now
			Script:      a.Script,
			Sudo:        a.Sudo,
		})
	}
	return actions
}
