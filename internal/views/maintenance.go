package views

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/frostyard/chairlift/internal/cmdlog"
	"github.com/frostyard/chairlift/internal/flatpak"
	"github.com/frostyard/chairlift/internal/frostui"
	"github.com/frostyard/chairlift/internal/homebrew"
	"github.com/frostyard/chairlift/internal/sysinfo"
	"github.com/frostyard/chairlift/internal/views/actionmsg"
	"github.com/frostyard/chairlift/internal/views/pageview"
)

// The Maintenance page's configuration page, its groups, and the source of
// its jobs and activity entries.
const (
	maintPage         = "maintenance_page"
	maintScriptsGroup = "maintenance_cleanup_group"
	maintBrewGroup    = "maintenance_brew_group"
	maintFlatpakGroup = "maintenance_flatpak_group"
	maintSource       = "maintenance"

	// maintScriptTimeout bounds one configured maintenance script, as
	// runMaintenanceAction did.
	maintScriptTimeout = 5 * time.Minute
	// maintWaitDelay is how long a script's run waits, after the script
	// ended or was killed, for children that still hold its output open.
	maintWaitDelay = 5 * time.Second
)

// The commands homebrew.Cleanup and flatpak.UninstallUnused run, as the
// rows show them. TestMaintenanceBackendCommandsMatchTheRows keeps them in
// step with the backends.
var (
	brewCleanupArgv   = []string{"brew", "cleanup"}
	flatpakUnusedArgv = []string{"flatpak", "uninstall", "--unused", "-y"}
)

// cleanupKind is what runs a cleanup.
type cleanupKind int

const (
	// cleanupByScript is a configured action of maintenance_cleanup_group,
	// run as pageview.MaintenanceCommand says.
	cleanupByScript cleanupKind = iota
	// cleanupByBrew is homebrew.Cleanup.
	cleanupByBrew
	// cleanupByFlatpak is flatpak.UninstallUnused.
	cleanupByFlatpak
)

// maintTool is what the page knows of a cleanup's tool.
type maintTool int

const (
	// maintToolUnknown is before the check ended (or began).
	maintToolUnknown maintTool = iota
	// maintToolPresent: the tool is installed; its cleanup is offered.
	maintToolPresent
	// maintToolMissing: the tool is not installed; its cleanup is left out.
	maintToolMissing
)

// cleanup is one row of the "Clean up" card.
type cleanup struct {
	// Key identifies the cleanup among the page's jobs: "brew",
	// "flatpak", or "script:<index>" for a configured action.
	Key  string
	Kind cleanupKind
	// Title heads the row; Short names it in the "Clean everything" list
	// and the palette.
	Title, Short string
	// Blurb says in one sentence what it does, in the confirmation.
	Blurb string
	Icon  string
	// Argv is the command it runs, exactly; the row shows it.
	Argv []string
	// Admin is true when the command asks for administrator
	// authentication (pkexec).
	Admin bool
	// Checking is true while the cleanup's tool is being looked for: the
	// row shows a spinner and offers nothing yet.
	Checking bool

	// script and sudo are the configured action (cleanupByScript only).
	script string
	sudo   bool
}

// Ready reports whether the cleanup can run now (its tool was found).
func (cl cleanup) Ready() bool { return !cl.Checking }

// cleanupOutcome is how a cleanup's last run ended.
type cleanupOutcome int

const (
	cleanupNever cleanupOutcome = iota
	cleanupDone
	// cleanupPreviewed: a dry run, which changed nothing.
	cleanupPreviewed
	cleanupFailed
	cleanupCanceled
)

// cleanupResult is a cleanup's last run, shown on its row.
type cleanupResult struct {
	Outcome cleanupOutcome
	Err     string
	At      time.Time
}

// Maintenance holds the cleanups: the configured maintenance actions, the
// Homebrew cleanup and the removal of unused Flatpak runtimes. It belongs
// to the UI thread.
type Maintenance struct {
	env   *Env
	store *Store

	brew, flatpak maintTool
	started       bool
	results       map[string]cleanupResult

	// The backends, replaced in tests. Every one of them except the
	// install checks runs on a job's goroutine.
	brewInstalled    func() bool
	flatpakInstalled func() bool
	brewCleanup      func() (string, error)
	flatpakCleanup   func() (string, error)
	// brewDryRun and flatpakDryRun are the wrappers' own dry-run flags,
	// which decide whether their cleanup ran: actionmsg.Cleanup's input.
	brewDryRun    func() bool
	flatpakDryRun func() bool
	// runCommand runs a configured action's command.
	runCommand func(ctx context.Context, cmd pageview.Command) error
	// disk and info read the shared state of Live and System.
	disk func() sysinfo.Disk
	info func() SystemInfo
}

