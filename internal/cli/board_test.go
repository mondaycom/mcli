package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gqlclient "github.com/Khan/genqlient/graphql"

	apigraphql "github.com/mondaycom/mcli/internal/api/graphql"
)

// installBoardFactory sets up boardClientFactory to use the given httptest
// server URL for the duration of the test, then restores the original value.
func installBoardFactory(t *testing.T, srvURL string) {
	t.Helper()
	orig := boardClientFactory
	boardClientFactory = func() (gqlclient.Client, error) {
		return gqlclient.NewClient(srvURL, http.DefaultClient), nil
	}
	t.Cleanup(func() { boardClientFactory = orig })
}

// graphqlResponse is a minimal GraphQL response envelope used in test handlers.
type graphqlResponse struct {
	Data json.RawMessage `json:"data"`
}

// mustMarshal marshals v or panics; used only in test setup.
func mustMarshal(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// newTestServer creates an httptest.Server whose handler calls fn.
// fn receives the decoded body and returns a JSON string for the "data" field.
func newTestServer(t *testing.T, fn func(body map[string]any) string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read body", http.StatusBadRequest)
			return
		}
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			http.Error(w, "parse body", http.StatusBadRequest)
			return
		}
		dataJSON := fn(body)
		w.Header().Set("Content-Type", "application/json")
		resp := graphqlResponse{Data: json.RawMessage(dataJSON)}
		enc, _ := json.Marshal(resp)
		_, _ = w.Write(enc)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// execBoardList runs 'board list' with the given flags and returns stdout, exit err.
func execBoardList(t *testing.T, args ...string) (string, error) {
	t.Helper()
	globals = GlobalFlags{JSON: true} // force JSON for deterministic output
	defer func() { globals = GlobalFlags{} }()

	var buf bytes.Buffer
	cmd := newBoardCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(append([]string{"list"}, args...))
	err := cmd.Execute()
	return buf.String(), err
}

// execBoardGet runs 'board get <id>' and returns stdout, exit err.
func execBoardGet(t *testing.T, id string, extraArgs ...string) (string, error) {
	t.Helper()
	globals = GlobalFlags{JSON: true}
	defer func() { globals = GlobalFlags{} }()

	var buf bytes.Buffer
	cmd := newBoardCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(append([]string{"get", id}, extraArgs...))
	err := cmd.Execute()
	return buf.String(), err
}

// ---- BoardsList tests ----

func TestBoardList_Empty(t *testing.T) {

	srv := newTestServer(t, func(body map[string]any) string {
		opName, _ := body["operationName"].(string)
		if opName != "BoardsList" {
			t.Errorf("unexpected operationName: %q", opName)
		}
		return `{"boards":[]}`
	})
	installBoardFactory(t, srv.URL)

	out, err := execBoardList(t)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	items, _ := result["items"].([]any)
	if len(items) != 0 {
		t.Errorf("expected empty items, got %d", len(items))
	}
	if result["cursor"] != "" {
		t.Errorf("expected empty cursor, got %v", result["cursor"])
	}
}

func TestBoardList_WithResults_NextPage(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		boards := []map[string]any{
			{"id": "9832181507", "name": "Test Board", "board_kind": "public", "state": "active", "workspace_id": "42"},
			{"id": "9832181508", "name": "Dev Board", "board_kind": "private", "state": "active", "workspace_id": "42"},
		}
		return mustMarshal(map[string]any{"boards": boards})
	})
	installBoardFactory(t, srv.URL)

	// Use --limit 2 so that len(results) == limit → next cursor is emitted.
	out, err := execBoardList(t, "--limit", "2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	items, _ := result["items"].([]any)
	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
	// Cursor should be "2" (next page).
	if result["cursor"] != "2" {
		t.Errorf("expected cursor '2', got %v", result["cursor"])
	}
}

