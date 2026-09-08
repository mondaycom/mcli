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
	"time"

	"github.com/spf13/cobra"

	apischema "github.com/mondaycom/mcli/internal/api/schema"
	"github.com/mondaycom/mcli/internal/errs"
	rawschema "github.com/mondaycom/mcli/schema"
)

// runSchemaCmd executes a 'schema' subcommand against an isolated config dir.
// Not safe for parallel tests: it mutates globals and the apischema cache.
func runSchemaCmd(t *testing.T, dir string, args ...string) (string, string, error) {
	t.Helper()

	origGlobals := globals
	t.Cleanup(func() { globals = origGlobals })
	globals.Config = filepath.Join(dir, "config.yaml")
	// Default to JSON so assertions don't depend on whether the test binary's
	// stdout happens to be a TTY. A caller that pre-sets another mode keeps it.
	if !globals.Pretty && !globals.Terse && !globals.CSV {
		globals.JSON = true
	}

	apischema.SetConfigDir(dir)
	t.Cleanup(func() { apischema.SetConfigDir("") })

	root := &cobra.Command{Use: "mcli", SilenceErrors: true, SilenceUsage: true}
	root.AddCommand(newSchemaCmd())

	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), errOut.String(), err
}

// introspectionServer serves a minimal but valid introspection response.
func introspectionServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, minimalIntrospectionJSONForCLI())
	}))
	t.Cleanup(srv.Close)

	origFetch := apischema.FetchHTTPClient
	apischema.FetchHTTPClient = srv.Client()
	t.Cleanup(func() { apischema.FetchHTTPClient = origFetch })

	t.Setenv("MONDAY_API_URL", srv.URL)
	return srv
}

func decodeStatus(t *testing.T, out string) schemaStatus {
	t.Helper()
	var st schemaStatus
	if err := json.Unmarshal([]byte(out), &st); err != nil {
		t.Fatalf("unmarshal status %q: %v", out, err)
	}
	return st
}

// TestSchemaStatus_embedded verifies status reports the embedded SDL when no
// local cache exists, dated from the provenance file written by 'make schema'.
func TestSchemaStatus_embedded(t *testing.T) {
	dir := t.TempDir()

	out, _, err := runSchemaCmd(t, dir, "schema", "status")
	if err != nil {
		t.Fatalf("schema status: %v", err)
	}

	st := decodeStatus(t, out)
	if st.Source != apischema.SourceEmbedded {
		t.Errorf("source = %q, want %q", st.Source, apischema.SourceEmbedded)
	}
	if st.Path != "" {
		t.Errorf("path = %q, want empty for the embedded schema", st.Path)
	}
	if st.Types < 100 {
		t.Errorf("types = %d, want the full embedded schema (>100)", st.Types)
	}
	if st.APIVersion == "" {
		t.Error("api_version is empty")
	}
	if want := rawschema.FetchedAt().Format("2006-01-02"); st.FetchedAt != want {
		t.Errorf("fetched_at = %q, want %q from schema/fetched_at.txt", st.FetchedAt, want)
	}
	// Don't hardcode staleness: it depends on today's date. Assert the invariant.
	if st.Stale != (st.AgeDays > staleAfterDays) {
		t.Errorf("stale = %t but age_days = %d (threshold %d)", st.Stale, st.AgeDays, staleAfterDays)
	}
}

// TestSchemaStatus_cached verifies a parseable local cache is reported as the
// live schema, aged by its file mtime.
func TestSchemaStatus_cached(t *testing.T) {
	dir := t.TempDir()
	writeCachedSchema(t, dir, time.Now())

	out, _, err := runSchemaCmd(t, dir, "schema", "status")
	if err != nil {
		t.Fatalf("schema status: %v", err)
	}

	st := decodeStatus(t, out)
	if st.Source != apischema.SourceCached {
		t.Errorf("source = %q, want %q", st.Source, apischema.SourceCached)
	}
	if st.Path != apischema.CachedSchemaPath(dir) {
		t.Errorf("path = %q, want %q", st.Path, apischema.CachedSchemaPath(dir))
	}
	if st.AgeDays != 0 {
		t.Errorf("age_days = %d, want 0 for a just-written cache", st.AgeDays)
	}
	if st.Stale {
		t.Error("stale = true for a just-written cache")
	}
}

// TestSchemaStatus_cachedStale verifies the staleness threshold is driven by the
// cache file's mtime.
func TestSchemaStatus_cachedStale(t *testing.T) {
	dir := t.TempDir()
	writeCachedSchema(t, dir, time.Now().AddDate(0, 0, -(staleAfterDays+5)))

	out, _, err := runSchemaCmd(t, dir, "schema", "status")
	if err != nil {
		t.Fatalf("schema status: %v", err)
	}

	st := decodeStatus(t, out)
	if !st.Stale {
		t.Errorf("stale = false for a schema %d days old", st.AgeDays)
	}
	if st.AgeDays <= staleAfterDays {
		t.Errorf("age_days = %d, want > %d", st.AgeDays, staleAfterDays)
	}
}

