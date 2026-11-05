package problems

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path"
	"strings"
	"time"
)

// CoredumpArgs are the arguments of the user crash query: the short JSON
// list of the coredumps of the last 90 days. systemd-coredump makes each
// coredump entry readable by the user it belongs to; ChairLift still keeps
// only the entries of its own UID.
var CoredumpArgs = []string{"--json=short", "--no-pager", "--since=-90d"}

// maxCommandOutput caps the output that ChairLift reads from a command.
const maxCommandOutput = 4 << 20

// CoredumpEntry is 1 entry of the coredumpctl list.
type CoredumpEntry struct {
	Time   time.Time
	PID    int
	UID    int
	Signal int
	Exe    string
}

// coredumpJSON is 1 object of `coredumpctl --json=short`: time is in
// microseconds since the epoch; sig and exe are absent when unknown.
type coredumpJSON struct {
	Time *int64  `json:"time"`
	PID  *int    `json:"pid"`
	UID  *int    `json:"uid"`
	Sig  *int    `json:"sig"`
	Exe  *string `json:"exe"`
}

// ParseCoredumps parses coredumpctl's JSON output: 1 JSON array of objects,
// or 1 object for each line. An object without a time or a uid is skipped.
// Empty output is an empty list.
func ParseCoredumps(data []byte) ([]CoredumpEntry, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, nil
	}
	var objects []coredumpJSON
	if trimmed[0] == '[' {
		if err := json.Unmarshal(trimmed, &objects); err != nil {
			return nil, fmt.Errorf("coredumpctl: %w", err)
		}
	} else {
		dec := json.NewDecoder(bytes.NewReader(trimmed))
		for dec.More() {
			var obj coredumpJSON
			if err := dec.Decode(&obj); err != nil {
				return nil, fmt.Errorf("coredumpctl: %w", err)
			}
			objects = append(objects, obj)
		}
	}
	entries := make([]CoredumpEntry, 0, len(objects))
	for _, obj := range objects {
		if obj.Time == nil || obj.UID == nil {
			continue
		}
		entry := CoredumpEntry{Time: time.UnixMicro(*obj.Time).UTC(), UID: *obj.UID}
		if obj.PID != nil {
			entry.PID = *obj.PID
		}
		if obj.Sig != nil {
			entry.Signal = *obj.Sig
		}
		if obj.Exe != nil {
			entry.Exe = *obj.Exe
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// RunCoredumpctl runs `bin CoredumpArgs...` and returns its standard
// output. A missing coredumpctl and "No coredumps found" (exit 1) give no
// output and no error. Output larger than 4 MiB is an error.
func RunCoredumpctl(ctx context.Context, bin string) ([]byte, error) {
	var stdout, stderr cappedBuffer
	stdout.max, stderr.max = maxCommandOutput, 4096
	cmd := exec.CommandContext(ctx, bin, CoredumpArgs...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if errors.Is(err, exec.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 && strings.Contains(stderr.String(), "No coredumps found") {
			return nil, nil
		}
		return nil, fmt.Errorf("coredumpctl: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	if stdout.over {
		return nil, fmt.Errorf("coredumpctl: output larger than %d bytes", maxCommandOutput)
	}
	return stdout.Bytes(), nil
}

// cappedBuffer keeps at most max bytes and records that more came. It
// does not embed bytes.Buffer: io.Copy would use the promoted ReadFrom and
// pass the cap.
type cappedBuffer struct {
	buf  bytes.Buffer
	max  int
	over bool
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	room := b.max - b.buf.Len()
	if len(p) > room {
		b.over = true
		if room > 0 {
			b.buf.Write(p[:room])
		}
		return len(p), nil
	}
	return b.buf.Write(p)
}

func (b *cappedBuffer) Bytes() []byte  { return b.buf.Bytes() }
func (b *cappedBuffer) String() string { return b.buf.String() }

// signalNames maps the signal numbers of Linux to the v1 signal enum.
var signalNames = map[int]string{
	4:  "SIGILL",
	5:  "SIGTRAP",
	6:  "SIGABRT",
	7:  "SIGBUS",
	8:  "SIGFPE",
	11: "SIGSEGV",
	31: "SIGSYS",
}

// SignalName maps a signal number to the v1 signal enum; any other number
// is "other".
func SignalName(n int) string {
	if name, ok := signalNames[n]; ok {
		return name
	}
	return "other"
}

// ReportableExe returns the v1 exe value of an executable path: its
// basename, when the path is under /usr/ and the basename matches the v1
// exe pattern. Else it returns false: the crash is shown but is not
// reportable, because a path outside /usr can name the user's own files.
func ReportableExe(exePath string) (string, bool) {
	if !strings.HasPrefix(exePath, "/usr/") || path.Clean(exePath) != exePath {
		return "", false
	}
	base := path.Base(exePath)
	if !exeRe.MatchString(base) {
		return "", false
	}
	return base, true
}

// crashGroup collects the coredump entries of 1 executable and signal.
type crashGroup struct {
	exe    string
	signal string
	times  []time.Time
}

// CrashItems groups the entries of uid by executable path and signal into
// items, newest first, at most MaxItems. Entries of other UIDs are dropped.
func CrashItems(entries []CoredumpEntry, uid int, env Envelope) []Item {
	groups := map[string]*crashGroup{}
	var order []string
	for _, e := range entries {
		if e.UID != uid {
			continue
		}
		signal := SignalName(e.Signal)
		key := e.Exe + "\x00" + signal
		g, ok := groups[key]
		if !ok {
			g = &crashGroup{exe: e.Exe, signal: signal}
			groups[key] = g
			order = append(order, key)
		}
		g.times = append(g.times, e.Time)
	}
	items := make([]Item, 0, len(order))
	for _, key := range order {
		items = append(items, crashItem(groups[key], env))
	}
	sortItems(items)
	if len(items) > MaxItems {
		items = items[:MaxItems]
	}
	return items
}

// crashItem builds the item of 1 crash group.
func crashItem(g *crashGroup, env Envelope) Item {
	first, last := g.times[0], g.times[0]
	for _, t := range g.times {
		if t.Before(first) {
			first = t
		}
		if t.After(last) {
			last = t
		}
	}
	exeField := Field{Name: "exe", Display: cutDisplay(path.Base(g.exe))}
	if g.exe == "" {
		exeField.Display = ""
	}
	if exe, ok := ReportableExe(g.exe); ok {
		exeField.Value, exeField.Valid = exe, true
	} else {
		exeField.Reason = "not a program under /usr"
	}
	item := Item{
		ID:        hashID("crash-", g.exe, g.signal),
		Origin:    OriginUserCrash,
		Kind:      KindCrash,
		KnownKind: true,
		Fields: []Field{
			exeField,
			{Name: "signal", Display: g.signal, Value: g.signal, Valid: true},
		},
		FirstSeen: first,
		LastSeen:  last,
		Count:     len(g.times),
		Envelope:  env,
		Local:     Local{ExePath: cutText(g.exe, maxUnitLen)},
		times:     g.times,
	}
	item.Week = isoWeek(last)
	item.WeekValid = weekRe.MatchString(item.Week)
	item.Reportable = item.DetailValid() && item.WeekValid && env.Valid
	return item
}
