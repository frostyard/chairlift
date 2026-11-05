package problems

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// countingReader counts the bytes that were read from it.
type countingReader struct {
	r io.Reader
	n int
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += n
	return n, err
}

// byteRun is a reader of n copies of b; it holds no buffer of n bytes.
type byteRun struct {
	b byte
	n int
}

func (r *byteRun) Read(p []byte) (int, error) {
	if r.n == 0 {
		return 0, io.EOF
	}
	k := min(len(p), r.n)
	for i := range p[:k] {
		p[i] = r.b
	}
	r.n -= k
	return k, nil
}

// repeatLine is an endless reader of line.
type repeatLine struct {
	line []byte
	at   int
}

func (r *repeatLine) Read(p []byte) (int, error) {
	n := 0
	for n < len(p) {
		k := copy(p[n:], r.line[r.at:])
		n += k
		r.at = (r.at + k) % len(r.line)
	}
	return n, nil
}

// fakeStream returns a streamFunc that gives out as the standard output and
// records the arguments and whether the command was killed.
type fakeStream struct {
	out     io.Reader
	stderr  string
	exitErr error
	args    []string
	killed  bool
	started int
	// only, when set, is the 1 command name the fake runs; any other name
	// is not installed.
	only string
}

func (f *fakeStream) run(_ context.Context, name string, args []string) (io.Reader, func(bool) (string, error), error) {
	if f.only != "" && name != f.only {
		return nil, nil, &exec.Error{Name: name, Err: exec.ErrNotFound}
	}
	f.args = args
	f.started++
	return f.out, func(kill bool) (string, error) {
		f.killed = kill
		return f.stderr, f.exitErr
	}, nil
}

// journalLine returns 1 journalctl JSON line of a valid Frostmail error
// record with a MESSAGE of messageLen bytes.
func journalLine(at time.Time, messageLen int) string {
	return fmt.Sprintf(`{"__CURSOR":"c","__REALTIME_TIMESTAMP":"%d","_UID":"1000","_COMM":"frostmaild",`+
		`"MESSAGE_ID":"8915bdd215c84cbab0c31c05a1a33e7e","PRIORITY":"3","MESSAGE":"%s",`+
		`"FROSTYARD_SCHEMA":"frostyard.problem/1","FROSTYARD_APP_ID":"org.frostyard.Frostmail",`+
		`"FROSTYARD_APP_VERSION":"1.2.0","FROSTYARD_RUNTIME":"go","FROSTYARD_COMPONENT":"daemon",`+
		`"FROSTYARD_CLASS":"error","FROSTYARD_CODE":"store.migrate_failed"}`+"\n",
		at.UnixMicro(), strings.Repeat("m", messageLen))
}

func TestReadUpTo(t *testing.T) {
	tests := []struct {
		name      string
		size      int
		limit     int
		truncated bool
	}{
		{"empty", 0, 10, false},
		{"under", 5, 10, false},
		{"exactly the limit", 10, 10, false},
		{"over by 1", 11, 10, true},
		{"large, over", 3 << 20, 1 << 20, true},
		{"large, under", 1 << 20, 3 << 20, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := &countingReader{r: &byteRun{b: 'x', n: tt.size}}
			got, truncated, err := readUpTo(src, tt.limit)
			if err != nil || truncated != tt.truncated {
				t.Fatalf("readUpTo = %d bytes, %v, %v; want truncated %v", len(got), truncated, err, tt.truncated)
			}
			if want := min(tt.size, tt.limit); len(got) != want {
				t.Errorf("read %d bytes, want %d", len(got), want)
			}
			if cap(got) > tt.limit {
				t.Errorf("buffer capacity %d is over the limit %d", cap(got), tt.limit)
			}
			if src.n > tt.limit+1 {
				t.Errorf("read %d bytes from the source, more than the limit %d + 1", src.n, tt.limit)
			}
		})
	}
	boom := errors.New("boom")
	if _, _, err := readUpTo(io.MultiReader(strings.NewReader("ab"), iotestErr{boom}), 10); !errors.Is(err, boom) {
		t.Errorf("read error = %v, want boom", err)
	}
	if _, _, err := readUpTo(io.MultiReader(strings.NewReader("abc"), iotestErr{boom}), 3); !errors.Is(err, boom) {
		t.Errorf("read error after the limit = %v, want boom", err)
	}
}

type iotestErr struct{ err error }

func (e iotestErr) Read([]byte) (int, error) { return 0, e.err }

func TestRunCapped(t *testing.T) {
	ctx := context.Background()
	exit := errors.New("exit status 3")
	f := &fakeStream{out: strings.NewReader("ok\n"), stderr: "warn", exitErr: exit}
	res := runCapped(ctx, f.run, "x", []string{"a"}, 10)
	if string(res.out) != "ok\n" || res.truncated || !errors.Is(res.err, exit) || res.stderr != "warn" || f.killed {
		t.Errorf("runCapped = %+v, killed %v", res, f.killed)
	}
	f = &fakeStream{out: &repeatLine{line: []byte("x")}, exitErr: errors.New("signal: killed")}
	res = runCapped(ctx, f.run, "x", nil, 10)
	if !res.truncated || res.err != nil || !f.killed || len(res.out) != 10 {
		t.Errorf("over the cap: %+v, killed %v; want truncated, no error, killed", res, f.killed)
	}
	start := func(context.Context, string, []string) (io.Reader, func(bool) (string, error), error) {
		return nil, nil, errors.New("no such program")
	}
	if res := runCapped(ctx, start, "x", nil, 10); res.err == nil {
		t.Error("a start error was lost")
	}
}

