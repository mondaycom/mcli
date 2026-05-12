package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// installQueryFactory sets up queryHTTPFactory to route requests to srv.
// It restores the previous factory on test cleanup.
func installQueryFactory(t *testing.T, srv *httptest.Server) {
	t.Helper()
	orig := queryHTTPFactory
	queryHTTPFactory = func() (*http.Client, string, error) {
		return http.DefaultClient, srv.URL, nil
	}
	t.Cleanup(func() { queryHTTPFactory = orig })
}

// execQuery runs 'mcli query' with args and returns stdout + error.
// Callers must set up globals before calling.
func execQuery(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	cmd := newQueryCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

// execQuerySub runs 'mcli query <sub>' with args and returns stdout + error.
func execQuerySub(t *testing.T, sub string, args ...string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	cmd := newQueryCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(append([]string{sub}, args...))
	err := cmd.Execute()
	return buf.String(), err
}

// mondayHandler returns an http.Handler that responds with body to POST requests.
func mondayHandler(t *testing.T, body string, statusCode int) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %q", ct)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		_, _ = w.Write([]byte(body))
	})
}

// --- parseVarValue tests (pure, parallel safe) ---

func TestParseVarValue_Int(t *testing.T) {
	t.Parallel()
	got := parseVarValue("42")
	v, ok := got.(int64)
	if !ok {
		t.Fatalf("expected int64, got %T", got)
	}
	if v != 42 {
		t.Errorf("got %d, want 42", v)
	}
}

func TestParseVarValue_NegativeInt(t *testing.T) {
	t.Parallel()
	got := parseVarValue("-7")
	v, ok := got.(int64)
	if !ok {
		t.Fatalf("expected int64, got %T", got)
	}
	if v != -7 {
		t.Errorf("got %d, want -7", v)
	}
}

func TestParseVarValue_Float(t *testing.T) {
	t.Parallel()
	got := parseVarValue("3.14")
	v, ok := got.(float64)
	if !ok {
		t.Fatalf("expected float64, got %T", got)
	}
	if v != 3.14 {
		t.Errorf("got %f, want 3.14", v)
	}
}

func TestParseVarValue_BoolTrue(t *testing.T) {
	t.Parallel()
	got := parseVarValue("true")
	if v, ok := got.(bool); !ok || !v {
		t.Errorf("expected true bool, got %v (%T)", got, got)
	}
}

func TestParseVarValue_BoolFalse(t *testing.T) {
	t.Parallel()
	got := parseVarValue("false")
	if v, ok := got.(bool); !ok || v {
		t.Errorf("expected false bool, got %v (%T)", got, got)
	}
}

func TestParseVarValue_Null(t *testing.T) {
	t.Parallel()
	got := parseVarValue("null")
	if got != nil {
		t.Errorf("expected nil, got %v (%T)", got, got)
	}
}

func TestParseVarValue_JSONObject(t *testing.T) {
	t.Parallel()
	got := parseVarValue(`{"a":1}`)
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", got)
	}
	if m["a"] != float64(1) {
		t.Errorf("m[a] = %v, want 1", m["a"])
	}
}

func TestParseVarValue_JSONArray(t *testing.T) {
	t.Parallel()
	got := parseVarValue(`[1,2,3]`)
	arr, ok := got.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", got)
	}
	if len(arr) != 3 {
		t.Errorf("len = %d, want 3", len(arr))
	}
}

func TestParseVarValue_String(t *testing.T) {
	t.Parallel()
	got := parseVarValue("hello world")
	if v, ok := got.(string); !ok || v != "hello world" {
		t.Errorf("expected string 'hello world', got %v (%T)", got, got)
	}
}

func TestParseVarValue_InvalidJSONObject_FallsBackToString(t *testing.T) {
	t.Parallel()
	got := parseVarValue("{not json}")
	if v, ok := got.(string); !ok || v != "{not json}" {
		t.Errorf("expected string '{not json}', got %v (%T)", got, got)
	}
}

// --- coerceJSONVars tests (pure, parallel safe) ---