// TestSchemaStatus_unparseableCacheReportsEmbedded guards the one case where the
// cache file's existence lies: apischema silently falls back to the embedded SDL
// when the cached file doesn't parse, and status must say so.
func TestSchemaStatus_unparseableCacheReportsEmbedded(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(apischema.CachedSchemaPath(dir), []byte("this is not graphql {{{"), 0o600); err != nil {
		t.Fatalf("write cache: %v", err)
	}

	out, _, err := runSchemaCmd(t, dir, "schema", "status")
	if err != nil {
		t.Fatalf("schema status: %v", err)
	}

	st := decodeStatus(t, out)
	if st.Source != apischema.SourceEmbedded {
		t.Errorf("source = %q, want %q when the cache doesn't parse", st.Source, apischema.SourceEmbedded)
	}
	if st.Types < 100 {
		t.Errorf("types = %d, want the embedded schema to be in use", st.Types)
	}
}

// TestSchemaStatus_terse verifies the single-line form stays greppable.
func TestSchemaStatus_terse(t *testing.T) {
	dir := t.TempDir()
	writeCachedSchema(t, dir, time.Now())

	origGlobals := globals
	t.Cleanup(func() { globals = origGlobals })
	globals.Terse = true

	out, _, err := runSchemaCmd(t, dir, "schema", "status")
	if err != nil {
		t.Fatalf("schema status: %v", err)
	}

	line := strings.TrimSpace(out)
	if strings.Contains(line, "\n") {
		t.Errorf("terse output is not a single line: %q", out)
	}
	for _, want := range []string{apischema.SourceCached, "age=0d", "stale=false", "types="} {
		if !strings.Contains(line, want) {
			t.Errorf("terse output %q is missing %q", line, want)
		}
	}
}

// TestSchemaRefresh_success verifies refresh writes the cache, reports the type
// delta, and flips status over to the cached schema.
func TestSchemaRefresh_success(t *testing.T) {
	dir := t.TempDir()
	introspectionServer(t)

	origGlobals := globals
	t.Cleanup(func() { globals = origGlobals })
	globals.Token = "fake-token"

	out, _, err := runSchemaCmd(t, dir, "schema", "refresh")
	if err != nil {
		t.Fatalf("schema refresh: %v", err)
	}

	var got schemaRefreshOutput
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal refresh output %q: %v", out, err)
	}
	if got.Path != apischema.CachedSchemaPath(dir) {
		t.Errorf("path = %q, want %q", got.Path, apischema.CachedSchemaPath(dir))
	}
	if got.APIVersion == "" {
		t.Error("api_version is empty")
	}
	if got.Types == 0 {
		t.Error("types = 0, want the refreshed schema's type count")
	}
	// The fixture schema is far smaller than the embedded one, so the delta must
	// show the embedded types dropping out.
	if len(got.Removed) == 0 {
		t.Error("removed is empty, want the embedded types absent from the fixture")
	}

	data, readErr := os.ReadFile(apischema.CachedSchemaPath(dir))
	if readErr != nil {
		t.Fatalf("cache not written: %v", readErr)
	}
	if !strings.Contains(string(data), "type Query") {
		t.Errorf("cache missing 'type Query': %s", data)
	}

	statusOut, _, statusErr := runSchemaCmd(t, dir, "schema", "status")
	if statusErr != nil {
		t.Fatalf("schema status: %v", statusErr)
	}
	if st := decodeStatus(t, statusOut); st.Source != apischema.SourceCached {
		t.Errorf("after refresh source = %q, want %q", st.Source, apischema.SourceCached)
	}
}

// TestSchemaRefresh_apiVersionOverrideNotPersisted verifies --api-version affects
// the fetch without writing to config.yaml.
func TestSchemaRefresh_apiVersionOverrideNotPersisted(t *testing.T) {
	dir := t.TempDir()

	var gotVersion string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotVersion = r.Header.Get("API-Version")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, minimalIntrospectionJSONForCLI())
	}))
	defer srv.Close()

	origFetch := apischema.FetchHTTPClient
	apischema.FetchHTTPClient = srv.Client()
	t.Cleanup(func() { apischema.FetchHTTPClient = origFetch })
	t.Setenv("MONDAY_API_URL", srv.URL)

	origGlobals := globals
	t.Cleanup(func() { globals = origGlobals })
	globals.Token = "fake-token"

	out, _, err := runSchemaCmd(t, dir, "schema", "refresh", "--api-version", "2026-08")
	if err != nil {
		t.Fatalf("schema refresh: %v", err)
	}

	if gotVersion != "2026-08" {
		t.Errorf("API-Version header = %q, want 2026-08", gotVersion)
	}
	var got schemaRefreshOutput
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal refresh output: %v", err)
	}
	if got.APIVersion != "2026-08" {
		t.Errorf("api_version = %q, want 2026-08", got.APIVersion)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "config.yaml")); statErr == nil {
		t.Error("refresh wrote config.yaml; --api-version must not be persisted")
	}
}

