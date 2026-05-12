package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

// execBoardColumnList runs 'board column list --board <id>' and returns stdout, exit err.
func execBoardColumnList(t *testing.T, boardID string, extra ...string) (string, error) {
	t.Helper()
	globals = GlobalFlags{JSON: true}
	defer func() { globals = GlobalFlags{} }()

	var buf bytes.Buffer
	cmd := newBoardCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	args := []string{"column", "list", "--board", boardID}
	args = append(args, extra...)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

// execBoardColumnCreate runs 'board column create' with the given flags.
func execBoardColumnCreate(t *testing.T, args ...string) (string, error) {
	t.Helper()
	globals = GlobalFlags{JSON: true}
	defer func() { globals = GlobalFlags{} }()

	var buf bytes.Buffer
	cmd := newBoardCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(append([]string{"column", "create"}, args...))
	err := cmd.Execute()
	return buf.String(), err
}

// execBoardColumnDelete runs 'board column delete' with the given flags.
func execBoardColumnDelete(t *testing.T, args ...string) (string, error) {
	t.Helper()
	globals = GlobalFlags{JSON: true}
	defer func() { globals = GlobalFlags{} }()

	var buf bytes.Buffer
	cmd := newBoardCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(append([]string{"column", "delete"}, args...))
	err := cmd.Execute()
	return buf.String(), err
}

// execBoardColumnRename runs 'board column rename' with the given flags.
func execBoardColumnRename(t *testing.T, args ...string) (string, error) {
	t.Helper()
	globals = GlobalFlags{JSON: true}
	defer func() { globals = GlobalFlags{} }()

	var buf bytes.Buffer
	cmd := newBoardCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(append([]string{"column", "rename"}, args...))
	err := cmd.Execute()
	return buf.String(), err
}

// execBoardColumnDescribe runs 'board column describe' with the given flags.
func execBoardColumnDescribe(t *testing.T, args ...string) (string, error) {
	t.Helper()
	globals = GlobalFlags{JSON: true}
	defer func() { globals = GlobalFlags{} }()

	var buf bytes.Buffer
	cmd := newBoardCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(append([]string{"column", "describe"}, args...))
	err := cmd.Execute()
	return buf.String(), err
}

// ---- BoardColumnList tests ----

func TestBoardColumnList_Success(t *testing.T) {
	srv := newTestServer(t, func(body map[string]any) string {
		opName, _ := body["operationName"].(string)
		if opName != "BoardColumnList" {
			t.Errorf("unexpected operationName: %q", opName)
		}
		cols := []map[string]any{
			{"id": "name", "title": "Name", "type": "name", "settings_str": "{}", "width": 200, "archived": false},
			{"id": "status", "title": "Status", "type": "status", "settings_str": "{}", "width": 150, "archived": false},
		}
		boards := []map[string]any{{"id": "9832181507", "columns": cols}}
		return mustMarshal(map[string]any{"boards": boards})
	})
	installBoardFactory(t, srv.URL)

	out, err := execBoardColumnList(t, "9832181507")
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

func TestBoardColumnList_BoardNotFound(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return `{"boards":[]}`
	})
	installBoardFactory(t, srv.URL)

	_, err := execBoardColumnList(t, "9999999999")
	if err == nil {
		t.Fatal("expected error for missing board")
	}
	if code := errsCode(err); code != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND, got %q", code)
	}
}

// ---- BoardColumnCreate tests ----

func TestBoardColumnCreate_Success(t *testing.T) {
	var capturedVars map[string]any

	srv := newTestServer(t, func(body map[string]any) string {
		opName, _ := body["operationName"].(string)
		if opName != "BoardColumnCreate" {
			t.Errorf("unexpected operationName: %q", opName)
		}
		if vars, ok := body["variables"].(map[string]any); ok {
			capturedVars = vars
		}
		col := map[string]any{"id": "text_col", "title": "My Text", "type": "text", "settings_str": "{}", "width": 100, "archived": false}
		return mustMarshal(map[string]any{"create_column": col})
	})
	installBoardFactory(t, srv.URL)

	out, err := execBoardColumnCreate(t, "--board", "9832181507", "--title", "My Text", "--type", "text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	if result["id"] != "text_col" {
		t.Errorf("expected id text_col, got %v", result["id"])
	}
	if result["type"] != "text" {
		t.Errorf("expected type 'text', got %v", result["type"])
	}
	if capturedVars["boardId"] != "9832181507" {
		t.Errorf("expected variables.boardId='9832181507', got %v", capturedVars["boardId"])
	}
	if capturedVars["columnType"] != "text" {
		t.Errorf("expected variables.columnType='text', got %v", capturedVars["columnType"])
	}
}

func TestBoardColumnCreate_InvalidType_UsageError(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return `{"create_column":null}`
	})
	installBoardFactory(t, srv.URL)

	_, err := execBoardColumnCreate(t, "--board", "9832181507", "--title", "X", "--type", "bogus_type")
	if err == nil {
		t.Fatal("expected error for invalid type")
	}
	if code := errsCode(err); code != "USAGE" {
		t.Errorf("expected USAGE, got %q", code)
	}
}

func TestBoardColumnCreate_InvalidDefaultsJSON_UsageError(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return `{"create_column":null}`
	})
	installBoardFactory(t, srv.URL)

	_, err := execBoardColumnCreate(t, "--board", "9832181507", "--title", "X", "--type", "text", "--defaults", "not-json")
	if err == nil {
		t.Fatal("expected error for invalid JSON defaults")
	}
	if code := errsCode(err); code != "USAGE" {
		t.Errorf("expected USAGE, got %q", code)
	}
}

