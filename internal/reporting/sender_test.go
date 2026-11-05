package reporting

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/frostyard/chairlift/internal/problems"
)

func TestParseEndpoint(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{name: "empty is the default", in: "", want: DefaultEndpoint},
		{name: "https", in: "https://report.example/v1/report", want: "https://report.example/v1/report"},
		{name: "http", in: "http://127.0.0.1:8080/v1/report", want: "http://127.0.0.1:8080/v1/report"},
		{name: "no scheme", in: "report.example/v1/report", wantErr: true},
		{name: "ftp", in: "ftp://report.example/v1/report", wantErr: true},
		{name: "no host", in: "https:///v1/report", wantErr: true},
		{name: "user information", in: "https://ada:secret@report.example/v1/report", wantErr: true},
		{name: "fragment", in: "https://report.example/v1/report#top", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseEndpoint(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseEndpoint(%q) = %q, want an error", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseEndpoint(%q): %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("ParseEndpoint(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// reportServer is an httptest server that counts the requests it answers and
// keeps the last body, content type and user agent.
type reportServer struct {
	*httptest.Server
	hits        atomic.Int32
	badKinds    atomic.Int32
	mu          sync.Mutex
	bodies      []string
	contentType atomic.Value
	userAgent   atomic.Value
}

func newReportServer(t *testing.T, status int) *reportServer {
	t.Helper()
	s := &reportServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.hits.Add(1)
		body, _ := readLimited(r)
		if kind, err := KindOf(body); err == nil && kind == "crash" {
			s.badKinds.Add(1)
		}
		s.mu.Lock()
		s.bodies = append(s.bodies, string(body))
		s.mu.Unlock()
		s.contentType.Store(r.Header.Get("Content-Type"))
		if value, ok := r.Header["User-Agent"]; ok {
			s.userAgent.Store(strings.Join(value, ","))
		} else {
			s.userAgent.Store("<absent>")
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"status":"accepted"}`))
	}))
	t.Cleanup(s.Close)
	return s
}

// received returns every body the server answered.
func (s *reportServer) received() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.bodies...)
}

// last returns the newest body the server answered.
func (s *reportServer) last() string {
	bodies := s.received()
	if len(bodies) == 0 {
		return ""
	}
	return bodies[len(bodies)-1]
}

// readLimited reads a request body for the test server.
func readLimited(r *http.Request) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r.Body, 1<<16))
}

// problemsCrashPayload returns the v1 body of a crash item: a document the
// schema accepts, which the sender still refuses to send.
func problemsCrashPayload(t *testing.T) ([]byte, error) {
	t.Helper()
	return problems.BuildPayload(crashItem(), "3f2a1b4c-5d6e-4f70-8a9b-0c1d2e3f4a5b", "chairlift/0.14.0")
}

// TestHTTPSenderPostsTheExactBody pins the request: the configured endpoint,
// the report's bytes, Content-Type application/json and no User-Agent.
func TestHTTPSenderPostsTheExactBody(t *testing.T) {
	srv := newReportServer(t, http.StatusAccepted)
	item, ok := kindItem("unit_failed")
	if !ok {
		t.Fatal("no unit_failed item")
	}
	r, err := ForItem(item, "chairlift/0.14.0")
	if err != nil {
		t.Fatalf("ForItem: %v", err)
	}
	if err := NewHTTPSender(srv.URL).Send(context.Background(), r.Body); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got := srv.last(); got != r.Text() {
		t.Errorf("the server received %v, want the report text", got)
	}
	if got := srv.contentType.Load(); got != "application/json" {
		t.Errorf("Content-Type = %v, want application/json", got)
	}
	if got := srv.userAgent.Load(); got != "<absent>" {
		t.Errorf("User-Agent = %v, want no header", got)
	}
	if srv.hits.Load() != 1 {
		t.Errorf("the server answered %d requests, want 1", srv.hits.Load())
	}
}

// TestHTTPSenderRedirectIsAFailure proves a redirect is not followed: the
// report must never reach a second host.
func TestHTTPSenderRedirectIsAFailure(t *testing.T) {
	var followed atomic.Int32
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		followed.Add(1)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer second.Close()
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, second.URL, http.StatusTemporaryRedirect)
	}))
	defer first.Close()

	item, _ := kindItem("unit_failed")
	r, err := ForItem(item, "chairlift/0.14.0")
	if err != nil {
		t.Fatal(err)
	}
	err = NewHTTPSender(first.URL).Send(context.Background(), r.Body)
	if err == nil {
		t.Fatal("Send followed a redirect, want a failure")
	}
	if !IsStatusError(err) {
		t.Errorf("Send error = %v, want a status error", err)
	}
	if followed.Load() != 0 {
		t.Errorf("the redirect target answered %d requests, want 0", followed.Load())
	}
}

// TestHTTPSenderRefusesBeforeTheNetwork proves the kind and size guards run
// before any request.
func TestHTTPSenderRefusesBeforeTheNetwork(t *testing.T) {
	srv := newReportServer(t, http.StatusAccepted)
	crash, err := problemsCrashPayload(t)
	if err != nil {
		t.Fatal(err)
	}
	if err := NewHTTPSender(srv.URL).Send(context.Background(), crash); err != ErrLocalOnly {
		t.Errorf("Send(crash) = %v, want ErrLocalOnly", err)
	}
	big := []byte(`{"kind":"count","pad":"` + strings.Repeat("x", MaxBodyBytes) + `"}`)
	if err := NewHTTPSender(srv.URL).Send(context.Background(), big); err == nil {
		t.Error("Send(8192 bytes) = nil, want a failure")
	}
	if srv.hits.Load() != 0 {
		t.Errorf("the server answered %d requests, want 0", srv.hits.Load())
	}
}

// TestHTTPSenderStatusCodes proves every non-2xx answer is a StatusError
// that carries the code.
func TestHTTPSenderStatusCodes(t *testing.T) {
	for _, code := range []int{http.StatusServiceUnavailable, http.StatusTooManyRequests,
		http.StatusUnsupportedMediaType, http.StatusRequestEntityTooLarge,
		http.StatusBadRequest, http.StatusBadGateway, http.StatusInternalServerError} {
		srv := newReportServer(t, code)
		item, _ := kindItem("unit_failed")
		r, err := ForItem(item, "chairlift/0.14.0")
		if err != nil {
			t.Fatal(err)
		}
		err = NewHTTPSender(srv.URL).Send(context.Background(), r.Body)
		if !IsStatusError(err) {
			t.Fatalf("Send() = %v for %d, want a status error", err, code)
		}
		var status *StatusError
		if !errors.As(err, &status) || status.Code != code {
			t.Errorf("Send() status = %v, want %d", err, code)
		}
		if srv.hits.Load() != 1 {
			t.Errorf("the server answered %d requests for %d, want 1", srv.hits.Load(), code)
		}
		srv.Close()
	}
}

// TestHTTPSenderWithoutAnEndpointSendsNothing proves an empty endpoint makes
// no request: only ParseEndpoint("") chooses the default, so an invalid
// administrator setting can never fall back to the public service.
func TestHTTPSenderWithoutAnEndpointSendsNothing(t *testing.T) {
	item, _ := kindItem("unit_failed")
	r, err := ForItem(item, "chairlift/0.14.0")
	if err != nil {
		t.Fatal(err)
	}
	for name, sender := range map[string]*HTTPSender{
		"empty struct": {},
		"new empty":    NewHTTPSender(""),
	} {
		t.Run(name, func(t *testing.T) {
			if err := sender.Send(context.Background(), r.Body); !errors.Is(err, ErrNoEndpoint) {
				t.Fatalf("Send() = %v, want ErrNoEndpoint", err)
			}
		})
	}
}
