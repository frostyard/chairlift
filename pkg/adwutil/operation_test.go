package adwutil

import (
	"testing"
	"time"
)

func TestCategory_Values(t *testing.T) {
	// Verify category constants have distinct values
	categories := []Category{CategoryInstall, CategoryUpdate, CategoryLoading, CategoryMaintenance}
	seen := make(map[Category]bool)

	for _, c := range categories {
		if seen[c] {
			t.Errorf("duplicate category value: %v", c)
		}
		seen[c] = true
	}
}

func TestCategory_StringValues(t *testing.T) {
	tests := map[Category]string{
		CategoryInstall:     "install",
		CategoryUpdate:      "update",
		CategoryLoading:     "loading",
		CategoryMaintenance: "maintenance",
	}

	for cat, want := range tests {
		if string(cat) != want {
			t.Errorf("Category %v = %q, want %q", cat, string(cat), want)
		}
	}
}

func TestState_Values(t *testing.T) {
	// Verify state constants have distinct values
	states := []State{StateActive, StateCompleted, StateFailed, StateCancelled}
	seen := make(map[State]bool)

	for _, s := range states {
		if seen[s] {
			t.Errorf("duplicate state value: %v", s)
		}
		seen[s] = true
	}
}

func TestState_String(t *testing.T) {
	tests := map[State]string{
		StateActive:    "Active",
		StateCompleted: "Completed",
		StateFailed:    "Failed",
		StateCancelled: "Cancelled",
	}

	for state, want := range tests {
		if state.String() != want {
			t.Errorf("State %d String() = %q, want %q", state, state.String(), want)
		}
	}

	// Test unknown state
	unknown := State(99)
	if unknown.String() != "Unknown" {
		t.Errorf("Unknown state String() = %q, want %q", unknown.String(), "Unknown")
	}
}

func TestOperation_UpdateProgress(t *testing.T) {
	r := newTestRegistry()
	op := r.start("Test", CategoryInstall, false, nil)

	// Initial progress should be -1 (indeterminate)
	if op.Progress != -1 {
		t.Errorf("initial Progress = %v, want -1", op.Progress)
	}

	// Update via operation method
	op.UpdateProgress(0.5, "Downloading...")

	got := r.Get(op.ID)
	if got.Progress != 0.5 {
		t.Errorf("Progress = %v, want 0.5", got.Progress)
	}
	if got.Message != "Downloading..." {
		t.Errorf("Message = %q, want %q", got.Message, "Downloading...")
	}
}

func TestOperation_UpdateProgress_NilRegistry(t *testing.T) {
	op := &Operation{
		ID:       1,
		Name:     "Test",
		registry: nil,
	}

	// Should not panic
	op.UpdateProgress(0.5, "Test")
}

func TestOperation_Complete(t *testing.T) {
	r := newTestRegistry()
	op := r.start("Test", CategoryInstall, false, nil)

	// Complete via operation method
	op.Complete(nil)

	if r.ActiveCount() != 0 {
		t.Errorf("after Complete, activeCount = %d, want 0", r.ActiveCount())
	}

	history := r.History()
	if len(history) != 1 {
		t.Fatalf("history length = %d, want 1", len(history))
	}
	if history[0].State != StateCompleted {
		t.Errorf("state = %v, want %v", history[0].State, StateCompleted)
	}
}

func TestOperation_Complete_NilRegistry(t *testing.T) {
	op := &Operation{
		ID:       1,
		Name:     "Test",
		registry: nil,
	}

	// Should not panic
	op.Complete(nil)
}

func TestOperation_Cancel(t *testing.T) {
	r := newTestRegistry()
	cancelled := false
	cancelFunc := func() { cancelled = true }

	op := r.start("Test", CategoryInstall, true, cancelFunc)

	// Cancel via operation method
	op.Cancel()

	if !cancelled {
		t.Error("cancel func was not called")
	}

	if r.ActiveCount() != 0 {
		t.Errorf("after Cancel, activeCount = %d, want 0", r.ActiveCount())
	}

	history := r.History()
	if len(history) != 1 {
		t.Fatalf("history length = %d, want 1", len(history))
	}
	if history[0].State != StateCancelled {
		t.Errorf("state = %v, want %v", history[0].State, StateCancelled)
	}
}

