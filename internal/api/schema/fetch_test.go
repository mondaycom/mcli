package apischema

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mondaycom/mcli/internal/config"
)

// minimalIntrospectionResponse returns a minimal valid introspection response
// JSON string containing a Query type.
func minimalIntrospectionResponse() string {
	resp := introspectionResponse{}
	resp.Data.Schema = introspSchema{
		QueryType: &namedType{Name: "Query"},
		Types: []introspType{
			{
				Kind: "OBJECT",
				Name: "Query",
				Fields: []introspField{
					{
						Name: "hello",
						Type: introspTypeRef{Kind: "SCALAR", Name: "String"},
					},
				},
			},
		},
	}
	data, _ := json.Marshal(resp)
	return string(data)
}

// installFetchClient replaces FetchHTTPClient for the duration of the test.
func installFetchClient(t *testing.T, client *http.Client) {
	t.Helper()
	orig := FetchHTTPClient
	FetchHTTPClient = client
	t.Cleanup(func() { FetchHTTPClient = orig })
}

func TestFetchSchema_success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if av := r.Header.Get("API-Version"); av != "2026-07" {
			t.Errorf("API-Version = %q, want 2026-07", av)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(minimalIntrospectionResponse()))
	}))
	defer srv.Close()

	installFetchClient(t, srv.Client())

	sdl, err := FetchSchema(t.Context(), config.APIToken("test-token"), srv.URL, "2026-07")
	if err != nil {
		t.Fatalf("FetchSchema: %v", err)
	}
	if sdl == "" {
		t.Fatal("expected non-empty SDL")
	}
	if !strings.Contains(sdl, "type Query") {
		t.Errorf("SDL missing 'type Query': %s", sdl)
	}
}

func TestFetchSchema_httpError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("forbidden"))
	}))
	defer srv.Close()

	installFetchClient(t, srv.Client())

	_, err := FetchSchema(t.Context(), config.APIToken("bad-token"), srv.URL, "2026-07")
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error should mention HTTP status 403, got: %v", err)
	}
}
