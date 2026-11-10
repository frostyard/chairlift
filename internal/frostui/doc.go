// Package frostui is frostyard's desktop UI kit on MyGo
// (github.com/egoist/mygo/ui): the command-center shell (top bar, left
// rail, status bar, console, command palette), the cards, figures, pills,
// banners and dialogs that every page is built from, a serial job queue
// whose output streams to the console, and desktop integration (the
// desktop's icon theme, its accent kept in inactive windows, and the
// Wayland app ID).
//
// It is written to leave ChairLift: it imports only MyGo, purego and the
// standard library, never a github.com/frostyard/chairlift package, which
// internal/installcheck.TestFrostUIImportsNoChairLift enforces. Other
// frostyard desktop apps (frostmail, frostnotes, frostread) can take it
// as a module of its own, github.com/frostyard/frostui; see
// docs/design/frostui.md for what moves and what stays.
//
// # Threads
//
// Views run on the UI thread. Every model here (Console, Jobs, Palette,
// Dialogs) is changed on the UI thread, except Console.Add, which any
// goroutine may call. Workers hand results back with the post function
// the app gives NewJobs (MyGo's Window.Update); in tests, a
// frostuitest.Loop stands in for it.
package frostui
