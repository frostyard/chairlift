package installcheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// goreleaserBuilds is the builds: subset of .goreleaser.yaml.
type goreleaserBuilds struct {
	Builds []struct {
		ID      string   `yaml:"id"`
		Main    string   `yaml:"main"`
		Ldflags []string `yaml:"ldflags"`
	} `yaml:"builds"`
}

// TestChairLiftRecordsItsProblemsWithTheReleaseVersion pins how ChairLift
// stamps the version of its own problem records (ADR-0015): GoReleaser sets
// main.buildVersion from the tag for the chairlift build; main copies it to
// version.Version and only then passes version.Version to selfreport.Init,
// which hands it to std's problems.New for the GUI component; main defers
// the recorder's RecoverPanic directly.
func TestChairLiftRecordsItsProblemsWithTheReleaseVersion(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(RepoRoot(), ".goreleaser.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg goreleaserBuilds
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, build := range cfg.Builds {
		if build.ID != "chairlift" {
			continue
		}
		found = true
		if build.Main != "./cmd/chairlift/main.go" {
			t.Errorf("chairlift build main = %q", build.Main)
		}
		flags := strings.Join(build.Ldflags, " ")
		if !strings.Contains(flags, "-X main.buildVersion={{ .Version }}") {
			t.Errorf("chairlift ldflags %q do not stamp main.buildVersion with {{ .Version }}", flags)
		}
	}
	if !found {
		t.Fatal(".goreleaser.yaml has no chairlift build")
	}

	mainSource, err := os.ReadFile(filepath.Join(RepoRoot(), "cmd", "chairlift", "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(mainSource)
	order := []string{
		"version.Version = buildVersion",
		"recorder := selfreport.Init(version.Version)",
		"\tdefer recorder.RecoverPanic()\n",
		"application := app.New()",
	}
	last := -1
	for _, fragment := range order {
		at := strings.Index(text, fragment)
		if at < 0 {
			t.Fatalf("cmd/chairlift/main.go does not contain %q", fragment)
		}
		if at <= last {
			t.Errorf("cmd/chairlift/main.go: %q comes too early; want the order %q", fragment, order)
		}
		last = at
	}
	if strings.Count(text, "selfreport.Init(") != 1 {
		t.Error("cmd/chairlift/main.go must call selfreport.Init exactly once")
	}

	selfSource, err := os.ReadFile(filepath.Join(RepoRoot(), "internal", "selfreport", "selfreport.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{
		`const AppID = "org.frostyard.ChairLift"`,
		"stdproblems.New(AppID, version, stdproblems.ComponentGUI, opts...)",
		"rec.CrashOutput()",
	} {
		if !strings.Contains(string(selfSource), fragment) {
			t.Errorf("internal/selfreport/selfreport.go does not contain %q", fragment)
		}
	}
}
