//go:build automation

package main

// Built with -tags automation, ChairLift lets mygo-automation list, press
// and capture its window (MYGO_AUTOMATION=/path/to.sock). Release builds
// never carry it.
import _ "github.com/egoist/mygo/plugins/automation"
