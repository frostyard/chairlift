package reporting

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/chairlift/internal/problems"
)

func TestStatePathUsesAbsoluteEnvironmentOnly(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
		err  bool
	}{
		{"xdg state home", map[string]string{"XDG_STATE_HOME": "/home/ada/.local/state", "HOME": "/home/ada"}, "/home/ada/.local/state/chairlift/reports.json", false},
		{"home fallback", map[string]string{"HOME": "/home/ada"}, "/home/ada/.local/state/chairlift/reports.json", false},
		{"relative values are ignored", map[string]string{"XDG_STATE_HOME": "state", "HOME": "home"}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := StatePath(func(name string) string { return tt.env[name] })
			if tt.err {
				if err == nil {
					t.Fatalf("StatePath() = %q, want an error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("StatePath(): %v", err)
			}
			if got != tt.want {
				t.Errorf("StatePath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStateMissingFileIsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chairlift", "reports.json")
	st, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState(missing): %v", err)
	}
	if len(st.Sent) != 0 || st.CountWeek != "" || !st.RetryAfter.IsZero() {
		t.Errorf("LoadState(missing) = %+v, want an empty state", st)
	}
	if got := st.Remaining(testStart); got != MaxPerDay {
		t.Errorf("Remaining() = %d, want %d", got, MaxPerDay)
	}
}

func TestStateMalformedIsUnreadable(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"not JSON":     `{`,
		"wrong format": `{"format":2,"sent":[]}`,
	} {
		t.Run(strings.ReplaceAll(name, " ", "-"), func(t *testing.T) {
			path := filepath.Join(dir, strings.ReplaceAll(name, " ", "-")+".json")
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			st, err := LoadState(path)
			if err == nil {
				t.Fatalf("LoadState(%s) = %+v, want an error", name, st)
			}
			if !strings.Contains(err.Error(), "unreadable") {
				t.Errorf("LoadState(%s) error = %v, want the unreadable marker", name, err)
			}
			if len(st.Sent) != 0 {
				t.Errorf("LoadState(%s) kept %d entries", name, len(st.Sent))
			}
		})
	}
}

func TestStateSaveRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "reports.json")
	st := &State{CountWeek: week}
	st.Record(Entry{ReportID: "3f2a1b4c-5d6e-4f70-8a9b-0c1d2e3f4a5b", Kind: problems.KindUnitFailed,
		ProblemID: "0123456789abcdef", Week: week, SentAt: testStart})
	st.Fail(testStart)
	if err := st.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("state file mode = %o, want 600", perm)
	}
	loaded, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if len(loaded.Sent) != 1 || loaded.Sent[0] != st.Sent[0] {
		t.Errorf("Sent = %+v, want %+v", loaded.Sent, st.Sent)
	}
	if loaded.CountWeek != week {
		t.Errorf("CountWeek = %q, want %q", loaded.CountWeek, week)
	}
	if !loaded.RetryAfter.Equal(testStart.Add(RetryAfter)) {
		t.Errorf("RetryAfter = %v, want %v", loaded.RetryAfter, testStart.Add(RetryAfter))
	}
}

// TestStateFileShape pins the JSON keys of the sent log.
func TestStateFileShape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reports.json")
	st := &State{CountWeek: week}
	st.Record(Entry{ReportID: "3f2a1b4c-5d6e-4f70-8a9b-0c1d2e3f4a5b", Kind: problems.KindCount, Week: week, SentAt: testStart})
	if err := st.Save(path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"format", "sent", "count_week"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("the sent log has no %q key:\n%s", key, data)
		}
	}
	if _, ok := raw["retry_after"]; ok {
		t.Errorf("an unpaused sent log writes retry_after:\n%s", data)
	}
	var entries []map[string]json.RawMessage
	if err := json.Unmarshal(raw["sent"], &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("sent holds %d entries, want 1", len(entries))
	}
	for _, key := range []string{"report_id", "kind", "problem_id", "week", "sent_at"} {
		if _, ok := entries[0][key]; !ok {
			t.Errorf("the entry has no %q key:\n%s", key, data)
		}
	}
}

