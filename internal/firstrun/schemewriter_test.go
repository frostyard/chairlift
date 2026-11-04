package firstrun

import (
	"sync"
	"testing"
	"time"
)

// TestSchemeWriterLatestChoiceWins blocks the first write, requests Dark,
// Light, Dark, Light, then releases it: the last write must be Light and no
// more than 2 writes may run.
func TestSchemeWriterLatestChoiceWins(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{}, 8)
	var mu sync.Mutex
	var writes []ColorScheme
	writer := NewSchemeWriter(func(scheme ColorScheme) error {
		started <- struct{}{}
		// The first write blocks until the test releases it.
		mu.Lock()
		first := len(writes) == 0
		writes = append(writes, scheme)
		mu.Unlock()
		if first {
			<-release
		}
		return nil
	})

	writer.Request(SchemeDark)
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the first write did not start")
	}
	writer.Request(SchemeLight)
	writer.Request(SchemeDark)
	writer.Request(SchemeLight)
	close(release)
	writer.Wait()

	snapshot := func() []ColorScheme {
		mu.Lock()
		defer mu.Unlock()
		return append([]ColorScheme(nil), writes...)
	}
	got := snapshot()
	if len(got) == 0 || got[len(got)-1] != SchemeLight {
		t.Fatalf("writes = %v, want the last one to be %s", got, SchemeLight)
	}
	if len(got) > 2 {
		t.Fatalf("writes = %v, want at most 2 (the running one and the latest)", got)
	}

	// After the worker stopped, a new request writes again.
	writer.Request(SchemeDark)
	writer.Wait()
	if got := snapshot(); got[len(got)-1] != SchemeDark {
		t.Fatalf("writes = %v, want a new request to run", got)
	}
}

// TestSchemeWriterLogsErrorsAndContinues proves a failed write does not
// stop the worker.
func TestSchemeWriterLogsErrorsAndContinues(t *testing.T) {
	var mu sync.Mutex
	var writes []ColorScheme
	writer := NewSchemeWriter(func(scheme ColorScheme) error {
		mu.Lock()
		defer mu.Unlock()
		writes = append(writes, scheme)
		return errTestWrite
	})
	writer.Request(SchemeLight)
	writer.Wait()
	writer.Request(SchemeDark)
	writer.Wait()
	mu.Lock()
	defer mu.Unlock()
	if len(writes) != 2 || writes[1] != SchemeDark {
		t.Fatalf("writes = %v, want both requests written", writes)
	}
}

type testWriteError struct{}

func (testWriteError) Error() string { return "gsettings failed" }

var errTestWrite error = testWriteError{}