// TestSchemaRefresh_invalidAPIVersion verifies the override is validated before
// any network call, so a typo is a usage error rather than an API error.
func TestSchemaRefresh_invalidAPIVersion(t *testing.T) {
	dir := t.TempDir()

	_, _, err := runSchemaCmd(t, dir, "schema", "refresh", "--api-version", "2026-7")
	if err == nil {
		t.Fatal("expected an error for a malformed api-version")
	}
	e, ok := err.(*errs.Error)
	if !ok || e.Code != errs.CodeUsage {
		t.Errorf("error = %T %v, want errs.CodeUsage", err, err)
	}
}

// TestSchemaRefresh_noToken verifies refresh fails with an auth error rather than
// silently leaving the schema alone.
func TestSchemaRefresh_noToken(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MONDAY_API_TOKEN", "")

	origGlobals := globals
	t.Cleanup(func() { globals = origGlobals })
	globals.Token = ""

	_, _, err := runSchemaCmd(t, dir, "schema", "refresh")
	if err == nil {
		t.Fatal("expected an error when no token is available")
	}
	e, ok := err.(*errs.Error)
	if !ok || e.Code != errs.CodeAuth {
		t.Errorf("error = %T %v, want errs.CodeAuth", err, err)
	}
	if _, statErr := os.Stat(apischema.CachedSchemaPath(dir)); statErr == nil {
		t.Error("cache was written despite the missing token")
	}
}

// TestSchemaRefresh_fetchError verifies an API failure leaves the existing cache
// untouched.
func TestSchemaRefresh_fetchError(t *testing.T) {
	dir := t.TempDir()
	writeCachedSchema(t, dir, time.Now())
	before, err := os.ReadFile(apischema.CachedSchemaPath(dir))
	if err != nil {
		t.Fatalf("read cache: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	origFetch := apischema.FetchHTTPClient
	apischema.FetchHTTPClient = srv.Client()
	t.Cleanup(func() { apischema.FetchHTTPClient = origFetch })
	t.Setenv("MONDAY_API_URL", srv.URL)

	origGlobals := globals
	t.Cleanup(func() { globals = origGlobals })
	globals.Token = "fake-token"

	if _, _, refreshErr := runSchemaCmd(t, dir, "schema", "refresh"); refreshErr == nil {
		t.Fatal("expected an error when introspection returns 403")
	}

	after, err := os.ReadFile(apischema.CachedSchemaPath(dir))
	if err != nil {
		t.Fatalf("cache disappeared after a failed refresh: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Error("a failed refresh modified the existing cache")
	}
}

// TestWarnIfSchemaStale_stderrOnly verifies the staleness warning never touches
// stdout, which carries the JSON contract that LLM callers parse.
func TestWarnIfSchemaStale_stderrOnly(t *testing.T) {
	dir := t.TempDir()
	writeCachedSchema(t, dir, time.Now().AddDate(0, 0, -(staleAfterDays+5)))

	origGlobals := globals
	t.Cleanup(func() { globals = origGlobals })
	globals.Config = filepath.Join(dir, "config.yaml")

	apischema.SetConfigDir(dir)
	t.Cleanup(func() { apischema.SetConfigDir("") })

	cmd := &cobra.Command{Use: "api"}
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)

	warnIfSchemaStale(cmd)

	if out.Len() != 0 {
		t.Errorf("warning leaked to stdout: %q", out.String())
	}
	if !strings.Contains(errOut.String(), "mcli schema refresh") {
		t.Errorf("stderr = %q, want the refresh hint", errOut.String())
	}
}

// TestWarnIfSchemaStale_silentWhenFresh verifies a fresh schema produces no noise.
func TestWarnIfSchemaStale_silentWhenFresh(t *testing.T) {
	dir := t.TempDir()
	writeCachedSchema(t, dir, time.Now())

	origGlobals := globals
	t.Cleanup(func() { globals = origGlobals })
	globals.Config = filepath.Join(dir, "config.yaml")

	apischema.SetConfigDir(dir)
	t.Cleanup(func() { apischema.SetConfigDir("") })

	cmd := &cobra.Command{Use: "api"}
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)

	warnIfSchemaStale(cmd)

	if out.Len() != 0 || errOut.Len() != 0 {
		t.Errorf("expected no output for a fresh schema, got stdout=%q stderr=%q", out.String(), errOut.String())
	}
}

// writeCachedSchema writes a parseable schema.graphql into dir and backdates its
// mtime, which is what liveSchemaInfo uses to age a cached schema.
func writeCachedSchema(t *testing.T, dir string, modTime time.Time) {
	t.Helper()
	path := apischema.CachedSchemaPath(dir)
	if err := os.WriteFile(path, []byte("type Query { hello: String }\n"), 0o600); err != nil {
		t.Fatalf("write cached schema: %v", err)
	}
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatalf("set cache mtime: %v", err)
	}
}
