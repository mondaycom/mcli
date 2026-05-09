package httpx_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/mondaycom/mcli/internal/config"
	"github.com/mondaycom/mcli/internal/httpx"
)

// opaqueReadCloser wraps an io.Reader so http.NewRequest cannot detect a
// known body type and therefore will not auto-populate req.GetBody. This is
// what triggers the buffer-once fallback path in RetryTransport.
type opaqueReadCloser struct{ r io.Reader }

func (o *opaqueReadCloser) Read(p []byte) (int, error) { return o.r.Read(p) }
func (o *opaqueReadCloser) Close() error               { return nil }

const testToken = "my-secret-token"
const testVersion = "1.2.3"

// newTestClient returns a client whose base transport is redirected to the
// given test server, bypassing TLS and default-transport pooling.
func newTestClient(srv *httptest.Server) *http.Client {
	c := httpx.NewClient(config.APIToken(testToken), testVersion)
	// Replace default transport with the test server's client transport so
	// requests are routed to the test server.
	c.Transport.(*httpx.RetryTransport).Base = srv.Client().Transport
	return c
}

func TestAuthorizationHeader(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != testToken {
			t.Errorf("Authorization header = %q, want %q", auth, testToken)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	resp, err := c.Get(srv.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	_ = resp.Body.Close()
}

func TestUserAgentHeader(t *testing.T) {
	t.Parallel()

	wantUA := "mcli/" + testVersion
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua := r.Header.Get("User-Agent")
		if ua != wantUA {
			t.Errorf("User-Agent = %q, want %q", ua, wantUA)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	resp, err := c.Get(srv.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	_ = resp.Body.Close()
}

func TestRetry_429TwiceThenSuccess(t *testing.T) {
	t.Parallel()

	var callCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		if n <= 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	resp, err := c.Get(srv.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("final status = %d, want 200", resp.StatusCode)
	}
	if n := callCount.Load(); n != 3 {
		t.Errorf("call count = %d, want 3", n)
	}
}

func TestRetry_Persistent5xx_GivesUp(t *testing.T) {
	t.Parallel()

	var callCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	resp, err := c.Get(srv.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("final status = %d, want 500", resp.StatusCode)
	}
	// maxRetries is 3, so total attempts = 4 (initial + 3 retries).
	if n := callCount.Load(); n != 4 {
		t.Errorf("call count = %d, want 4", n)
	}
}

func TestRetry_PostBodyPreservedAcrossRetries(t *testing.T) {
	t.Parallel()

	const payload = `{"query":"{ boards { id } }"}`
	const failCount = 2 // first two requests respond 503, third responds 200

	var (
		callCount      atomic.Int32
		receivedBodies []string
		mu             sync.Mutex
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body on attempt %d: %v", callCount.Load(), err)
		}

		mu.Lock()
		receivedBodies = append(receivedBodies, string(body))
		mu.Unlock()

		n := callCount.Add(1)
		if n <= failCount {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	resp, err := c.Post(srv.URL, "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("final status = %d, want 200", resp.StatusCode)
	}

	wantCalls := int32(failCount + 1)
	if n := callCount.Load(); n != wantCalls {
		t.Errorf("call count = %d, want %d", n, wantCalls)
	}

	mu.Lock()
	defer mu.Unlock()
	for i, got := range receivedBodies {
		if got != payload {
			t.Errorf("attempt %d body = %q, want %q", i+1, got, payload)
		}
	}

	echoed, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if !bytes.Equal(echoed, []byte(payload)) {
		t.Errorf("echoed body = %q, want %q", echoed, payload)
	}
}

// TestRetry_PostBodyPreserved_FallbackBuffersWhenGetBodyNil exercises the
// path where req.Body is set but req.GetBody is nil — the transport must
// drain the body once and install GetBody so retries see the same payload.
func TestRetry_PostBodyPreserved_FallbackBuffersWhenGetBodyNil(t *testing.T) {
	t.Parallel()

	const payload = `{"query":"{ me { id } }"}`
	const failCount = 2

	var (
		callCount      atomic.Int32
		receivedBodies []string
		mu             sync.Mutex
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		mu.Lock()
		receivedBodies = append(receivedBodies, string(body))
		mu.Unlock()

		n := callCount.Add(1)
		if n <= failCount {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	req, err := http.NewRequest(http.MethodPost, srv.URL, &opaqueReadCloser{r: strings.NewReader(payload)})
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if req.GetBody != nil {
		t.Fatalf("precondition: expected GetBody to be nil for opaque body, got non-nil")
	}

	c := newTestClient(srv)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("final status = %d, want 200", resp.StatusCode)
	}

	wantCalls := int32(failCount + 1)
	if n := callCount.Load(); n != wantCalls {
		t.Errorf("call count = %d, want %d", n, wantCalls)
	}

	mu.Lock()
	defer mu.Unlock()
	for i, got := range receivedBodies {
		if got != payload {
			t.Errorf("attempt %d body = %q, want %q", i+1, got, payload)
		}
	}
}
