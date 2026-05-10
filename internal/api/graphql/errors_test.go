package graphql

import (
	"errors"
	"net/http"
	"testing"

	gqlclient "github.com/Khan/genqlient/graphql"
	"github.com/vektah/gqlparser/v2/gqlerror"

	"github.com/mondaycom/mcli/internal/errs"
)

// makeGQLError builds a gqlerror.Error with the given message and optional
// extensions.code value.
func makeGQLError(msg, code string) *gqlerror.Error {
	e := &gqlerror.Error{Message: msg}
	if code != "" {
		e.Extensions = map[string]any{"code": code}
	}
	return e
}

func TestNormalize_Nil(t *testing.T) {
	if got := Normalize(nil); got != nil {
		t.Fatalf("Normalize(nil) = %v; want nil", got)
	}
}

func TestNormalize_GraphQLError_MapsToAPI(t *testing.T) {
	list := gqlerror.List{makeGQLError("something went wrong", "")}
	err := Normalize(list)

	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("Normalize(gqlerror.List) = %T; want *errs.Error", err)
	}
	if e.Code != errs.CodeAPI {
		t.Errorf("Code = %v; want %v", e.Code, errs.CodeAPI)
	}
	if e.Message == "" {
		t.Error("Message is empty")
	}
}

func TestNormalize_ComplexityExhausted_MapsToRateLimited(t *testing.T) {
	list := gqlerror.List{
		makeGQLError("Complexity limit exceeded", "COMPLEXITY_BUDGET_EXHAUSTED"),
	}
	err := Normalize(list)

	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("Normalize(complexity) = %T; want *errs.Error", err)
	}
	if e.Code != errs.CodeRateLimited {
		t.Errorf("Code = %v; want %v", e.Code, errs.CodeRateLimited)
	}
}

func TestNormalize_AuthError_MapsToAuth(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "Unauthorized extension code",
			err:  gqlerror.List{makeGQLError("Unauthorized", "Unauthorized")},
		},
		{
			name: "AuthenticationException extension code",
			err:  gqlerror.List{makeGQLError("not authenticated", "AuthenticationException")},
		},
		{
			name: "HTTP 401",
			err: &gqlclient.HTTPError{
				StatusCode: http.StatusUnauthorized,
				Response:   gqlclient.Response{},
			},
		},
		{
			name: "HTTP 403",
			err: &gqlclient.HTTPError{
				StatusCode: http.StatusForbidden,
				Response:   gqlclient.Response{},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := Normalize(tc.err)
			var e *errs.Error
			if !errors.As(err, &e) {
				t.Fatalf("Normalize(%v) = %T; want *errs.Error", tc.err, err)
			}
			if e.Code != errs.CodeAuth {
				t.Errorf("Code = %v; want %v", e.Code, errs.CodeAuth)
			}
		})
	}
}

func TestNormalize_HTTPError_NonAuth_MapsToAPI(t *testing.T) {
	httpErr := &gqlclient.HTTPError{
		StatusCode: http.StatusInternalServerError,
		Response:   gqlclient.Response{},
	}
	err := Normalize(httpErr)
	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("Normalize(HTTP 500) = %T; want *errs.Error", err)
	}
	if e.Code != errs.CodeAPI {
		t.Errorf("Code = %v; want %v", e.Code, errs.CodeAPI)
	}
}

func TestNormalize_ComplexityPriorityOverOtherErrors(t *testing.T) {
	// Mix of a regular error and a complexity error — complexity wins.
	list := gqlerror.List{
		makeGQLError("other error", ""),
		makeGQLError("budget exhausted", "COMPLEXITY_BUDGET_EXHAUSTED"),
	}
	err := Normalize(list)
	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("type = %T; want *errs.Error", err)
	}
	if e.Code != errs.CodeRateLimited {
		t.Errorf("Code = %v; want %v", e.Code, errs.CodeRateLimited)
	}
}

func TestNormalize_UnknownError_WrappedTransparently(t *testing.T) {
	sentinel := errors.New("some transport error")
	wrapped := Normalize(sentinel)
	if !errors.Is(wrapped, sentinel) {
		t.Errorf("Normalize(sentinel): errors.Is = false; want true")
	}
}

func TestComplexityResetIn(t *testing.T) {
	tests := []struct {
		name string
		list gqlerror.List
		want int
	}{
		{
			name: "no complexity error",
			list: gqlerror.List{makeGQLError("other", "")},
			want: 0,
		},
		{
			name: "complexity with reset_in",
			list: gqlerror.List{
				{
					Message: "exhausted",
					Extensions: map[string]any{
						"code":     "COMPLEXITY_BUDGET_EXHAUSTED",
						"reset_in": float64(30),
					},
				},
			},
			want: 30,
		},
		{
			name: "complexity without reset_in",
			list: gqlerror.List{
				makeGQLError("exhausted", "COMPLEXITY_BUDGET_EXHAUSTED"),
			},
			want: 0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := complexityResetIn(tc.list); got != tc.want {
				t.Errorf("complexityResetIn = %d; want %d", got, tc.want)
			}
		})
	}
}