func TestBoardColumnCreate_DescriptionOmitempty(t *testing.T) {
	var capturedVars map[string]any

	srv := newTestServer(t, func(body map[string]any) string {
		if vars, ok := body["variables"].(map[string]any); ok {
			capturedVars = vars
		}
		col := map[string]any{"id": "status_col", "title": "Status", "type": "status", "settings_str": "{}", "width": 100, "archived": false}
		return mustMarshal(map[string]any{"create_column": col})
	})
	installBoardFactory(t, srv.URL)

	_, err := execBoardColumnCreate(t, "--board", "9832181507", "--title", "Status", "--type", "status")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// description and defaults must be absent (omitempty) when not provided.
	if _, present := capturedVars["description"]; present {
		t.Errorf("expected description absent (omitempty), but it was present: %v", capturedVars["description"])
	}
	if _, present := capturedVars["defaults"]; present {
		t.Errorf("expected defaults absent (omitempty), but it was present: %v", capturedVars["defaults"])
	}
}

func TestBoardColumnCreate_WithDefaults(t *testing.T) {
	var capturedVars map[string]any

	srv := newTestServer(t, func(body map[string]any) string {
		if vars, ok := body["variables"].(map[string]any); ok {
			capturedVars = vars
		}
		col := map[string]any{"id": "dropdown_col", "title": "Category", "type": "dropdown", "settings_str": "{}", "width": 100, "archived": false}
		return mustMarshal(map[string]any{"create_column": col})
	})
	installBoardFactory(t, srv.URL)

	defaults := `{"labels":{"0":"Option A","1":"Option B"}}`
	_, err := execBoardColumnCreate(t, "--board", "9832181507", "--title", "Category", "--type", "dropdown", "--defaults", defaults)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedVars["defaults"] != defaults {
		t.Errorf("expected variables.defaults=%q, got %v", defaults, capturedVars["defaults"])
	}
}

// ---- BoardColumnDelete tests ----

func TestBoardColumnDelete_Success(t *testing.T) {
	srv := newTestServer(t, func(body map[string]any) string {
		opName, _ := body["operationName"].(string)
		if opName != "BoardColumnDelete" {
			t.Errorf("unexpected operationName: %q", opName)
		}
		col := map[string]any{"id": "status", "title": "Status"}
		return mustMarshal(map[string]any{"delete_column": col})
	})
	installBoardFactory(t, srv.URL)

	out, err := execBoardColumnDelete(t, "--board", "9832181507", "--column", "status")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	if result["id"] != "status" {
		t.Errorf("expected id 'status', got %v", result["id"])
	}
}

func TestBoardColumnDelete_MissingColumn_Error(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return `{"delete_column":null}`
	})
	installBoardFactory(t, srv.URL)

	_, err := execBoardColumnDelete(t, "--board", "9832181507")
	if err == nil {
		t.Fatal("expected error for missing --column")
	}
}

// ---- BoardColumnRename tests ----

func TestBoardColumnRename_Success(t *testing.T) {
	var capturedVars map[string]any

	srv := newTestServer(t, func(body map[string]any) string {
		opName, _ := body["operationName"].(string)
		if opName != "BoardColumnRename" {
			t.Errorf("unexpected operationName: %q", opName)
		}
		if vars, ok := body["variables"].(map[string]any); ok {
			capturedVars = vars
		}
		col := map[string]any{"id": "status", "title": "New Title", "type": "status", "settings_str": "{}", "width": 150, "archived": false}
		return mustMarshal(map[string]any{"change_column_title": col})
	})
	installBoardFactory(t, srv.URL)

	out, err := execBoardColumnRename(t, "--board", "9832181507", "--column", "status", "--title", "New Title")
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
	if capturedVars["columnId"] != "status" {
		t.Errorf("expected variables.columnId='status', got %v", capturedVars["columnId"])
	}
}

func TestBoardColumnRename_MissingTitle_Error(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return `{"change_column_title":null}`
	})
	installBoardFactory(t, srv.URL)

	_, err := execBoardColumnRename(t, "--board", "9832181507", "--column", "status")
	if err == nil {
		t.Fatal("expected error for missing --title")
	}
}

// ---- BoardColumnDescribe tests ----

func TestBoardColumnDescribe_Success(t *testing.T) {
	var capturedVars map[string]any

	srv := newTestServer(t, func(body map[string]any) string {
		opName, _ := body["operationName"].(string)
		if opName != "BoardColumnDescribe" {
			t.Errorf("unexpected operationName: %q", opName)
		}
		if vars, ok := body["variables"].(map[string]any); ok {
			capturedVars = vars
		}
		col := map[string]any{"id": "status", "title": "Status", "type": "status", "settings_str": "{}", "width": 150, "archived": false}
		return mustMarshal(map[string]any{"change_column_metadata": col})
	})
	installBoardFactory(t, srv.URL)

	out, err := execBoardColumnDescribe(t, "--board", "9832181507", "--column", "status", "--text", "Track item status")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	if result["id"] != "status" {
		t.Errorf("expected id 'status', got %v", result["id"])
	}
	if capturedVars["description"] != "Track item status" {
		t.Errorf("expected variables.description='Track item status', got %v", capturedVars["description"])
	}
}

func TestBoardColumnDescribe_MissingText_Error(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return `{"change_column_metadata":null}`
	})
	installBoardFactory(t, srv.URL)

	_, err := execBoardColumnDescribe(t, "--board", "9832181507", "--column", "status")
	if err == nil {
		t.Fatal("expected error for missing --text")
	}
}