func TestBoardList_WorkspaceFilter(t *testing.T) {
	var capturedVars map[string]any

	srv := newTestServer(t, func(body map[string]any) string {
		if vars, ok := body["variables"].(map[string]any); ok {
			capturedVars = vars
		}
		return `{"boards":[]}`
	})
	installBoardFactory(t, srv.URL)

	_, err := execBoardList(t, "--workspace", "99")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedVars == nil {
		t.Fatal("no variables captured")
	}
	wsIDs, _ := capturedVars["workspaceIds"].([]any)
	if len(wsIDs) != 1 || wsIDs[0] != "99" {
		t.Errorf("expected workspaceIds=[99], got %v", wsIDs)
	}
}

func TestBoardList_BadCursor_ExitCode1(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return `{"boards":[]}`
	})
	installBoardFactory(t, srv.URL)

	_, err := execBoardList(t, "--cursor", "notanumber")
	if err == nil {
		t.Fatal("expected error for bad cursor")
	}
	// Must be a USAGE error (exit code 1).
	code := errsCode(err)
	if code != "USAGE" {
		t.Errorf("expected USAGE code, got %q", code)
	}
}

func TestBoardList_CursorPagePassthrough(t *testing.T) {
	var capturedPage float64

	srv := newTestServer(t, func(body map[string]any) string {
		if vars, ok := body["variables"].(map[string]any); ok {
			capturedPage, _ = vars["page"].(float64)
		}
		return `{"boards":[]}`
	})
	installBoardFactory(t, srv.URL)

	_, err := execBoardList(t, "--cursor", "3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedPage != 3 {
		t.Errorf("expected page=3, got %v", capturedPage)
	}
}

// ---- BoardGet tests ----

func TestBoardGet_Success(t *testing.T) {
	srv := newTestServer(t, func(body map[string]any) string {
		opName, _ := body["operationName"].(string)
		if opName != "BoardGet" {
			t.Errorf("unexpected operationName: %q", opName)
		}
		board := map[string]any{
			"id":           "9832181507",
			"name":         "Test Board",
			"board_kind":   "public",
			"state":        "active",
			"description":  "A test board",
			"workspace_id": "42",
			"workspace":    map[string]any{"id": "42", "name": "Main WS", "kind": "open"},
			"owners":       []map[string]any{{"id": "1001", "name": "Alice"}},
			"groups":       []map[string]any{{"id": "topics", "title": "Group 1", "color": "#579bfc", "position": "0.1"}},
			"columns": []map[string]any{
				{"id": "name", "title": "Name", "type": "name", "settings_str": "{}", "width": 200, "archived": false},
			},
		}
		return mustMarshal(map[string]any{"boards": []any{board}})
	})
	installBoardFactory(t, srv.URL)

	out, err := execBoardGet(t, "9832181507")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	if result["id"] != "9832181507" {
		t.Errorf("expected id 9832181507, got %v", result["id"])
	}
	if result["name"] != "Test Board" {
		t.Errorf("expected name 'Test Board', got %v", result["name"])
	}
	if result["kind"] != "public" {
		t.Errorf("expected kind 'public', got %v", result["kind"])
	}
}

func TestBoardGet_NotFound_ExitCode2(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		// API returns empty boards list → not found.
		return `{"boards":[]}`
	})
	installBoardFactory(t, srv.URL)

	_, err := execBoardGet(t, "9999999999")
	if err == nil {
		t.Fatal("expected error for missing board")
	}
	code := errsCode(err)
	if code != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND, got %q", code)
	}
}

func TestBoardGet_NonNumericID_UsageError(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return `{"boards":[]}`
	})
	installBoardFactory(t, srv.URL)

	_, err := execBoardGet(t, "not-a-number")
	if err == nil {
		t.Fatal("expected error for non-numeric id")
	}
	code := errsCode(err)
	if code != "USAGE" {
		t.Errorf("expected USAGE, got %q", code)
	}
}

