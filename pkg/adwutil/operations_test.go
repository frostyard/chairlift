package adwutil

import (
	"errors"
	"testing"
)

// newTestRegistry creates an isolated registry for testing.
// This avoids the singleton DefaultRegistry which uses RunOnMain
// for listener notifications, ensuring isolated unit tests.
func newTestRegistry() *Registry {
	return &Registry{
		operations: make(map[uint64]*Operation),
	}
}

func TestRegistry_Start(t *testing.T) {
	r := newTestRegistry()

	op := r.start("Test Operation", CategoryInstall, false, nil)

	if op.Name != "Test Operation" {
		t.Errorf("Name = %q, want %q", op.Name, "Test Operation")
	}
	if op.Category != CategoryInstall {
		t.Errorf("Category = %v, want %v", op.Category, CategoryInstall)
	}
	if op.State != StateActive {
		t.Errorf("State = %v, want %v", op.State, StateActive)
	}
	if op.Progress != -1 {
		t.Errorf("Progress = %v, want -1 (indeterminate)", op.Progress)
	}
	if op.registry != r {
		t.Error("registry reference not set")
	}
}

func TestRegistry_Start_AssignsUniqueIDs(t *testing.T) {
	r := newTestRegistry()

	op1 := r.start("Op 1", CategoryInstall, false, nil)
	op2 := r.start("Op 2", CategoryInstall, false, nil)
	op3 := r.start("Op 3", CategoryInstall, false, nil)

	if op1.ID == op2.ID || op2.ID == op3.ID || op1.ID == op3.ID {
		t.Errorf("Operations should have unique IDs: %d, %d, %d", op1.ID, op2.ID, op3.ID)
	}
}

func TestRegistry_Get(t *testing.T) {
	r := newTestRegistry()
	op := r.start("Test", CategoryUpdate, false, nil)

	got := r.Get(op.ID)
	if got == nil {
		t.Fatal("Get returned nil for existing operation")
	}
	if got.ID != op.ID {
		t.Errorf("got ID %d, want %d", got.ID, op.ID)
	}
}

func TestRegistry_Get_NonExistent(t *testing.T) {
	r := newTestRegistry()

	if r.Get(99999) != nil {
		t.Error("Get should return nil for non-existent ID")
	}
}

func TestRegistry_ActiveCount(t *testing.T) {
	r := newTestRegistry()

	if r.ActiveCount() != 0 {
		t.Errorf("initial ActiveCount = %d, want 0", r.ActiveCount())
	}

	r.start("Op 1", CategoryInstall, false, nil)
	if r.ActiveCount() != 1 {
		t.Errorf("after 1 start, ActiveCount = %d, want 1", r.ActiveCount())
	}

	r.start("Op 2", CategoryUpdate, false, nil)
	if r.ActiveCount() != 2 {
		t.Errorf("after 2 starts, ActiveCount = %d, want 2", r.ActiveCount())
	}
}

func TestRegistry_Active(t *testing.T) {
	r := newTestRegistry()

	r.start("Op 1", CategoryInstall, false, nil)
	r.start("Op 2", CategoryUpdate, false, nil)

	active := r.Active()
	if len(active) != 2 {
		t.Errorf("Active length = %d, want 2", len(active))
	}

	// Verify copies are returned (not references to internal state)
	for _, op := range active {
		// Modifications should not affect the registry
		op.Name = "Modified"
	}

	// Check original is unaffected
	activeAgain := r.Active()
	for _, op := range activeAgain {
		if op.Name == "Modified" {
			t.Error("Active() should return copies, not references")
		}
	}
}

func TestRegistry_Complete_Success(t *testing.T) {
	r := newTestRegistry()
	op := r.start("Test", CategoryInstall, false, nil)

	r.complete(op.ID, nil) // nil error = success

	// Should be removed from active
	if r.ActiveCount() != 0 {
		t.Errorf("after complete, ActiveCount = %d, want 0", r.ActiveCount())
	}

	// Should not be retrievable via Get
	if r.Get(op.ID) != nil {
		t.Error("completed operation should not be in active operations")
	}

	// Should be in history
	history := r.History()
	if len(history) != 1 {
		t.Fatalf("history length = %d, want 1", len(history))
	}
	if history[0].State != StateCompleted {
		t.Errorf("history state = %v, want %v", history[0].State, StateCompleted)
	}
	if history[0].EndedAt.IsZero() {
		t.Error("EndedAt should be set on completed operation")
	}
}

func TestRegistry_Complete_Failure(t *testing.T) {
	r := newTestRegistry()
	op := r.start("Test", CategoryInstall, false, nil)

	testErr := errors.New("installation failed")
	r.complete(op.ID, testErr) // non-nil error = failure

	// Failed operations stay in active list for retry
	if r.ActiveCount() != 1 {
		t.Errorf("after failure, ActiveCount = %d, want 1", r.ActiveCount())
	}

	// Verify state and error
	got := r.Get(op.ID)
	if got == nil {
		t.Fatal("failed operation should still be in active operations")
	}
	if got.State != StateFailed {
		t.Errorf("state = %v, want %v", got.State, StateFailed)
	}
	if got.Error != testErr {
		t.Error("error not set on failed operation")
	}
	if got.EndedAt.IsZero() {
		t.Error("EndedAt should be set on failed operation")
	}

	// Should NOT be in history
	history := r.History()
	if len(history) != 0 {
		t.Errorf("failed operations should not be in history, got %d entries", len(history))
	}
}

