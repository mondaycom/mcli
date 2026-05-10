// Package graphql provides a typed GraphQL client for the monday.com API.
package graphql

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	gqlclient "github.com/Khan/genqlient/graphql"
	"github.com/vektah/gqlparser/v2/gqlerror"

	"github.com/mondaycom/mcli/internal/errs"
)

// Normalize translates a raw genqlient or HTTP error into an *errs.Error.
// It inspects:
//   - gqlerror.List: a 200-with-errors GraphQL response
//   - *gqlclient.HTTPError: a non-200 HTTP response
//   - nil: returned as-is
//
// Mapping rules (first match wins per error in the list):
//   - extensions.code == "COMPLEXITY_BUDGET_EXHAUSTED" → errs.CodeRateLimited
//   - extensions.code == "Unauthorized" or "AuthenticationException" → errs.CodeAuth
//   - HTTP 401 / 403 → errs.CodeAuth
//   - all other GraphQL errors → errs.CodeAPI
//   - all other transport errors → wrapped with %w (caller decides)
func Normalize(err error) error {
	if err == nil {
		return nil
	}

	// Check for a non-200 HTTP error first.
	if httpErr, ok := errors.AsType[*gqlclient.HTTPError](err); ok {
		if httpErr.StatusCode == http.StatusUnauthorized || httpErr.StatusCode == http.StatusForbidden {
			return errs.Auth("authentication failed: HTTP %d", httpErr.StatusCode)
		}
		// Try to extract GraphQL errors from the HTTP error body.
		if len(httpErr.Response.Errors) > 0 {
			return normalizeGQLErrors(httpErr.Response.Errors)
		}
		return errs.API("API request failed: HTTP %d", httpErr.StatusCode)
	}

	// Check for a GraphQL error list (200-with-errors).
	if gqlList, ok := errors.AsType[gqlerror.List](err); ok {
		return normalizeGQLErrors(gqlList)
	}

	// Wrap unknown errors transparently.
	return fmt.Errorf("graphql: %w", err)
}

// normalizeGQLErrors converts a gqlerror.List to the most specific *errs.Error.
// If the list contains a COMPLEXITY_BUDGET_EXHAUSTED error, that takes priority.
// Auth errors take second priority. Everything else becomes CodeAPI.
func normalizeGQLErrors(list gqlerror.List) *errs.Error {
	var msgs []string
	hasComplexity := false
	hasAuth := false

	for _, e := range list {
		msgs = append(msgs, e.Message)
		code := extensionCode(e)
		switch code {
		case "COMPLEXITY_BUDGET_EXHAUSTED":
			hasComplexity = true
		case "Unauthorized", "AuthenticationException":
			hasAuth = true
		}
	}

	combined := strings.Join(msgs, "; ")

	switch {
	case hasComplexity:
		return errs.RateLimited("complexity budget exhausted: %s", combined)
	case hasAuth:
		return errs.Auth("authentication failed: %s", combined)
	default:
		return errs.API("GraphQL error: %s", combined)
	}
}

// extensionCode extracts the string "code" from a gqlerror.Error's Extensions,
// returning "" if absent or not a string.
func extensionCode(e *gqlerror.Error) string {
	if e == nil || e.Extensions == nil {
		return ""
	}
	v, ok := e.Extensions["code"]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

// complexityResetIn extracts the reset_in value (seconds) from the first
// COMPLEXITY_BUDGET_EXHAUSTED error in a gqlerror.List.
// Returns 0 if not present.
func complexityResetIn(list gqlerror.List) int {
	for _, e := range list {
		if extensionCode(e) != "COMPLEXITY_BUDGET_EXHAUSTED" {
			continue
		}
		if e.Extensions == nil {
			return 0
		}
		v, ok := e.Extensions["reset_in"]
		if !ok {
			return 0
		}
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		}
	}
	return 0
}

// isComplexityExhausted reports whether err is (or wraps) a
// COMPLEXITY_BUDGET_EXHAUSTED GraphQL error.
func isComplexityExhausted(err error) (gqlerror.List, bool) {
	if err == nil {
		return nil, false
	}
	list, ok := errors.AsType[gqlerror.List](err)
	if !ok {
		return nil, false
	}
	for _, e := range list {
		if extensionCode(e) == "COMPLEXITY_BUDGET_EXHAUSTED" {
			return list, true
		}
	}
	return nil, false
}