// NewMaintenance returns the maintenance state.
func NewMaintenance(env *Env, s *Store) *Maintenance {
	return &Maintenance{
		env:              env,
		store:            s,
		results:          map[string]cleanupResult{},
		brewInstalled:    homebrew.IsInstalledCached,
		flatpakInstalled: flatpak.IsInstalledCached,
		brewCleanup:      homebrew.Cleanup,
		flatpakCleanup:   flatpak.UninstallUnused,
		brewDryRun:       homebrew.IsDryRun,
		flatpakDryRun:    flatpak.IsDryRun,
		runCommand:       runMaintenanceCommand,
		disk: func() sysinfo.Disk {
			if s == nil || s.Live == nil {
				return sysinfo.Disk{}
			}
			return s.Live.Latest().Disk
		},
		info: func() SystemInfo {
			if s == nil || s.System == nil {
				return SystemInfo{}
			}
			return s.System.Info()
		},
	}
}

// Start looks for Homebrew and Flatpak, for the groups that are on. Until
// a check ends its cleanup shows as being checked; a missing tool's cleanup
// is then left out, as the old page hid its group.
func (m *Maintenance) Start() {
	if m.started {
		return
	}
	m.started = true
	if m.env.Enabled(maintPage, maintBrewGroup) {
		Go(m.env, m.brewInstalled, func(ok bool) { m.brew = maintPresence(ok) })
	}
	if m.env.Enabled(maintPage, maintFlatpakGroup) {
		Go(m.env, m.flatpakInstalled, func(ok bool) { m.flatpak = maintPresence(ok) })
	}
}

func maintPresence(ok bool) maintTool {
	if ok {
		return maintToolPresent
	}
	return maintToolMissing
}

// Cleanups are the rows of the "Clean up" card, in the order "Clean
// everything" runs them: the configured actions, Homebrew, Flatpak. A
// disabled group's cleanup is never listed; a tool not yet found is listed
// as Checking; a missing tool's is left out.
func (m *Maintenance) Cleanups() []cleanup {
	var out []cleanup
	if m.env.Enabled(maintPage, maintScriptsGroup) {
		if g := m.env.Group(maintPage, maintScriptsGroup); g != nil {
			for i, a := range g.Actions {
				cmd := pageview.MaintenanceCommand(a.Script, a.Sudo)
				blurb := "Runs " + a.Script + "."
				if a.Sudo {
					blurb = "Runs " + a.Script + " as administrator."
				}
				out = append(out, cleanup{
					Key:    "script:" + strconv.Itoa(i),
					Kind:   cleanupByScript,
					Title:  a.Title,
					Short:  a.Title,
					Blurb:  blurb,
					Icon:   "system-run",
					Argv:   append([]string{cmd.Name}, cmd.Args...),
					Admin:  a.Sudo,
					script: a.Script,
					sudo:   a.Sudo,
				})
			}
		}
	}
	if m.env.Enabled(maintPage, maintBrewGroup) && m.brew != maintToolMissing {
		out = append(out, cleanup{
			Key:      "brew",
			Kind:     cleanupByBrew,
			Title:    "Homebrew downloads and old versions",
			Short:    "Homebrew",
			Blurb:    "Removes old versions of installed packages and clears Homebrew's download cache.",
			Icon:     "package-x-generic",
			Argv:     brewCleanupArgv,
			Checking: m.brew == maintToolUnknown,
		})
	}
	if m.env.Enabled(maintPage, maintFlatpakGroup) && m.flatpak != maintToolMissing {
		out = append(out, cleanup{
			Key:      "flatpak",
			Kind:     cleanupByFlatpak,
			Title:    "Unused Flatpak runtimes",
			Short:    "Flatpak runtimes",
			Blurb:    "Uninstalls the runtimes and extensions no installed app uses, in every Flatpak installation.",
			Icon:     "application-x-addon",
			Argv:     flatpakUnusedArgv,
			Checking: m.flatpak == maintToolUnknown,
		})
	}
	return out
}

// ready returns the cleanups that can run now.
func (m *Maintenance) ready() []cleanup {
	var out []cleanup
	for _, cl := range m.Cleanups() {
		if cl.Ready() {
			out = append(out, cl)
		}
	}
	return out
}

func (m *Maintenance) find(key string) (cleanup, bool) {
	for _, cl := range m.Cleanups() {
		if cl.Key == key {
			return cl, true
		}
	}
	return cleanup{}, false
}

