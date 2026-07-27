package config

import (
	"bytes"
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func withReadFile(t *testing.T, fn func(string) ([]byte, error)) {
	t.Helper()
	original := readFile
	t.Cleanup(func() { readFile = original })
	readFile = fn
}

func assertAllKnownGroupsDisabled(t *testing.T, got *Config) {
	t.Helper()
	pages, err := SchemaPages()
	if err != nil {
		t.Fatalf("SchemaPages(): %v", err)
	}
	if len(pages) == 0 {
		t.Fatal("SchemaPages() returned no pages")
	}

	defaults := defaultConfig()
	for _, page := range pages {
		groups, err := SchemaGroups(page)
		if err != nil {
			t.Fatalf("SchemaGroups(%q): %v", page, err)
		}
		if len(groups) == 0 {
			t.Fatalf("SchemaGroups(%q) returned no groups", page)
		}
		for _, group := range groups {
			gotGroup := got.GetGroupConfig(page, group)
			if gotGroup == nil {
				t.Errorf("fail-closed config missing %s.%s", page, group)
				continue
			}
			if gotGroup.Enabled {
				t.Errorf("fail-closed config left %s.%s enabled", page, group)
			}

			wantGroup := defaults.GetGroupConfig(page, group)
			if wantGroup == nil {
				t.Errorf("default config missing canonical %s.%s", page, group)
				continue
			}
			wantGroup.Enabled = false
			if !reflect.DeepEqual(*gotGroup, *wantGroup) {
				t.Errorf("fail-closed %s.%s = %+v, want defaults with Enabled=false: %+v",
					page, group, *gotGroup, *wantGroup)
			}
		}
	}
}

func TestLoadMissingHigherPriorityContinuesToValidCandidate(t *testing.T) {
	dir := t.TempDir()
	high := filepath.Join(dir, "missing.yml")
	low := writeConfigFile(t, "system_page:\n  health_group:\n    enabled: false\n")
	withConfigPaths(t, []string{high, low})

	cfg, loadErr := Load()
	if loadErr != nil {
		t.Fatalf("Load() error = %v, want nil", loadErr)
	}
	if cfg.SystemPage["health_group"].Enabled {
		t.Fatal("lower-priority valid overlay was not loaded after absent candidate")
	}
}

func TestLoadReadFailureStopsPrecedenceAndFailsClosed(t *testing.T) {
	dir := t.TempDir()
	high := filepath.Join(dir, "high.yml")
	low := filepath.Join(dir, "low.yml")
	withConfigPaths(t, []string{high, low})

	var reads []string
	withReadFile(t, func(path string) ([]byte, error) {
		reads = append(reads, path)
		switch path {
		case high:
			return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrPermission}
		case low:
			return []byte("system_page:\n  health_group:\n    enabled: true\n"), nil
		default:
			return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
		}
	})

	cfg, loadErr := Load()
	if loadErr == nil {
		t.Fatal("Load() error = nil, want authoritative read failure")
	}
	if loadErr.Kind != KindRead {
		t.Fatalf("Load() error kind = %q, want %q", loadErr.Kind, KindRead)
	}
	if loadErr.Path != high {
		t.Fatalf("Load() error path = %q, want %q", loadErr.Path, high)
	}
	if !errors.Is(loadErr, fs.ErrPermission) {
		t.Fatalf("errors.Is(%v, fs.ErrPermission) = false", loadErr)
	}
	if !reflect.DeepEqual(reads, []string{high}) {
		t.Fatalf("read paths = %v, want only authoritative candidate %q", reads, high)
	}
	assertAllKnownGroupsDisabled(t, cfg)
}

func TestLoadInvalidAuthoritativeStopsPrecedenceAndFailsClosed(t *testing.T) {
	tests := []struct {
		name     string
		contents string
		wantKind ErrorKind
	}{
		{
			name:     "schema",
			contents: "updates_pages:\n  brew_updates_group:\n    enabled: false\n",
			wantKind: KindSchema,
		},
		{
			name:     "parse-type",
			contents: "system_page: [\n",
			wantKind: KindParseType,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			high := filepath.Join(dir, "high.yml")
			low := filepath.Join(dir, "low.yml")
			withConfigPaths(t, []string{high, low})

			var reads []string
			withReadFile(t, func(path string) ([]byte, error) {
				reads = append(reads, path)
				switch path {
				case high:
					return []byte(tt.contents), nil
				case low:
					return []byte("system_page:\n  health_group:\n    enabled: true\n"), nil
				default:
					return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
				}
			})

			cfg, loadErr := Load()
			if loadErr == nil {
				t.Fatal("Load() error = nil, want authoritative validation failure")
			}
			if loadErr.Kind != tt.wantKind {
				t.Fatalf("Load() error kind = %q, want %q: %v", loadErr.Kind, tt.wantKind, loadErr)
			}
			if loadErr.Path != high {
				t.Fatalf("Load() error path = %q, want %q", loadErr.Path, high)
			}
			if !reflect.DeepEqual(reads, []string{high}) {
				t.Fatalf("read paths = %v, want only authoritative candidate %q", reads, high)
			}
			assertAllKnownGroupsDisabled(t, cfg)
		})
	}
}