func TestCoerceJSONVars_StringifiesMapForJSONType(t *testing.T) {
	t.Parallel()
	vars := map[string]any{
		"cols":  map[string]any{"sku": "WGT-001", "price": "29.99"},
		"board": int64(123),
	}
	coerceJSONVars(`mutation($board: ID!, $name: String!, $cols: JSON!) { create_item(board_id: $board, item_name: $name, column_values: $cols) { id } }`, vars)
	s, ok := vars["cols"].(string)
	if !ok {
		t.Fatalf("expected cols to be string, got %T", vars["cols"])
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(s), &parsed); err != nil {
		t.Fatalf("cols is not valid JSON string: %v", err)
	}
	if parsed["sku"] != "WGT-001" {
		t.Errorf("parsed[sku] = %v, want WGT-001", parsed["sku"])
	}
	// board should be untouched (not JSON type)
	if vars["board"] != int64(123) {
		t.Errorf("board = %v, want 123", vars["board"])
	}
}

func TestCoerceJSONVars_StringifiesArrayForJSONType(t *testing.T) {
	t.Parallel()
	vars := map[string]any{
		"ids": []any{float64(1), float64(2), float64(3)},
	}
	coerceJSONVars(`query($ids: JSON!) { items(ids: $ids) { id } }`, vars)
	s, ok := vars["ids"].(string)
	if !ok {
		t.Fatalf("expected ids to be string, got %T", vars["ids"])
	}
	if s != "[1,2,3]" {
		t.Errorf("ids = %q, want [1,2,3]", s)
	}
}

func TestCoerceJSONVars_LeavesStringAlone(t *testing.T) {
	t.Parallel()
	vars := map[string]any{
		"cols": `{"already":"a string"}`,
	}
	coerceJSONVars(`mutation($cols: JSON!) { x }`, vars)
	if vars["cols"] != `{"already":"a string"}` {
		t.Errorf("cols was modified: %v", vars["cols"])
	}
}

func TestCoerceJSONVars_HandlesNullableJSON(t *testing.T) {
	t.Parallel()
	vars := map[string]any{
		"data": map[string]any{"key": "val"},
	}
	coerceJSONVars(`mutation($data: JSON) { x }`, vars)
	if _, ok := vars["data"].(string); !ok {
		t.Fatalf("expected string, got %T", vars["data"])
	}
}

func TestCoerceJSONVars_NoVarDeclarations(t *testing.T) {
	t.Parallel()
	vars := map[string]any{
		"x": map[string]any{"a": "b"},
	}
	coerceJSONVars(`{ boards { id } }`, vars)
	// No declarations → no coercion
	if _, ok := vars["x"].(map[string]any); !ok {
		t.Errorf("x should remain a map when no declarations present")
	}
}

func TestCoerceJSONVars_NilVars(t *testing.T) {
	t.Parallel()
	// Should not panic
	coerceJSONVars(`mutation($cols: JSON!) { x }`, nil)
}

// --- parseVarFlags tests (pure, parallel safe) ---

func TestParseVarFlags_Empty(t *testing.T) {
	t.Parallel()
	out, err := parseVarFlags(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected empty map, got %v", out)
	}
}

