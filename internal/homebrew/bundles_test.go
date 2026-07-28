package homebrew

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeBundleFile(t *testing.T, dir, name, contents string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write bundle %s: %v", path, err)
	}
	return path
}

func TestAvailableBundlesDiscoversEveryConfiguredDirectory(t *testing.T) {
	root := t.TempDir()
	firstDir := filepath.Join(root, "a")
	secondDir := filepath.Join(root, "b")
	if err := os.Mkdir(firstDir, 0o755); err != nil {
		t.Fatalf("create first bundle directory: %v", err)
	}
	if err := os.Mkdir(secondDir, 0o755); err != nil {
		t.Fatalf("create second bundle directory: %v", err)
	}
	firstCLI := writeBundleFile(t, firstDir, "cli.Brewfile", "# Command-line tools\nbrew \"bat\"\n")
	secondCLI := writeBundleFile(t, secondDir, "cli.Brewfile", "# Alternate CLI set\nbrew \"fd\"\n")
	fonts := writeBundleFile(t, secondDir, "fonts.Brewfile", "cask \"font-test\"\n")
	writeBundleFile(t, firstDir, "ignored.txt", "# not a Brewfile\n")
	if err := os.Mkdir(filepath.Join(firstDir, "directory.Brewfile"), 0o700); err != nil {
		t.Fatalf("create Brewfile-shaped directory: %v", err)
	}

	missingDir := filepath.Join(t.TempDir(), "not-installed")
	bundles, err := AvailableBundles([]string{
		firstDir,
		missingDir,
		secondDir,
		filepath.Join(firstDir, "."),
	})
	if err != nil {
		t.Fatalf("AvailableBundles() error = %v, want nil", err)
	}

	want := []Bundle{
		{Name: "cli", Description: "Command-line tools", Path: firstCLI},
		{Name: "cli", Description: "Alternate CLI set", Path: secondCLI},
		{Name: "fonts", Description: "", Path: fonts},
	}
	if !reflect.DeepEqual(bundles, want) {
		t.Fatalf("AvailableBundles() = %#v, want %#v", bundles, want)
	}
	for _, bundle := range bundles {
		if !filepath.IsAbs(bundle.Path) {
			t.Errorf("bundle path %q is not absolute", bundle.Path)
		}
	}
}

func TestAvailableBundlesMissingPathsAreNotErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	bundles, err := AvailableBundles([]string{missing})
	if err != nil {
		t.Fatalf("AvailableBundles(%q) error = %v, want nil", missing, err)
	}
	if len(bundles) != 0 {
		t.Fatalf("AvailableBundles(%q) = %#v, want no bundles", missing, bundles)
	}
}

func TestAvailableBundlesReturnsPartialResultsWithPathErrors(t *testing.T) {
	validDir := t.TempDir()
	wantPath := writeBundleFile(t, validDir, "working.Brewfile", "# Working set\n")
	notDirectory := writeBundleFile(t, t.TempDir(), "not-a-directory", "contents\n")

	bundles, err := AvailableBundles([]string{notDirectory, "", validDir})
	if err == nil {
		t.Fatal("AvailableBundles() error = nil, want invalid-path diagnostics")
	}
	for _, want := range []string{notDirectory, "bundle directory path is empty"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("AvailableBundles() error %q does not contain %q", err, want)
		}
	}

	want := []Bundle{{Name: "working", Description: "Working set", Path: wantPath}}
	if !reflect.DeepEqual(bundles, want) {
		t.Fatalf("AvailableBundles() partial result = %#v, want %#v", bundles, want)
	}
}

func TestAvailableBundlesRejectsUnreadableEntriesWithoutLosingOthers(t *testing.T) {
	dir := t.TempDir()
	goodPath := writeBundleFile(t, dir, "good.Brewfile", "# Good bundle\n")
	brokenPath := filepath.Join(dir, "broken.Brewfile")
	if err := os.Symlink(filepath.Join(dir, "missing-target"), brokenPath); err != nil {
		t.Fatalf("create broken Brewfile symlink: %v", err)
	}

	bundles, err := AvailableBundles([]string{dir})
	if err == nil || !strings.Contains(err.Error(), brokenPath) {
		t.Fatalf("AvailableBundles() error = %v, want broken entry path", err)
	}
	want := []Bundle{{Name: "good", Description: "Good bundle", Path: goodPath}}
	if !reflect.DeepEqual(bundles, want) {
		t.Fatalf("AvailableBundles() partial result = %#v, want %#v", bundles, want)
	}
}

func TestAvailableBundlesBoundsDescriptionRead(t *testing.T) {
	dir := t.TempDir()
	path := writeBundleFile(
		t,
		dir,
		"oversized.Brewfile",
		"#"+strings.Repeat("x", maxBundleDescriptionBytes)+"\nbrew \"bat\"\n",
	)

	bundles, err := AvailableBundles([]string{dir})
	if err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("AvailableBundles() error = %v, want bounded-read error naming %q", err, path)
	}
	if len(bundles) != 0 {
		t.Fatalf("AvailableBundles() = %#v, want oversized description omitted", bundles)
	}
}

func TestBundleInstallHonorsDryRun(t *testing.T) {
	original := IsDryRun()
	SetDryRun(true)
	t.Cleanup(func() { SetDryRun(original) })

	if err := BundleInstall("/definitely/not/a/real/Brewfile"); err != nil {
		t.Fatalf("BundleInstall() dry-run error = %v, want nil without invoking brew", err)
	}
}