// TestJournalReadIsCapped gives the journal read output far over the cap:
// (a) endless valid records with a 1 MiB MESSAGE, and (b) 3 valid records
// and then 1 line of 20 MiB with a large non-schema field. ChairLift reads
// at most the cap, kills journalctl, keeps the valid records of the complete
// lines before the cap, and logs the truncation.
func TestJournalReadIsCapped(t *testing.T) {
	base := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	big := journalLine(base, 1<<20)
	small := journalLine(base, 10) + journalLine(base.Add(time.Minute), 10) + journalLine(base.Add(2*time.Minute), 10)
	hugeLine := io.MultiReader(strings.NewReader(`{"X_BIG":"`), &byteRun{b: 'a', n: 20 << 20}, strings.NewReader("\"}\n"))
	tests := []struct {
		name  string
		out   io.Reader
		count int
	}{
		{"1 MiB messages", &repeatLine{line: []byte(big)}, MaxJournalOutput / len(big)},
		{"20 MiB line", io.MultiReader(strings.NewReader(small), hugeLine), 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := &countingReader{r: tt.out}
			f := &fakeStream{out: src, exitErr: errors.New("signal: killed"), only: "journalctl"}
			snap := Load(context.Background(), Options{Now: base, Apps: true, stream: f.run})
			if f.started != 1 {
				t.Fatalf("started %d commands", f.started)
			}
			if src.n > MaxJournalOutput+1 {
				t.Errorf("read %d bytes of journalctl output, over the cap %d", src.n, MaxJournalOutput)
			}
			if !f.killed {
				t.Error("journalctl was not killed at the cap")
			}
			if snap.JournalErr != nil || !snap.JournalTruncated {
				t.Fatalf("journal err = %v, truncated = %v", snap.JournalErr, snap.JournalTruncated)
			}
			if len(snap.AppItems) != 1 || snap.AppItems[0].Count != tt.count {
				t.Fatalf("app items = %d, want 1 group of %d records", len(snap.AppItems), tt.count)
			}
			if !strings.Contains(strings.Join(snap.LogLines(), "\n"), "problems: journalctl output reached the cap of 8388608 bytes") {
				t.Errorf("the truncation is not logged: %q", snap.LogLines())
			}
		})
	}
}

func TestReadJournalArgsAndErrors(t *testing.T) {
	now := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	f := &fakeStream{out: strings.NewReader(journalLine(now, 5))}
	data, truncated, err := readJournal(context.Background(), Options{Now: now, stream: f.run})
	if err != nil || truncated || !bytes.HasSuffix(data, []byte("}\n")) {
		t.Fatalf("readJournal = %q, %v, %v", data, truncated, err)
	}
	if strings.Join(f.args, " ") != strings.Join(JournalArgs(now), " ") {
		t.Errorf("args = %q, want JournalArgs", f.args)
	}
	f = &fakeStream{out: strings.NewReader(""), stderr: "No journal files were found.\n", exitErr: errors.New("exit status 1")}
	if _, _, err := readJournal(context.Background(), Options{Now: now, stream: f.run}); err == nil ||
		!strings.Contains(err.Error(), "No journal files were found.") {
		t.Errorf("err = %v", err)
	}
	// A cap with no complete line keeps nothing.
	f = &fakeStream{out: &repeatLine{line: []byte("x")}}
	data, truncated, err = readJournal(context.Background(), Options{Now: now, stream: f.run})
	if err != nil || !truncated || len(data) != 0 {
		t.Errorf("no complete line: %d bytes, %v, %v", len(data), truncated, err)
	}
}

// TestCoredumpReadIsCapped: coredumpctl output over the cap is read only to
// the cap, coredumpctl is killed, and the read is an error.
func TestCoredumpReadIsCapped(t *testing.T) {
	src := &countingReader{r: &repeatLine{line: []byte(`{"time": 1, "uid": 1000, "exe": "/usr/bin/x"}` + "\n")}}
	f := &fakeStream{out: src}
	if _, err := runCoredumpctl(context.Background(), f.run, "coredumpctl"); err == nil || !strings.Contains(err.Error(), "cap") {
		t.Fatalf("err = %v, want the cap error", err)
	}
	if src.n > maxCommandOutput+1 || !f.killed {
		t.Errorf("read %d bytes (cap %d), killed %v", src.n, maxCommandOutput, f.killed)
	}
	if strings.Join(f.args, " ") != "--json=short --no-pager --since=-90d" {
		t.Errorf("args = %q", f.args)
	}
}