func TestStateRemainingAndWasSent(t *testing.T) {
	st := &State{}
	for i := 0; i < 3; i++ {
		st.Record(Entry{ReportID: "id", Kind: problems.KindUnitFailed, ProblemID: "0123456789abcdef", Week: week, SentAt: testStart.Add(-time.Duration(i) * time.Hour)})
	}
	if got := st.SentInLast24h(testStart); got != 3 {
		t.Errorf("SentInLast24h() = %d, want 3", got)
	}
	if got := st.Remaining(testStart); got != MaxPerDay-3 {
		t.Errorf("Remaining() = %d, want %d", got, MaxPerDay-3)
	}
	if !st.WasSent("0123456789abcdef", week) {
		t.Error("WasSent() = false for an entry of this week")
	}
	if st.WasSent("0123456789abcdef", "2026-W46") {
		t.Error("WasSent() = true for another week")
	}
	if st.WasSent("", week) {
		t.Error("WasSent(\"\") = true; a count has no problem ID")
	}
	if got := st.SentInLast24h(testStart.Add(25 * time.Hour)); got != 0 {
		t.Errorf("SentInLast24h(25 h later) = %d, want 0", got)
	}
}

func TestStateDailyLimitIsNeverNegative(t *testing.T) {
	st := &State{}
	for i := 0; i < MaxPerDay+5; i++ {
		st.Record(Entry{Kind: problems.KindUnitFailed, ProblemID: "0123456789abcdef", Week: week, SentAt: testStart.Add(-time.Minute)})
	}
	if got := st.Remaining(testStart); got != 0 {
		t.Errorf("Remaining() = %d, want 0", got)
	}
}

func TestStateCountDueAndPaused(t *testing.T) {
	st := &State{}
	if !st.CountDue(testStart) {
		t.Error("CountDue() = false for an empty state")
	}
	st.RecordCount(week, Entry{Kind: problems.KindCount, Week: week, SentAt: testStart})
	if st.CountDue(testStart) {
		t.Error("CountDue() = true in the week of the count")
	}
	if !st.CountDue(testStart.AddDate(0, 1, 0)) {
		t.Error("CountDue() = false a month later")
	}
	if st.Paused(testStart) {
		t.Error("Paused() = true without a pause")
	}
	st.Fail(testStart)
	if !st.Paused(testStart.Add(time.Hour)) {
		t.Error("Paused() = false an hour after a failure")
	}
	if st.Paused(testStart.Add(RetryAfter)) {
		t.Error("Paused() = true when the pause ended")
	}
}

// TestStateTrimKeepsTheNewest500 proves the sent log keeps at most 500
// entries and drops the oldest first.
func TestStateTrimKeepsTheNewest500(t *testing.T) {
	st := &State{}
	for i := 0; i < MaxEntries+25; i++ {
		st.Record(Entry{Kind: problems.KindUnitFailed, ProblemID: "0123456789abcdef", Week: week,
			SentAt: testStart.Add(-time.Duration(MaxEntries+25-i) * time.Minute)})
	}
	if len(st.Sent) != MaxEntries {
		t.Fatalf("Sent holds %d entries, want %d", len(st.Sent), MaxEntries)
	}
	oldest := st.Sent[0].SentAt
	if oldest.Before(testStart.Add(-MaxEntries * time.Minute)) {
		t.Errorf("the oldest kept entry is %v; older entries were not dropped first", oldest)
	}
}

// TestStateTrimDropsEntriesOlderThan90Days proves the 90-day retention.
func TestStateTrimDropsEntriesOlderThan90Days(t *testing.T) {
	st := &State{}
	st.Record(Entry{Kind: problems.KindUnitFailed, ProblemID: "0123456789abcdef", Week: week, SentAt: testStart})
	st.Record(Entry{Kind: problems.KindUnitFailed, ProblemID: "fedcba9876543210", Week: week, SentAt: testStart.Add(91 * 24 * time.Hour)})
	if len(st.Sent) != 1 {
		t.Fatalf("Sent holds %d entries, want 1", len(st.Sent))
	}
	if st.Sent[0].ProblemID != "fedcba9876543210" {
		t.Errorf("the kept entry is %q, want the newer one", st.Sent[0].ProblemID)
	}
}