func TestBoardGet_WithWorkspaceAndOwners(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		board := map[string]any{
			"id":           "9832181507",
			"name":         "Test Board",
			"board_kind":   "private",
			"state":        "active",
			"description":  "",
			"workspace_id": "55",
			"workspace":    map[string]any{"id": "55", "name": "WS", "kind": "closed"},
			"owners": []map[string]any{
				{"id": "1001", "name": "Alice"},
				{"id": "1002", "name": "Bob"},
			},
			"groups":  []map[string]any{},
			"columns": []map[string]any{},
		}
		return mustMarshal(map[string]any{"boards": []any{board}})
	})
	installBoardFactory(t, srv.URL)

	out, err := execBoardGet(t, "9832181507")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	owners, _ := result["owners"].([]any)
	if len(owners) != 2 {
		t.Errorf("expected 2 owners, got %d", len(owners))
	}
	ws, _ := result["workspace"].(map[string]any)
	if ws == nil || ws["kind"] != "closed" {
		t.Errorf("expected workspace kind 'closed', got %v", ws)
	}
}

// boardWithItemsResponse builds a BoardGet response whose board carries an
// items_page (as returned when --items is set).
func boardWithItemsResponse(cursor string, items []map[string]any) map[string]any {
	board := map[string]any{
		"id":           "9832181507",
		"name":         "Test Board",
		"board_kind":   "public",
		"state":        "active",
		"description":  "",
		"workspace_id": "42",
		"workspace":    map[string]any{"id": "42", "name": "Main WS", "kind": "open"},
		"owners":       []map[string]any{},
		"groups":       []map[string]any{},
		"columns":      []map[string]any{},
		"items_page":   map[string]any{"cursor": cursor, "items": items},
	}
	return map[string]any{"boards": []any{board}}
}

