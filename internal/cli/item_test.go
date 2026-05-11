package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	gqlclient "github.com/Khan/genqlient/graphql"
)

// installItemFactory sets up itemClientFactory to use the given httptest
// server URL for the duration of the test, then restores the original value.
func installItemFactory(t *testing.T, srvURL string) {
	t.Helper()
	orig := itemClientFactory
	itemClientFactory = func() (gqlclient.Client, error) {
		return gqlclient.NewClient(srvURL, http.DefaultClient), nil
	}
	t.Cleanup(func() { itemClientFactory = orig })
}

// execItemList runs 'item list' with the given flags and returns stdout, exit err.
func execItemList(t *testing.T, args ...string) (string, error) {
	t.Helper()
	globals = GlobalFlags{JSON: true}
	defer func() { globals = GlobalFlags{} }()

	var buf bytes.Buffer
	cmd := newItemCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(append([]string{"list"}, args...))
	err := cmd.Execute()
	return buf.String(), err
}

// execItemGet runs 'item get <id>' and returns stdout, exit err.
func execItemGet(t *testing.T, id string, extraArgs ...string) (string, error) {
	t.Helper()
	globals = GlobalFlags{JSON: true}
	defer func() { globals = GlobalFlags{} }()

	var buf bytes.Buffer
	cmd := newItemCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(append([]string{"get", id}, extraArgs...))
	err := cmd.Execute()
	return buf.String(), err
}

// itemsPageResponse builds the nested GraphQL response for ItemsListByBoard.
func itemsPageResponse(cursor string, items []map[string]any) map[string]any {
	return map[string]any{
		"boards": []any{
			map[string]any{
				"items_page": map[string]any{
					"cursor": cursor,
					"items":  items,
				},
			},
		},
	}
}

// groupItemsPageResponse builds the nested GraphQL response for ItemsListByGroup.
func groupItemsPageResponse(cursor string, items []map[string]any) map[string]any {
	return map[string]any{
		"boards": []any{
			map[string]any{
				"groups": []any{
					map[string]any{
						"id": "topics",
						"items_page": map[string]any{
							"cursor": cursor,
							"items":  items,
						},
					},
				},
			},
		},
	}
}

// sampleItems returns a slice of realistic item maps for use in tests.
// status column has a direct label; date column has a date+time.
func sampleItems() []map[string]any {
	return []map[string]any{
		{
			"id":    "1234567890",
			"name":  "Build feature",
			"state": "active",
			"group": map[string]any{"id": "topics", "title": "Sprint 1"},
			"column_values": []map[string]any{
				{
					"__typename": "StatusValue",
					"id":         "status",
					"type":       "status",
					"value":      `{"label":"Done","index":1}`,
					"text":       "Done",
					"column":     map[string]any{"id": "status", "title": "Status", "settings_str": `{"labels":{"1":"Done"}}`},
				},
				{
					"__typename": "DateValue",
					"id":         "due_date",
					"type":       "date",
					"value":      `{"date":"2026-05-10","time":null}`,
					"text":       "2026-05-10",
					"column":     map[string]any{"id": "due_date", "title": "Due Date", "settings_str": "{}"},
				},
			},
		},
	}
}

// ---- ItemList tests ----

func TestItemList_Empty(t *testing.T) {
	srv := newTestServer(t, func(body map[string]any) string {
		opName, _ := body["operationName"].(string)
		if opName != "ItemsListByBoard" {
			t.Errorf("unexpected operationName: %q", opName)
		}
		return mustMarshal(itemsPageResponse("", nil))
	})
	installItemFactory(t, srv.URL)

	out, err := execItemList(t, "--board", "9832181507")
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

func TestItemList_WithItems(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return mustMarshal(itemsPageResponse("", sampleItems()))
	})
	installItemFactory(t, srv.URL)

	out, err := execItemList(t, "--board", "9832181507")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}

	items, _ := result["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}

	item := items[0].(map[string]any)
	if item["id"] != "1234567890" {
		t.Errorf("expected id 1234567890, got %v", item["id"])
	}
	if item["name"] != "Build feature" {
		t.Errorf("expected name 'Build feature', got %v", item["name"])
	}

	// Verify decoded columns.
	cols, _ := item["columns"].([]any)
	if len(cols) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(cols))
	}

	statusCol := cols[0].(map[string]any)
	if statusCol["id"] != "status" {
		t.Errorf("expected status col id, got %v", statusCol["id"])
	}
	if statusCol["type"] != "status" {
		t.Errorf("expected type 'status', got %v", statusCol["type"])
	}
	if statusCol["value"] != "Done" {
		t.Errorf("expected value 'Done', got %v", statusCol["value"])
	}

	dateCol := cols[1].(map[string]any)
	if dateCol["id"] != "due_date" {
		t.Errorf("expected due_date col id, got %v", dateCol["id"])
	}
	if dateCol["type"] != "date" {
		t.Errorf("expected type 'date', got %v", dateCol["type"])
	}
	if dateCol["value"] != "2026-05-10" {
		t.Errorf("expected value '2026-05-10', got %v", dateCol["value"])
	}
}

