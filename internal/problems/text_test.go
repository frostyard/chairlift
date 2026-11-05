package problems

import (
	"strings"
	"testing"
)

func TestDetailsTextSystemItem(t *testing.T) {
	file := fixtureSystem(t)
	details, err := readDetails(fixturePath("snosi", "details.json"))
	if err != nil {
		t.Fatal(err)
	}
	applyDetails(file.Items, details)
	unit := itemByID(t, file.Items, "a6d8d298c66718de")
	text := DetailsText(unit)
	for _, want := range []string{
		"A system service failed\n",
		"First seen: 2026-11-05T03:56:38Z",
		"Last seen: 2026-11-05T03:56:38Z",
		"Times: 1",
		"Source: system checks (snosi-health)",
		valueHints["unit_failed.result=exit-code"],
		"  result: exit-code",
		"  unit: (not recorded)",
		"  shipped: (not recorded)",
		"  week: 2026-W45",
		"Reportable: no",
		"Local details (never sent):",
		"  unit: brian-backup.service",
		"  Units: brian-backup.service",
		"  Log: Failed to start backup of /home/brian on host frosty-desk",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("details text does not contain %q:\n%s", want, text)
		}
	}
	reportable := DetailsText(itemByID(t, file.Items, "7353e332c0c70754"))
	for _, want := range []string{"Reportable: yes (ChairLift sends nothing)", "  Program: /usr/bin/gnome-shell", "  signal: SIGSEGV"} {
		if !strings.Contains(reportable, want) {
			t.Errorf("crash details text does not contain %q:\n%s", want, reportable)
		}
	}
}

func TestDetailsTextUserItems(t *testing.T) {
	env := fixtureSystem(t).Envelope
	apps := AppItems(fixtureJournal(t), env)
	text := DetailsText(apps[0])
	for _, want := range []string{
		"Frostmail reported an error",
		"Times: 2",
		"Source: app problem records (your journal)",
		codeHints["store.migrate_failed"],
		"  Process: frostmaild",
		"  Unit: frostmail.service",
		"  Message: migrate /home/brian/.local/share/frostmail/store.db: database is locked",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("app details text does not contain %q:\n%s", want, text)
		}
	}
	tool := fixtureCrashes(t, env)[1]
	text = DetailsText(tool)
	for _, want := range []string{"Source: your crash records (coredumpctl)", "  exe: tool (not valid: not a program under /usr)", "  Program: /home/brian/bin/tool"} {
		if !strings.Contains(text, want) {
			t.Errorf("crash details text does not contain %q:\n%s", want, text)
		}
	}
	bare := DetailsText(Item{Kind: KindCount, KnownKind: true, Origin: Origin(7)})
	for _, want := range []string{"First seen: unknown", "Source: unknown"} {
		if !strings.Contains(bare, want) {
			t.Errorf("bare details text does not contain %q:\n%s", want, bare)
		}
	}
	if strings.Contains(bare, "Local details") {
		t.Error("an item with no local text shows a local section")
	}
}
