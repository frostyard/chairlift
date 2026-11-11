// Package cmdlog reports every external command ChairLift's backends run,
// with its output, as it runs. ChairLift's console subscribes with SetSink;
// with no sink every call is a cheap no-op, so the backends and their tests
// behave exactly as before.
//
// The package is a leaf: it imports no UI package and no backend. A backend
// announces a command with Start, tees the command's stdout and stderr
// through Run.Stdout and Run.Stderr (or reports lines it already reads with
// Run.Line), and ends it with Run.End. A dry-run preview that starts no
// process is reported with DryRun.
package cmdlog

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Kind says what an Event reports.
type Kind uint8

const (
	// KindStart announces a command; Event.Argv holds its argument vector.
	KindStart Kind = iota
	// KindLine is one line of a running command's output.
	KindLine
	// KindEnd ends a command; Event.Err is its error and Event.Elapsed its
	// run time.
	KindEnd
	// KindDryRun is a command that dry-run mode did not start.
	KindDryRun
)

// Stream says where an output line came from.
type Stream uint8

const (
	// Stdout is the command's standard output.
	Stdout Stream = iota
	// Stderr is the command's standard error.
	Stderr
)

// MaxLine is the longest line an Event carries, in bytes. A longer line is
// cut there and ends with an ellipsis, so one runaway line cannot hold the
// console's memory.
const MaxLine = 4096

// Event is one report about one command. ID is the same for every event of
// one command.
type Event struct {
	ID      uint64
	Kind    Kind
	Argv    []string
	Stream  Stream
	Text    string
	Err     error
	Elapsed time.Duration
}

// Sink receives events. It is called from the goroutine that runs the
// command, so it must be safe for concurrent use and must not block for
// long.
type Sink func(Event)

var (
	current atomic.Pointer[Sink]
	nextID  atomic.Uint64
)

// SetSink installs s as the receiver of every later event; nil removes it.
func SetSink(s Sink) {
	if s == nil {
		current.Store(nil)
		return
	}
	current.Store(&s)
}

func emit(e Event) {
	if p := current.Load(); p != nil {
		(*p)(e)
	}
}

// Run is one announced command. The zero of *Run (nil) is valid and does
// nothing, so a caller never needs to check for a sink.
type Run struct {
	id      uint64
	started time.Time
	once    sync.Once
	out     *lineWriter
	err     *lineWriter
}

// Start announces the command name with args and returns its Run. It
// returns nil when no sink is installed.
func Start(name string, args ...string) *Run {
	if current.Load() == nil {
		return nil
	}
	r := &Run{id: nextID.Add(1), started: time.Now()}
	r.out = &lineWriter{run: r, stream: Stdout}
	r.err = &lineWriter{run: r, stream: Stderr}
	argv := append([]string{name}, args...)
	emit(Event{ID: r.id, Kind: KindStart, Argv: argv})
	return r
}

// Stdout returns a writer that writes to w and reports each complete line
// as standard output. With a nil Run it returns w.
func (r *Run) Stdout(w io.Writer) io.Writer {
	if r == nil {
		return w
	}
	return tee(w, r.out)
}

// Stderr returns a writer that writes to w and reports each complete line
// as standard error. With a nil Run it returns w.
func (r *Run) Stderr(w io.Writer) io.Writer {
	if r == nil {
		return w
	}
	return tee(w, r.err)
}

// Line reports one output line the caller has read itself.
func (r *Run) Line(text string) {
	if r == nil {
		return
	}
	r.line(Stdout, text)
}

// End reports any unfinished output line and then the end of the command
// with err. Only the first End counts.
func (r *Run) End(err error) {
	if r == nil {
		return
	}
	r.once.Do(func() {
		r.out.flush()
		r.err.flush()
		emit(Event{ID: r.id, Kind: KindEnd, Err: err, Elapsed: time.Since(r.started)})
	})
}

func (r *Run) line(s Stream, text string) {
	text = strings.TrimRight(text, " \t\r")
	if strings.TrimSpace(text) == "" {
		return
	}
	emit(Event{ID: r.id, Kind: KindLine, Stream: s, Text: clip(text)})
}

// DryRun reports a command that dry-run mode did not start.
func DryRun(name string, args ...string) {
	if current.Load() == nil {
		return
	}
	emit(Event{ID: nextID.Add(1), Kind: KindDryRun, Argv: append([]string{name}, args...)})
}

// clip cuts text to MaxLine bytes on a rune boundary.
func clip(text string) string {
	if len(text) <= MaxLine {
		return text
	}
	cut := MaxLine - len("…")
	for cut > 0 && !utf8Start(text[cut]) {
		cut--
	}
	return text[:cut] + "…"
}

func utf8Start(b byte) bool { return b&0xC0 != 0x80 }

func tee(w io.Writer, lw *lineWriter) io.Writer {
	if w == nil {
		return lw
	}
	return io.MultiWriter(w, lw)
}

// lineWriter splits what is written into lines on \n or \r and reports
// each one. A pending partial line is bounded by MaxLine.
type lineWriter struct {
	mu     sync.Mutex
	run    *Run
	stream Stream
	buf    []byte
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := len(p)
	for len(p) > 0 {
		i := bytes.IndexAny(p, "\r\n")
		if i < 0 {
			w.buf = append(w.buf, p...)
			if len(w.buf) > MaxLine {
				w.run.line(w.stream, string(w.buf))
				w.buf = w.buf[:0]
			}
			break
		}
		w.buf = append(w.buf, p[:i]...)
		w.run.line(w.stream, string(w.buf))
		w.buf = w.buf[:0]
		p = p[i+1:]
	}
	return n, nil
}

func (w *lineWriter) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.buf) > 0 {
		w.run.line(w.stream, string(w.buf))
		w.buf = w.buf[:0]
	}
}
