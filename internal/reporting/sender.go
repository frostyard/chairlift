package reporting

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/frostyard/chairlift/internal/problems"
)

// maxAnswerBytes bounds how much of the intake API's answer ChairLift
// reads before it closes the connection.
const maxAnswerBytes = 4 << 10

// SendTimeout bounds 1 HTTP request, answer included.
const SendTimeout = 15 * time.Second

// Sender sends 1 report body. The interface exists so a test can count
// requests without a network.
type Sender interface {
	Send(ctx context.Context, body []byte) error
}

// StatusError is the answer of the intake API to a request it did not
// accept: 503 (kill switch), 429 (rate limit), 415, 413, 400, 502 or 500.
type StatusError struct {
	Code int
}

// Error returns the status code.
func (e *StatusError) Error() string {
	return fmt.Sprintf("reporting: the report endpoint answered %d", e.Code)
}

// HTTPSender posts a report to the intake endpoint.
type HTTPSender struct {
	Endpoint string
	Client   *http.Client
}

// NewHTTPSender returns a sender for endpoint with a 15-second timeout. A
// redirect is a failure, not a second request: the endpoint is fixed and a
// redirect would carry the report to another host.
func NewHTTPSender(endpoint string) *HTTPSender {
	return &HTTPSender{
		Endpoint: endpoint,
		Client: &http.Client{
			Timeout: SendTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// Send posts body to the endpoint. It refuses a body of MaxBodyBytes or
// more, and a kind crash report, before any network call. A 2xx answer is
// success; any other answer is a *StatusError, and a transport failure is a
// wrapped error.
func (s *HTTPSender) Send(ctx context.Context, body []byte) error {
	if len(body) >= MaxBodyBytes {
		return fmt.Errorf("reporting: report of %d bytes is not under %d", len(body), MaxBodyBytes)
	}
	kind, err := KindOf(body)
	if err != nil {
		return err
	}
	if kind == problems.KindCrash {
		return ErrLocalOnly
	}
	endpoint := s.Endpoint
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("reporting: build the report request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	// Go sends no User-Agent header when the request sets it to "".
	req.Header.Set("User-Agent", "")
	client := s.Client
	if client == nil {
		client = NewHTTPSender(endpoint).Client
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("reporting: send the report: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxAnswerBytes))
		_ = resp.Body.Close()
	}()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return &StatusError{Code: resp.StatusCode}
}

// IsStatusError reports whether err is a *StatusError.
func IsStatusError(err error) bool {
	var status *StatusError
	return errors.As(err, &status)
}