func TestRegistry_Cancel(t *testing.T) {
	r := newTestRegistry()

	cancelled := false
	cancelFunc := func() { cancelled = true }

	op := r.start("Test", CategoryInstall, true, cancelFunc)
	r.cancel(op.ID)

	// Should call cancel func
	if !cancelled {
		t.Error("cancel func was not called")
	}

	// Should be removed from active
	if r.ActiveCount() != 0 {
		t.Errorf("after cancel, ActiveCount = %d, want 0", r.ActiveCount())
	}

	// Should not be retrievable via Get
	if r.Get(op.ID) != nil {
		t.Error("cancelled operation should not be in active operations")
	}

	// Should be in history with cancelled state
	history := r.History()
	if len(history) != 1 {
		t.Fatalf("history length = %d, want 1", len(history))
	}
	if history[0].State != StateCancelled {
		t.Errorf("history state = %v, want %v", history[0].State, StateCancelled)
	}
	if history[0].EndedAt.IsZero() {
		t.Error("EndedAt should be set on cancelled operation")
	}
}

func TestRegistry_Cancel_NilCancelFunc(t *testing.T) {
	r := newTestRegistry()

	// Cancellable but without a cancel func
	op := r.start("Test", CategoryInstall, true, nil)
	r.cancel(op.ID)

	// Should still move to history without panic
	if r.ActiveCount() != 0 {
		t.Errorf("after cancel, ActiveCount = %d, want 0", r.ActiveCount())
	}

	history := r.History()
	if len(history) != 1 {
		t.Fatalf("history length = %d, want 1", len(history))
	}
	if history[0].State != StateCancelled {
		t.Errorf("history state = %v, want %v", history[0].State, StateCancelled)
	}
}

func TestRegistry_Cancel_AlreadyCompleted(t *testing.T) {
	r := newTestRegistry()

	op := r.start("Test", CategoryInstall, true, nil)
	r.complete(op.ID, nil) // Complete first

	// Cancel should be no-op
	r.cancel(op.ID)

	// Should still be in history as completed (not cancelled)
	history := r.History()
	if len(history) != 1 {
		t.Fatalf("history length = %d, want 1", len(history))
	}
	if history[0].State != StateCompleted {
		t.Errorf("state = %v, want %v (cancel should not change completed state)", history[0].State, StateCompleted)
	}
}

func TestRegistry_HistoryCap(t *testing.T) {
	r := newTestRegistry()

	// Add more than MaxHistory operations and complete them
	for i := 0; i < MaxHistory+10; i++ {
		op := r.start("Op", CategoryInstall, false, nil)
		r.complete(op.ID, nil)
	}

	history := r.History()
	if len(history) > MaxHistory {
		t.Errorf("history length = %d, exceeds MaxHistory %d", len(history), MaxHistory)
	}
	if len(history) != MaxHistory {
		t.Errorf("history length = %d, want exactly %d", len(history), MaxHistory)
	}
}

func TestRegistry_GetHistory_ReturnsCopies(t *testing.T) {
	r := newTestRegistry()

	op := r.start("Test", CategoryInstall, false, nil)
	r.complete(op.ID, nil)

	history := r.History()
	if len(history) != 1 {
		t.Fatalf("history length = %d, want 1", len(history))
	}

	// Modify the returned copy
	history[0].Name = "Modified"

	// Check original is unaffected
	historyAgain := r.History()
	if historyAgain[0].Name == "Modified" {
		t.Error("History() should return copies, not references")
	}
}

func TestRegistry_UpdateProgress(t *testing.T) {
	r := newTestRegistry()
	op := r.start("Test", CategoryInstall, false, nil)

	r.updateProgress(op.ID, 0.5, "Downloading...")

	got := r.Get(op.ID)
	if got.Progress != 0.5 {
		t.Errorf("Progress = %v, want 0.5", got.Progress)
	}
	if got.Message != "Downloading..." {
		t.Errorf("Message = %q, want %q", got.Message, "Downloading...")
	}
}

func TestRegistry_UpdateProgress_NonExistent(t *testing.T) {
	r := newTestRegistry()

	// Should not panic
	r.updateProgress(99999, 0.5, "Test")
}

func TestRegistry_UpdateProgress_AlreadyCompleted(t *testing.T) {
	r := newTestRegistry()
	op := r.start("Test", CategoryInstall, false, nil)
	r.complete(op.ID, nil)

	// Should be no-op
	r.updateProgress(op.ID, 0.5, "Should not update")

	// History entry should not change
	history := r.History()
	if len(history) != 1 {
		t.Fatal("expected operation in history")
	}
	if history[0].Progress != -1 {
		t.Errorf("Progress = %v, should not have been updated", history[0].Progress)
	}
}
