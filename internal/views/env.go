// Package views holds ChairLift's command-center pages (Overview, Updates,
// Apps, Features, Problems, Maintenance) and the state they share, drawn
// with MyGo through the frostui kit.
//
// # Threads
//
// Views and every state struct here belong to the UI thread. A loader or
// action runs its backend call on a goroutine, copies what it needs before
// it starts (never reading state from the goroutine), and hands the result
// back with Env.Post. Mutations go through Env.Jobs, the serial queue, so
// one change runs at a time and its output streams to the console.
//
// # Configuration
//
// Every card checks its own group with Env.Enabled(page, group); a
// disabled group's card is never drawn and its data never loaded. Code
// that runs after an async action must not assume another group's state
// exists.
package views

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/frostyard/chairlift/internal/activity"
	"github.com/frostyard/chairlift/internal/cmdlog"
	"github.com/frostyard/chairlift/internal/config"
	"github.com/frostyard/frostui"
)

// Env is what every page gets from the window.
type Env struct {
	Config *config.Config
	// ConfigErr is the configuration load error, nil when the file was
	// read; every configurable group is disabled then.
	ConfigErr error
	DryRun    bool

	// Post runs fn on the UI thread and redraws: MyGo's Window.Update.
	// It is safe from any goroutine.
	Post func(fn func())

	Jobs     *frostui.Jobs
	Console  *frostui.Console
	Dialogs  *frostui.Dialogs
	Activity *activity.Log

	// Now is the clock, replaceable in tests.
	Now func() time.Time
	// DesktopApps lists visible desktop entries on a worker; nil disables lookup.
	DesktopApps func() ([]mygo.InstalledApp, error)

	toasts   []ui.Toast
	navigate func(page string)
}

// NewEnv returns an Env for tests and the window; the window sets Post and
// the navigation callback.
func NewEnv(cfg *config.Config, dryRun bool, post func(func())) *Env {
	e := &Env{Config: cfg, DryRun: dryRun, Post: post, Now: time.Now, Dialogs: &frostui.Dialogs{}}
	e.Console = frostui.NewConsole(5000)
	e.Jobs = frostui.NewJobs(post, e.Console)
	return e
}

// Enabled reports whether the configuration enables group on page.
func (e *Env) Enabled(page, group string) bool {
	return e.Config != nil && e.Config.IsGroupEnabled(page, group)
}

// Group returns a group's configuration, nil when it has none.
func (e *Env) Group(page, group string) *config.GroupConfig {
	if e.Config == nil {
		return nil
	}
	return e.Config.GetGroupConfig(page, group)
}

// Toast shows a short message that closes on its own. Messages are plain
// text: MyGo toasts parse no markup.
func (e *Env) Toast(message string) {
	e.toasts = append(e.toasts, ui.Toast{Title: message, Type: "info"})
}

// ToastSuccess shows a message about something that went well.
func (e *Env) ToastSuccess(message string) {
	e.toasts = append(e.toasts, ui.Toast{Title: message, Type: "success"})
}

// ToastError shows a failure that stays until the user closes it.
func (e *Env) ToastError(message string) {
	log.Printf("error: %s", message)
	e.toasts = append(e.toasts, ui.Toast{Title: message, Type: "error", Timeout: -1})
}

// TakeToasts returns the toasts queued since the last call; the window
// shows them.
func (e *Env) TakeToasts() []ui.Toast {
	t := e.toasts
	e.toasts = nil
	return t
}

// SetNavigate sets the function Navigate calls; the window sets it.
func (e *Env) SetNavigate(fn func(page string)) { e.navigate = fn }

// Navigate shows page, when it is visible.
func (e *Env) Navigate(page string) {
	if e.navigate != nil {
		e.navigate(page)
	}
}

// Go runs work on a goroutine and then apply on the UI thread with work's
// result: the loader pattern. Copy every input before calling Go.
func Go[T any](e *Env, work func() T, apply func(T)) {
	go func() {
		v := work()
		e.Post(func() { apply(v) })
	}()
}

// Run queues one job: run works on a goroutine with the job's context and
// progress, done applies the result on the UI thread.
func (e *Env) Run(source, key, title string, run func(ctx context.Context, p *frostui.Progress) error, done func(err error)) *frostui.Job {
	j := &frostui.Job{Source: source, Key: key, Title: title, Run: run, Done: done}
	e.Jobs.Enqueue(j)
	return j
}

// Record adds an entry to the activity log, unless in dry run: a dry run
// changed nothing.
func (e *Env) Record(source, title, detail string) {
	if e.DryRun || e.Activity == nil {
		return
	}
	if err := e.Activity.Add(source, title, detail); err != nil {
		log.Printf("activity: %v", err)
	}
}

// launchDesktopApp starts a desktop application by its desktop file ID and
// shows the launch in the console (cmdlog.Launch). frostui.LaunchDesktopApp
// resolves the desktop entry and launches it in process through GIO: no
// launcher program runs, and the app does not inherit ChairLift's stdout or
// stderr. A missing or unlaunchable entry fails with GIO's reason. The
// lookup and the launch are synchronous and may block, so it runs on a
// worker (launchApp), never on the UI thread.
func launchDesktopApp(id string) error {
	trace := cmdlog.Launch(id)
	err := frostui.LaunchDesktopApp(id)
	trace.End(err)
	return err
}

// launchApp starts the desktop application id with launch, a page's
// backend (launchDesktopApp, or a test's fake), and does not wait for the
// application. Called on the UI thread, it runs launch on a goroutine and
// hands the result back with Post: a failure is an error toast, with the
// reason in the log. A launch changes nothing on the system, so a dry run
// launches too.
func (e *Env) launchApp(launch func(id string) error, id string) {
	log.Printf("Launching app: %s", id)
	Go(e, func() error { return launch(id) }, func(err error) {
		if err != nil {
			log.Printf("Failed to launch app %s: %v", id, err)
			e.ToastError(fmt.Sprintf("Failed to launch %s", id))
		}
	})
}
