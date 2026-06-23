// Package httpx provides an authenticated HTTP client for the monday.com API
// with retry logic for 429 and 5xx responses.
package httpx

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/mondaycom/mcli/internal/config"
)

const (
	maxRetries    = 3
	baseBackoff   = 100 * time.Millisecond
	backoffFactor = 4.0
	maxRetryAfter = 60 * time.Second
)

// RetryTransport is an http.RoundTripper that injects auth headers and retries
// on 429 and 5xx responses with exponential backoff.
// The Base field is exported to allow test code to substitute a test-server
// transport without re-implementing construction.
type RetryTransport struct {
	// Base is the underlying transport. Defaults to http.DefaultTransport.
	Base       http.RoundTripper
	token      config.APIToken
	userAgent  string
	apiVersion string
	routingKey string
}

// RoundTrip implements http.RoundTripper.
func (t *RetryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", string(t.token))
	req.Header.Set("User-Agent", t.userAgent)
	req.Header.Set("API-Version", t.apiVersion)
	if t.routingKey != "" {
		req.Header.Set("baggage", "routingKey="+t.routingKey)
	}

	// Ensure the request body can be replayed across retries.
	// If GetBody is not set but a body is present, buffer it once and install
	// GetBody so subsequent attempts get a fresh reader.
	if req.Body != nil && req.GetBody == nil {
		buf, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, fmt.Errorf("http round-trip: buffer request body: %w", err)
		}
		_ = req.Body.Close()
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(buf)), nil
		}
		req.Body, _ = req.GetBody()
	}

	var (
		resp *http.Response
		err  error
	)
	for attempt := range maxRetries + 1 {
		if attempt > 0 {
			delay := t.backoffDelay(resp, attempt)
			select {
			case <-req.Context().Done():
				return nil, req.Context().Err()
			case <-time.After(delay):
			}
			// Must close the previous response body before retrying.
			// Ignoring the close error here: this is intermediate cleanup
			// and the retry response supersedes it.
			if resp != nil {
				_ = resp.Body.Close()
			}

			// Restore a fresh body for the next attempt.
			if req.GetBody != nil {
				req.Body, err = req.GetBody()
				if err != nil {
					return nil, fmt.Errorf("http round-trip: restore request body: %w", err)
				}
			}
		}

		resp, err = t.Base.RoundTrip(req)
		if err != nil {
			return nil, fmt.Errorf("http round-trip: %w", err)
		}
		if !shouldRetry(resp.StatusCode) {
			return resp, nil
		}
	}
	return resp, nil
}

// backoffDelay computes the delay before the next retry.
// On 429 it honours the Retry-After header (up to maxRetryAfter).
func (t *RetryTransport) backoffDelay(resp *http.Response, attempt int) time.Duration {
	if resp != nil && resp.StatusCode == http.StatusTooManyRequests {
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if secs, err := strconv.Atoi(ra); err == nil {
				return min(time.Duration(secs)*time.Second, maxRetryAfter)
			}
		}
	}
	exp := math.Pow(backoffFactor, float64(attempt-1))
	return time.Duration(float64(baseBackoff) * exp)
}

// shouldRetry reports whether the status code warrants a retry.
func shouldRetry(code int) bool {
	return code == http.StatusTooManyRequests || (code >= 500 && code < 600)
}

// NewClient constructs an *http.Client that:
//   - injects Authorization: <token> (raw, no "Bearer" prefix per Monday docs),
//   - injects User-Agent: mcli/<version>,
//   - injects API-Version: <apiVersion>,
//   - injects baggage: routingKey=<routingKey> when routingKey is non-empty,
//   - retries on 429 and 5xx with exponential backoff (max 3 retries).
func NewClient(token config.APIToken, version string, apiVersion string, routingKey string) *http.Client {
	return &http.Client{
		Transport: &RetryTransport{
			Base:       http.DefaultTransport,
			token:      token,
			userAgent:  "mcli/" + version,
			apiVersion: apiVersion,
			routingKey: routingKey,
		},
	}
}
