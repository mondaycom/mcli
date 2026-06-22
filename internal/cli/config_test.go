package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	apischema "github.com/mondaycom/mcli/internal/api/schema"
	"github.com/mondaycom/mcli/internal/config"
	"github.com/mondaycom/mcli/internal/errs"
)

// runConfigCmd executes a config sub-command in an isolated temp dir and
// returns the captured stdout output.
func runConfigCmd(t *testing.T, cfgPath string, args ...string) (string, error) {
	t.Helper()
	origGlobals := globals
	t.Cleanup(func() { globals = origGlobals })
	globals.Config = cfgPath

	root := &cobra.Command{Use: "mcli", SilenceErrors: true, SilenceUsage: true}
	root.AddCommand(newConfigCmd())

	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs(args)

	err := root.Execute()
	return buf.String(), err
}

func TestConfigSetGet_OutputModeRoundTrip(t *testing.T) {
	for _, mode := range []string{"json", "pretty", "terse", "csv"} {
		mode := mode
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			cfgPath := filepath.Join(dir, "config.yaml")

			if _, err := runConfigCmd(t, cfgPath, "config", "set", "output-mode", mode); err != nil {
				t.Fatalf("set output-mode %s: %v", mode, err)
			}

			out, err := runConfigCmd(t, cfgPath, "config", "get", "output-mode")
			if err != nil {
				t.Fatalf("get output-mode after set %s: %v", mode, err)
			}
			got := strings.TrimSpace(out)
			if got != mode {
				t.Errorf("round-trip output-mode: got %q, want %q", got, mode)
			}
		})
	}
}