func TestItemList_WithGroup(t *testing.T) {
	var capturedVars map[string]any

	srv := newTestServer(t, func(body map[string]any) string {
		opName, _ := body["operationName"].(string)
		if opName != "ItemsListByGroup" {
			t.Errorf("expected ItemsListByGroup, got %q", opName)
		}
		if vars, ok := body["variables"].(map[string]any); ok {
			capturedVars = vars
		}
		return mustMarshal(groupItemsPageResponse("", nil))
	})
	installItemFactory(t, srv.URL)

	_, err := execItemList(t, "--board", "9832181507", "--group", "topics")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedVars == nil {
		t.Fatal("no variables captured")
	}
	if capturedVars["groupId"] != "topics" {
		t.Errorf("expected groupId='topics', got %v", capturedVars["groupId"])
	}
	if capturedVars["boardId"] != "9832181507" {
		t.Errorf("expected boardId='9832181507', got %v", capturedVars["boardId"])
	}
}

func TestItemList_WithCursor(t *testing.T) {
	var capturedVars map[string]any

	srv := newTestServer(t, func(body map[string]any) string {
		if vars, ok := body["variables"].(map[string]any); ok {
			capturedVars = vars
		}
		return mustMarshal(itemsPageResponse("next-cursor-abc", nil))
	})
	installItemFactory(t, srv.URL)

	out, err := execItemList(t, "--board", "9832181507", "--cursor", "opaque-abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedVars["cursor"] != "opaque-abc" {
		t.Errorf("expected cursor='opaque-abc', got %v", capturedVars["cursor"])
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v", err)
	}
	if result["cursor"] != "next-cursor-abc" {
		t.Errorf("expected response cursor 'next-cursor-abc', got %v", result["cursor"])
	}
}

func TestItemList_LimitClamped(t *testing.T) {
	var capturedLimit float64

	srv := newTestServer(t, func(body map[string]any) string {
		if vars, ok := body["variables"].(map[string]any); ok {
			capturedLimit, _ = vars["limit"].(float64)
		}
		return mustMarshal(itemsPageResponse("", nil))
	})
	installItemFactory(t, srv.URL)

	_, err := execItemList(t, "--board", "9832181507", "--limit", "9999")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedLimit != float64(itemListMaxLimit) {
		t.Errorf("expected limit clamped to %d, got %v", itemListMaxLimit, capturedLimit)
	}
}

func TestItemList_InvalidBoard(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return mustMarshal(itemsPageResponse("", nil))
	})
	installItemFactory(t, srv.URL)

	_, err := execItemList(t, "--board", "not-a-number")
	if err == nil {
		t.Fatal("expected error for non-numeric board id")
	}
	code := errsCode(err)
	if code != "USAGE" {
		t.Errorf("expected USAGE, got %q", code)
	}
}

// ---- ItemGet tests ----