func TestParseVarFlags_MultipleTypes(t *testing.T) {
	t.Parallel()
	out, err := parseVarFlags([]string{"n=42", "s=hello", "b=true", "x=null"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out["n"] != int64(42) {
		t.Errorf("n = %v (%T), want int64(42)", out["n"], out["n"])
	}
	if out["s"] != "hello" {
		t.Errorf("s = %v, want 'hello'", out["s"])
	}
	if out["b"] != true {
		t.Errorf("b = %v, want true", out["b"])
	}
	if out["x"] != nil {
		t.Errorf("x = %v, want nil", out["x"])
	}
}

func TestParseVarFlags_LastWins(t *testing.T) {
	t.Parallel()
	out, err := parseVarFlags([]string{"k=first", "k=second"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out["k"] != "second" {
		t.Errorf("k = %v, want 'second'", out["k"])
	}
}

func TestParseVarFlags_MissingEquals(t *testing.T) {
	t.Parallel()
	_, err := parseVarFlags([]string{"noequals"})
	if err == nil {
		t.Fatal("expected error for missing '='")
	}
}

func TestParseVarFlags_EmptyKey(t *testing.T) {
	t.Parallel()
	_, err := parseVarFlags([]string{"=value"})
	if err == nil {
		t.Fatal("expected error for empty key")
	}
}

// --- mergeVarsFile tests (filesystem, but temp dir is per-test, safe) ---

func TestMergeVarsFile_Basic(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	f := filepath.Join(dir, "vars.json")
	if err := os.WriteFile(f, []byte(`{"a":1,"b":"two"}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	dst := map[string]any{}
	if err := mergeVarsFile(dst, f); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dst["a"] != float64(1) {
		t.Errorf("a = %v, want 1", dst["a"])
	}
	if dst["b"] != "two" {
		t.Errorf("b = %v, want 'two'", dst["b"])
	}
}

func TestMergeVarsFile_VarFlagWins(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	f := filepath.Join(dir, "vars.json")
	if err := os.WriteFile(f, []byte(`{"k":"from-file"}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	dst := map[string]any{"k": "from-flag"}
	if err := mergeVarsFile(dst, f); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dst["k"] != "from-flag" {
		t.Errorf("k = %v, want 'from-flag'", dst["k"])
	}
}

func TestMergeVarsFile_NotJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	f := filepath.Join(dir, "vars.json")
	if err := os.WriteFile(f, []byte(`not json`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	err := mergeVarsFile(map[string]any{}, f)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// --- integration tests that modify global state: NOT parallel ---
// These tests mutate globals (globals.Config, queryHTTPFactory).
// They use serial subtests to avoid interference.

func TestQueryCmd_Integration(t *testing.T) {
	// All subtests here share a temp dir but are serial so no chdir races.
	dir := t.TempDir()

	// Override globals.Config for all subtests to an isolated path.
	origConf := globals.Config
	globals.Config = filepath.Join(dir, "config.yaml")
	t.Cleanup(func() { globals.Config = origConf })

	origJSON := globals.JSON
	globals.JSON = true
	t.Cleanup(func() { globals.JSON = origJSON })

	t.Run("Inline_Success", func(t *testing.T) {
		const respBody = `{"data":{"boards":[{"id":"1"}]}}`
		srv := httptest.NewServer(mondayHandler(t, respBody, http.StatusOK))
		defer srv.Close()
		installQueryFactory(t, srv)

		out, err := execQuery(t, `{ boards { id } }`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, `"boards"`) {
			t.Errorf("output missing 'boards': %s", out)
		}
	})

	t.Run("Inline_GraphQLErrors_ExitCode2", func(t *testing.T) {
		const respBody = `{"errors":[{"message":"bad query"}]}`
		srv := httptest.NewServer(mondayHandler(t, respBody, http.StatusOK))
		defer srv.Close()
		installQueryFactory(t, srv)

		out, err := execQuery(t, `{ invalid }`)
		if err == nil {
			t.Fatal("expected error for GraphQL errors")
		}
		if !strings.Contains(out, `"errors"`) {
			t.Errorf("output missing 'errors': %s", out)
		}
	})

	t.Run("File", func(t *testing.T) {
		const respBody = `{"data":{"me":{"id":"99"}}}`
		srv := httptest.NewServer(mondayHandler(t, respBody, http.StatusOK))
		defer srv.Close()
		installQueryFactory(t, srv)

		qf := filepath.Join(dir, "q.graphql")
		if err := os.WriteFile(qf, []byte("{ me { id } }"), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}

		out, err := execQuery(t, "-f", qf)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, `"me"`) {
			t.Errorf("output missing 'me': %s", out)
		}
	})

	t.Run("WithVars", func(t *testing.T) {
		var capturedBody []byte
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			capturedBody = b
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintln(w, `{"data":null}`)
		}))
		defer srv.Close()
		installQueryFactory(t, srv)

		_, _ = execQuery(t, `query($n: Int!) { boards(limit: $n) { id } }`, "--var", "n=10")

		var payload map[string]any
		if err := json.Unmarshal(capturedBody, &payload); err != nil {
			t.Fatalf("parse captured body: %v", err)
		}
		vars, ok := payload["variables"].(map[string]any)
		if !ok {
			t.Fatalf("variables not present: %v", payload)
		}
		if vars["n"] != float64(10) {
			t.Errorf("vars[n] = %v, want 10", vars["n"])
		}
	})

	t.Run("WithVarsFile", func(t *testing.T) {
		var capturedBody []byte
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			capturedBody = b
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintln(w, `{"data":null}`)
		}))
		defer srv.Close()
		installQueryFactory(t, srv)

		vf := filepath.Join(dir, "vars.json")
		if err := os.WriteFile(vf, []byte(`{"limit":5}`), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}

		_, _ = execQuery(t, `{ boards { id } }`, "--vars-file", vf)

		var payload map[string]any
		if err := json.Unmarshal(capturedBody, &payload); err != nil {
			t.Fatalf("parse captured body: %v", err)
		}
		vars, ok := payload["variables"].(map[string]any)
		if !ok {
			t.Fatalf("variables not present: %v", payload)
		}
		if vars["limit"] != float64(5) {
			t.Errorf("vars[limit] = %v, want 5", vars["limit"])
		}
	})

	t.Run("VarFlagBeatsVarsFile", func(t *testing.T) {
		var capturedBody []byte
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			capturedBody = b
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintln(w, `{"data":null}`)
		}))
		defer srv.Close()
		installQueryFactory(t, srv)

		vf := filepath.Join(dir, "vf2.json")
		if err := os.WriteFile(vf, []byte(`{"k":"from-file"}`), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}

		_, _ = execQuery(t, `{ boards { id } }`, "--var", "k=from-flag", "--vars-file", vf)

		var payload map[string]any
		if err := json.Unmarshal(capturedBody, &payload); err != nil {
			t.Fatalf("parse body: %v", err)
		}
		vars, _ := payload["variables"].(map[string]any)
		if vars["k"] != "from-flag" {
			t.Errorf("vars[k] = %v, want 'from-flag'", vars["k"])
		}
	})

	t.Run("JSONVar_CoercedToString_OnWire", func(t *testing.T) {
		var capturedBody []byte
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			capturedBody = b
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintln(w, `{"data":{"create_item":{"id":"1"}}}`)
		}))
		defer srv.Close()
		installQueryFactory(t, srv)

		_, err := execQuery(t,
			`mutation($board: ID!, $cols: JSON!) { create_item(board_id: $board, column_values: $cols) { id } }`,
			"--var", "board=123",
			"--var", `cols={"sku":"WGT-001","price":"29.99"}`,
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var payload map[string]any
		if err := json.Unmarshal(capturedBody, &payload); err != nil {
			t.Fatalf("parse body: %v", err)
		}
		vars, _ := payload["variables"].(map[string]any)
		// cols should be a string on the wire, not an object
		colsStr, ok := vars["cols"].(string)
		if !ok {
			t.Fatalf("vars[cols] should be string, got %T: %v", vars["cols"], vars["cols"])
		}
		// Verify it's valid JSON inside the string
		var parsed map[string]any
		if err := json.Unmarshal([]byte(colsStr), &parsed); err != nil {
			t.Fatalf("cols string not valid JSON: %v", err)
		}
		if parsed["sku"] != "WGT-001" {
			t.Errorf("parsed[sku] = %v, want WGT-001", parsed["sku"])
		}
	})

	t.Run("NoArgsNoFile_Error", func(t *testing.T) {
		_, err := execQuery(t)
		if err == nil {
			t.Fatal("expected error when no query provided")
		}
	})
}