func TestConfigSet_UnsetOutputMode(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	if _, err := runConfigCmd(t, cfgPath, "config", "set", "output-mode", "json"); err != nil {
		t.Fatalf("set output-mode json: %v", err)
	}
	if _, err := runConfigCmd(t, cfgPath, "config", "set", "output-mode", ""); err != nil {
		t.Fatalf("unset output-mode: %v", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.OutputMode != "" {
		t.Errorf("output-mode should be empty after unset, got %q", cfg.OutputMode)
	}
}

func TestConfigSet_DefaultAliasClearsMode(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	if _, err := runConfigCmd(t, cfgPath, "config", "set", "output-mode", "terse"); err != nil {
		t.Fatalf("set output-mode terse: %v", err)
	}
	if _, err := runConfigCmd(t, cfgPath, "config", "set", "output-mode", "default"); err != nil {
		t.Fatalf("set output-mode default: %v", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.OutputMode != "" {
		t.Errorf("'default' should clear output-mode, got %q", cfg.OutputMode)
	}
}

func TestConfigSet_InvalidOutputMode(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	if _, err := runConfigCmd(t, cfgPath, "config", "set", "output-mode", "bogus"); err == nil {
		t.Fatal("expected error for invalid output-mode value, got nil")
	}
}

func TestConfigSet_UnknownKey(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	if _, err := runConfigCmd(t, cfgPath, "config", "set", "no-such-key", "value"); err == nil {
		t.Fatal("expected error for unknown key, got nil")
	}
}

func TestConfigGet_UnknownKey(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	if _, err := runConfigCmd(t, cfgPath, "config", "get", "no-such-key"); err == nil {
		t.Fatal("expected error for unknown key, got nil")
	}
}

func TestConfigSet_PreservesExistingFields(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	// Prime the config with a secret_store value.
	if err := config.Save(cfgPath, config.Config{SecretStore: config.BackendKeychain}); err != nil {
		t.Fatalf("save initial config: %v", err)
	}

	if _, err := runConfigCmd(t, cfgPath, "config", "set", "output-mode", "terse"); err != nil {
		t.Fatalf("set output-mode: %v", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.SecretStore != config.BackendKeychain {
		t.Errorf("SecretStore was overwritten: got %q", cfg.SecretStore)
	}
	if cfg.OutputMode != "terse" {
		t.Errorf("OutputMode not set: got %q", cfg.OutputMode)
	}
}

// ---- api-version tests ----

// minimalIntrospectionJSONForCLI returns a valid minimal introspection response.
func minimalIntrospectionJSONForCLI() string {
	type namedType struct {
		Name string `json:"name"`
	}
	type typeRef struct {
		Kind string `json:"kind"`
		Name string `json:"name"`
	}
	type field struct {
		Name string  `json:"name"`
		Type typeRef `json:"type"`
	}
	type iType struct {
		Kind   string  `json:"kind"`
		Name   string  `json:"name"`
		Fields []field `json:"fields"`
	}
	type schema struct {
		QueryType *namedType `json:"queryType"`
		Types     []iType    `json:"types"`
	}
	type data struct {
		Schema schema `json:"__schema"`
	}
	type resp struct {
		Data data `json:"data"`
	}
	r := resp{
		Data: data{
			Schema: schema{
				QueryType: &namedType{Name: "Query"},
				Types: []iType{
					{
						Kind: "OBJECT",
						Name: "Query",
						Fields: []field{
							{Name: "hello", Type: typeRef{Kind: "SCALAR", Name: "String"}},
						},
					},
				},
			},
		},
	}
	b, _ := json.Marshal(r)
	return string(b)
}

// TestConfigSetAPIVersion_invalidFormat verifies that a bad format returns a usage error.
// Not parallel — mutates globals.
func TestConfigSetAPIVersion_invalidFormat(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	origGlobals := globals
	t.Cleanup(func() { globals = origGlobals })
	globals.Config = cfgPath
	globals.Token = "fake-token"

	_, err := runConfigCmd(t, cfgPath, "config", "set", "api-version", "2026-7")
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
	e, ok := err.(*errs.Error)
	if !ok || e.Code != errs.CodeUsage {
		t.Errorf("expected CodeUsage error, got %T: %v", err, err)
	}
}

// TestConfigSetAPIVersion_fetchError verifies that a failed schema fetch reverts config.
// Not parallel — mutates globals and apischema state.
func TestConfigSetAPIVersion_fetchError(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	origGlobals := globals
	t.Cleanup(func() { globals = origGlobals })
	globals.Config = cfgPath
	globals.Token = "fake-token"

	// Reset schema cache to use our temp dir.
	apischema.SetConfigDir(dir)
	t.Cleanup(func() { apischema.SetConfigDir("") })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	origFetch := apischema.FetchHTTPClient
	apischema.FetchHTTPClient = srv.Client()
	t.Cleanup(func() { apischema.FetchHTTPClient = origFetch })

	// Point the endpoint at the test server via env.
	t.Setenv("MONDAY_API_URL", srv.URL)

	_, err := runConfigCmd(t, cfgPath, "config", "set", "api-version", "2026-08")
	if err == nil {
		t.Fatal("expected error when fetch returns 403")
	}
	e, ok := err.(*errs.Error)
	if !ok || e.Code != errs.CodeUsage {
		t.Errorf("expected CodeUsage error, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "reverted") {
		t.Errorf("expected 'reverted' in error, got: %v", err)
	}

	// Config should have been reverted.
	cfg, loadErr := config.Load(cfgPath)
	if loadErr != nil {
		t.Fatalf("load config: %v", loadErr)
	}
	if cfg.APIVersion != "" {
		t.Errorf("expected APIVersion reverted to empty, got %q", cfg.APIVersion)
	}
}

// TestConfigSetAPIVersion_success verifies a successful fetch saves version and schema.
// Not parallel — mutates globals and apischema state.
func TestConfigSetAPIVersion_success(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	origGlobals := globals
	t.Cleanup(func() { globals = origGlobals })
	globals.Config = cfgPath
	globals.Token = "fake-token"

	// Reset schema cache to use our temp dir.
	apischema.SetConfigDir(dir)
	t.Cleanup(func() { apischema.SetConfigDir("") })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, minimalIntrospectionJSONForCLI())
	}))
	defer srv.Close()

	origFetch := apischema.FetchHTTPClient
	apischema.FetchHTTPClient = srv.Client()
	t.Cleanup(func() { apischema.FetchHTTPClient = origFetch })

	t.Setenv("MONDAY_API_URL", srv.URL)

	out, err := runConfigCmd(t, cfgPath, "config", "set", "api-version", "2026-08")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "2026-08") {
		t.Errorf("expected version in output, got: %s", out)
	}

	// Schema file should exist.
	schemaPath := apischema.CachedSchemaPath(dir)
	data, readErr := os.ReadFile(schemaPath)
	if readErr != nil {
		t.Fatalf("schema file not written: %v", readErr)
	}
	if !strings.Contains(string(data), "type Query") {
		t.Errorf("schema file missing 'type Query': %s", data)
	}

	// Config should persist the version.
	cfg, loadErr := config.Load(cfgPath)
	if loadErr != nil {
		t.Fatalf("load config: %v", loadErr)
	}
	if cfg.APIVersion != "2026-08" {
		t.Errorf("expected APIVersion 2026-08, got %q", cfg.APIVersion)
	}
}
