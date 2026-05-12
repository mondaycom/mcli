package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	gqlclient "github.com/Khan/genqlient/graphql"

	apigraphql "github.com/mondaycom/mcli/internal/api/graphql"
)

// installWorkspaceFactory sets up workspaceClientFactory to use the given
// httptest server URL for the duration of the test, then restores the original.
func installWorkspaceFactory(t *testing.T, srvURL string) {
	t.Helper()
	orig := workspaceClientFactory
	workspaceClientFactory = func() (gqlclient.Client, error) {
		return gqlclient.NewClient(srvURL, http.DefaultClient), nil
	}
	t.Cleanup(func() { workspaceClientFactory = orig })
}

// execWorkspaceList runs 'workspace list' with the given flags and returns stdout, exit err.
func execWorkspaceList(t *testing.T, args ...string) (string, error) {
	t.Helper()
	globals = GlobalFlags{JSON: true}
	defer func() { globals = GlobalFlags{} }()

	var buf bytes.Buffer
	cmd := newWorkspaceCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(append([]string{"list"}, args...))
	err := cmd.Execute()
	return buf.String(), err
}

// execWorkspaceGet runs 'workspace get <id>' and returns stdout, exit err.
func execWorkspaceGet(t *testing.T, id string, extraArgs ...string) (string, error) {
	t.Helper()
	globals = GlobalFlags{JSON: true}
	defer func() { globals = GlobalFlags{} }()

	var buf bytes.Buffer
	cmd := newWorkspaceCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(append([]string{"get", id}, extraArgs...))
	err := cmd.Execute()
	return buf.String(), err
}

// execWorkspaceCreate runs 'workspace create' with the given flags and returns stdout, exit err.
func execWorkspaceCreate(t *testing.T, args ...string) (string, error) {
	t.Helper()
	globals = GlobalFlags{JSON: true}
	defer func() { globals = GlobalFlags{} }()

	var buf bytes.Buffer
	cmd := newWorkspaceCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(append([]string{"create"}, args...))
	err := cmd.Execute()
	return buf.String(), err
}

// execWorkspaceUpdate runs 'workspace update <id>' with the given flags.
func execWorkspaceUpdate(t *testing.T, args ...string) (string, error) {
	t.Helper()
	globals = GlobalFlags{JSON: true}
	defer func() { globals = GlobalFlags{} }()

	var buf bytes.Buffer
	cmd := newWorkspaceCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(append([]string{"update"}, args...))
	err := cmd.Execute()
	return buf.String(), err
}

// execWorkspaceDelete runs 'workspace delete <id>' and returns stdout, exit err.
func execWorkspaceDelete(t *testing.T, id string) (string, error) {
	t.Helper()
	globals = GlobalFlags{JSON: true}
	defer func() { globals = GlobalFlags{} }()

	var buf bytes.Buffer
	cmd := newWorkspaceCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"delete", id})
	err := cmd.Execute()
	return buf.String(), err
}

// ---- WorkspacesList tests ----

func TestWorkspaceList_Empty(t *testing.T) {
	srv := newTestServer(t, func(body map[string]any) string {
		opName, _ := body["operationName"].(string)
		if opName != "WorkspacesList" {
			t.Errorf("unexpected operationName: %q", opName)
		}
		return `{"workspaces":[]}`
	})
	installWorkspaceFactory(t, srv.URL)

	out, err := execWorkspaceList(t)
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

func TestWorkspaceList_WithResults_NextPage(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		workspaces := []map[string]any{
			{"id": "111", "name": "Alpha", "kind": "open", "description": "", "created_at": "2024-01-01", "state": "active"},
			{"id": "222", "name": "Beta", "kind": "closed", "description": "desc", "created_at": "2024-02-01", "state": "active"},
		}
		return mustMarshal(map[string]any{"workspaces": workspaces})
	})
	installWorkspaceFactory(t, srv.URL)

	out, err := execWorkspaceList(t, "--limit", "2")
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
	if result["cursor"] != "2" {
		t.Errorf("expected cursor '2', got %v", result["cursor"])
	}
}

