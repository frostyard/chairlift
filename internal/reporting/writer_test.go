package reporting

import (
	"errors"
	"sync"
	"testing"
)

// fakeConsentFile is an injectable consent writer: it records every value it
// is asked to write, can be slow, and can fail.
type fakeConsentFile struct {
	mu      sync.Mutex
	writes  []Consent
	onDisk  Consent
	fail    map[int]error // write number (1-based) to its failure
	release chan struct{} // when not nil, a write waits for it
	started chan Consent  // when not nil, a write reports that it began
}

func (f *fakeConsentFile) write(c Consent) error {
	f.mu.Lock()
	f.writes = append(f.writes, c)
	n := len(f.writes)
	release, started := f.release, f.started
	failure := f.fail[n]
	f.mu.Unlock()
	if started != nil {
		started <- c
	}
	if release != nil {
		<-release
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if failure != nil {
		return failure
	}
	f.onDisk = c
	return nil
}

func (f *fakeConsentFile) onDiskValue() Consent {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.onDisk
}

func (f *fakeConsentFile) writeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.writes)
}

// TestConsentWriterLatestChoiceWins proves a fast On then Off ends Off on
// disk and in memory, whatever order the writes run in.
func TestConsentWriterLatestChoiceWins(t *testing.T) {
	file := &fakeConsentFile{release: make(chan struct{}), started: make(chan Consent, 4)}
	writer := NewConsentWriter(Consent{}, file.write, nil)
	defer writer.Close()

	on := Decide(true, ModeAuto, testStart)
	off := Decide(false, ModeAsk, testStart)
	genOn := writer.Set(on)
	<-file.started // the On write is running and blocked
	genOff := writer.Set(off)

	if got := writer.Wanted(); got != off {
		t.Errorf("Wanted() = %+v, want the Off choice at once", got)
	}
	if genOff <= genOn {
		t.Fatalf("generations %d then %d, want an increasing generation", genOn, genOff)
	}
	close(file.release)
	writer.Wait()

	if got := file.onDiskValue(); got != off {
		t.Errorf("the file holds %+v, want the Off choice", got)
	}
	if got := file.writeCount(); got > 2 {
		t.Errorf("%d writes were made, want at most 2", got)
	}
	if got := writer.Persisted(); got != off {
		t.Errorf("Persisted() = %+v, want the Off choice", got)
	}
}

// TestConsentWriterOnlyWritesTheLatest proves a choice that arrives during a
// write replaces the pending value, so 4 quick choices make 2 writes at most.
func TestConsentWriterOnlyWritesTheLatest(t *testing.T) {
	file := &fakeConsentFile{release: make(chan struct{}), started: make(chan Consent, 8)}
	writer := NewConsentWriter(Consent{}, file.write, nil)
	defer writer.Close()

	writer.Set(Decide(true, ModeAsk, testStart))
	<-file.started
	for _, mode := range []Mode{ModeAuto, ModeAsk, ModeAuto, ModeAsk} {
		writer.Set(Decide(true, mode, testStart))
	}
	last := Decide(false, ModeAuto, testStart)
	writer.Set(last)
	close(file.release)
	writer.Wait()

	if got := file.onDiskValue(); got != last {
		t.Errorf("the file holds %+v, want the last choice", got)
	}
	if got := file.writeCount(); got > 2 {
		t.Errorf("%d writes were made, want at most 2", got)
	}
}

// TestConsentWriterFailedWriteRestoresThePersistedValue proves a failed
// choice with no newer one puts the value on disk back as the wanted value.
func TestConsentWriterFailedWriteRestoresThePersistedValue(t *testing.T) {
	onDisk := Decide(true, ModeAsk, testStart)
	file := &fakeConsentFile{onDisk: onDisk, fail: map[int]error{1: errors.New("disk is full")}}
	var results []ConsentResult
	var mu sync.Mutex
	writer := NewConsentWriter(onDisk, file.write, func(res ConsentResult) {
		mu.Lock()
		results = append(results, res)
		mu.Unlock()
	})
	defer writer.Close()

	writer.Set(Decide(false, ModeAsk, testStart))
	writer.Wait()

	mu.Lock()
	defer mu.Unlock()
	if len(results) != 1 {
		t.Fatalf("%d results, want 1", len(results))
	}
	if results[0].Err == nil || !results[0].Restored {
		t.Errorf("result = %+v, want a failure that restores", results[0])
	}
	if results[0].Persisted != onDisk {
		t.Errorf("result.Persisted = %+v, want the value on disk", results[0].Persisted)
	}
	if got := writer.Wanted(); got != onDisk {
		t.Errorf("Wanted() = %+v, want the value on disk again", got)
	}
	if got := writer.Persisted(); got != onDisk {
		t.Errorf("Persisted() = %+v, want the value on disk", got)
	}
}

// TestConsentWriterOldResultDoesNotRestore proves a failed write whose choice
// is no longer the newest one does not touch the wanted value.
func TestConsentWriterOldResultDoesNotRestore(t *testing.T) {
	first := Decide(true, ModeAsk, testStart)
	second := Decide(true, ModeAuto, testStart)
	file := &fakeConsentFile{release: make(chan struct{}), started: make(chan Consent, 4), fail: map[int]error{1: errors.New("disk is full")}}
	var results []ConsentResult
	var mu sync.Mutex
	writer := NewConsentWriter(Consent{}, file.write, func(res ConsentResult) {
		mu.Lock()
		results = append(results, res)
		mu.Unlock()
	})
	defer writer.Close()

	writer.Set(first)
	<-file.started
	writer.Set(second)
	close(file.release)
	writer.Wait()

	mu.Lock()
	defer mu.Unlock()
	if len(results) == 0 {
		t.Fatal("no result")
	}
	if results[0].Restored {
		t.Errorf("the old failure restored the value: %+v", results[0])
	}
	if got := writer.Wanted(); got != second {
		t.Errorf("Wanted() = %+v, want the newer choice", got)
	}
	if got := file.onDiskValue(); got != second {
		t.Errorf("the file holds %+v, want the newer choice", got)
	}
}

// TestWithdrawalDuringASlowConsentWriteStopsARun proves the rule the page
// depends on: the in-memory wanted consent changes at once, so a run that
// reads it before each request stops before its next one.
func TestWithdrawalDuringASlowConsentWriteStopsARun(t *testing.T) {
	file := &fakeConsentFile{release: make(chan struct{}), started: make(chan Consent, 4)}
	writer := NewConsentWriter(autoConsent(), file.write, nil)

	off := Consent{}
	writer.Set(off) // the user turns reporting off; the write is in flight
	<-file.started

	clk := newClock(testStart)
	sender := &recordingSender{}
	in := input(clk, newStatePath(t), sender, nil)
	in.Consent = writer.Wanted
	in.Sender = sender
	in.Pace = 0

	res := Run(t.Context(), in)
	if res.Err != nil {
		t.Fatalf("Run: %v", res.Err)
	}
	if sender.count() != 0 {
		t.Fatalf("%d requests were made, want 0 after a withdrawal", sender.count())
	}
	if res.Skipped != "consent" {
		t.Errorf("Skipped = %q, want consent", res.Skipped)
	}
	if got := writer.Wanted(); got != off {
		t.Errorf("Wanted() = %+v, want the withdrawal", got)
	}
	close(file.release)
	writer.Close()
}