// Pending reports whether a job of the cleanup waits or runs.
func (m *Maintenance) Pending(key string) bool { return m.env.Jobs.Pending(maintSource, key) }

// Result is the cleanup's last run.
func (m *Maintenance) Result(key string) cleanupResult { return m.results[key] }

// Ask asks before it queues the cleanup with key. It does nothing for a
// cleanup that cannot run now or whose job already waits or runs.
func (m *Maintenance) Ask(key string) {
	cl, ok := m.find(key)
	if !ok || !cl.Ready() || m.Pending(key) {
		return
	}
	req := frostui.Confirm{
		Title:   cl.question(),
		Body:    cl.Blurb + m.dryRunNote(),
		Details: []frostui.KeyValue{{Key: "Command", Value: strings.Join(cl.Argv, " "), Mono: true}},
		Action:  "Run cleanup",
		OnConfirm: func() {
			// The configuration and the tools cannot change under a
			// dialog, but look again: the job may have been queued from
			// elsewhere meanwhile.
			if cl, ok := m.find(key); ok && cl.Ready() && !m.Pending(key) {
				m.env.Jobs.Enqueue(m.job(cl))
			}
		},
	}
	if cl.Admin {
		req.Note = "Needs administrator authentication."
	}
	m.env.Dialogs.Ask(req)
}

// AskEverything asks once, listing every cleanup that can run, and then
// queues them all in order. Each privileged one asks for administrator
// authentication when its turn comes.
func (m *Maintenance) AskEverything() {
	cls := m.idle()
	if len(cls) == 0 {
		return
	}
	details := make([]frostui.KeyValue, 0, len(cls))
	admin := 0
	for _, cl := range cls {
		details = append(details, frostui.KeyValue{Key: cl.Short, Value: strings.Join(cl.Argv, " "), Mono: true})
		if cl.Admin {
			admin++
		}
	}
	body := "Runs the cleanup."
	if len(cls) > 1 {
		body = fmt.Sprintf("Runs %d cleanups, one after the other.", len(cls))
	}
	req := frostui.Confirm{
		Title:   "Clean everything?",
		Body:    body + m.dryRunNote(),
		Details: details,
		Action:  "Run all",
		OnConfirm: func() {
			var jobs []*frostui.Job
			for _, cl := range m.idle() {
				jobs = append(jobs, m.job(cl))
			}
			if len(jobs) > 0 {
				m.env.Jobs.Enqueue(jobs...)
			}
		},
	}
	switch {
	case admin == 1:
		req.Note = "One of them needs administrator authentication."
	case admin > 1:
		req.Note = "Each one marked admin asks for administrator authentication."
	}
	m.env.Dialogs.Ask(req)
}

// idle returns the cleanups that can run now and have no job waiting or
// running: what "Clean everything" queues.
func (m *Maintenance) idle() []cleanup {
	var out []cleanup
	for _, cl := range m.ready() {
		if !m.Pending(cl.Key) {
			out = append(out, cl)
		}
	}
	return out
}

func (m *Maintenance) dryRunNote() string {
	if m.env.DryRun {
		return " Dry run: the command is only shown in the console; nothing changes."
	}
	return ""
}

func (cl cleanup) question() string {
	switch cl.Kind {
	case cleanupByBrew:
		return "Clean up Homebrew?"
	case cleanupByFlatpak:
		return "Remove unused Flatpak runtimes?"
	}
	return "Run " + cl.Title + "?"
}

// paletteTitle is the cleanup's entry in the command palette.
func (cl cleanup) paletteTitle() string {
	switch cl.Kind {
	case cleanupByBrew:
		return "Clean up Homebrew"
	case cleanupByFlatpak:
		return "Clean up unused Flatpak runtimes"
	}
	return cl.Title
}

