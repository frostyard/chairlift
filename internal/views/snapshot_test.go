package views

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// snapshot saves what tt shows as <name>.png in $CHAIRLIFT_SNAPSHOT_DIR,
// when it is set, so a person (or an agent) can look at a page without a
// display: CHAIRLIFT_SNAPSHOT_DIR=/tmp/shots go test ./internal/views -run Snapshot
func snapshot(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	dir := os.Getenv("CHAIRLIFT_SNAPSHOT_DIR")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := png.Encode(f, tt.Image()); err != nil {
		t.Fatal(err)
	}
	t.Logf("snapshot: %s", f.Name())
}