func TestBoardGet_WithItems_DecodesColumnsAndCursor(t *testing.T) {
	var capturedVars map[string]any
	srv := newTestServer(t, func(body map[string]any) string {
		if vars, ok := body["variables"].(map[string]any); ok {
			capturedVars = vars
		}
		return mustMarshal(boardWithItemsResponse("next123", sampleItems()))
	})
	installBoardFactory(t, srv.URL)

	out, err := execBoardGet(t, "9832181507", "--items", "--items-limit", "10")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The query must request items with the given limit.
	if capturedVars["withItems"] != true {
		t.Errorf("expected withItems=true, got %v", capturedVars["withItems"])
	}
	if capturedVars["itemsLimit"] != float64(10) {
		t.Errorf("expected itemsLimit=10, got %v", capturedVars["itemsLimit"])
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	if result["items_cursor"] != "next123" {
		t.Errorf("expected items_cursor 'next123', got %v", result["items_cursor"])
	}
	items, _ := result["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	cols, _ := items[0].(map[string]any)["columns"].([]any)
	if len(cols) != 2 {
		t.Fatalf("expected 2 decoded columns, got %d", len(cols))
	}
	if cols[0].(map[string]any)["value"] != "Done" {
		t.Errorf("expected status value 'Done', got %v", cols[0].(map[string]any)["value"])
	}
}

func TestBoardGet_WithoutItems_OmitsItemsKeys(t *testing.T) {
	var capturedVars map[string]any
	srv := newTestServer(t, func(body map[string]any) string {
		if vars, ok := body["variables"].(map[string]any); ok {
			capturedVars = vars
		}
		return mustMarshal(boardWithItemsResponse("", nil))
	})
	installBoardFactory(t, srv.URL)

	out, err := execBoardGet(t, "9832181507")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedVars["withItems"] != false {
		t.Errorf("expected withItems=false, got %v", capturedVars["withItems"])
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	if _, ok := result["items"]; ok {
		t.Error("items key should be absent without --items")
	}
	if _, ok := result["items_cursor"]; ok {
		t.Error("items_cursor key should be absent without --items")
	}
}

func TestBoardGet_ItemsCursorImpliesItems(t *testing.T) {
	var capturedVars map[string]any
	srv := newTestServer(t, func(body map[string]any) string {
		if vars, ok := body["variables"].(map[string]any); ok {
			capturedVars = vars
		}
		return mustMarshal(boardWithItemsResponse("", sampleItems()))
	})
	installBoardFactory(t, srv.URL)

	// Only --items-cursor is passed; it must imply --items.
	if _, err := execBoardGet(t, "9832181507", "--items-cursor", "abc"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedVars["withItems"] != true {
		t.Errorf("expected withItems=true from cursor, got %v", capturedVars["withItems"])
	}
	if capturedVars["itemsCursor"] != "abc" {
		t.Errorf("expected itemsCursor 'abc', got %v", capturedVars["itemsCursor"])
	}
}

// execBoardCreate runs 'board create' with the given flags and returns stdout, exit err.
func execBoardCreate(t *testing.T, args ...string) (string, error) {
	t.Helper()
	globals = GlobalFlags{JSON: true}
	defer func() { globals = GlobalFlags{} }()

	var buf bytes.Buffer
	cmd := newBoardCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(append([]string{"create"}, args...))
	err := cmd.Execute()
	return buf.String(), err
}

// newErrorTestServer creates an httptest.Server that always returns a 200
// response with a GraphQL errors payload (no data field).
func newErrorTestServer(t *testing.T, message string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Consume body so the client doesn't get a broken-pipe.
		_, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		payload := map[string]any{
			"errors": []map[string]any{
				{"message": message},
			},
		}
		enc, _ := json.Marshal(payload)
		_, _ = w.Write(enc)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// ---- BoardCreate tests ----

func TestBoardCreate_Success(t *testing.T) {
	var capturedVars map[string]any

	srv := newTestServer(t, func(body map[string]any) string {
		opName, _ := body["operationName"].(string)
		if opName != "BoardCreate" {
			t.Errorf("unexpected operationName: %q", opName)
		}
		if vars, ok := body["variables"].(map[string]any); ok {
			capturedVars = vars
		}
		board := map[string]any{
			"id":           "9832181507",
			"name":         "Sprint Board",
			"board_kind":   "public",
			"state":        "active",
			"workspace_id": "",
			"description":  "",
		}
		return mustMarshal(map[string]any{"create_board": board})
	})
	installBoardFactory(t, srv.URL)

	out, err := execBoardCreate(t, "--name", "Sprint Board", "--kind", "public")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	if result["id"] != "9832181507" {
		t.Errorf("expected id 9832181507, got %v", result["id"])
	}
	if result["name"] != "Sprint Board" {
		t.Errorf("expected name 'Sprint Board', got %v", result["name"])
	}

	// Assert required vars are present.
	if capturedVars["name"] != "Sprint Board" {
		t.Errorf("expected variables.name='Sprint Board', got %v", capturedVars["name"])
	}
	if capturedVars["kind"] != "public" {
		t.Errorf("expected variables.kind='public', got %v", capturedVars["kind"])
	}

	// Assert omitempty: optional unset keys must NOT be present in the request.
	if _, present := capturedVars["workspaceId"]; present {
		t.Errorf("expected workspaceId absent (omitempty), but it was present: %v", capturedVars["workspaceId"])
	}
	if _, present := capturedVars["description"]; present {
		t.Errorf("expected description absent (omitempty), but it was present: %v", capturedVars["description"])
	}
	if _, present := capturedVars["empty"]; present {
		t.Errorf("expected empty absent (omitempty), but it was present: %v", capturedVars["empty"])
	}
}

func TestBoardCreate_MissingName_ExitCode1(t *testing.T) {
	// No server needed — validation must fail before any network call.
	called := false
	srv := newTestServer(t, func(_ map[string]any) string {
		called = true
		return `{"create_board":null}`
	})
	installBoardFactory(t, srv.URL)

	_, err := execBoardCreate(t /* no --name flag */)
	if err == nil {
		t.Fatal("expected error for missing --name")
	}
	if called {
		t.Error("server should not have been called")
	}
	// Cobra marks the flag required, so the error is a USAGE-class error.
	// The cobra error message won't match the errsCode pattern, but exit is 1.
	// Accept any non-nil error here as long as the server was not called.
}

func TestBoardCreate_InvalidKind_ExitCode1(t *testing.T) {
	called := false
	srv := newTestServer(t, func(_ map[string]any) string {
		called = true
		return `{"create_board":null}`
	})
	installBoardFactory(t, srv.URL)

	_, err := execBoardCreate(t, "--name", "X", "--kind", "bogus")
	if err == nil {
		t.Fatal("expected error for invalid --kind")
	}
	if called {
		t.Error("server should not have been called")
	}
	code := errsCode(err)
	if code != "USAGE" {
		t.Errorf("expected USAGE code, got %q", code)
	}
}

func TestBoardCreate_WithWorkspaceAndDescription(t *testing.T) {
	var capturedVars map[string]any

	srv := newTestServer(t, func(body map[string]any) string {
		if vars, ok := body["variables"].(map[string]any); ok {
			capturedVars = vars
		}
		board := map[string]any{
			"id":           "1234567890",
			"name":         "Team Board",
			"board_kind":   "private",
			"state":        "active",
			"workspace_id": "77",
			"description":  "Team planning board",
		}
		return mustMarshal(map[string]any{"create_board": board})
	})
	installBoardFactory(t, srv.URL)

	out, err := execBoardCreate(t,
		"--name", "Team Board",
		"--kind", "private",
		"--workspace", "77",
		"--description", "Team planning board",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	if result["workspace_id"] != "77" {
		t.Errorf("expected workspace_id '77', got %v", result["workspace_id"])
	}
	if result["description"] != "Team planning board" {
		t.Errorf("expected description 'Team planning board', got %v", result["description"])
	}

	// Variables should contain workspaceId and description since they were set.
	if capturedVars["workspaceId"] != "77" {
		t.Errorf("expected variables.workspaceId='77', got %v", capturedVars["workspaceId"])
	}
	if capturedVars["description"] != "Team planning board" {
		t.Errorf("expected variables.description='Team planning board', got %v", capturedVars["description"])
	}
}

func TestBoardCreate_APIError_ExitCode2(t *testing.T) {
	srv := newErrorTestServer(t, "board name already taken")

	// Use a normalising client (apigraphql) so GraphQL errors are mapped to
	// errs.API before returning to the command. The plain gqlclient.NewClient
	// seam bypasses normalisation, which is fine for happy-path tests but
	// wrong for testing the error-code contract.
	orig := boardClientFactory
	boardClientFactory = func() (gqlclient.Client, error) {
		c := apigraphql.New("test-token", "test", apigraphql.WithEndpoint(srv.URL))
		return c.GQL(), nil
	}
	t.Cleanup(func() { boardClientFactory = orig })

	_, err := execBoardCreate(t, "--name", "Taken Board", "--kind", "public")
	if err == nil {
		t.Fatal("expected error from API")
	}
	code := errsCode(err)
	if code != "API" {
		t.Errorf("expected API code, got %q", code)
	}
}

// errsCode extracts the error code string from a structured errs.Error,
// or returns the raw message if it is not structured.
func errsCode(err error) string {
	if err == nil {
		return ""
	}
	// Use fmt.Sprintf because errs.Error formats as "[CODE] msg".
	msg := fmt.Sprintf("%v", err)
	// Extract code from "[CODE] msg" format.
	if len(msg) > 2 && msg[0] == '[' {
		end := strings.Index(msg, "]")
		if end > 1 {
			return msg[1:end]
		}
	}
	return msg
}

// execBoardRename runs 'board rename <id> <name>' and returns stdout, exit err.
func execBoardRename(t *testing.T, id, name string) (string, error) {
	t.Helper()
	globals = GlobalFlags{JSON: true}
	defer func() { globals = GlobalFlags{} }()

	var buf bytes.Buffer
	cmd := newBoardCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"rename", id, name})
	err := cmd.Execute()
	return buf.String(), err
}

// execBoardDelete runs 'board delete <id>' and returns stdout, exit err.
func execBoardDelete(t *testing.T, id string) (string, error) {
	t.Helper()
	globals = GlobalFlags{JSON: true}
	defer func() { globals = GlobalFlags{} }()

	var buf bytes.Buffer
	cmd := newBoardCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"delete", id})
	err := cmd.Execute()
	return buf.String(), err
}

// execBoardArchive runs 'board archive <id>' and returns stdout, exit err.
func execBoardArchive(t *testing.T, id string) (string, error) {
	t.Helper()
	globals = GlobalFlags{JSON: true}
	defer func() { globals = GlobalFlags{} }()

	var buf bytes.Buffer
	cmd := newBoardCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"archive", id})
	err := cmd.Execute()
	return buf.String(), err
}

