package selfreport

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"

	stdproblems "github.com/frostyard/std/problems"
)

var codePattern = regexp.MustCompile(`^[a-z][a-z0-9_.]{0,63}$`)

func TestCodesAreValidAndUnique(t *testing.T) {
	if len(Codes) < 3 || len(Codes) > 5 {
		t.Fatalf("ChairLift has %d codes, want 3 to 5", len(Codes))
	}
	seen := map[string]bool{}
	for _, code := range Codes {
		if !codePattern.MatchString(code) {
			t.Errorf("code %q fails the v1 code pattern", code)
		}
		if seen[code] {
			t.Errorf("code %q is listed twice", code)
		}
		seen[code] = true
	}
}

func TestDevVersionMakesTheRecorderInert(t *testing.T) {
	for _, version := range []string{"dev", "abc1234-snapshot", "v0.14.0", ""} {
		rec := Init(version)
		if rec.Err() == nil {
			t.Errorf("Init(%q) gave a recorder that writes", version)
		}
		if Recorder() != rec {
			t.Error("Recorder() is not the recorder of the last Init")
		}
		Error(CodeConfigLoadFailed, errors.New("no write")) // must not panic
	}
}

// TestErrorWritesARecord sends a coded error to a fake journal socket and
// checks its fields: ChairLift's app ID, the release version, component
// gui, the code, and the error text as the local MESSAGE.
func TestErrorWritesARecord(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STATE_DIRECTORY", "")
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))
	socket := filepath.Join(dir, "journal.sock")
	fd, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_DGRAM|syscall.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatalf("socket: %v", err)
	}
	defer func() { _ = syscall.Close(fd) }()
	if err := syscall.Bind(fd, &syscall.SockaddrUnix{Name: socket}); err != nil {
		t.Fatalf("bind %s: %v", socket, err)
	}
	timeout := syscall.NsecToTimeval((5 * time.Second).Nanoseconds())
	if err := syscall.SetsockoptTimeval(fd, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, &timeout); err != nil {
		t.Fatal(err)
	}

	rec := Init("0.14.0", stdproblems.WithSocket(socket))
	if rec.Err() != nil {
		t.Fatalf("Init(0.14.0) is inert: %v", rec.Err())
	}
	crashDir := filepath.Join(dir, "state", "frostyard-problems", AppID)
	if entries, err := os.ReadDir(crashDir); err != nil || len(entries) == 0 {
		t.Errorf("Init did not call CrashOutput: %v, %d files in %s", err, len(entries), crashDir)
	}

	Error(CodeProblemsSourceInvalid, errors.New("problems.json: format is not 1"))
	buf := make([]byte, 65536)
	n, _, err := syscall.Recvfrom(fd, buf, 0)
	if err != nil {
		t.Fatalf("no record reached the socket: %v", err)
	}
	fields, err := stdproblems.DecodeEntry(buf[:n])
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"MESSAGE_ID":            stdproblems.MessageID,
		"FROSTYARD_APP_ID":      "org.frostyard.ChairLift",
		"FROSTYARD_APP_VERSION": "0.14.0",
		"FROSTYARD_COMPONENT":   "gui",
		"FROSTYARD_CLASS":       "error",
		"FROSTYARD_CODE":        "problems.source_invalid",
		"MESSAGE":               "problems.json: format is not 1",
	}
	for k, v := range want {
		if fields[k] != v {
			t.Errorf("%s = %q, want %q", k, fields[k], v)
		}
	}
	if _, err := stdproblems.Validate(fields); err != nil {
		t.Errorf("the record fails std's validation: %v", err)
	}
}

// codeSites names, for each code constant, the files and the exact call
// that records it at its real error path.
var codeSites = map[string][]struct{ file, call string }{
	"CodeConfigLoadFailed": {
		{"internal/window/window.go", "selfreport.Error(selfreport.CodeConfigLoadFailed, configErr)"},
		{"internal/window/firstrun.go", "selfreport.Error(selfreport.CodeConfigLoadFailed, cfgErr)"},
	},
	"CodeProblemsSourceInvalid": {
		{"internal/views/problems_page.go", "selfreport.Error(selfreport.CodeProblemsSourceInvalid, err)"},
	},
	"CodeFirstRunCompleteFailed": {
		{"internal/window/firstrun.go", "selfreport.Error(selfreport.CodeFirstRunCompleteFailed, decision.Err)"},
	},
	"CodeUpdexResultsUnreadable": {
		{"internal/updex/updex.go", "selfreport.Error(selfreport.CodeUpdexResultsUnreadable, unreadable)"},
	},
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Dir(filepath.Dir(wd))
}

// TestCodesAreRecordedAtTheirPaths reads the source: each code constant is
// used at its error path, every code constant has a site, no other
// production file spells a code's text, and no file outside the listed
// sites records a code.
func TestCodesAreRecordedAtTheirPaths(t *testing.T) {
	root := repoRoot(t)
	source, err := os.ReadFile("selfreport.go")
	if err != nil {
		t.Fatal(err)
	}
	constants := regexp.MustCompile(`(?m)^\t(Code[A-Za-z]+) += "`).FindAllStringSubmatch(string(source), -1)
	if len(constants) != len(Codes) || len(codeSites) != len(Codes) {
		t.Fatalf("%d code constants, %d listed codes, %d code sites; want all equal", len(constants), len(Codes), len(codeSites))
	}
	sites := map[string]bool{}
	for _, c := range constants {
		list, ok := codeSites[c[1]]
		if !ok {
			t.Errorf("code constant %s has no site", c[1])
		}
		for _, site := range list {
			data, err := os.ReadFile(filepath.Join(root, site.file))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(data), site.call) {
				t.Errorf("%s does not record %s: missing %q", site.file, c[1], site.call)
			}
			sites[site.file] = true
		}
	}

	var goFiles []string
	for _, top := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, top), func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && (strings.HasPrefix(d.Name(), ".") || d.Name() == "testdata") {
				return filepath.SkipDir
			}
			if !d.IsDir() && strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
				goFiles = append(goFiles, path)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range goFiles {
		rel, _ := filepath.Rel(root, path)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		if rel != "internal/selfreport/selfreport.go" {
			for _, code := range Codes {
				if strings.Contains(text, `"`+code+`"`) {
					t.Errorf("%s spells the code %q; use the selfreport constant", rel, code)
				}
			}
		}
		if strings.Contains(text, "selfreport.Error(") && !sites[rel] {
			t.Errorf("%s records a code but is not a listed site", rel)
		}
	}
}