// job returns the job that runs cl. Its Done applies the result on the UI
// thread: the row's outcome, a toast built by actionmsg as the old page
// did, and an activity entry after a live success.
func (m *Maintenance) job(cl cleanup) *frostui.Job {
	// A sudo: true script runs as root through pkexec: a stop could signal
	// only pkexec, so the job finishes first (frostui.Job.Unstoppable).
	j := &frostui.Job{Source: maintSource, Key: cl.Key, Unstoppable: cl.Kind == cleanupByScript && cl.sudo}
	switch cl.Kind {
	case cleanupByBrew:
		j.Title = "Cleaning up Homebrew"
		var out string
		run := m.brewCleanup
		j.Run = func(context.Context, *frostui.Progress) (err error) {
			out, err = run()
			return err
		}
		j.Done = func(err error) {
			m.toolDone(cl, "Homebrew", m.brewDryRun(), out, err)
		}
	case cleanupByFlatpak:
		j.Title = "Removing unused Flatpak runtimes"
		var out string
		run := m.flatpakCleanup
		j.Run = func(context.Context, *frostui.Progress) (err error) {
			out, err = run()
			return err
		}
		j.Done = func(err error) {
			m.toolDone(cl, "Flatpak", m.flatpakDryRun(), out, err)
		}
	default:
		j.Title = "Running " + cl.Title
		// The decision is made once, on the UI thread, before the job
		// runs; the job never recomputes it (actionmsg.MaintenanceScript).
		decision := actionmsg.MaintenanceScript(m.env.DryRun, cl.Title)
		cmd := pageview.MaintenanceCommand(cl.script, cl.sudo)
		title, script, sudo := cl.Title, cl.script, cl.sudo
		run := m.runCommand
		j.Run = func(ctx context.Context, _ *frostui.Progress) error {
			log.Printf("Running action: %s (script: %s, sudo: %v)", title, script, sudo)
			if !decision.Execute {
				// No exec.Cmd exists in a dry run.
				log.Printf("[DRY-RUN] Would execute: %s", strings.Join(append([]string{cmd.Name}, cmd.Args...), " "))
				cmdlog.DryRun(cmd.Name, cmd.Args...)
				return nil
			}
			ctx, cancel := context.WithTimeout(ctx, maintScriptTimeout)
			defer cancel()
			return run(ctx, cmd)
		}
		j.Done = func(err error) {
			switch {
			case errors.Is(err, context.Canceled):
				m.setResult(cl.Key, cleanupCanceled, "")
			case err != nil:
				m.setResult(cl.Key, cleanupFailed, err.Error())
				m.env.ToastError(fmt.Sprintf("%s failed: %v", title, err))
			case !decision.Execute:
				m.setResult(cl.Key, cleanupPreviewed, "")
				m.env.Toast(decision.Toast)
			default:
				m.setResult(cl.Key, cleanupDone, "")
				m.env.ToastSuccess(decision.Toast)
				m.env.Record(maintSource, decision.Toast, strings.Join(cl.Argv, " "))
			}
		}
	}
	return j
}

// toolDone applies the end of the Homebrew or Flatpak cleanup.
func (m *Maintenance) toolDone(cl cleanup, tool string, dryRun bool, out string, err error) {
	switch {
	case errors.Is(err, context.Canceled):
		m.setResult(cl.Key, cleanupCanceled, "")
	case err != nil:
		m.setResult(cl.Key, cleanupFailed, err.Error())
		m.env.ToastError(fmt.Sprintf("%s cleanup failed: %v", tool, err))
	case dryRun:
		m.setResult(cl.Key, cleanupPreviewed, "")
		m.env.Toast(actionmsg.Cleanup(true, tool, out))
	default:
		msg := actionmsg.Cleanup(false, tool, out)
		m.setResult(cl.Key, cleanupDone, "")
		m.env.ToastSuccess(msg)
		m.env.Record(maintSource, msg, strings.Join(cl.Argv, " "))
	}
}

func (m *Maintenance) setResult(key string, o cleanupOutcome, err string) {
	m.results[key] = cleanupResult{Outcome: o, Err: err, At: m.env.Now()}
}

// runMaintenanceCommand runs a configured action's command, exactly as
// pageview.MaintenanceCommand built it, and streams its output to the
// console through cmdlog. Nothing else may start pkexec here.
func runMaintenanceCommand(ctx context.Context, cmd pageview.Command) error {
	c := exec.CommandContext(ctx, cmd.Name, cmd.Args...) //nolint:gosec // the configured action, as pageview.MaintenanceCommand builds it
	c.WaitDelay = maintWaitDelay
	if !cmd.Privileged() {
		// An unprivileged script runs in its own process group, and a stop
		// or the timeout kills the whole group, children included. A
		// pkexec run cannot be signaled past pkexec: its job is
		// Unstoppable.
		c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		c.Cancel = func() error { return syscall.Kill(-c.Process.Pid, syscall.SIGKILL) }
	}
	trace := cmdlog.Start(cmd.Name, cmd.Args...)
	c.Stdout = trace.Stdout(nil)
	c.Stderr = trace.Stderr(nil)
	err := c.Run()
	trace.End(err)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("timed out after %s", maintScriptTimeout)
		}
		if errors.Is(ctx.Err(), context.Canceled) {
			return context.Canceled
		}
	}
	return err
}