// ---- BoardRename tests ----

func TestBoardRename_Success(t *testing.T) {
	var capturedVars map[string]any

	srv := newTestServer(t, func(body map[string]any) string {
		opName, _ := body["operationName"].(string)
		if opName != "BoardRename" {
			t.Errorf("unexpected operationName: %q", opName)
		}
		if vars, ok := body["variables"].(map[string]any); ok {
			capturedVars = vars
		}
		return `{"update_board":"true"}`
	})
	installBoardFactory(t, srv.URL)

	out, err := execBoardRename(t, "9832181507", "New Name")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	if result["id"] != "9832181507" {
		t.Errorf("expected id 9832181507, got %v", result["id"])
	}
	if result["name"] != "New Name" {
		t.Errorf("expected name 'New Name', got %v", result["name"])
	}
	if capturedVars["boardId"] != "9832181507" {
		t.Errorf("expected variables.boardId='9832181507', got %v", capturedVars["boardId"])
	}
	if capturedVars["name"] != "New Name" {
		t.Errorf("expected variables.name='New Name', got %v", capturedVars["name"])
	}
}

func TestBoardRename_NonNumericID_UsageError(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return `{"update_board":"true"}`
	})
	installBoardFactory(t, srv.URL)

	_, err := execBoardRename(t, "not-a-number", "New Name")
	if err == nil {
		t.Fatal("expected error for non-numeric id")
	}
	if code := errsCode(err); code != "USAGE" {
		t.Errorf("expected USAGE, got %q", code)
	}
}

