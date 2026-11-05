package problems

import (
	"testing"
	"time"
)

func TestMergeCrashesFixture(t *testing.T) {
	file := fixtureSystem(t)
	crashes := fixtureCrashes(t, file.Envelope)
	kept, merged := MergeCrashes(file.Items, crashes)
	if merged != 1 || len(kept) != len(file.Items)-1 {
		t.Fatalf("merged = %d, kept = %d; want 1 and %d", merged, len(kept), len(file.Items)-1)
	}
	for _, it := range kept {
		if it.ID == "7353e332c0c70754" {
			t.Fatal("the gnome-shell system crash stayed in the system group")
		}
	}
}

func TestMergeCrashesRule(t *testing.T) {
	first := time.Date(2026, 11, 5, 2, 0, 0, 0, time.UTC)
	last := first.Add(time.Hour)
	system := Item{
		ID: "00000000000000aa", Kind: KindCrash, KnownKind: true, FirstSeen: first, LastSeen: last,
		Fields: []Field{
			{Name: "exe", Display: "gnome-shell", Value: "gnome-shell", Valid: true},
			{Name: "signal", Display: "SIGSEGV", Value: "SIGSEGV", Valid: true},
		},
	}
	user := func(exe string, signal int, at time.Time) []Item {
		return CrashItems([]CoredumpEntry{{Time: at, UID: 1, Signal: signal, Exe: exe}}, 1, Envelope{})
	}
	tests := []struct {
		name   string
		user   []Item
		merged int
	}{
		{"inside", user("/usr/bin/gnome-shell", 11, first.Add(30*time.Minute)), 1},
		{"at first seen", user("/usr/bin/gnome-shell", 11, first), 1},
		{"at last seen", user("/usr/bin/gnome-shell", 11, last), 1},
		{"1 minute before", user("/usr/bin/gnome-shell", 11, first.Add(-time.Minute)), 1},
		{"1 minute after", user("/usr/bin/gnome-shell", 11, last.Add(time.Minute)), 1},
		{"more than 1 minute before", user("/usr/bin/gnome-shell", 11, first.Add(-time.Minute-time.Second)), 0},
		{"more than 1 minute after", user("/usr/bin/gnome-shell", 11, last.Add(time.Minute+time.Second)), 0},
		{"other signal", user("/usr/bin/gnome-shell", 6, first), 0},
		{"other program", user("/usr/bin/nautilus", 11, first), 0},
		{"same basename outside /usr", user("/home/brian/gnome-shell", 11, first), 1},
		{"no user crash", nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kept, merged := MergeCrashes([]Item{system}, tt.user)
			if merged != tt.merged || len(kept) != 1-tt.merged {
				t.Fatalf("merged = %d, kept = %d; want %d", merged, len(kept), tt.merged)
			}
		})
	}
	t.Run("system crash without exe stays", func(t *testing.T) {
		noExe := system
		noExe.Fields = noExe.Fields[1:]
		if _, merged := MergeCrashes([]Item{noExe}, user("/usr/bin/gnome-shell", 11, first)); merged != 0 {
			t.Fatal("a system crash without exe merged")
		}
	})
	t.Run("other kinds stay", func(t *testing.T) {
		other := system
		other.Kind = KindUnitFailed
		if _, merged := MergeCrashes([]Item{other}, user("/usr/bin/gnome-shell", 11, first)); merged != 0 {
			t.Fatal("a unit item merged")
		}
	})
}