func TestOperation_Cancel_NilRegistry(t *testing.T) {
	op := &Operation{
		ID:       1,
		Name:     "Test",
		registry: nil,
	}

	// Should not panic
	op.Cancel()
}

func TestOperation_IsCancellable_RequiresBothConditions(t *testing.T) {
	r := newTestRegistry()

	// Not cancellable flag
	op1 := r.start("Test", CategoryInstall, false, nil)
	got1 := r.Get(op1.ID)
	if got1.IsCancellable() {
		t.Error("operation without Cancellable flag should not be cancellable")
	}

	// Cancellable flag but just started (< 5s)
	op2 := r.start("Test", CategoryInstall, true, nil)
	got2 := r.Get(op2.ID)
	if got2.IsCancellable() {
		t.Error("operation running < 5s should not be cancellable yet")
	}
}

func TestOperation_IsCancellable_AfterFiveSeconds(t *testing.T) {
	// Create an operation with StartedAt in the past
	op := &Operation{
		ID:          1,
		Name:        "Test",
		State:       StateActive,
		Cancellable: true,
		StartedAt:   time.Now().Add(-6 * time.Second), // Started 6 seconds ago
	}

	if !op.IsCancellable() {
		t.Error("cancellable operation running > 5s should be cancellable")
	}
}

func TestOperation_IsCancellable_NotActive(t *testing.T) {
	// Completed operation should not be cancellable
	op := &Operation{
		ID:          1,
		Name:        "Test",
		State:       StateCompleted,
		Cancellable: true,
		StartedAt:   time.Now().Add(-6 * time.Second),
	}

	if op.IsCancellable() {
		t.Error("completed operation should not be cancellable")
	}

	// Failed operation should not be cancellable
	op.State = StateFailed
	if op.IsCancellable() {
		t.Error("failed operation should not be cancellable")
	}

	// Cancelled operation should not be cancellable
	op.State = StateCancelled
	if op.IsCancellable() {
		t.Error("cancelled operation should not be cancellable")
	}
}

func TestOperation_Duration_Active(t *testing.T) {
	r := newTestRegistry()
	op := r.start("Test", CategoryInstall, false, nil)

	// Duration for active operation should be positive
	got := r.Get(op.ID)
	duration := got.Duration()
	if duration < 0 {
		t.Error("active operation should have non-negative duration")
	}
}

func TestOperation_Duration_Completed(t *testing.T) {
	r := newTestRegistry()
	op := r.start("Test", CategoryInstall, false, nil)

	// Complete the operation
	r.complete(op.ID, nil)

	// Duration for completed operation uses EndedAt
	history := r.History()
	if len(history) == 0 {
		t.Fatal("expected operation in history")
	}
	if history[0].Duration() < 0 {
		t.Error("completed operation should have non-negative duration")
	}
}

func TestOperation_Duration_FixedEndedAt(t *testing.T) {
	// Test with specific times for deterministic result
	start := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 1, 12, 0, 30, 0, time.UTC) // 30 seconds later

	op := &Operation{
		ID:        1,
		StartedAt: start,
		EndedAt:   end,
	}

	duration := op.Duration()
	if duration != 30*time.Second {
		t.Errorf("Duration = %v, want 30s", duration)
	}
}

func TestNextOperationID_IsAtomic(t *testing.T) {
	// Get some IDs and verify they're sequential and unique
	ids := make(map[uint64]bool)
	for i := 0; i < 100; i++ {
		id := nextOperationID()
		if ids[id] {
			t.Errorf("duplicate ID generated: %d", id)
		}
		ids[id] = true
	}
}
