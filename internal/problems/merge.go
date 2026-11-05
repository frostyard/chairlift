package problems

import (
	"path"
	"time"
)

// MergeWindow is the slack of the merge rule on each side of a system
// crash's first_seen and last_seen.
const MergeWindow = time.Minute

// MergeCrashes applies the merge rule: a system crash item and a user crash
// item with the same exe basename and signal, where a time of the user item
// is between the system item's first_seen and last_seen (inclusive, with
// MergeWindow of slack on each side), are 1 crash. The user item stays (in
// the apps group); the system item is removed. It returns the system items
// that stay and the number removed.
func MergeCrashes(system, user []Item) ([]Item, int) {
	kept := make([]Item, 0, len(system))
	merged := 0
	for _, sys := range system {
		if sys.Kind == KindCrash && sys.KnownKind && matchesUserCrash(sys, user) {
			merged++
			continue
		}
		kept = append(kept, sys)
	}
	return kept, merged
}

// matchesUserCrash reports whether a user crash item is the same crash as
// the system crash item sys.
func matchesUserCrash(sys Item, user []Item) bool {
	exe := sys.ValidString("exe")
	signal := sys.ValidString("signal")
	if exe == "" || signal == "" {
		return false
	}
	from := sys.FirstSeen.Add(-MergeWindow)
	to := sys.LastSeen.Add(MergeWindow)
	for _, u := range user {
		if u.Kind != KindCrash || u.ValidString("signal") != signal {
			continue
		}
		if path.Base(u.Local.ExePath) != exe {
			continue
		}
		for _, t := range u.times {
			if !t.Before(from) && !t.After(to) {
				return true
			}
		}
	}
	return false
}
