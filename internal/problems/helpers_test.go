package problems

import (
	"os"
	"path/filepath"
	"testing"

	stdproblems "github.com/frostyard/std/problems"
)

// fixtureUID is the UID of the user crash fixture.
const fixtureUID = 1000

func fixturePath(parts ...string) string {
	return filepath.Join(append([]string{"testdata"}, parts...)...)
}

// fixtureSystem reads the real snosi-health problems.json.
func fixtureSystem(t *testing.T) SystemFile {
	t.Helper()
	file := ReadSystem(fixturePath("snosi", "problems.json"))
	if file.Outcome != SourceOK {
		t.Fatalf("snosi fixture outcome = %s (%v), want ok", file.Outcome, file.Err)
	}
	return file
}

// fixtureCrashes returns the user crash items of the coredump fixture.
func fixtureCrashes(t *testing.T, env Envelope) []Item {
	t.Helper()
	data, err := os.ReadFile(fixturePath("user", "coredumps.json"))
	if err != nil {
		t.Fatal(err)
	}
	entries, err := ParseCoredumps(data)
	if err != nil {
		t.Fatal(err)
	}
	return CrashItems(entries, fixtureUID, env)
}

// fixtureJournal parses the journal fixture with std's reader.
func fixtureJournal(t *testing.T) stdproblems.Result {
	t.Helper()
	f, err := os.Open(fixturePath("user", "journal.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	res, err := stdproblems.ParseJSON(f)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// reportableFixtureItems returns every reportable item of all fixtures.
func reportableFixtureItems(t *testing.T) []Item {
	t.Helper()
	system := fixtureSystem(t)
	all := append([]Item(nil), system.Items...)
	all = append(all, fixtureCrashes(t, system.Envelope)...)
	all = append(all, AppItems(fixtureJournal(t), system.Envelope)...)
	var out []Item
	for _, it := range all {
		if it.Reportable {
			out = append(out, it)
		}
	}
	return out
}

func writeTestFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func itemByID(t *testing.T, items []Item, id string) Item {
	t.Helper()
	for _, it := range items {
		if it.ID == id {
			return it
		}
	}
	t.Fatalf("no item %s", id)
	return Item{}
}
