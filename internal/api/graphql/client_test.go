package graphql

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/mondaycom/mcli/internal/api/gen"
	"github.com/mondaycom/mcli/internal/config"
	"github.com/mondaycom/mcli/internal/errs"
)

// meResponse returns the JSON body for a successful Me query.
func meSuccessBody() string {
	return `{"data":{"me":{"id":"12345","name":"Test User"}}}`
}

// complexityErrorBody returns a 200-with-errors body for COMPLEXITY_BUDGET_EXHAUSTED.
func complexityErrorBody(resetIn int) string {
	body := map[string]interface{}{
		"data": nil,
		"errors": []map[string]interface{}{
			{
				"message": "Complexity budget exhausted",
				"extensions": map[string]interface{}{
					"code":     "COMPLEXITY_BUDGET_EXHAUSTED",
					"reset_in": float64(resetIn),
				},
			},
		},
	}
	b, _ := json.Marshal(body)
	return string(b)
}

// TestClient_Me_Success verifies that a successful Me query returns a typed result.
func TestClient_Me_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(meSuccessBody()))
	}))
	defer srv.Close()

	c := New(config.APIToken("test-token"), "test", WithEndpoint(srv.URL))
	resp, err := gen.Me(context.Background(), c.GQL())
	if err != nil {
		t.Fatalf("Me returned error: %v", err)
	}
	if resp.Me.Id != "12345" {
		t.Errorf("Me.Id = %q; want %q", resp.Me.Id, "12345")
	}
	if resp.Me.Name != "Test User" {
		t.Errorf("Me.Name = %q; want %q", resp.Me.Name, "Test User")
	}
}

// TestClient_RetryOnComplexity verifies that a COMPLEXITY_BUDGET_EXHAUSTED error
// triggers a retry and the second (success) response is returned.
// reset_in=1 is used so the test completes in about 1 second rather than the
// default 5-second exponential fallback.
func TestClient_RetryOnComplexity(t *testing.T) {
	var callCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if n == 1 {
			_, _ = w.Write([]byte(complexityErrorBody(1))) // reset_in=1s keeps test fast
		} else {
			_, _ = w.Write([]byte(meSuccessBody()))
		}
	}))
	defer srv.Close()

	c := New(config.APIToken("test-token"), "test", WithEndpoint(srv.URL))
	resp, err := gen.Me(context.Background(), c.GQL())
	if err != nil {
		t.Fatalf("Me returned error after retry: %v", err)
	}
	if resp.Me.Id != "12345" {
		t.Errorf("Me.Id = %q; want %q", resp.Me.Id, "12345")
	}
	if callCount.Load() != 2 {
		t.Errorf("server received %d calls; want 2", callCount.Load())
	}
}

// TestClient_ComplexityExhausted_MaxRetries verifies that after maxComplexityRetry
// the error is normalized to CodeRateLimited.
// reset_in=1 keeps the inter-retry delay short (1s × retries).
func TestClient_ComplexityExhausted_MaxRetries(t *testing.T) {
	var callCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(complexityErrorBody(1))) // reset_in=1s keeps test fast
	}))
	defer srv.Close()

	c := New(config.APIToken("test-token"), "test", WithEndpoint(srv.URL))
	_, err := gen.Me(context.Background(), c.GQL())
	if err == nil {
		t.Fatal("expected error; got nil")
	}

	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("error type = %T; want *errs.Error", err)
	}
	if e.Code != errs.CodeRateLimited {
		t.Errorf("Code = %v; want %v", e.Code, errs.CodeRateLimited)
	}

	// Should have been called 1 + maxComplexityRetry times.
	want := int32(1 + maxComplexityRetry)
	if callCount.Load() != want {
		t.Errorf("server received %d calls; want %d", callCount.Load(), want)
	}
}

// TestClient_AuthError verifies HTTP 401 is normalized to CodeAuth.
func TestClient_AuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"errors":[{"message":"Unauthorized"}]}`))
	}))
	defer srv.Close()

	c := New(config.APIToken("bad-token"), "test", WithEndpoint(srv.URL))
	_, err := gen.Me(context.Background(), c.GQL())
	if err == nil {
		t.Fatal("expected error; got nil")
	}

	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("error type = %T; want *errs.Error", err)
	}
	if e.Code != errs.CodeAuth {
		t.Errorf("Code = %v; want %v", e.Code, errs.CodeAuth)
	}
}

// TestLastComplexity verifies that LastComplexity returns data after a
// COMPLEXITY_BUDGET_EXHAUSTED response.
// reset_in=1 keeps the inter-retry delay short.
func TestLastComplexity(t *testing.T) {
	var callCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if n == 1 {
			_, _ = w.Write([]byte(complexityErrorBody(1))) // reset_in=1s keeps test fast
		} else {
			_, _ = w.Write([]byte(meSuccessBody()))
		}
	}))
	defer srv.Close()

	c := New(config.APIToken("test-token"), "test", WithEndpoint(srv.URL))
	_, err := gen.Me(context.Background(), c.GQL())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ci := c.LastComplexity()
	if ci.ResetAt.IsZero() {
		t.Error("LastComplexity.ResetAt is zero; want non-zero after complexity response")
	}
}