func TestLoadFromPathRejectsUnknownSchemaName(t *testing.T) {
	path := writeConfigFile(t, "not_a_page:\n  anything: true\n")
	cfg, loadErr := loadFromPath(path)
	if cfg != nil {
		t.Fatalf("loadFromPath(%q) config = %+v, want nil", path, cfg)
	}
	if loadErr == nil || loadErr.Kind != KindSchema {
		t.Fatalf("loadFromPath(%q) error = %v, want KindSchema", path, loadErr)
	}
	if loadErr.Path != path || !strings.Contains(loadErr.Detail, "not_a_page") {
		t.Fatalf("loadFromPath(%q) error = %+v, want path and offending key", path, loadErr)
	}
}

func TestLoadAuthoritativeFailureLogsHighSignalDiagnostic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	withConfigPaths(t, []string{path})
	withReadFile(t, func(got string) ([]byte, error) {
		return nil, &fs.PathError{Op: "open", Path: got, Err: fs.ErrPermission}
	})

	var output bytes.Buffer
	originalOutput := log.Writer()
	originalFlags := log.Flags()
	originalPrefix := log.Prefix()
	t.Cleanup(func() {
		log.SetOutput(originalOutput)
		log.SetFlags(originalFlags)
		log.SetPrefix(originalPrefix)
	})
	log.SetOutput(&output)
	log.SetFlags(0)
	log.SetPrefix("")

	_, loadErr := Load()
	if loadErr == nil {
		t.Fatal("Load() error = nil, want read failure")
	}
	got := output.String()
	for _, want := range []string{
		"CONFIGURATION ERROR",
		path,
		"permission denied",
		"all feature groups were disabled",
		"restart ChairLift",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("startup log %q does not contain %q", got, want)
		}
	}
}

func TestLoadAllCandidatesAbsentReturnsDefaultsWithoutError(t *testing.T) {
	dir := t.TempDir()
	withConfigPaths(t, []string{
		filepath.Join(dir, "one.yml"),
		filepath.Join(dir, "two.yml"),
		filepath.Join(dir, "three.yml"),
	})
	withReadFile(t, func(path string) ([]byte, error) {
		return nil, &fs.PathError{Op: "open", Path: path, Err: os.ErrNotExist}
	})

	cfg, loadErr := Load()
	if loadErr != nil {
		t.Fatalf("Load() error = %v, want nil", loadErr)
	}
	if !reflect.DeepEqual(cfg, defaultConfig()) {
		t.Fatalf("Load() all-absent config = %+v, want built-in defaults %+v", cfg, defaultConfig())
	}
}

func TestWindowConfigFailureWiringIsPersistent(t *testing.T) {
	sourcePath := filepath.Join(repoRoot(), "internal", "window", "window.go")
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read %s: %v", sourcePath, err)
	}

	text := string(source)
	for _, required := range []string{
		"config.Load()",
		"configError:       configErr",
		"w.ShowErrorToast(w.configError.ToastMessage())",
		"toast.SetTimeout(0)",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("window config-error wiring does not contain %q", required)
		}
	}
}

func TestConfigurationGuideExamplePassesStrictValidation(t *testing.T) {
	guidePath := filepath.Join(repoRoot(), "CONFIG.md")
	guide, err := os.ReadFile(guidePath)
	if err != nil {
		t.Fatalf("read %s: %v", guidePath, err)
	}

	const heading = "## Example: Disabling Homebrew Features"
	afterHeading := string(guide)
	headingIndex := strings.Index(afterHeading, heading)
	if headingIndex < 0 {
		t.Fatalf("%s does not contain %q", guidePath, heading)
	}
	afterHeading = afterHeading[headingIndex+len(heading):]
	fenceStart := strings.Index(afterHeading, "```yaml")
	if fenceStart < 0 {
		t.Fatalf("%s example has no YAML fence", heading)
	}
	afterFence := afterHeading[fenceStart+len("```yaml"):]
	fenceEnd := strings.Index(afterFence, "```")
	if fenceEnd < 0 {
		t.Fatalf("%s example has no closing fence", heading)
	}

	example := []byte(afterFence[:fenceEnd])
	if _, loadErr := parseAndValidate(guidePath, example); loadErr != nil {
		t.Fatalf("%s documented YAML is rejected by the runtime validator: %v", heading, loadErr)
	}
}
