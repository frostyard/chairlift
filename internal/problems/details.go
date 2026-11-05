package problems

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"sort"
)

// Limits of the local text of details.json.
const (
	maxMessages   = 5
	maxMessageLen = 1024
	maxUnits      = 16
	maxUnitLen    = 256
)

// localEntry is the local text of 1 problem in details.json.
type localEntry struct {
	kind  string
	local Local
}

// readDetails reads details.json (format 1, mode 0640 root:sudo). A file
// that does not exist or that this user may not read is normal and gives no
// details and no error. Any other failure, a file larger than
// MaxDetailsBytes, or content that is not format 1 gives an error, and the
// page shows the items without local details.
func readDetails(path string) (map[string]localEntry, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrPermission) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, MaxDetailsBytes+1))
	if err != nil {
		return nil, err
	}
	return parseDetails(data)
}

// parseDetails parses the content of a details.json file. Every value is
// local display text: nothing of it enters a payload.
func parseDetails(data []byte) (map[string]localEntry, error) {
	if len(data) > MaxDetailsBytes {
		return nil, fmt.Errorf("details.json: larger than %d bytes", MaxDetailsBytes)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil || top == nil {
		return nil, errors.New("details.json: not a JSON object")
	}
	var format int
	if !decodeStrict(top["format"], &format) || format != 1 {
		return nil, errors.New("details.json: format is not 1")
	}
	var problems map[string]json.RawMessage
	if !decodeStrict(top["problems"], &problems) {
		return nil, errors.New("details.json: problems is not an object")
	}
	entries := make(map[string]localEntry, len(problems))
	for id, raw := range problems {
		if !idRe.MatchString(id) {
			continue
		}
		var entry map[string]json.RawMessage
		if !decodeStrict(raw, &entry) {
			continue
		}
		var e localEntry
		_ = decodeStrict(entry["kind"], &e.kind)
		e.local = parseLocal(entry)
		entries[id] = e
	}
	return entries, nil
}

// parseLocal reads "local", "extra" (units, exe_path) and "messages".
func parseLocal(entry map[string]json.RawMessage) Local {
	var local Local
	var values map[string]json.RawMessage
	if decodeStrict(entry["local"], &values) {
		names := make([]string, 0, len(values))
		for name := range values {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			local.Values = append(local.Values, Field{Name: cutDisplay(name), Display: rawDisplay(values[name])})
		}
	}
	var extra map[string]json.RawMessage
	if decodeStrict(entry["extra"], &extra) {
		var units []json.RawMessage
		if decodeStrict(extra["units"], &units) {
			for _, raw := range units {
				var unit string
				if len(local.Units) < maxUnits && decodeStrict(raw, &unit) {
					local.Units = append(local.Units, cutText(unit, maxUnitLen))
				}
			}
		}
		var exePath string
		if decodeStrict(extra["exe_path"], &exePath) {
			local.ExePath = cutText(exePath, maxUnitLen)
		}
	}
	var messages []json.RawMessage
	if decodeStrict(entry["messages"], &messages) {
		for _, raw := range messages {
			var msg string
			if len(local.Messages) < maxMessages && decodeStrict(raw, &msg) {
				local.Messages = append(local.Messages, cutText(msg, maxMessageLen))
			}
		}
	}
	return local
}

// applyDetails adds the local text of details to the system items whose id
// and kind match.
func applyDetails(items []Item, details map[string]localEntry) {
	for i := range items {
		entry, ok := details[items[i].ID]
		if !ok || entry.kind != items[i].Kind {
			continue
		}
		items[i].Local = entry.local
	}
}
