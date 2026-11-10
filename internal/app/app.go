// Package app starts ChairLift's MyGo application: the main command-center
// window, or the first-run window with `--first-run` (ADR-0014). It sets
// the app ID the desktop matches to the .desktop file, keeps one instance
// per ID, sets every backend's dry-run flag, and wires the console to the
// commands the backends run and to the log.
package app

import (
	"errors"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/frostyard/chairlift/internal/activity"
	"github.com/frostyard/chairlift/internal/bootc"
	"github.com/frostyard/chairlift/internal/cmdlog"
	"github.com/frostyard/chairlift/internal/config"
	"github.com/frostyard/chairlift/internal/firstrun"
	"github.com/frostyard/chairlift/internal/flatpak"
	"github.com/frostyard/chairlift/internal/frostui"
	"github.com/frostyard/chairlift/internal/fwupd"
	"github.com/frostyard/chairlift/internal/homebrew"
	"github.com/frostyard/chairlift/internal/selfreport"
	"github.com/frostyard/chairlift/internal/sysupdate"
	"github.com/frostyard/chairlift/internal/updex"
	"github.com/frostyard/chairlift/internal/views"
	"github.com/frostyard/chairlift/internal/window"
)

// AppID is the main window's application ID: the Wayland app ID and the
// name of the .desktop file.
const AppID = "org.frostyard.ChairLift"

// FirstRunAppID is the application ID of `chairlift --first-run`
// (ADR-0014). With the main ID, a running ChairLift would take the
// activation and the first-run window would never show.
const FirstRunAppID = "org.frostyard.ChairLift.FirstRun"

// Flags are the command-line switches the window understands.
const (
	FlagDryRun      = "--dry-run"
	FlagDryRunShort = "-d"
	FlagFirstRun    = "--first-run"
)

// Options are the parsed command line.
type Options struct {
	DryRun   bool
	FirstRun bool
}

// ParseArgs reads the window's switches from args (os.Args[1:]).
func ParseArgs(args []string) Options {
	var o Options
	for _, a := range args {
		switch a {
		case FlagDryRun, FlagDryRunShort:
			o.DryRun = true
		case FlagFirstRun:
			o.FirstRun = true
		}
	}
	return o
}

// ID is the application ID for o.
func (o Options) ID() string {
	if o.FirstRun {
		return FirstRunAppID
	}
	return AppID
}

// SetDryRun puts every backend in dry-run mode (ADR-0009).
func SetDryRun() {
	log.Println("Running in dry-run mode")
	flatpak.SetDryRun(true)
	fwupd.SetDryRun(true)
	homebrew.SetDryRun(true)
	bootc.SetDryRun(true)
	sysupdate.SetDryRun(true)
	updex.SetDryRun(true)
	firstrun.SetDryRun(true)
}

// Run runs the application until its last window closes.
func Run(o Options) error {
	if o.DryRun {
		SetDryRun()
	}
	if o.FirstRun {
		log.Println("Running in first-run mode")
	}
	if err := frostui.SetAppID(o.ID(), "ChairLift"); err != nil {
		log.Printf("app: cannot set the app ID: %v", err)
	}
	mygo.App.SetName(o.ID())
	if !mygo.App.RequestSingleInstanceLock() {
		log.Printf("app: %s already runs; it was asked to show its window", o.ID())
		return nil
	}
	frostui.SetIcons(frostui.NewIcons(map[string][]byte{"org.frostyard.ChairLift-symbolic": window.LogoSVG()}))

	var win *mygo.Window
	mygo.App.OnSecondInstance(func([]string, string) {
		if win != nil {
			win.Show()
			win.Focus()
		}
	})
	mygo.App.WhenReady(func() {
		start := time.Now()
		if o.FirstRun {
			log.Println("ChairLift first run activated")
			win = openFirstRun()
			return
		}
		log.Println("ChairLift activated")
		win = openMain(o)
		window.Ready(start)
	})
	return mygo.App.Run()
}

