package bootc

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// nonBootcJSON is real output captured from `bootc status --format json`
// on a non-bootc (non-bootc-booted) host.
const nonBootcJSON = `{"apiVersion":"org.containers.bootc/v1","kind":"BootcHost","metadata":{"name":"host"},"spec":{"bootOrder":"default","image":null},"status":{"booted":null,"rollback":null,"rollbackQueued":false,"staged":null,"type":null,"usrOverlay":null}}`

// bootedStagedJSON follows the org.containers.bootc/v1 schema for a booted
// host with a staged update (constructed; validate on a bootc VM post-merge).
const bootedStagedJSON = `{
  "apiVersion": "org.containers.bootc/v1",
  "kind": "BootcHost",
  "metadata": {"name": "host"},
  "spec": {
    "bootOrder": "default",
    "image": {"image": "ghcr.io/frostyard/snow:stable", "transport": "containers-storage"}
  },
  "status": {
    "booted": {
      "image": {
        "image": {"image": "ghcr.io/frostyard/snow:stable", "transport": "containers-storage"},
        "version": "20260701.0",
        "timestamp": "2026-07-01T10:00:00Z",
        "imageDigest": "sha256:aaaa1111bbbb2222cccc3333dddd4444eeee5555ffff6666aaaa7777bbbb8888"
      },
      "cachedUpdate": null,
      "incompatible": false,
      "pinned": false,
      "store": "ostreeContainer"
    },
    "staged": {
      "image": {
        "image": {"image": "ghcr.io/frostyard/snow:stable", "transport": "containers-storage"},
        "version": "20260706.0",
        "timestamp": "2026-07-06T09:00:00Z",
        "imageDigest": "sha256:9999aaaa0000bbbb1111cccc2222dddd3333eeee4444ffff5555aaaa6666bbbb"
      },
      "cachedUpdate": null,
      "incompatible": false,
      "pinned": false,
      "store": "ostreeContainer"
    },
    "rollback": null,
    "rollbackQueued": false,
    "type": "bootcHost"
  }
}`

func TestParseStatusNonBootcHost(t *testing.T) {
	s, err := parseStatus([]byte(nonBootcJSON))
	if err != nil {
		t.Fatalf("parseStatus: %v", err)
	}
	if s.Status.Booted != nil {
		t.Errorf("Booted = %+v, want nil", s.Status.Booted)
	}
	if s.Status.Staged != nil {
		t.Errorf("Staged = %+v, want nil", s.Status.Staged)
	}
	if s.Booted() {
		t.Error("Booted() = true, want false")
	}
}

func TestParseStatusBootedWithStaged(t *testing.T) {
	s, err := parseStatus([]byte(bootedStagedJSON))
	if err != nil {
		t.Fatalf("parseStatus: %v", err)
	}
	if !s.Booted() {
		t.Fatal("Booted() = false, want true")
	}
	if got, want := s.Status.Booted.ImageRef(), "ghcr.io/frostyard/snow:stable"; got != want {
		t.Errorf("booted ImageRef = %q, want %q", got, want)
	}
	if got, want := s.Status.Booted.Version(), "20260701.0"; got != want {
		t.Errorf("booted Version = %q, want %q", got, want)
	}
	if s.Status.Staged == nil {
		t.Fatal("Staged = nil, want non-nil")
	}
	if got, want := s.Status.Staged.Version(), "20260706.0"; got != want {
		t.Errorf("staged Version = %q, want %q", got, want)
	}
	if got := s.Status.Staged.Digest(); got == "" {
		t.Error("staged Digest = empty, want sha256:...")
	}
}

func TestDeploymentNilSafe(t *testing.T) {
	var d *Deployment
	if d.ImageRef() != "" || d.Version() != "" || d.Timestamp() != "" || d.Digest() != "" {
		t.Error("nil Deployment accessors must return empty strings")
	}
}

func TestParseStatusMalformed(t *testing.T) {
	if _, err := parseStatus([]byte("not json")); err == nil {
		t.Error("parseStatus(garbage) = nil error, want error")
	}
}

// All getStatusFrom tests drive a fake #!/bin/sh script through the same seam
// GetStatus uses in production, so none of them needs a real bootc binary.

func TestGetStatusFromSuccess(t *testing.T) {
	script := writeScript(t, "cat <<'JSON'\n"+bootedStagedJSON+"\nJSON\n")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	s, err := getStatusFrom(ctx, script)
	if err != nil {
		t.Fatalf("getStatusFrom: %v", err)
	}
	if !s.Booted() {
		t.Errorf("Booted() = false, want true")
	}
	if got := s.Status.Staged.Version(); got != "20260706.0" {
		t.Errorf("staged version = %q, want 20260706.0", got)
	}
}

func TestGetStatusFromCommandFailure(t *testing.T) {
	script := writeScript(t, "echo \"cannot read status\" >&2\nexit 3\n")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	s, err := getStatusFrom(ctx, script)
	if err == nil {
		t.Fatalf("getStatusFrom = %+v, nil error; want failure", s)
	}
	var bootcErr *Error
	if !errors.As(err, &bootcErr) {
		t.Fatalf("errors.As(*Error) = false; err = %T %v", err, err)
	}
	if !strings.Contains(bootcErr.Message, "exit 3") {
		t.Errorf("message %q missing exit code", bootcErr.Message)
	}
	if !strings.Contains(bootcErr.Message, "cannot read status") {
		t.Errorf("message %q missing stderr text", bootcErr.Message)
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		t.Errorf("plain exit failure matches a context sentinel: %v", err)
	}
}

func TestGetStatusFromMissingExecutable(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-such-bootc")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	_, err := getStatusFrom(ctx, missing)
	var notFound *NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("errors.As(*NotFoundError) = false; err = %T %v", err, err)
	}
}

func TestGetStatusFromDeadline(t *testing.T) {
	// `exec sleep` replaces the shell, so the kill leaves no stray process.
	script := writeScript(t, "exec sleep 30\n")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	t.Cleanup(cancel)

	_, err := getStatusFrom(ctx, script)
	if err == nil {
		t.Fatal("getStatusFrom = nil error, want deadline failure")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("errors.Is(err, DeadlineExceeded) = false; err = %v", err)
	}
	if errors.Is(err, context.Canceled) {
		t.Errorf("deadline error also matches context.Canceled: %v", err)
	}
	if strings.Contains(err.Error(), "signal: killed") {
		t.Errorf("message leaks raw kill signal: %q", err.Error())
	}
}

func TestGetStatusFromCanceled(t *testing.T) {
	script := writeScript(t, "exec sleep 30\n")

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	done := make(chan error, 1)
	go func() {
		_, err := getStatusFrom(ctx, script)
		done <- err
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()

	var err error
	select {
	case err = <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("getStatusFrom did not return")
	}
	if err == nil {
		t.Fatal("getStatusFrom = nil error, want cancellation failure")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("errors.Is(err, Canceled) = false; err = %v", err)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("cancellation error also matches context.DeadlineExceeded: %v", err)
	}
	if strings.Contains(err.Error(), "signal: killed") {
		t.Errorf("message leaks raw kill signal: %q", err.Error())
	}
}
