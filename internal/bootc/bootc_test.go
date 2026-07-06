package bootc

import "testing"

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
