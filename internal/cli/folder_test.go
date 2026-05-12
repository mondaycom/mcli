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

// installFolderFactory sets up folderClientFactory to use the given httptest
// server URL for the duration of the test, then restores the original value.
func installFolderFactory(t *testing.T, srvURL string) {
	t.Helper()
	orig := folderClientFactory
	folderClientFactory = func() (gqlclient.Client, error) {
		return gqlclient.NewClient(srvURL, http.DefaultClient), nil
	}
	t.Cleanup(func() { folderClientFactory = orig })
}

// execFolderList runs 'folder list' with the given flags and returns stdout, exit err.
func execFolderList(t *testing.T, args ...string) (string, error) {
	t.Helper()
	globals = GlobalFlags{JSON: true}
	defer func() { globals = GlobalFlags{} }()

	var buf bytes.Buffer
	cmd := newFolderCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(append([]string{"list"}, args...))
	err := cmd.Execute()
	return buf.String(), err
}

// execFolderCreate runs 'folder create' with the given flags and returns stdout, exit err.
func execFolderCreate(t *testing.T, args ...string) (string, error) {
	t.Helper()
	globals = GlobalFlags{JSON: true}
	defer func() { globals = GlobalFlags{} }()

	var buf bytes.Buffer
	cmd := newFolderCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(append([]string{"create"}, args...))
	err := cmd.Execute()
	return buf.String(), err
}

// execFolderRename runs 'folder rename <id> <name>' and returns stdout, exit err.
func execFolderRename(t *testing.T, id, name string) (string, error) {
	t.Helper()
	globals = GlobalFlags{JSON: true}
	defer func() { globals = GlobalFlags{} }()

	var buf bytes.Buffer
	cmd := newFolderCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"rename", id, name})
	err := cmd.Execute()
	return buf.String(), err
}

// execFolderDelete runs 'folder delete <id>' and returns stdout, exit err.
func execFolderDelete(t *testing.T, id string) (string, error) {
	t.Helper()
	globals = GlobalFlags{JSON: true}
	defer func() { globals = GlobalFlags{} }()

	var buf bytes.Buffer
	cmd := newFolderCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"delete", id})
	err := cmd.Execute()
	return buf.String(), err
}

// ---- FoldersList tests ----

func TestFolderList_Empty(t *testing.T) {
	srv := newTestServer(t, func(body map[string]any) string {
		opName, _ := body["operationName"].(string)
		if opName != "FoldersList" {
			t.Errorf("unexpected operationName: %q", opName)
		}
		return `{"folders":[]}`
	})
	installFolderFactory(t, srv.URL)

	out, err := execFolderList(t)
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

func TestFolderList_WithResults_NextPage(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		folders := []map[string]any{
			{"id": "10", "name": "Folder A", "color": "green", "created_at": "2024-01-01", "owner_id": "1001"},
			{"id": "20", "name": "Folder B", "color": "red", "created_at": "2024-02-01", "owner_id": "1002"},
		}
		return mustMarshal(map[string]any{"folders": folders})
	})
	installFolderFactory(t, srv.URL)

	out, err := execFolderList(t, "--limit", "2")
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

func TestFolderList_WorkspaceFilter(t *testing.T) {
	var capturedVars map[string]any

	srv := newTestServer(t, func(body map[string]any) string {
		if vars, ok := body["variables"].(map[string]any); ok {
			capturedVars = vars
		}
		return `{"folders":[]}`
	})
	installFolderFactory(t, srv.URL)

	_, err := execFolderList(t, "--workspace", "55")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedVars == nil {
		t.Fatal("no variables captured")
	}
	wsIDs, _ := capturedVars["workspaceIds"].([]any)
	if len(wsIDs) != 1 || wsIDs[0] != "55" {
		t.Errorf("expected workspaceIds=['55'], got %v", wsIDs)
	}
}

func TestFolderList_BadCursor_UsageError(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return `{"folders":[]}`
	})
	installFolderFactory(t, srv.URL)

	_, err := execFolderList(t, "--cursor", "notanumber")
	if err == nil {
		t.Fatal("expected error for bad cursor")
	}
	if errsCode(err) != "USAGE" {
		t.Errorf("expected USAGE, got %q", errsCode(err))
	}
}

func TestFolderList_CursorPagePassthrough(t *testing.T) {
	var capturedPage float64

	srv := newTestServer(t, func(body map[string]any) string {
		if vars, ok := body["variables"].(map[string]any); ok {
			capturedPage, _ = vars["page"].(float64)
		}
		return `{"folders":[]}`
	})
	installFolderFactory(t, srv.URL)

	_, err := execFolderList(t, "--cursor", "5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedPage != 5 {
		t.Errorf("expected page=5, got %v", capturedPage)
	}
}

// ---- FolderCreate tests ----

func TestFolderCreate_Success(t *testing.T) {
	var capturedVars map[string]any

	srv := newTestServer(t, func(body map[string]any) string {
		opName, _ := body["operationName"].(string)
		if opName != "FolderCreate" {
			t.Errorf("unexpected operationName: %q", opName)
		}
		if vars, ok := body["variables"].(map[string]any); ok {
			capturedVars = vars
		}
		f := map[string]any{
			"id":         "99",
			"name":       "Sprint",
			"color":      "green",
			"created_at": "2024-05-01",
			"owner_id":   "1001",
		}
		return mustMarshal(map[string]any{"create_folder": f})
	})
	installFolderFactory(t, srv.URL)

	out, err := execFolderCreate(t, "--name", "Sprint")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	if result["id"] != "99" {
		t.Errorf("expected id=99, got %v", result["id"])
	}
	if result["name"] != "Sprint" {
		t.Errorf("expected name='Sprint', got %v", result["name"])
	}

	if capturedVars["name"] != "Sprint" {
		t.Errorf("expected variables.name='Sprint', got %v", capturedVars["name"])
	}
	// workspaceId is omitempty — must be absent when not set.
	if _, present := capturedVars["workspaceId"]; present {
		t.Errorf("expected workspaceId absent (omitempty), but present: %v", capturedVars["workspaceId"])
	}
	// parentFolderId is omitempty — must be absent when not set.
	if _, present := capturedVars["parentFolderId"]; present {
		t.Errorf("expected parentFolderId absent (omitempty), but present: %v", capturedVars["parentFolderId"])
	}
}

