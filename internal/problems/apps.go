package problems

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"

	stdproblems "github.com/frostyard/std/problems"
)

// Journal read of the apps group: the user journal of the last 90 days, at
// most 500 entries.
const (
	JournalWindow = 90 * 24 * time.Hour
	JournalLimit  = 500
)

// maxAppMessage bounds the MESSAGE that the page shows.
const maxAppMessage = 2048

// JournalArgs returns the journalctl arguments of the apps group: std's
// problems.Read arguments for the user journal since now − JournalWindow,
// at most JournalLimit entries. --all prints fields longer than 4096 bytes
// (FROSTYARD_FRAMES can be longer) instead of null. ChairLift runs
// journalctl itself, instead of std's Read, so that it can cap the output
// (MaxJournalOutput): Read keeps all of journalctl's output in memory.
//
// It sets no --output-fields: std's Validate drops a record with a
// FROSTYARD_* field that the registry does not name, and a field list would
// hide such a field from it.
func JournalArgs(now time.Time) []string {
	return []string{
		"--no-pager", "-q", "-o", "json", "--all", "--user",
		"--since=@" + strconv.FormatInt(now.Add(-JournalWindow).Unix(), 10),
		"-n", strconv.Itoa(JournalLimit),
		"MESSAGE_ID=" + stdproblems.MessageID,
	}
}

// appGroupKey is the grouping key of app problem records: app ID, version,
// class, code, and the first 3 frames or the location.
func appGroupKey(d stdproblems.Detail) string {
	frames := d.Frames
	if len(frames) > 3 {
		frames = frames[:3]
	}
	return strings.Join([]string{
		d.AppID, d.AppVersion, d.Class, d.Code,
		strings.Join(frames, "\n"), d.Location,
	}, "\x00")
}

// AppItems groups the valid records of res into items, newest first, at
// most MaxItems. std's reader already dropped every record that failed its
// validation; each item's fields are checked again by the same v1 rules as
// the system items, so the page and BuildPayload have 1 path.
func AppItems(res stdproblems.Result, env Envelope) []Item {
	groups := map[string][]stdproblems.Record{}
	var order []string
	for _, rec := range res.Records {
		key := appGroupKey(rec.Detail)
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], rec)
	}
	items := make([]Item, 0, len(order))
	for _, key := range order {
		items = append(items, appItem(key, groups[key], env))
	}
	sortItems(items)
	if len(items) > MaxItems {
		items = items[:MaxItems]
	}
	return items
}

// appItem builds the item of 1 group of records. The newest record gives
// the detail, the MESSAGE and the trusted fields.
func appItem(key string, records []stdproblems.Record, env Envelope) Item {
	sort.SliceStable(records, func(i, j int) bool {
		return records[i].Trusted.Time.Before(records[j].Trusted.Time)
	})
	newest := records[len(records)-1]
	item := Item{
		ID:        hashID("app-", key),
		Origin:    OriginAppRecord,
		Kind:      KindAppProblem,
		KnownKind: true,
		FirstSeen: records[0].Trusted.Time,
		LastSeen:  newest.Trusted.Time,
		Count:     len(records),
		Envelope:  env,
		Local: Local{
			Message: cutText(newest.Message, maxAppMessage),
			Comm:    cutDisplay(newest.Trusted.Comm),
			Unit:    cutDisplay(firstNonEmpty(newest.Trusted.SystemdUserUnit, newest.Trusted.SystemdUnit)),
		},
	}
	detailOK := false
	if data, err := json.Marshal(newest.Detail); err == nil {
		var raw map[string]json.RawMessage
		if json.Unmarshal(data, &raw) == nil {
			item.Fields, item.Missing, detailOK = validateDetail(KindAppProblem, raw)
		}
	}
	item.Week = isoWeek(item.LastSeen)
	item.WeekValid = weekRe.MatchString(item.Week)
	item.Reportable = detailOK && item.WeekValid && env.Valid
	return item
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