func openMain(o Options) *mygo.Window {
	cfg, cfgErr := config.Load()
	if cfgErr != nil {
		selfreport.Error(selfreport.CodeConfigLoadFailed, cfgErr)
	}
	var win *mygo.Window
	post := func(fn func()) {
		if win != nil {
			win.Update(fn)
		}
	}
	env := views.NewEnv(cfg, o.DryRun, post)
	if cfgErr != nil {
		env.ConfigErr = cfgErr
	}
	wireConsole(env.Console, post)
	if path, err := activity.DefaultPath(os.Getenv); err == nil {
		l, err := activity.Open(path, o.DryRun)
		if err != nil {
			log.Printf("activity: %v", err)
		}
		env.Activity = l
	}
	store := views.NewStore(env)
	main := window.NewMain(env, store)
	main.Quit = mygo.App.Quit
	win = mygo.NewWindow(mygo.WindowOptions{
		Title: "ChairLift", Width: 1360, Height: 880, MinWidth: 1040, MinHeight: 640,
		TitleBarStyle: mygo.TitleBarHidden,
		Content:       ui.View(main.View),
	})
	log.Printf("window: built (%s)", main.Describe())
	store.Start()
	return win
}

func openFirstRun() *mygo.Window {
	var win *mygo.Window
	post := func(fn func()) {
		if win != nil {
			win.Update(fn)
		}
	}
	fr := window.NewFirstRun(post)
	win = mygo.NewWindow(mygo.WindowOptions{
		Title: "ChairLift", Width: 760, Height: 680, MinWidth: 640, MinHeight: 560,
		Content: ui.View(fr.View),
	})
	win.OnClose(func(e *mygo.CloseEvent) {
		// While the first run's actions run, a close would end the app and
		// lose the completion markers: keep the window.
		if !fr.CanClose() {
			e.PreventDefault()
		}
	})
	fr.Presented()
	return win
}

// wireConsole sends every command the backends run, and every log line,
// to the console. Redraws are coalesced: Window.Update rebuilds the view
// on every call.
func wireConsole(con *frostui.Console, post func(func())) {
	waker := window.NewWaker(post, 60*time.Millisecond)
	con.SetNotify(waker.Wake)
	cmdlog.SetSink(func(e cmdlog.Event) {
		switch e.Kind {
		case cmdlog.KindStart:
			con.Add(frostui.LevelCommand, sourceOf(e.Argv), strings.Join(e.Argv, " "))
		case cmdlog.KindLine:
			level := frostui.LevelOutput
			if e.Stream == cmdlog.Stderr {
				level = frostui.LevelOutput
			}
			con.Add(level, "", e.Text)
		case cmdlog.KindDryRun:
			con.Add(frostui.LevelDryRun, sourceOf(e.Argv), "would run: "+strings.Join(e.Argv, " "))
		case cmdlog.KindEnd:
			if e.Err != nil {
				con.Addf(frostui.LevelWarn, "", "exited after %s: %v", e.Elapsed.Round(time.Millisecond), e.Err)
			} else {
				con.Addf(frostui.LevelDebug, "", "exited after %s", e.Elapsed.Round(time.Millisecond))
			}
		}
	})
	w := con.Writer(frostui.LevelDebug, "log")
	w.Classify = func(text string) frostui.Level {
		switch {
		case strings.Contains(text, "[DRY-RUN]"):
			return frostui.LevelDryRun
		case strings.Contains(strings.ToLower(text), "error"), strings.Contains(strings.ToLower(text), "failed"):
			return frostui.LevelWarn
		}
		return frostui.LevelDebug
	}
	log.SetOutput(io.MultiWriter(os.Stderr, stripTimestamp{w}))
}

func sourceOf(argv []string) string {
	if len(argv) == 0 {
		return ""
	}
	name := argv[0]
	if i := strings.LastIndexByte(name, '/'); i >= 0 {
		name = name[i+1:]
	}
	if name == "pkexec" && len(argv) > 1 {
		return sourceOf(argv[1:])
	}
	return name
}

// stripTimestamp drops the standard logger's date and time, which the
// console shows itself.
type stripTimestamp struct{ w io.Writer }

func (s stripTimestamp) Write(p []byte) (int, error) {
	text := string(p)
	// "2026/11/10 10:50:19 file.go:12: message"
	if len(text) > 20 && text[4] == '/' && text[7] == '/' && text[13] == ':' {
		text = text[20:]
	}
	if _, err := s.w.Write([]byte(text)); err != nil {
		return 0, err
	}
	return len(p), nil
}

// ErrNoDisplay is returned when no display is open.
var ErrNoDisplay = errors.New("cannot open a display")