func TestFolderCreate_WithWorkspaceAndParent(t *testing.T) {
	var capturedVars map[string]any

	srv := newTestServer(t, func(body map[string]any) string {
		if vars, ok := body["variables"].(map[string]any); ok {
			capturedVars = vars
		}
		f := map[string]any{
			"id":         "101",
			"name":       "Sub",
			"color":      "",
			"created_at": "2024-05-01",
			"owner_id":   "",
		}
		return mustMarshal(map[string]any{"create_folder": f})
	})
	installFolderFactory(t, srv.URL)

	out, err := execFolderCreate(t, "--name", "Sub", "--workspace", "55", "--parent", "77")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	if result["id"] != "101" {
		t.Errorf("expected id=101, got %v", result["id"])
	}

	if capturedVars["workspaceId"] != "55" {
		t.Errorf("expected variables.workspaceId='55', got %v", capturedVars["workspaceId"])
	}
	if capturedVars["parentFolderId"] != "77" {
		t.Errorf("expected variables.parentFolderId='77', got %v", capturedVars["parentFolderId"])
	}
}

func TestFolderCreate_MissingName(t *testing.T) {
	called := false
	srv := newTestServer(t, func(_ map[string]any) string {
		called = true
		return `{"create_folder":null}`
	})
	installFolderFactory(t, srv.URL)

	_, err := execFolderCreate(t)
	if err == nil {
		t.Fatal("expected error for missing --name")
	}
	if called {
		t.Error("server should not have been called")
	}
}

func TestFolderCreate_APIError(t *testing.T) {
	srv := newErrorTestServer(t, "folder creation failed")

	orig := folderClientFactory
	folderClientFactory = func() (gqlclient.Client, error) {
		c := apigraphql.New("test-token", "test", apigraphql.WithEndpoint(srv.URL))
		return c.GQL(), nil
	}
	t.Cleanup(func() { folderClientFactory = orig })

	_, err := execFolderCreate(t, "--name", "Bad")
	if err == nil {
		t.Fatal("expected error from API")
	}
	if errsCode(err) != "API" {
		t.Errorf("expected API, got %q", errsCode(err))
	}
}

// ---- FolderRename tests ----

func TestFolderRename_Success(t *testing.T) {
	var capturedVars map[string]any

	srv := newTestServer(t, func(body map[string]any) string {
		opName, _ := body["operationName"].(string)
		if opName != "FolderRename" {
			t.Errorf("unexpected operationName: %q", opName)
		}
		if vars, ok := body["variables"].(map[string]any); ok {
			capturedVars = vars
		}
		f := map[string]any{
			"id":         "10",
			"name":       "New Name",
			"color":      "blue",
			"created_at": "2024-01-01",
			"owner_id":   "1001",
		}
		return mustMarshal(map[string]any{"update_folder": f})
	})
	installFolderFactory(t, srv.URL)

	out, err := execFolderRename(t, "10", "New Name")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	if result["id"] != "10" {
		t.Errorf("expected id=10, got %v", result["id"])
	}
	if result["name"] != "New Name" {
		t.Errorf("expected name='New Name', got %v", result["name"])
	}

	if capturedVars["folderId"] != "10" {
		t.Errorf("expected variables.folderId='10', got %v", capturedVars["folderId"])
	}
	if capturedVars["name"] != "New Name" {
		t.Errorf("expected variables.name='New Name', got %v", capturedVars["name"])
	}
}

func TestFolderRename_NonNumericID_UsageError(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return `{"update_folder":null}`
	})
	installFolderFactory(t, srv.URL)

	_, err := execFolderRename(t, "bad-id", "Name")
	if err == nil {
		t.Fatal("expected error for non-numeric id")
	}
	if errsCode(err) != "USAGE" {
		t.Errorf("expected USAGE, got %q", errsCode(err))
	}
}

// ---- FolderDelete tests ----

func TestFolderDelete_Success(t *testing.T) {
	srv := newTestServer(t, func(body map[string]any) string {
		opName, _ := body["operationName"].(string)
		if opName != "FolderDelete" {
			t.Errorf("unexpected operationName: %q", opName)
		}
		return mustMarshal(map[string]any{
			"delete_folder": map[string]any{"id": "10", "name": "Old Folder"},
		})
	})
	installFolderFactory(t, srv.URL)

	out, err := execFolderDelete(t, "10")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	if result["id"] != "10" {
		t.Errorf("expected id=10, got %v", result["id"])
	}
	if result["name"] != "Old Folder" {
		t.Errorf("expected name='Old Folder', got %v", result["name"])
	}
}

func TestFolderDelete_NonNumericID_UsageError(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return `{"delete_folder":null}`
	})
	installFolderFactory(t, srv.URL)

	_, err := execFolderDelete(t, "not-a-number")
	if err == nil {
		t.Fatal("expected error for non-numeric id")
	}
	if errsCode(err) != "USAGE" {
		t.Errorf("expected USAGE, got %q", errsCode(err))
	}
}
