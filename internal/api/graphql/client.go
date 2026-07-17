package graphql

import (
	"context"
	"math"
	"time"

	gqlclient "github.com/Khan/genqlient/graphql"
	"github.com/vektah/gqlparser/v2/gqlerror"

	"github.com/mondaycom/mcli/internal/config"
	"github.com/mondaycom/mcli/internal/errs"
	"github.com/mondaycom/mcli/internal/httpx"
)

const (
	defaultEndpoint    = "https://api.monday.com/v2"
	maxComplexityRetry = 2
	complexityBaseWait = 5 * time.Second
)

// Option configures a Client.
type Option func(*Client)

// WithEndpoint overrides the default GraphQL endpoint URL.
// Primarily used in tests to point at an httptest.Server.
func WithEndpoint(url string) Option {
	return func(c *Client) {
		c.endpoint = url
	}
}

// WithAPIVersion overrides the monday.com API version header sent with each request.
func WithAPIVersion(v string) Option {
	return func(c *Client) {
		c.apiVersion = v
	}
}

// WithRoutingKey sets the baggage: routingKey=<v> header for local-api-proxy debugging.
func WithRoutingKey(v string) Option {
	return func(c *Client) {
		c.routingKey = v
	}
}

// Client is a typed GraphQL client for the monday.com API.
// It wraps an authenticated httpx.Client and a genqlient graphql.Client,
// adding error normalisation and complexity-budget retry on top.
type Client struct {
	endpoint   string
	apiVersion string
	routingKey string
	inner      gqlclient.Client
	complexity complexityStore
}

// New constructs a Client from an API token and a version string.
// opts may be used to override defaults (e.g. endpoint for testing).
func New(token config.APIToken, version string, opts ...Option) *Client {
	c := &Client{
		endpoint:   defaultEndpoint,
		apiVersion: config.ResolveAPIVersion(config.Config{}),
	}
	for _, o := range opts {
		o(c)
	}

	httpClient := httpx.NewClient(token, version, c.apiVersion, c.routingKey)
	c.inner = gqlclient.NewClient(c.endpoint, httpClient)
	return c
}

// GQL returns the underlying genqlient graphql.Client.
// The returned client is a wrapper that normalises errors and retries on
// COMPLEXITY_BUDGET_EXHAUSTED before returning to the caller.
func (c *Client) GQL() gqlclient.Client {
	return &normalisingClient{parent: c}
}

// LastComplexity returns the most recently observed complexity budget data.
// Fields are zero when the server has not yet returned any budget information.
func (c *Client) LastComplexity() ComplexityInfo {
	return c.complexity.get()
}

// normalisingClient wraps the inner genqlient client, performing error
// normalisation and complexity-budget retry.
type normalisingClient struct {
	parent *Client
}

// MakeRequest implements graphql.Client. It delegates to the inner client,
// retrying up to maxComplexityRetry times when a COMPLEXITY_BUDGET_EXHAUSTED
// GraphQL error is returned (200-with-errors), then normalises any remaining
// error.
func (n *normalisingClient) MakeRequest(
	ctx context.Context,
	req *gqlclient.Request,
	resp *gqlclient.Response,
) error {
	// Save the original Data pointer so we can restore it before each retry.
	// genqlient pre-populates resp.Data with a typed struct pointer; a
	// 200-with-errors response that contains "data":null will overwrite it to nil
	// during JSON decoding, so we must reset it for subsequent attempts.
	origData := resp.Data

	var lastErr error
	for attempt := range maxComplexityRetry + 1 {
		if attempt > 0 {
			wait := complexityWait(lastErr, attempt)
			select {
			case <-ctx.Done():
				return errs.Interrupted("operation cancelled")
			case <-time.After(wait):
			}
		}

		// Restore Data and clear Errors from any prior attempt so that the JSON
		// decoder starts from a clean state on each try.
		resp.Data = origData
		resp.Errors = nil

		err := n.parent.inner.MakeRequest(ctx, req, resp)
		if err == nil {
			return nil
		}
		lastErr = err

		list, exhausted := isComplexityExhausted(err)
		if !exhausted {
			break
		}

		// Update complexity info from the error extensions.
		resetIn := complexityResetIn(list)
		if resetIn > 0 {
			n.parent.complexity.set(ComplexityInfo{
				ResetAt: time.Now().Add(time.Duration(resetIn) * time.Second),
			})
		}
	}

	return Normalize(lastErr)
}

// complexityWait computes the delay before the next complexity retry.
// It uses reset_in from the error if available, otherwise exponential backoff.
func complexityWait(err error, attempt int) time.Duration {
	if list, ok := isComplexityExhausted(err); ok {
		if ri := complexityResetIn(list); ri > 0 {
			return time.Duration(ri) * time.Second
		}
	}
	exp := math.Pow(2, float64(attempt-1))
	return time.Duration(float64(complexityBaseWait) * exp)
}

// Ensure gqlerror.List implements error for compile-time check.
var _ error = gqlerror.List{}
