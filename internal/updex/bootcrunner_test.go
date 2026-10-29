package updex

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/frostyard/updex/v2/imagecompat"
)

// fakeBootcStatus is a stand-in for imagecompat.DefaultBootcRunner: it returns
// a fixed answer without running bootc.
type fakeBootcStatus struct {
	out []byte
	err error
}

func (f fakeBootcStatus) Status(context.Context) ([]byte, error) {
	return f.out, f.err
}

// errBootcFailed is the error a bootc 1.17.1 on Snow Linux gives an
// unprivileged caller: the command exists, and it refuses to run without root.
var errBootcFailed = errors.New("bootc status --json: exit status 1: This command must be executed as the root user")

func euidFunc(uid int) func() int {
	return func() int { return uid }
}

// TestCheckBootcRunnerPassesThroughOrHidesTheUnprivilegedFailure is the
// decision table of checkBootcRunner.Status: exactly one row turns a failure
// into exec.ErrNotFound, and it is the unprivileged, non-cancelled one.
func TestCheckBootcRunnerPassesThroughOrHidesTheUnprivilegedFailure(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name string
		// ctx is the context handed to Status; nil means context.Background().
		ctx   context.Context
		euid  int
		inner fakeBootcStatus
		// wantErrNotFound is true when Status must report exec.ErrNotFound.
		wantErrNotFound bool
		// wantOriginalErr is true when Status must report the inner error
		// itself, untouched.
		wantOriginalErr bool
	}{
		{
			name: "success passes through",
			euid: 1000,
			inner: fakeBootcStatus{
				out: []byte(`{"status":{}}`),
			},
		},
		{
			name: "success passes through for root too",
			euid: 0,
			inner: fakeBootcStatus{
				out: []byte(`{"status":{}}`),
			},
		},
		{
			name: "inner exec.ErrNotFound passes through",
			euid: 1000,
			inner: fakeBootcStatus{
				err: exec.ErrNotFound,
			},
			wantOriginalErr: true,
		},
		{
			name:            "unprivileged failure becomes exec.ErrNotFound",
			euid:            1000,
			inner:           fakeBootcStatus{err: errBootcFailed},
			wantErrNotFound: true,
		},
		{
			name:            "root keeps the original failure",
			euid:            0,
			inner:           fakeBootcStatus{err: errBootcFailed},
			wantOriginalErr: true,
		},
		{
			name:            "a cancelled context keeps the original failure",
			ctx:             cancelled,
			euid:            1000,
			inner:           fakeBootcStatus{err: context.Canceled},
			wantOriginalErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.ctx
			if ctx == nil {
				ctx = context.Background()
			}

			runner := checkBootcRunner{inner: tt.inner, euid: euidFunc(tt.euid)}
			out, err := runner.Status(ctx)

			switch {
			case tt.wantErrNotFound:
				if !errors.Is(err, exec.ErrNotFound) {
					t.Fatalf("Status error = %v, want it to wrap exec.ErrNotFound", err)
				}
				if out != nil {
					t.Errorf("Status output = %q, want nil: the fallback must not read a partial answer", out)
				}
				if !strings.Contains(err.Error(), errBootcFailed.Error()) {
					t.Errorf("Status error = %v, want it to keep the inner error text %q", err, errBootcFailed.Error())
				}
			case tt.wantOriginalErr:
				if errors.Is(err, exec.ErrNotFound) && !errors.Is(tt.inner.err, exec.ErrNotFound) {
					t.Fatalf("Status error = %v, want the inner error unchanged, not exec.ErrNotFound", err)
				}
				if !errors.Is(err, tt.inner.err) {
					t.Errorf("Status error = %v, want the inner error %v", err, tt.inner.err)
				}
			default:
				if err != nil {
					t.Fatalf("Status error = %v, want nil", err)
				}
				if string(out) != string(tt.inner.out) {
					t.Errorf("Status output = %q, want %q", out, tt.inner.out)
				}
			}
		})
	}
}

