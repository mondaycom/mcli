package httpx_test

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/mondaycom/mcli/internal/config"
	"github.com/mondaycom/mcli/internal/httpx"
)

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