func TestWorkspaceList_KindFilter(t *testing.T) {
	var capturedVars map[string]any

	srv := newTestServer(t, func(body map[string]any) string {
		if vars, ok := body["variables"].(map[string]any); ok {
			capturedVars = vars
		}
		return `{"workspaces":[]}`
	})
	installWorkspaceFactory(t, srv.URL)

	_, err := execWorkspaceList(t, "--kind", "open")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedVars == nil {
		t.Fatal("no variables captured")
	}
	if capturedVars["kind"] != "open" {
		t.Errorf("expected kind=open, got %v", capturedVars["kind"])
	}
}

func TestWorkspaceList_InvalidKind_UsageError(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return `{"workspaces":[]}`
	})
	installWorkspaceFactory(t, srv.URL)

	_, err := execWorkspaceList(t, "--kind", "bogus")
	if err == nil {
		t.Fatal("expected error for invalid kind")
	}
	if errsCode(err) != "USAGE" {
		t.Errorf("expected USAGE, got %q", errsCode(err))
	}
}

func TestWorkspaceList_BadCursor_ExitCode1(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return `{"workspaces":[]}`
	})
	installWorkspaceFactory(t, srv.URL)

	_, err := execWorkspaceList(t, "--cursor", "notanumber")
	if err == nil {
		t.Fatal("expected error for bad cursor")
	}
	if errsCode(err) != "USAGE" {
		t.Errorf("expected USAGE, got %q", errsCode(err))
	}
}

func TestWorkspaceList_CursorPagePassthrough(t *testing.T) {
	var capturedPage float64

	srv := newTestServer(t, func(body map[string]any) string {
		if vars, ok := body["variables"].(map[string]any); ok {
			capturedPage, _ = vars["page"].(float64)
		}
		return `{"workspaces":[]}`
	})
	installWorkspaceFactory(t, srv.URL)

	_, err := execWorkspaceList(t, "--cursor", "4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedPage != 4 {
		t.Errorf("expected page=4, got %v", capturedPage)
	}
}

// ---- WorkspaceGet tests ----

func TestWorkspaceGet_Success(t *testing.T) {
	srv := newTestServer(t, func(body map[string]any) string {
		opName, _ := body["operationName"].(string)
		if opName != "WorkspaceGet" {
			t.Errorf("unexpected operationName: %q", opName)
		}
		ws := map[string]any{
			"id":          "42",
			"name":        "Main WS",
			"kind":        "open",
			"description": "The main workspace",
			"created_at":  "2024-01-01",
			"state":       "active",
		}
		return mustMarshal(map[string]any{"workspaces": []any{ws}})
	})
	installWorkspaceFactory(t, srv.URL)

	out, err := execWorkspaceGet(t, "42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	if result["id"] != "42" {
		t.Errorf("expected id=42, got %v", result["id"])
	}
	if result["name"] != "Main WS" {
		t.Errorf("expected name='Main WS', got %v", result["name"])
	}
	if result["kind"] != "open" {
		t.Errorf("expected kind='open', got %v", result["kind"])
	}
}

func TestWorkspaceGet_NotFound(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return `{"workspaces":[]}`
	})
	installWorkspaceFactory(t, srv.URL)

	_, err := execWorkspaceGet(t, "9999999999")
	if err == nil {
		t.Fatal("expected error for missing workspace")
	}
	if errsCode(err) != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND, got %q", errsCode(err))
	}
}

func TestWorkspaceGet_NonNumericID_UsageError(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return `{"workspaces":[]}`
	})
	installWorkspaceFactory(t, srv.URL)

	_, err := execWorkspaceGet(t, "not-a-number")
	if err == nil {
		t.Fatal("expected error for non-numeric id")
	}
	if errsCode(err) != "USAGE" {
		t.Errorf("expected USAGE, got %q", errsCode(err))
	}
}

// ---- WorkspaceCreate tests ----

