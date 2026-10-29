package updex

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"

	"github.com/frostyard/updex/v2/imagecompat"
)

// checkBootcRunner is the bootc runner the read-only check side of ChairLift
// gives the updex client. The update check runs as the ordinary, unprivileged
// user, and it only reads: it decides what to show on the Features page, it
// changes nothing. bootc does not agree: "bootc status --json" needs root,
// and on a bootc host it fails for that user with "This command must be
// executed as the root user".
//
// A failure there is not a missing bootc, so updex's minimum image version
// gate (ADR-0016) would report an error on every component of a feature whose
// staged image sets UPDEX_MIN_IMAGE_VERSION, and ChairLift would drop those
// components and lose the "waiting for a newer OS image" state entirely.
//
// checkBootcRunner therefore turns a bootc failure that is not a cancellation
// and not "bootc is not installed" and not root into exec.ErrNotFound. That is
// exactly the answer imagecompat.NextBootImageVersion falls back on: it
// compares the staged image with the IMAGE_VERSION of the booted OS image
// from os-release. The comparison stays read-only, and it still answers the
// question the row shows.
//
// The real gate for linking stays where it was: the root helper
// (cmd/chairlift-updex-helper) builds its client with updex.ClientConfig{}
// and the real bootc, so an unprivileged client can never make the privileged
// link step fail open. Root keeps its own fail-closed answer.
//
// Known limit: when bootc has staged a newer OS deployment, the booted
// os-release is older than the image that boots next, so a row can say
// "waiting for a newer OS image" although the next update would link. The
// reboot into that OS clears it.
type checkBootcRunner struct {
	inner imagecompat.BootcRunner
	euid  func() int
}

var _ imagecompat.BootcRunner = checkBootcRunner{}

// newCheckBootcRunner returns the runner the check client uses: the real
// bootc, and the process's own effective user ID to decide whether root could
// read bootc anyway.
func newCheckBootcRunner() checkBootcRunner {
	return checkBootcRunner{inner: imagecompat.DefaultBootcRunner{}, euid: os.Geteuid}
}

// Status runs "bootc status --json" through the inner runner. Every answer but
// an unprivileged failure passes through unchanged: a success, a bootc that is
// not installed (already exec.ErrNotFound), a cancelled context (the caller
// asked to stop, which is not an unprivileged read), and root (whose bootc
// answer is the real one, failures included, so the check stays fail-closed
// where it can).
//
// Any other failure from an unprivileged process is reported as
// exec.ErrNotFound, with one log line and the original error text kept, so
// imagecompat.NextBootImageVersion falls back to the booted os-release image
// version instead of failing the whole component check.
func (r checkBootcRunner) Status(ctx context.Context) ([]byte, error) {
	out, err := r.inner.Status(ctx)
	switch {
	case err == nil:
		return out, nil
	case errors.Is(err, exec.ErrNotFound):
		return out, err
	case ctx.Err() != nil:
		return out, err
	case r.euid() == 0:
		return out, err
	}

	log.Printf("Feature update check: bootc status is not readable without root, comparing the staged image with the booted OS image instead: %v", err)
	return nil, fmt.Errorf("bootc status is not readable without root (%v): %w", err, exec.ErrNotFound)
}
