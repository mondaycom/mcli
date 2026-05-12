package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

// execBoardGroupList runs 'board group list --board <id>' and returns stdout, exit err.
func execBoardGroupList(t *testing.T, boardID string, extra ...string) (string, error) {
	t.Helper()
	globals = GlobalFlags{JSON: true}
	defer func() { globals = GlobalFlags{} }()

	var buf bytes.Buffer
	cmd := newBoardCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	args := []string{"group", "list", "--board", boardID}
	args = append(args, extra...)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

// execBoardGroupCreate runs 'board group create' with the given flags.
func execBoardGroupCreate(t *testing.T, args ...string) (string, error) {
	t.Helper()
	globals = GlobalFlags{JSON: true}
	defer func() { globals = GlobalFlags{} }()

	var buf bytes.Buffer
	cmd := newBoardCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(append([]string{"group", "create"}, args...))
	err := cmd.Execute()
	return buf.String(), err
}

// execBoardGroupDelete runs 'board group delete' with the given flags.
func execBoardGroupDelete(t *testing.T, args ...string) (string, error) {
	t.Helper()
	globals = GlobalFlags{JSON: true}
	defer func() { globals = GlobalFlags{} }()

	var buf bytes.Buffer
	cmd := newBoardCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(append([]string{"group", "delete"}, args...))
	err := cmd.Execute()
	return buf.String(), err
}

// execBoardGroupArchive runs 'board group archive' with the given flags.
func execBoardGroupArchive(t *testing.T, args ...string) (string, error) {
	t.Helper()
	globals = GlobalFlags{JSON: true}
	defer func() { globals = GlobalFlags{} }()

	var buf bytes.Buffer
	cmd := newBoardCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(append([]string{"group", "archive"}, args...))
	err := cmd.Execute()
	return buf.String(), err
}

// execBoardGroupRename runs 'board group rename' with the given flags.
func execBoardGroupRename(t *testing.T, args ...string) (string, error) {
	t.Helper()
	globals = GlobalFlags{JSON: true}
	defer func() { globals = GlobalFlags{} }()

	var buf bytes.Buffer
	cmd := newBoardCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(append([]string{"group", "rename"}, args...))
	err := cmd.Execute()
	return buf.String(), err
}

// ---- BoardGroupList tests ----

func TestBoardGroupList_Success(t *testing.T) {
	srv := newTestServer(t, func(body map[string]any) string {
		opName, _ := body["operationName"].(string)
		if opName != "BoardGroupList" {
			t.Errorf("unexpected operationName: %q", opName)
		}
		groups := []map[string]any{
			{"id": "topics", "title": "Topics", "color": "#579bfc", "position": "0.1"},
			{"id": "done", "title": "Done", "color": "#00c875", "position": "0.2"},
		}
		boards := []map[string]any{{"id": "9832181507", "groups": groups}}
		return mustMarshal(map[string]any{"boards": boards})
	})
	installBoardFactory(t, srv.URL)

	out, err := execBoardGroupList(t, "9832181507")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	if result["board_id"] != "9832181507" {
		t.Errorf("expected board_id 9832181507, got %v", result["board_id"])
	}
	items, _ := result["items"].([]any)
	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
}

func TestBoardGroupList_BoardNotFound(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return `{"boards":[]}`
	})
	installBoardFactory(t, srv.URL)

	_, err := execBoardGroupList(t, "9999999999")
	if err == nil {
		t.Fatal("expected error for missing board")
	}
	if code := errsCode(err); code != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND, got %q", code)
	}
}

func TestBoardGroupList_MissingBoard_UsageError(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return `{"boards":[]}`
	})
	installBoardFactory(t, srv.URL)

	// Run without --board flag to trigger cobra's required-flag error.
	globals = GlobalFlags{JSON: true}
	defer func() { globals = GlobalFlags{} }()

	var buf bytes.Buffer
	cmd := newBoardCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"group", "list"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing --board flag")
	}
	_ = srv // keep server alive
}

// ---- BoardGroupCreate tests ----

func TestBoardGroupCreate_Success(t *testing.T) {
	var capturedVars map[string]any

	srv := newTestServer(t, func(body map[string]any) string {
		opName, _ := body["operationName"].(string)
		if opName != "BoardGroupCreate" {
			t.Errorf("unexpected operationName: %q", opName)
		}
		if vars, ok := body["variables"].(map[string]any); ok {
			capturedVars = vars
		}
		g := map[string]any{"id": "new_group_123", "title": "Sprint 1", "color": "#579bfc", "position": "0.5"}
		return mustMarshal(map[string]any{"create_group": g})
	})
	installBoardFactory(t, srv.URL)

	out, err := execBoardGroupCreate(t, "--board", "9832181507", "--name", "Sprint 1", "--color", "#579bfc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	if result["id"] != "new_group_123" {
		t.Errorf("expected id new_group_123, got %v", result["id"])
	}
	if result["title"] != "Sprint 1" {
		t.Errorf("expected title 'Sprint 1', got %v", result["title"])
	}
	if capturedVars["boardId"] != "9832181507" {
		t.Errorf("expected variables.boardId='9832181507', got %v", capturedVars["boardId"])
	}
}