// TestQuerySavedCmds tests the save/list/delete/run subcommands.
// Uses absolute paths for query directories to avoid os.Chdir.
func TestQuerySavedCmds(t *testing.T) {
	dir := t.TempDir()

	// Override globals to use isolated config dir.
	origConf := globals.Config
	globals.Config = filepath.Join(dir, "config.yaml")
	t.Cleanup(func() { globals.Config = origConf })

	origJSON := globals.JSON
	globals.JSON = true
	t.Cleanup(func() { globals.JSON = origJSON })

	// Save local query via file path so we don't need chdir.
	// We write the file directly using saveSavedQuery with a custom local dir
	// by calling it through the command with --file pointing to a temp file.
	// For local save we must chdir, but we do it once at the start of this test
	// since all subtests here are serial.
	origWD, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir to %s: %v", dir, err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })

	t.Run("Save_Local", func(t *testing.T) {
		out, err := execQuerySub(t, "save", "myq", "--query", "{ me { id } }")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "myq") {
			t.Errorf("output missing query name: %s", out)
		}
		qPath := filepath.Join(dir, ".mcli", "queries", "myq.graphql")
		if _, statErr := os.Stat(qPath); statErr != nil {
			t.Errorf("expected file %s to exist: %v", qPath, statErr)
		}
	})

	t.Run("Save_Global", func(t *testing.T) {
		out, err := execQuerySub(t, "save", "gq", "--query", "{ boards { id } }", "--global")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "global") {
			t.Errorf("output missing 'global': %s", out)
		}
		qPath := filepath.Join(dir, "queries", "gq.graphql")
		if _, statErr := os.Stat(qPath); statErr != nil {
			t.Errorf("expected file %s to exist: %v", qPath, statErr)
		}
	})

	t.Run("List", func(t *testing.T) {
		out, err := execQuerySub(t, "list")
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		var result struct {
			Items []savedQueryEntry `json:"items"`
		}
		if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); jsonErr != nil {
			t.Fatalf("parse output JSON: %v (output: %s)", jsonErr, out)
		}
		if len(result.Items) != 2 {
			t.Fatalf("expected 2 items, got %d: %v", len(result.Items), result.Items)
		}
		// Local comes first.
		if result.Items[0].Name != "myq" || result.Items[0].Source != "local" {
			t.Errorf("item[0] = %+v, want {myq local}", result.Items[0])
		}
		if result.Items[1].Name != "gq" || result.Items[1].Source != "global" {
			t.Errorf("item[1] = %+v, want {gq global}", result.Items[1])
		}
	})

	t.Run("Delete_Local", func(t *testing.T) {
		// Save a query to delete.
		if _, err := execQuerySub(t, "save", "todel", "--query", "{ me { id } }"); err != nil {
			t.Fatalf("save: %v", err)
		}
		out, err := execQuerySub(t, "delete", "todel")
		if err != nil {
			t.Fatalf("delete: %v", err)
		}
		if !strings.Contains(out, "todel") {
			t.Errorf("output missing name: %s", out)
		}
		qPath := filepath.Join(dir, ".mcli", "queries", "todel.graphql")
		if _, statErr := os.Stat(qPath); !os.IsNotExist(statErr) {
			t.Errorf("file %s should not exist after delete", qPath)
		}
	})

	t.Run("Delete_NotFound", func(t *testing.T) {
		_, err := execQuerySub(t, "delete", "nonexistent")
		if err == nil {
			t.Fatal("expected error for nonexistent query")
		}
	})

	t.Run("Run_LocalFirst", func(t *testing.T) {
		const respBody = `{"data":{"me":{"id":"1"}}}`
		srv := httptest.NewServer(mondayHandler(t, respBody, http.StatusOK))
		defer srv.Close()
		installQueryFactory(t, srv)

		out, err := execQuerySub(t, "run", "myq")
		if err != nil {
			t.Fatalf("run: %v", err)
		}
		if !strings.Contains(out, `"me"`) {
			t.Errorf("output missing 'me': %s", out)
		}
	})

	t.Run("Run_GlobalFallback", func(t *testing.T) {
		const respBody = `{"data":{"boards":[]}}`
		srv := httptest.NewServer(mondayHandler(t, respBody, http.StatusOK))
		defer srv.Close()
		installQueryFactory(t, srv)

		out, err := execQuerySub(t, "run", "gq")
		if err != nil {
			t.Fatalf("run: %v", err)
		}
		if !strings.Contains(out, `"boards"`) {
			t.Errorf("output missing 'boards': %s", out)
		}
	})

	t.Run("Run_NotFound", func(t *testing.T) {
		_, err := execQuerySub(t, "run", "doesnotexist")
		if err == nil {
			t.Fatal("expected error for nonexistent query")
		}
	})

	t.Run("Run_WithVars", func(t *testing.T) {
		var capturedBody []byte
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			capturedBody = b
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintln(w, `{"data":null}`)
		}))
		defer srv.Close()
		installQueryFactory(t, srv)

		if _, err := execQuerySub(t, "save", "pq", "--query", `query($n: Int!) { boards(limit: $n) { id } }`); err != nil {
			t.Fatalf("save: %v", err)
		}
		_, _ = execQuerySub(t, "run", "pq", "--var", "n=5")

		var payload map[string]any
		if err := json.Unmarshal(capturedBody, &payload); err != nil {
			t.Fatalf("parse body: %v", err)
		}
		vars, _ := payload["variables"].(map[string]any)
		if vars["n"] != float64(5) {
			t.Errorf("vars[n] = %v, want 5", vars["n"])
		}
	})

	t.Run("List_Empty_After_Deletes", func(t *testing.T) {
		// Delete the remaining local query (myq) and global (gq).
		if _, err := execQuerySub(t, "delete", "myq"); err != nil {
			t.Fatalf("delete myq: %v", err)
		}
		if _, err := execQuerySub(t, "delete", "gq", "--global"); err != nil {
			t.Fatalf("delete gq: %v", err)
		}
		// pq was also created; delete it.
		if _, err := execQuerySub(t, "delete", "pq"); err != nil {
			t.Fatalf("delete pq: %v", err)
		}

		out, err := execQuerySub(t, "list")
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		var result struct {
			Items []savedQueryEntry `json:"items"`
		}
		if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); jsonErr != nil {
			t.Fatalf("parse: %v (out: %s)", jsonErr, out)
		}
		if len(result.Items) != 0 {
			t.Errorf("expected 0 items, got %d: %v", len(result.Items), result.Items)
		}
	})
}