func TestItemGet_Success(t *testing.T) {
	srv := newTestServer(t, func(body map[string]any) string {
		opName, _ := body["operationName"].(string)
		if opName != "ItemGet" {
			t.Errorf("unexpected operationName: %q", opName)
		}
		item := map[string]any{
			"id":          "1234567890",
			"name":        "Build feature",
			"state":       "active",
			"created_at":  "2026-05-01",
			"updated_at":  "2026-05-09",
			"creator":     map[string]any{"id": "1001", "name": "Alice"},
			"group":       map[string]any{"id": "topics", "title": "Sprint 1"},
			"board":       map[string]any{"id": "9832181507", "name": "Dev Board"},
			"parent_item": map[string]any{"id": "", "name": ""},
			"subitems": []map[string]any{
				{"id": "5555555555", "name": "Sub 1", "state": "active"},
			},
			"column_values": []map[string]any{
				{
					"__typename": "StatusValue",
					"id":         "status",
					"type":       "status",
					"value":      `{"label":"In Progress","index":2}`,
					"text":       "In Progress",
					"column":     map[string]any{"id": "status", "title": "Status", "settings_str": `{"labels":{"2":"In Progress"}}`},
				},
			},
		}
		return mustMarshal(map[string]any{"items": []any{item}})
	})
	installItemFactory(t, srv.URL)

	out, err := execItemGet(t, "1234567890")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}

	if result["id"] != "1234567890" {
		t.Errorf("expected id 1234567890, got %v", result["id"])
	}
	if result["name"] != "Build feature" {
		t.Errorf("expected name 'Build feature', got %v", result["name"])
	}

	creator, _ := result["creator"].(map[string]any)
	if creator == nil || creator["name"] != "Alice" {
		t.Errorf("expected creator.name='Alice', got %v", creator)
	}

	board, _ := result["board"].(map[string]any)
	if board == nil || board["id"] != "9832181507" {
		t.Errorf("expected board.id='9832181507', got %v", board)
	}

	cols, _ := result["columns"].([]any)
	if len(cols) != 1 {
		t.Fatalf("expected 1 column, got %d", len(cols))
	}
	col := cols[0].(map[string]any)
	if col["value"] != "In Progress" {
		t.Errorf("expected column value 'In Progress', got %v", col["value"])
	}
}

func TestItemGet_NotFound(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return `{"items":[]}`
	})
	installItemFactory(t, srv.URL)

	_, err := execItemGet(t, "9999999999")
	if err == nil {
		t.Fatal("expected error for missing item")
	}
	code := errsCode(err)
	if code != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND, got %q", code)
	}
}

func TestItemGet_InvalidID(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return `{"items":[]}`
	})
	installItemFactory(t, srv.URL)

	_, err := execItemGet(t, "not-a-number")
	if err == nil {
		t.Fatal("expected error for non-numeric id")
	}
	code := errsCode(err)
	if code != "USAGE" {
		t.Errorf("expected USAGE, got %q", code)
	}
}

func TestItemGet_Subitems(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		item := map[string]any{
			"id":          "1234567890",
			"name":        "Parent item",
			"state":       "active",
			"created_at":  "2026-05-01",
			"updated_at":  "2026-05-09",
			"creator":     map[string]any{"id": "1001", "name": "Alice"},
			"group":       map[string]any{"id": "topics", "title": "Sprint 1"},
			"board":       map[string]any{"id": "9832181507", "name": "Dev Board"},
			"parent_item": map[string]any{"id": "", "name": ""},
			"subitems": []map[string]any{
				{"id": "5555555555", "name": "Sub A", "state": "active"},
				{"id": "6666666666", "name": "Sub B", "state": "done"},
				{"id": "7777777777", "name": "Sub C", "state": "archived"},
			},
			"column_values": []map[string]any{},
		}
		return mustMarshal(map[string]any{"items": []any{item}})
	})
	installItemFactory(t, srv.URL)

	out, err := execItemGet(t, "1234567890")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}

	subitems, _ := result["subitems"].([]any)
	if len(subitems) != 3 {
		t.Fatalf("expected 3 subitems, got %d", len(subitems))
	}

	names := make([]string, len(subitems))
	for i, s := range subitems {
		sm := s.(map[string]any)
		names[i] = fmt.Sprintf("%v", sm["name"])
	}
	expected := []string{"Sub A", "Sub B", "Sub C"}
	for i, exp := range expected {
		if names[i] != exp {
			t.Errorf("subitems[%d] name: expected %q, got %q", i, exp, names[i])
		}
	}

	// Verify states round-trip.
	states := []string{"active", "done", "archived"}
	for i, exp := range states {
		sm := subitems[i].(map[string]any)
		if sm["state"] != exp {
			t.Errorf("subitems[%d] state: expected %q, got %q", i, exp, sm["state"])
		}
	}
}