func TestBoardGroupCreate_ColorOmitempty(t *testing.T) {
	var capturedVars map[string]any

	srv := newTestServer(t, func(body map[string]any) string {
		if vars, ok := body["variables"].(map[string]any); ok {
			capturedVars = vars
		}
		g := map[string]any{"id": "grp1", "title": "No Color", "color": "", "position": "0.1"}
		return mustMarshal(map[string]any{"create_group": g})
	})
	installBoardFactory(t, srv.URL)

	_, err := execBoardGroupCreate(t, "--board", "9832181507", "--name", "No Color")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// color must be absent (omitempty) when not provided.
	if _, present := capturedVars["color"]; present {
		t.Errorf("expected color absent (omitempty), but it was present: %v", capturedVars["color"])
	}
}

func TestBoardGroupCreate_MissingName_Error(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return `{"create_group":null}`
	})
	installBoardFactory(t, srv.URL)

	_, err := execBoardGroupCreate(t, "--board", "9832181507")
	if err == nil {
		t.Fatal("expected error for missing --name")
	}
}

// ---- BoardGroupDelete tests ----

func TestBoardGroupDelete_Success(t *testing.T) {
	srv := newTestServer(t, func(body map[string]any) string {
		opName, _ := body["operationName"].(string)
		if opName != "BoardGroupDelete" {
			t.Errorf("unexpected operationName: %q", opName)
		}
		g := map[string]any{"id": "topics", "title": "Topics"}
		return mustMarshal(map[string]any{"delete_group": g})
	})
	installBoardFactory(t, srv.URL)

	out, err := execBoardGroupDelete(t, "--board", "9832181507", "--group", "topics")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	if result["id"] != "topics" {
		t.Errorf("expected id 'topics', got %v", result["id"])
	}
}

func TestBoardGroupDelete_MissingGroup_Error(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return `{"delete_group":null}`
	})
	installBoardFactory(t, srv.URL)

	_, err := execBoardGroupDelete(t, "--board", "9832181507")
	if err == nil {
		t.Fatal("expected error for missing --group")
	}
}

// ---- BoardGroupArchive tests ----

func TestBoardGroupArchive_Success(t *testing.T) {
	srv := newTestServer(t, func(body map[string]any) string {
		opName, _ := body["operationName"].(string)
		if opName != "BoardGroupArchive" {
			t.Errorf("unexpected operationName: %q", opName)
		}
		g := map[string]any{"id": "topics", "title": "Topics"}
		return mustMarshal(map[string]any{"archive_group": g})
	})
	installBoardFactory(t, srv.URL)

	out, err := execBoardGroupArchive(t, "--board", "9832181507", "--group", "topics")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	if result["id"] != "topics" {
		t.Errorf("expected id 'topics', got %v", result["id"])
	}
}

func TestBoardGroupArchive_MissingFlags_Error(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return `{"archive_group":null}`
	})
	installBoardFactory(t, srv.URL)

	_, err := execBoardGroupArchive(t, "--board", "9832181507")
	if err == nil {
		t.Fatal("expected error for missing --group")
	}
}

// ---- BoardGroupRename tests ----

func TestBoardGroupRename_Success(t *testing.T) {
	var capturedVars map[string]any

	srv := newTestServer(t, func(body map[string]any) string {
		opName, _ := body["operationName"].(string)
		if opName != "BoardGroupRename" {
			t.Errorf("unexpected operationName: %q", opName)
		}
		if vars, ok := body["variables"].(map[string]any); ok {
			capturedVars = vars
		}
		g := map[string]any{"id": "topics", "title": "New Title", "color": "#579bfc", "position": "0.1"}
		return mustMarshal(map[string]any{"update_group": g})
	})
	installBoardFactory(t, srv.URL)

	out, err := execBoardGroupRename(t, "--board", "9832181507", "--group", "topics", "--name", "New Title")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	if result["title"] != "New Title" {
		t.Errorf("expected title 'New Title', got %v", result["title"])
	}
	if capturedVars["groupId"] != "topics" {
		t.Errorf("expected variables.groupId='topics', got %v", capturedVars["groupId"])
	}
	if capturedVars["name"] != "New Title" {
		t.Errorf("expected variables.name='New Title', got %v", capturedVars["name"])
	}
}

func TestBoardGroupRename_MissingName_Error(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return `{"update_group":null}`
	})
	installBoardFactory(t, srv.URL)

	_, err := execBoardGroupRename(t, "--board", "9832181507", "--group", "topics")
	if err == nil {
		t.Fatal("expected error for missing --name")
	}
}
