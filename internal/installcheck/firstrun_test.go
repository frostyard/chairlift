package installcheck

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// firstRunPurePackages are the packages of the first-run gate binary and the
// first-run logic. puregotk dlopens GTK during package initialization, so if
// any of them pulled in a GTK binding, chairlift-first-run could no longer
// exit before GTK loads (ADR-0014).
var firstRunPurePackages = []string{
	"./cmd/chairlift-first-run",
	"./internal/firstrun",
	"./internal/i18n",
}

// TestFirstRunGateImportsNoGTK lists the transitive dependencies of the
// first-run packages with the go tool and fails on any puregotk or snowkit
// package.
func TestFirstRunGateImportsNoGTK(t *testing.T) {
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Skip("the go tool is not on PATH; cannot list the first-run dependencies")
	}
	for _, pkg := range firstRunPurePackages {
		t.Run(pkg, func(t *testing.T) {
			cmd := exec.Command(goTool, "list", "-deps", pkg)
			cmd.Dir = RepoRoot()
			cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("go list -deps %s: %v\n%s", pkg, err, stderr.String())
			}
			deps := strings.Fields(stdout.String())
			self := "github.com/frostyard/chairlift/" + strings.TrimPrefix(pkg, "./")
			found := false
			for _, dep := range deps {
				if dep == self {
					found = true
				}
				if strings.HasPrefix(dep, "codeberg.org/puregotk") || strings.HasPrefix(dep, "github.com/frostyard/snowkit") {
					t.Errorf("%s depends on %s, which loads GTK", pkg, dep)
				}
			}
			if !found {
				t.Fatalf("go list -deps %s did not list %s itself: %v", pkg, self, deps)
			}
		})
	}
}

// goreleaserBuild is the subset of a builds[] entry the first-run checks
// read.
type goreleaserBuild struct {
	ID     string   `yaml:"id"`
	Binary string   `yaml:"binary"`
	Env    []string `yaml:"env"`
	Goos   []string `yaml:"goos"`
	Goarch []string `yaml:"goarch"`
	Flags  []string `yaml:"flags"`
	Main   string   `yaml:"main"`
}

// TestFirstRunGateShipsInTheGUIPackageOnly pins the packaging of
// chairlift-first-run: a pure-Go build for both release architectures, in
// the frostyard-chairlift package and not in the system-integration package
// (ADR-0006).
func TestFirstRunGateShipsInTheGUIPackageOnly(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(RepoRoot(), ".goreleaser.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Builds []goreleaserBuild `yaml:"builds"`
	}
	if err := yaml.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	var build *goreleaserBuild
	for i := range file.Builds {
		if file.Builds[i].ID == "chairlift-first-run" {
			if build != nil {
				t.Fatal("two builds have id chairlift-first-run")
			}
			build = &file.Builds[i]
		}
	}
	if build == nil {
		t.Fatal(".goreleaser.yaml has no chairlift-first-run build")
	}
	want := goreleaserBuild{
		ID:     "chairlift-first-run",
		Binary: "chairlift-first-run",
		Env:    []string{"CGO_ENABLED=0"},
		Goos:   []string{"linux"},
		Goarch: []string{"amd64", "arm64"},
		Flags:  []string{"-trimpath"},
		Main:   "./cmd/chairlift-first-run/main.go",
	}
	if !reflect.DeepEqual(*build, want) {
		t.Errorf("chairlift-first-run build = %+v, want %+v", *build, want)
	}

	cfg := loadGoreleaserConfig(t)
	full := nfpmByPackageName(t, cfg, fullPackageName)
	integration := nfpmByPackageName(t, cfg, integrationPackageName)
	if !containsString(full.IDs, "chairlift-first-run") {
		t.Errorf("%s ids = %v, want chairlift-first-run", fullPackageName, full.IDs)
	}
	if containsString(integration.IDs, "chairlift-first-run") {
		t.Errorf("%s ids = %v, must not ship chairlift-first-run", integrationPackageName, integration.IDs)
	}
}

func containsString(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

// TestNoAutostartEntryShips proves ChairLift starts nothing on its own: no
// package entry and no install line names an autostart file, and the
// repository holds no autostart or first-run .desktop file. The image that
// wants a first run ships the trigger (ADR-0014).
func TestNoAutostartEntryShips(t *testing.T) {
	cfg := loadGoreleaserConfig(t)
	if len(cfg.Nfpms) == 0 {
		t.Fatal(".goreleaser.yaml has no nfpms entries")
	}
	for i, nfpm := range cfg.Nfpms {
		for _, content := range nfpm.Contents {
			for _, path := range []string{content.Src, content.Dst} {
				if strings.Contains(strings.ToLower(path), "autostart") {
					t.Errorf("nfpms[%d] (%s) packages autostart path %q", i, nfpm.PackageName, path)
				}
			}
		}
	}

	makefile, err := os.ReadFile(filepath.Join(RepoRoot(), "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	for n, line := range strings.Split(string(makefile), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "install") && strings.Contains(strings.ToLower(trimmed), "autostart") {
			t.Errorf("Makefile line %d installs an autostart file: %s", n+1, trimmed)
		}
	}
	if !strings.Contains(string(makefile), "install -Dm755 $(BUILD_DIR)/$(FIRSTRUN_NAME) $(DESTDIR)$(BINDIR)/$(FIRSTRUN_NAME)") {
		t.Error("Makefile does not install chairlift-first-run to $(BINDIR)")
	}

	err = filepath.WalkDir(filepath.Join(RepoRoot(), "data"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := strings.ToLower(d.Name())
		if strings.Contains(name, "autostart") || strings.Contains(name, "firstrun") || strings.Contains(name, "first-run") {
			t.Errorf("data/ holds a first-run or autostart file: %s", path)
		}
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
}
