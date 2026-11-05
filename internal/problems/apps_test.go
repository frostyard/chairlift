package problems

import (
	"strings"
	"testing"
	"time"

	stdproblems "github.com/frostyard/std/problems"
)

func TestJournalReadOptions(t *testing.T) {
	now := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	got := JournalReadOptions(now)
	if !got.User || got.Limit != 500 || !got.Since.Equal(now.Add(-90*24*time.Hour)) || got.Journalctl != "" {
		t.Fatalf("JournalReadOptions = %+v", got)
	}
}

// TestAppItemsFixture: std drops the 3 forged records; the 5 valid ones form
// 4 groups, newest first, each with its count, newest MESSAGE and trusted
// fields.
func TestAppItemsFixture(t *testing.T) {
	res := fixtureJournal(t)
	if res.Dropped != 3 || len(res.Records) != 5 {
		t.Fatalf("records = %d, dropped = %d; want 5 and 3", len(res.Records), res.Dropped)
	}
	env := fixtureSystem(t).Envelope
	items := AppItems(res, env)
	if len(items) != 4 {
		t.Fatalf("items = %d, want 4", len(items))
	}
	wantTitles := []string{
		"Frostmail reported an error",
		"nsl stopped unexpectedly",
		"ChairLift reported an error",
		"Frostread stopped unexpectedly",
	}
	for i, it := range items {
		if got := Title(it); got != wantTitles[i] {
			t.Errorf("item %d title = %q, want %q", i, got, wantTitles[i])
		}
		if !it.Reportable || it.Origin != OriginAppRecord || !strings.HasPrefix(it.ID, "app-") || !it.WeekValid {
			t.Errorf("item %d = %+v", i, it)
		}
	}
	mail := items[0]
	if mail.Count != 2 || !strings.Contains(mail.Local.Message, "database is locked") ||
		mail.Local.Comm != "frostmaild" || mail.Local.Unit != "frostmail.service" {
		t.Errorf("Frostmail item = %+v", mail)
	}
	if mail.ValidString("code") != "store.migrate_failed" {
		t.Errorf("code = %q", mail.ValidString("code"))
	}
	if want := time.Date(2026, 11, 4, 8, 0, 0, 0, time.UTC); !mail.FirstSeen.Equal(want) {
		t.Errorf("first seen = %v, want %v", mail.FirstSeen, want)
	}
	rust := items[3]
	if rust.ValidString("location") != "src/render/page.rs:88" {
		t.Errorf("location = %q", rust.ValidString("location"))
	}
}

func TestAppGroupKeyUsesTheFirst3Frames(t *testing.T) {
	a := stdproblems.Detail{AppID: "org.frostyard.nsl", AppVersion: "1.0.0", Class: "panic", Frames: []string{"a.A", "b.B", "c.C", "d.D"}}
	b := a
	b.Frames = []string{"a.A", "b.B", "c.C", "e.E"}
	c := a
	c.Frames = []string{"a.A", "b.B", "x.X"}
	if appGroupKey(a) != appGroupKey(b) {
		t.Error("records that differ after frame 3 are 2 groups")
	}
	if appGroupKey(a) == appGroupKey(c) {
		t.Error("records that differ in frame 3 are 1 group")
	}
	d := a
	d.AppVersion = "1.0.1"
	if appGroupKey(a) == appGroupKey(d) {
		t.Error("records of 2 versions are 1 group")
	}
}

func TestAppItemsAreCapped(t *testing.T) {
	var res stdproblems.Result
	for i := 0; i < MaxItems+1; i++ {
		res.Records = append(res.Records, stdproblems.Record{
			Trusted: stdproblems.Trusted{Time: time.Unix(int64(i), 0).UTC()},
			Detail: stdproblems.Detail{AppID: "org.frostyard.nsl", AppVersion: "1.0.0", Runtime: "go",
				Component: "cli", Class: "error", Code: "c" + strings.Repeat("a", i%60) + string(rune('a'+i/60))},
		})
	}
	if got := AppItems(res, Envelope{}); len(got) != MaxItems {
		t.Fatalf("items = %d, want %d", len(got), MaxItems)
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if firstNonEmpty("", "b", "c") != "b" || firstNonEmpty() != "" {
		t.Error("firstNonEmpty")
	}
}