func TestWorkspaceCreate_Success(t *testing.T) {
	var capturedVars map[string]any

	srv := newTestServer(t, func(body map[string]any) string {
		opName, _ := body["operationName"].(string)
		if opName != "WorkspaceCreate" {
			t.Errorf("unexpected operationName: %q", opName)
		}
		if vars, ok := body["variables"].(map[string]any); ok {
			capturedVars = vars
		}
		ws := map[string]any{
			"id":          "100",
			"name":        "My WS",
			"kind":        "open",
			"description": "",
			"created_at":  "2024-05-01",
			"state":       "active",
		}
		return mustMarshal(map[string]any{"create_workspace": ws})
	})
	installWorkspaceFactory(t, srv.URL)

	out, err := execWorkspaceCreate(t, "--name", "My WS", "--kind", "open")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	if result["id"] != "100" {
		t.Errorf("expected id=100, got %v", result["id"])
	}
	if result["name"] != "My WS" {
		t.Errorf("expected name='My WS', got %v", result["name"])
	}

	if capturedVars["name"] != "My WS" {
		t.Errorf("expected variables.name='My WS', got %v", capturedVars["name"])
	}
	if capturedVars["kind"] != "open" {
		t.Errorf("expected variables.kind='open', got %v", capturedVars["kind"])
	}
	// description is omitempty — it should not be present when not set.
	if _, present := capturedVars["description"]; present {
		t.Errorf("expected description absent (omitempty), but it was present: %v", capturedVars["description"])
	}
}

func TestWorkspaceCreate_InvalidKind_UsageError(t *testing.T) {
	called := false
	srv := newTestServer(t, func(_ map[string]any) string {
		called = true
		return `{"create_workspace":null}`
	})
	installWorkspaceFactory(t, srv.URL)

	_, err := execWorkspaceCreate(t, "--name", "X", "--kind", "bogus")
	if err == nil {
		t.Fatal("expected error for invalid --kind")
	}
	if called {
		t.Error("server should not have been called")
	}
	if errsCode(err) != "USAGE" {
		t.Errorf("expected USAGE, got %q", errsCode(err))
	}
}

func TestWorkspaceCreate_MissingName(t *testing.T) {
	called := false
	srv := newTestServer(t, func(_ map[string]any) string {
		called = true
		return `{"create_workspace":null}`
	})
	installWorkspaceFactory(t, srv.URL)

	_, err := execWorkspaceCreate(t, "--kind", "open")
	if err == nil {
		t.Fatal("expected error for missing --name")
	}
	if called {
		t.Error("server should not have been called")
	}
}

func TestWorkspaceCreate_WithDescription(t *testing.T) {
	var capturedVars map[string]any

	srv := newTestServer(t, func(body map[string]any) string {
		if vars, ok := body["variables"].(map[string]any); ok {
			capturedVars = vars
		}
		ws := map[string]any{
			"id":          "200",
			"name":        "Team WS",
			"kind":        "closed",
			"description": "For the team",
			"created_at":  "2024-05-01",
			"state":       "active",
		}
		return mustMarshal(map[string]any{"create_workspace": ws})
	})
	installWorkspaceFactory(t, srv.URL)

	out, err := execWorkspaceCreate(t, "--name", "Team WS", "--kind", "closed", "--description", "For the team")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	if result["description"] != "For the team" {
		t.Errorf("expected description='For the team', got %v", result["description"])
	}
	if capturedVars["description"] != "For the team" {
		t.Errorf("expected variables.description='For the team', got %v", capturedVars["description"])
	}
}

func TestWorkspaceCreate_APIError(t *testing.T) {
	srv := newErrorTestServer(t, "workspace name already taken")

	orig := workspaceClientFactory
	workspaceClientFactory = func() (gqlclient.Client, error) {
		c := apigraphql.New("test-token", "test", apigraphql.WithEndpoint(srv.URL))
		return c.GQL(), nil
	}
	t.Cleanup(func() { workspaceClientFactory = orig })

	_, err := execWorkspaceCreate(t, "--name", "Taken", "--kind", "open")
	if err == nil {
		t.Fatal("expected error from API")
	}
	if errsCode(err) != "API" {
		t.Errorf("expected API, got %q", errsCode(err))
	}
}

// ---- WorkspaceUpdate tests ----