// ---- BoardDelete tests ----

func TestBoardDelete_Success(t *testing.T) {
	srv := newTestServer(t, func(body map[string]any) string {
		opName, _ := body["operationName"].(string)
		if opName != "BoardDelete" {
			t.Errorf("unexpected operationName: %q", opName)
		}
		board := map[string]any{"id": "9832181507", "name": "Deleted Board"}
		return mustMarshal(map[string]any{"delete_board": board})
	})
	installBoardFactory(t, srv.URL)

	out, err := execBoardDelete(t, "9832181507")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	if result["id"] != "9832181507" {
		t.Errorf("expected id 9832181507, got %v", result["id"])
	}
	if result["name"] != "Deleted Board" {
		t.Errorf("expected name 'Deleted Board', got %v", result["name"])
	}
}

func TestBoardDelete_NonNumericID_UsageError(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return `{"delete_board":null}`
	})
	installBoardFactory(t, srv.URL)

	_, err := execBoardDelete(t, "abc")
	if err == nil {
		t.Fatal("expected error for non-numeric id")
	}
	if code := errsCode(err); code != "USAGE" {
		t.Errorf("expected USAGE, got %q", code)
	}
}

// ---- BoardArchive tests ----

func TestBoardArchive_Success(t *testing.T) {
	srv := newTestServer(t, func(body map[string]any) string {
		opName, _ := body["operationName"].(string)
		if opName != "BoardArchive" {
			t.Errorf("unexpected operationName: %q", opName)
		}
		board := map[string]any{"id": "9832181507", "name": "Archived Board"}
		return mustMarshal(map[string]any{"archive_board": board})
	})
	installBoardFactory(t, srv.URL)

	out, err := execBoardArchive(t, "9832181507")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	if result["id"] != "9832181507" {
		t.Errorf("expected id 9832181507, got %v", result["id"])
	}
	if result["name"] != "Archived Board" {
		t.Errorf("expected name 'Archived Board', got %v", result["name"])
	}
}

func TestBoardArchive_NonNumericID_UsageError(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return `{"archive_board":null}`
	})
	installBoardFactory(t, srv.URL)

	_, err := execBoardArchive(t, "xyz")
	if err == nil {
		t.Fatal("expected error for non-numeric id")
	}
	if code := errsCode(err); code != "USAGE" {
		t.Errorf("expected USAGE, got %q", code)
	}
}
