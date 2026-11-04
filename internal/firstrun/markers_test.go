package firstrun

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func envOf(values map[string]string) func(string) string {
	return func(name string) string { return values[name] }
}

// testPaths resolves the paths for a temporary config home.
func testPaths(t *testing.T) Paths {
	t.Helper()
	paths, err := ResolvePaths(envOf(map[string]string{"XDG_CONFIG_HOME": filepath.Join(t.TempDir(), "cfg")}), "")
	if err != nil {
		t.Fatal(err)
	}
	return paths
}

func exists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stat %s: %v", path, err)
	}
	return err == nil
}

func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResolvePaths(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		home    string
		want    string
		wantErr bool
	}{
		{name: "XDG_CONFIG_HOME wins", env: map[string]string{"XDG_CONFIG_HOME": "/x/cfg"}, home: "/home/ada", want: "/x/cfg"},
		{name: "empty XDG_CONFIG_HOME falls back", env: map[string]string{"XDG_CONFIG_HOME": ""}, home: "/home/ada", want: "/home/ada/.config"},
		{name: "unset XDG_CONFIG_HOME falls back", home: "/home/ada", want: "/home/ada/.config"},
		{name: "XDG_CONFIG_HOME without home", env: map[string]string{"XDG_CONFIG_HOME": "/x/cfg"}, want: "/x/cfg"},
		{name: "neither", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolvePaths(envOf(tt.env), tt.home)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ResolvePaths() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			want := Paths{
				ConfigHome:      tt.want,
				Marker:          tt.want + "/chairlift/first-run.done",
				LegacyMarker:    tt.want + "/snow-first-setup.done",
				LegacyAutostart: tt.want + "/autostart/org.frostyard.FirstSetup.autostart.desktop",
			}
			if got != want {
				t.Fatalf("ResolvePaths() = %+v, want %+v", got, want)
			}
		})
	}
}

func TestMarkerPathsWritesBothDuringTransition(t *testing.T) {
	if !writeLegacyMarker {
		t.Fatal("writeLegacyMarker = false; ADR-0014's end rule has not been met by any recorded change")
	}
	p := Paths{Marker: "/c/chairlift/first-run.done", LegacyMarker: "/c/snow-first-setup.done"}
	if got, want := p.MarkerPaths(), []string{p.Marker, p.LegacyMarker}; !reflect.DeepEqual(got, want) {
		t.Fatalf("MarkerPaths() = %v, want %v", got, want)
	}
}

func TestDoneMarkerDetection(t *testing.T) {
	tests := []struct {
		name   string
		create func(Paths) []string
		want   bool
	}{
		{"no marker", func(Paths) []string { return nil }, false},
		{"chairlift marker", func(p Paths) []string { return []string{p.Marker} }, true},
		{"legacy marker only", func(p Paths) []string { return []string{p.LegacyMarker} }, true},
		{"both markers", func(p Paths) []string { return []string{p.Marker, p.LegacyMarker} }, true},
		{"legacy autostart is not a marker", func(p Paths) []string { return []string{p.LegacyAutostart} }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := testPaths(t)
			for _, path := range tt.create(p) {
				touch(t, path)
			}
			if got := IsDone(p); got != tt.want {
				t.Fatalf("IsDone() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMarkDoneWritesBothMarkersAndRemovesLegacyAutostart(t *testing.T) {
	p := testPaths(t)
	touch(t, p.LegacyAutostart)

	if err := MarkDone(p); err != nil {
		t.Fatalf("MarkDone() error = %v", err)
	}
	for _, path := range []string{p.Marker, p.LegacyMarker} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("marker %s: %v", path, err)
		}
		if info.Size() != 0 || info.Mode().Perm()&^0o644 != 0 || !info.Mode().IsRegular() {
			t.Errorf("marker %s: size %d mode %v, want an empty regular file with mode 0644 or less", path, info.Size(), info.Mode())
		}
	}
	for _, dir := range []string{p.ConfigHome, filepath.Dir(p.Marker)} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o700 {
			t.Errorf("created directory %s has mode %v, want 0700", dir, info.Mode().Perm())
		}
	}
	if exists(t, p.LegacyAutostart) {
		t.Error("legacy autostart copy still exists")
	}
	if !IsDone(p) {
		t.Error("IsDone() = false after MarkDone")
	}
}

func TestMarkDoneIsIdempotentAndKeepsExistingFiles(t *testing.T) {
	p := testPaths(t)
	if err := os.MkdirAll(filepath.Dir(p.Marker), 0o755); err != nil {
		t.Fatal(err)
	}
	// Chmod, not the umask, sets the mode the test expects to be kept.
	if err := os.Chmod(filepath.Dir(p.Marker), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.LegacyMarker, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := MarkDone(p); err != nil {
			t.Fatalf("MarkDone() call %d error = %v (a missing autostart copy is not an error)", i+1, err)
		}
	}
	data, err := os.ReadFile(p.LegacyMarker)
	if err != nil || string(data) != "keep" {
		t.Fatalf("existing legacy marker = %q, %v; want it kept", data, err)
	}
	info, err := os.Stat(filepath.Dir(p.Marker))
	if err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("existing chairlift directory mode = %v, %v; want it kept", info.Mode().Perm(), err)
	}
}

func TestMarkDoneReportsFailures(t *testing.T) {
	p := testPaths(t)
	// A regular file where the config home must be a directory.
	touch(t, p.ConfigHome)
	err := MarkDone(p)
	if err == nil {
		t.Fatal("MarkDone() error = nil, want an error")
	}
	for _, path := range []string{filepath.Dir(p.Marker), p.Marker, p.LegacyMarker} {
		if !strings.Contains(err.Error(), path) {
			t.Errorf("MarkDone() error %q does not name %s", err, path)
		}
	}
}
