package errs_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/mondaycom/mcli/internal/errs"
)

func TestToExitCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		wantCode int
	}{
		{"nil", nil, 0},
		{"usage", errs.Usage("bad flag"), 1},
		{"api", errs.API("api error"), 2},
		{"not_found", errs.NotFound("missing"), 2},
		{"auth", errs.Auth("no token"), 3},
		{"rate_limited", errs.RateLimited("429"), 4},
		{"unknown non-errs error", fmt.Errorf("something else"), 5},
		{"wrapped usage", fmt.Errorf("wrap: %w", errs.Usage("bad")), 1},
		{"wrapped auth", fmt.Errorf("wrap: %w", errs.Auth("no token")), 3},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := errs.ToExitCode(tc.err)
			if got != tc.wantCode {
				t.Errorf("ToExitCode(%v) = %d, want %d", tc.err, got, tc.wantCode)
			}
		})
	}
}

func TestErrorMessage(t *testing.T) {
	t.Parallel()

	e := errs.Usage("bad input %s", "x")
	if e.Message != "bad input x" {
		t.Errorf("unexpected message: %q", e.Message)
	}
	if e.Code != errs.CodeUsage {
		t.Errorf("unexpected code: %q", e.Code)
	}
}

func TestErrorWithCause(t *testing.T) {
	t.Parallel()

	cause := errors.New("underlying")
	e := &errs.Error{Code: errs.CodeInternal, Message: "boom", Cause: cause}
	if !errors.Is(e, cause) {
		t.Error("errors.Is should find the cause through Unwrap")
	}
	if e.Error() == "" {
		t.Error("Error() must not be empty")
	}
}

func TestErrorsAs(t *testing.T) {
	t.Parallel()

	wrapped := fmt.Errorf("outer: %w", errs.Auth("token missing"))
	var e *errs.Error
	if !errors.As(wrapped, &e) {
		t.Fatal("errors.As should unwrap to *errs.Error")
	}
	if e.Code != errs.CodeAuth {
		t.Errorf("expected AUTH code, got %q", e.Code)
	}
}

func TestAllConstructors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		e    *errs.Error
		code errs.Code
	}{
		{errs.Usage("u"), errs.CodeUsage},
		{errs.Auth("a"), errs.CodeAuth},
		{errs.API("ap"), errs.CodeAPI},
		{errs.RateLimited("r"), errs.CodeRateLimited},
		{errs.NotFound("n"), errs.CodeNotFound},
	}
	for _, tc := range cases {
		t.Run(string(tc.code), func(t *testing.T) {
			t.Parallel()
			if tc.e.Code != tc.code {
				t.Errorf("constructor for %q produced code %q", tc.code, tc.e.Code)
			}
		})
	}
}
