package reporting

import (
	"context"
	"errors"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/frostyard/chairlift/internal/problems"
)

// sendPaceForTest is the production pace of the GUI and the command line.
const sendPaceForTest = 7 * time.Second

// TestLocalIntakeAcceptsEveryBody sends 1 body of each sendable kind and 1 count
// through the real HTTPSender to a local problem-intake Worker (`npx wrangler
// dev --local` in frostyard/problem-intake). It runs only when
// CHAIRLIFT_INTAKE_URL names a loopback endpoint, such as
// http://127.0.0.1:8787/v1/report; without it the test skips (CI has none).
//
// The local Worker has no GlitchTip secrets, so a forwarded kind answers 502
// (upstream) after it passed validation; a count is never forwarded and
// answers 202. A 400, 413 or 415 means ChairLift's body broke the contract.
func TestLocalIntakeAcceptsEveryBody(t *testing.T) {
	endpoint := os.Getenv("CHAIRLIFT_INTAKE_URL")
	if endpoint == "" {
		t.Skip("CHAIRLIFT_INTAKE_URL is not set")
	}
	u, err := url.Parse(endpoint)
	if err != nil || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost" && u.Hostname() != "::1") {
		t.Fatalf("CHAIRLIFT_INTAKE_URL %q is not a loopback endpoint; this test never sends to a live intake", endpoint)
	}
	sender := NewHTTPSender(endpoint)
	// 8 requests at the production pace stay under the Worker's 10 a minute.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	for _, kind := range []string{
		problems.KindUnitFailed, problems.KindUpdateStageFailed, problems.KindBootedOlderImage,
		problems.KindSysextUpdateFailed, problems.KindTPMUnlock, problems.KindIntegrityError,
		problems.KindAppProblem,
	} {
		it, ok := kindItem(kind)
		if !ok {
			t.Fatalf("no test item for kind %s", kind)
		}
		r, err := ForItem(it, "chairlift/0.15.0")
		if err != nil {
			t.Fatalf("%s: ForItem: %v", kind, err)
		}
		err = sender.Send(ctx, r.Body)
		time.Sleep(sendPaceForTest)
		var status *StatusError
		upstream := errors.As(err, &status) && status.Code == 502
		if err != nil && !upstream {
			t.Errorf("%s: the local intake refused the body: %v\n%s", kind, err, r.Text())
		}
	}

	r, err := NewCountReport(validEnvelope(), "chairlift/0.15.0", "<6m", "latest", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := sender.Send(ctx, r.Body); err != nil {
		t.Errorf("count: %v\n%s", err, r.Text())
	}
}
