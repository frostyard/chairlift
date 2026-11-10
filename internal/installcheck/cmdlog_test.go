package installcheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEveryCommandIsTraced keeps AGENTS.md's "Every command shows in the
// console" true: each production file under internal/ that starts an
// external command also announces it through internal/cmdlog. The kit
// (internal/frostui) is exempt: it cannot import ChairLift, and the views
// trace what they launch through it (launchDesktopApp).
func TestEveryCommandIsTraced(t *testing.T) {
	root := filepath.Join(RepoRoot(), "internal")
	checked := 0
	for _, path := range goFilesUnder(t, root) {
		rel, _ := filepath.Rel(RepoRoot(), path)
		rel = filepath.ToSlash(rel)
		if strings.HasSuffix(rel, "_test.go") || strings.HasPrefix(rel, "internal/frostui/") || strings.HasPrefix(rel, "internal/cmdlog/") {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		src := string(data)
		if !strings.Contains(src, "exec.Command(") && !strings.Contains(src, "exec.CommandContext(") {
			continue
		}
		checked++
		if !strings.Contains(src, "cmdlog.Start(") {
			t.Errorf("%s starts a command but never announces it with cmdlog.Start", rel)
		}
	}
	if checked < 8 {
		t.Fatalf("saw %d files that start commands, too few for the scan to be working", checked)
	}
}