// TestCheckBootcRunnerFallsBackToTheBootedImageVersion drives the real public
// imagecompat.NextBootImageVersion with this runner, which is the whole point
// of it: an unprivileged bootc failure must end in the booted os-release
// answer, and root must still see the failure.
func TestCheckBootcRunnerFallsBackToTheBootedImageVersion(t *testing.T) {
	osRelease := filepath.Join(t.TempDir(), "os-release")
	if err := os.WriteFile(osRelease, []byte("IMAGE_VERSION=20261001000000\n"), 0o644); err != nil {
		t.Fatalf("writing %s: %v", osRelease, err)
	}
	paths := []string{osRelease}

	t.Run("unprivileged failure gives the booted image version", func(t *testing.T) {
		runner := checkBootcRunner{inner: fakeBootcStatus{err: errBootcFailed}, euid: euidFunc(1000)}

		version, source, err := imagecompat.NextBootImageVersion(context.Background(), imagecompat.Options{
			Runner:         runner,
			OSReleasePaths: paths,
		})
		if err != nil {
			t.Fatalf("NextBootImageVersion error = %v, want nil", err)
		}
		if version != "20261001000000" {
			t.Errorf("NextBootImageVersion version = %q, want %q", version, "20261001000000")
		}
		if source != imagecompat.SourceBootedOSRelease {
			t.Errorf("NextBootImageVersion source = %q, want %q", source, imagecompat.SourceBootedOSRelease)
		}
	})

	t.Run("root keeps the failure", func(t *testing.T) {
		runner := checkBootcRunner{inner: fakeBootcStatus{err: errBootcFailed}, euid: euidFunc(0)}

		version, source, err := imagecompat.NextBootImageVersion(context.Background(), imagecompat.Options{
			Runner:         runner,
			OSReleasePaths: paths,
		})
		if err == nil {
			t.Fatalf("NextBootImageVersion = (%q, %q, nil), want an error: root's bootc failure must fail closed", version, source)
		}
		if !errors.Is(err, errBootcFailed) {
			t.Errorf("NextBootImageVersion error = %v, want the inner bootc error", err)
		}
	})
}

func TestNewCheckBootcRunnerUsesTheRealBootcAndTheProcessID(t *testing.T) {
	runner := newCheckBootcRunner()

	if _, ok := runner.inner.(imagecompat.DefaultBootcRunner); !ok {
		t.Errorf("newCheckBootcRunner().inner = %T, want imagecompat.DefaultBootcRunner", runner.inner)
	}
	if runner.euid == nil {
		t.Fatal("newCheckBootcRunner().euid = nil, want os.Geteuid")
	}
	if got, want := runner.euid(), os.Geteuid(); got != want {
		t.Errorf("newCheckBootcRunner().euid() = %d, want os.Geteuid() = %d", got, want)
	}
}

// TestCheckClientUsesTheUnprivilegedBootcRunnerOnly pins which client gets the
// fallback: the read-only check client in internal/updex, and never the root
// helper, which keeps the real bootc and fails closed.
func TestCheckClientUsesTheUnprivilegedBootcRunnerOnly(t *testing.T) {
	cases := []struct {
		name     string
		path     string
		required []string
		forbid   []string
	}{
		{
			name:     "the check client passes the runner",
			path:     filepath.Join("internal", "updex", "updex.go"),
			required: []string{"BootcRunner: newCheckBootcRunner()"},
		},
		{
			name:   "the root helper keeps the real bootc",
			path:   filepath.Join("cmd", "chairlift-updex-helper", "main.go"),
			forbid: []string{"newCheckBootcRunner", "BootcRunner"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source, err := os.ReadFile(filepath.Join(repoRoot(t), tc.path))
			if err != nil {
				t.Fatalf("read %s: %v", tc.path, err)
			}
			text := string(source)

			for _, fragment := range tc.required {
				if !strings.Contains(text, fragment) {
					t.Errorf("%s does not contain %q", tc.path, fragment)
				}
			}
			for _, fragment := range tc.forbid {
				if strings.Contains(text, fragment) {
					t.Errorf("%s contains %q, which only the unprivileged check client may use", tc.path, fragment)
				}
			}
		})
	}
}

// repoRoot returns the repository root, found from this test file's own path.
func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}