func TestWorkspaceUpdate_Success(t *testing.T) {
	var capturedVars map[string]any

	srv := newTestServer(t, func(body map[string]any) string {
		opName, _ := body["operationName"].(string)
		if opName != "WorkspaceUpdate" {
			t.Errorf("unexpected operationName: %q", opName)
		}
		if vars, ok := body["variables"].(map[string]any); ok {
			capturedVars = vars
		}
		ws := map[string]any{
			"id":          "42",
			"name":        "Renamed WS",
			"kind":        "open",
			"description": "",
			"created_at":  "2024-01-01",
			"state":       "active",
		}
		return mustMarshal(map[string]any{"update_workspace": ws})
	})
	installWorkspaceFactory(t, srv.URL)

	out, err := execWorkspaceUpdate(t, "42", "--name", "Renamed WS")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	if result["name"] != "Renamed WS" {
		t.Errorf("expected name='Renamed WS', got %v", result["name"])
	}

	if capturedVars["id"] != "42" {
		t.Errorf("expected id=42 in variables, got %v", capturedVars["id"])
	}
	attrs, _ := capturedVars["attributes"].(map[string]any)
	if attrs == nil {
		t.Fatal("expected attributes in variables")
	}
	if attrs["name"] != "Renamed WS" {
		t.Errorf("expected attributes.name='Renamed WS', got %v", attrs["name"])
	}
}

func TestWorkspaceUpdate_NoFlags_UsageError(t *testing.T) {
	called := false
	srv := newTestServer(t, func(_ map[string]any) string {
		called = true
		return `{"update_workspace":null}`
	})
	installWorkspaceFactory(t, srv.URL)

	_, err := execWorkspaceUpdate(t, "42")
	if err == nil {
		t.Fatal("expected error when no update flags given")
	}
	if called {
		t.Error("server should not have been called")
	}
	if errsCode(err) != "USAGE" {
		t.Errorf("expected USAGE, got %q", errsCode(err))
	}
}

func TestWorkspaceUpdate_NonNumericID_UsageError(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return `{"update_workspace":null}`
	})
	installWorkspaceFactory(t, srv.URL)

	_, err := execWorkspaceUpdate(t, "not-a-num", "--name", "X")
	if err == nil {
		t.Fatal("expected error for non-numeric id")
	}
	if errsCode(err) != "USAGE" {
		t.Errorf("expected USAGE, got %q", errsCode(err))
	}
}

func TestWorkspaceUpdate_InvalidKind_UsageError(t *testing.T) {
	called := false
	srv := newTestServer(t, func(_ map[string]any) string {
		called = true
		return `{"update_workspace":null}`
	})
	installWorkspaceFactory(t, srv.URL)

	_, err := execWorkspaceUpdate(t, "42", "--kind", "bad")
	if err == nil {
		t.Fatal("expected error for invalid --kind")
	}
	if called {
		t.Error("server should not have been called")
	}
	if errsCode(err) != "USAGE" {
		t.Errorf("expected USAGE, got %q", errsCode(err))
	}
}

// ---- WorkspaceDelete tests ----

func TestWorkspaceDelete_Success(t *testing.T) {
	srv := newTestServer(t, func(body map[string]any) string {
		opName, _ := body["operationName"].(string)
		if opName != "WorkspaceDelete" {
			t.Errorf("unexpected operationName: %q", opName)
		}
		return mustMarshal(map[string]any{
			"delete_workspace": map[string]any{"id": "42", "name": "Gone WS"},
		})
	})
	installWorkspaceFactory(t, srv.URL)

	out, err := execWorkspaceDelete(t, "42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	if result["id"] != "42" {
		t.Errorf("expected id=42, got %v", result["id"])
	}
	if result["name"] != "Gone WS" {
		t.Errorf("expected name='Gone WS', got %v", result["name"])
	}
}

func TestWorkspaceDelete_NonNumericID_UsageError(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return `{"delete_workspace":null}`
	})
	installWorkspaceFactory(t, srv.URL)

	_, err := execWorkspaceDelete(t, "bad-id")
	if err == nil {
		t.Fatal("expected error for non-numeric id")
	}
	if errsCode(err) != "USAGE" {
		t.Errorf("expected USAGE, got %q", errsCode(err))
	}
}
