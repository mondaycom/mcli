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
)

// execItemCreate runs 'item create' with the given args and returns stdout, exit err.
func execItemCreate(t *testing.T, args ...string) (string, error) {
	t.Helper()
	globals = GlobalFlags{JSON: true}
	defer func() { globals = GlobalFlags{} }()

	var buf bytes.Buffer
	cmd := newItemCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(append([]string{"create"}, args...))
	err := cmd.Execute()
	return buf.String(), err
}

// execItemUpdate runs 'item update <id>' with the given args and returns stdout, exit err.
func execItemUpdate(t *testing.T, args ...string) (string, error) {
	t.Helper()
	globals = GlobalFlags{JSON: true}
	defer func() { globals = GlobalFlags{} }()

	var buf bytes.Buffer
	cmd := newItemCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(append([]string{"update"}, args...))
	err := cmd.Execute()
	return buf.String(), err
}

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

// execItemListMode runs 'item list' with an explicit output mode via globals.
func execItemListMode(t *testing.T, g GlobalFlags, args ...string) (string, error) {
	t.Helper()
	globals = g
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

func TestItemList_JSON_HasColumnTitle(t *testing.T) {
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
		t.Fatalf("parse output: %v", err)
	}
	items, _ := result["items"].([]any)
	statusCol := items[0].(map[string]any)["columns"].([]any)[0].(map[string]any)
	if statusCol["title"] != "Status" {
		t.Errorf("expected column title 'Status', got %v", statusCol["title"])
	}
}

func TestItemList_Pretty_ShowsColumnValues(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return mustMarshal(itemsPageResponse("", sampleItems()))
	})
	installItemFactory(t, srv.URL)

	out, err := execItemListMode(t, GlobalFlags{Pretty: true}, "--board", "9832181507")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(out, "column(s)") {
		t.Errorf("pretty output still shows the column count summary:\n%s", out)
	}
	for _, want := range []string{"Status", "Due Date", "Done", "2026-05-10"} {
		if !strings.Contains(out, want) {
			t.Errorf("pretty output missing %q:\n%s", want, out)
		}
	}
}

func TestItemList_Terse_ShowsColumnValues(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return mustMarshal(itemsPageResponse("", sampleItems()))
	})
	installItemFactory(t, srv.URL)

	out, err := execItemListMode(t, GlobalFlags{Terse: true}, "--board", "9832181507")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "#1234567890 Build feature · active · Sprint 1 · Done · 2026-05-10\n"
	if out != want {
		t.Errorf("terse output:\n got %q\nwant %q", out, want)
	}
	// Labels belong to pretty/JSON, not terse.
	if strings.Contains(out, "Status=") || strings.Contains(out, "=") {
		t.Errorf("terse output should be values-only, got:\n%s", out)
	}
}

func TestItemList_CSV_HasDynamicColumns(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return mustMarshal(itemsPageResponse("", sampleItems()))
	})
	installItemFactory(t, srv.URL)

	out, err := execItemListMode(t, GlobalFlags{CSV: true}, "--board", "9832181507")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected header + 1 row, got %d lines:\n%s", len(lines), out)
	}
	if lines[0] != "id,name,state,group,Status,Due Date" {
		t.Errorf("unexpected CSV header: %q", lines[0])
	}
	if lines[1] != "1234567890,Build feature,active,Sprint 1,Done,2026-05-10" {
		t.Errorf("unexpected CSV row: %q", lines[1])
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

// ---- parseColFlags unit tests ----

func TestParseColFlags_Single(t *testing.T) {
	m, err := parseColFlags([]string{`status={"label":"Done"}`})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(m))
	}
	if string(m["status"]) != `{"label":"Done"}` {
		t.Errorf("unexpected value: %s", m["status"])
	}
}

func TestParseColFlags_Multi(t *testing.T) {
	m, err := parseColFlags([]string{`status={"label":"Done"}`, `due={"date":"2026-05-10"}`})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(m))
	}
}

func TestParseColFlags_DuplicateLastWins(t *testing.T) {
	m, err := parseColFlags([]string{`status={"label":"Done"}`, `status={"label":"In Progress"}`})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(m["status"]) != `{"label":"In Progress"}` {
		t.Errorf("expected last value to win, got: %s", m["status"])
	}
}

func TestParseColFlags_MalformedJSON(t *testing.T) {
	_, err := parseColFlags([]string{`status=not-json`})
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	code := errsCode(err)
	if code != "USAGE" {
		t.Errorf("expected USAGE, got %q", code)
	}
}

func TestParseColFlags_MissingEquals(t *testing.T) {
	_, err := parseColFlags([]string{`statusonly`})
	if err == nil {
		t.Fatal("expected error for missing =")
	}
	code := errsCode(err)
	if code != "USAGE" {
		t.Errorf("expected USAGE, got %q", code)
	}
}

func TestParseColFlags_EmptyColID(t *testing.T) {
	_, err := parseColFlags([]string{`={"label":"Done"}`})
	if err == nil {
		t.Fatal("expected error for empty col id")
	}
	code := errsCode(err)
	if code != "USAGE" {
		t.Errorf("expected USAGE, got %q", code)
	}
}

// ---- item create tests ----

// sampleItemCreateResponse builds the GraphQL response for ItemCreate.
func sampleItemCreateResponse(id, name string) map[string]any {
	return map[string]any{
		"create_item": map[string]any{
			"id":    id,
			"name":  name,
			"state": "active",
			"group": map[string]any{"id": "topics", "title": "Sprint 1"},
			"board": map[string]any{"id": "9832181507", "name": "Dev Board"},
		},
	}
}

// sampleSubitemCreateResponse builds the GraphQL response for SubitemCreate.
func sampleSubitemCreateResponse(id, name, parentID string) map[string]any {
	return map[string]any{
		"create_subitem": map[string]any{
			"id":          id,
			"name":        name,
			"state":       "active",
			"parent_item": map[string]any{"id": parentID, "name": "Parent item"},
			"board":       map[string]any{"id": "9832181507", "name": "Dev Board"},
		},
	}
}

func TestItemCreate_HappyPathBoard(t *testing.T) {
	var capturedVars map[string]any

	srv := newTestServer(t, func(body map[string]any) string {
		opName, _ := body["operationName"].(string)
		if opName != "ItemCreate" {
			t.Errorf("expected ItemCreate op, got %q", opName)
		}
		if vars, ok := body["variables"].(map[string]any); ok {
			capturedVars = vars
		}
		return mustMarshal(sampleItemCreateResponse("111222333", "My Task"))
	})
	installItemFactory(t, srv.URL)

	out, err := execItemCreate(t, "--board", "9832181507", "--name", "My Task",
		"--col", `status={"label":"Done"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify request shape.
	if capturedVars["boardId"] != "9832181507" {
		t.Errorf("expected boardId='9832181507', got %v", capturedVars["boardId"])
	}
	if capturedVars["name"] != "My Task" {
		t.Errorf("expected name='My Task', got %v", capturedVars["name"])
	}
	// columnValues should be a stringified JSON object.
	colVals, _ := capturedVars["columnValues"].(string)
	if colVals == "" {
		t.Error("expected columnValues to be non-empty string")
	}
	var colMap map[string]any
	if err := json.Unmarshal([]byte(colVals), &colMap); err != nil {
		t.Errorf("columnValues is not valid JSON object: %v", err)
	}
	if _, ok := colMap["status"]; !ok {
		t.Error("expected 'status' key in columnValues")
	}

	// Verify output.
	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	if result["id"] != "111222333" {
		t.Errorf("expected id='111222333', got %v", result["id"])
	}
	if result["state"] != "active" {
		t.Errorf("expected state='active', got %v", result["state"])
	}
}

func TestItemCreate_WithMultipleColFlags(t *testing.T) {
	var capturedVars map[string]any

	srv := newTestServer(t, func(body map[string]any) string {
		if vars, ok := body["variables"].(map[string]any); ok {
			capturedVars = vars
		}
		return mustMarshal(sampleItemCreateResponse("111222333", "Foo"))
	})
	installItemFactory(t, srv.URL)

	_, err := execItemCreate(t, "--board", "9832181507", "--name", "Foo",
		"--col", `status={"label":"Done"}`,
		"--col", `due={"date":"2026-05-10"}`,
		"--col", `points=5`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	colVals, _ := capturedVars["columnValues"].(string)
	var colMap map[string]any
	if err := json.Unmarshal([]byte(colVals), &colMap); err != nil {
		t.Fatalf("columnValues is not valid JSON: %v", err)
	}
	if len(colMap) != 3 {
		t.Errorf("expected 3 columns, got %d", len(colMap))
	}
}

func TestItemCreate_WithParent(t *testing.T) {
	var capturedOp string

	srv := newTestServer(t, func(body map[string]any) string {
		capturedOp, _ = body["operationName"].(string)
		return mustMarshal(sampleSubitemCreateResponse("999888777", "Sub Task", "1234567890"))
	})
	installItemFactory(t, srv.URL)

	out, err := execItemCreate(t, "--parent", "1234567890", "--name", "Sub Task")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedOp != "SubitemCreate" {
		t.Errorf("expected SubitemCreate operation, got %q", capturedOp)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	parent, _ := result["parent_item"].(map[string]any)
	if parent == nil {
		t.Fatal("expected parent_item in subitem output")
	}
	if parent["id"] != "1234567890" {
		t.Errorf("expected parent_item.id='1234567890', got %v", parent["id"])
	}
	// group should be omitted for subitems.
	if _, ok := result["group"]; ok {
		t.Error("expected no group field in subitem output")
	}
}

func TestItemCreate_BothBoardAndParent(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return mustMarshal(sampleItemCreateResponse("1", "x"))
	})
	installItemFactory(t, srv.URL)

	_, err := execItemCreate(t, "--board", "9832181507", "--parent", "1234567890", "--name", "x")
	if err == nil {
		t.Fatal("expected error when both --board and --parent are given")
	}
	// cobra enforces mutual exclusivity; error need not be USAGE-coded but must be non-nil.
}

func TestItemCreate_NeitherBoardNorParent(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return mustMarshal(sampleItemCreateResponse("1", "x"))
	})
	installItemFactory(t, srv.URL)

	_, err := execItemCreate(t, "--name", "x")
	if err == nil {
		t.Fatal("expected error when neither --board nor --parent is given")
	}
	code := errsCode(err)
	if code != "USAGE" {
		t.Errorf("expected USAGE, got %q", code)
	}
}

func TestItemCreate_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"errors":[{"message":"access denied","extensions":{"code":"Unauthorized"}}]}`)
	}))
	t.Cleanup(srv.Close)
	installItemFactory(t, srv.URL)

	_, err := execItemCreate(t, "--board", "9832181507", "--name", "Fail")
	if err == nil {
		t.Fatal("expected error from API error response")
	}
}

// ---- item update tests ----

// sampleItemUpdateResponse builds the GraphQL response for ItemUpdate.
func sampleItemUpdateResponse(id, name string) map[string]any {
	return map[string]any{
		"change_multiple_column_values": map[string]any{
			"id":    id,
			"name":  name,
			"state": "active",
			"board": map[string]any{"id": "9832181507", "name": "Dev Board"},
			"group": map[string]any{"id": "topics", "title": "Sprint 1"},
		},
	}
}

func TestItemUpdate_HappyPathNameAndCol(t *testing.T) {
	var capturedVars map[string]any

	srv := newTestServer(t, func(body map[string]any) string {
		opName, _ := body["operationName"].(string)
		if opName != "ItemUpdate" {
			t.Errorf("expected ItemUpdate op, got %q", opName)
		}
		if vars, ok := body["variables"].(map[string]any); ok {
			capturedVars = vars
		}
		return mustMarshal(sampleItemUpdateResponse("1234567890", "Renamed Task"))
	})
	installItemFactory(t, srv.URL)

	out, err := execItemUpdate(t, "1234567890", "--board", "9832181507",
		"--name", "Renamed Task",
		"--col", `status={"label":"Done"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify columnValues includes both name and status.
	colVals, _ := capturedVars["columnValues"].(string)
	var colMap map[string]any
	if err := json.Unmarshal([]byte(colVals), &colMap); err != nil {
		t.Fatalf("columnValues is not valid JSON: %v", err)
	}
	if _, ok := colMap["name"]; !ok {
		t.Error("expected 'name' key in columnValues for --name flag")
	}
	if _, ok := colMap["status"]; !ok {
		t.Error("expected 'status' key in columnValues")
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	if result["id"] != "1234567890" {
		t.Errorf("expected id='1234567890', got %v", result["id"])
	}
}

func TestItemUpdate_NameOnly(t *testing.T) {
	var capturedVars map[string]any

	srv := newTestServer(t, func(body map[string]any) string {
		if vars, ok := body["variables"].(map[string]any); ok {
			capturedVars = vars
		}
		return mustMarshal(sampleItemUpdateResponse("1234567890", "New Name"))
	})
	installItemFactory(t, srv.URL)

	_, err := execItemUpdate(t, "1234567890", "--board", "9832181507", "--name", "New Name")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	colVals, _ := capturedVars["columnValues"].(string)
	var colMap map[string]any
	if err := json.Unmarshal([]byte(colVals), &colMap); err != nil {
		t.Fatalf("columnValues is not valid JSON: %v", err)
	}
	if len(colMap) != 1 {
		t.Errorf("expected exactly 1 col (name), got %d", len(colMap))
	}
	if _, ok := colMap["name"]; !ok {
		t.Error("expected 'name' key in columnValues")
	}
}

func TestItemUpdate_ColOnly(t *testing.T) {
	var capturedVars map[string]any

	srv := newTestServer(t, func(body map[string]any) string {
		if vars, ok := body["variables"].(map[string]any); ok {
			capturedVars = vars
		}
		return mustMarshal(sampleItemUpdateResponse("1234567890", "Unchanged"))
	})
	installItemFactory(t, srv.URL)

	_, err := execItemUpdate(t, "1234567890", "--board", "9832181507",
		"--col", `status={"label":"Done"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	colVals, _ := capturedVars["columnValues"].(string)
	var colMap map[string]any
	if err := json.Unmarshal([]byte(colVals), &colMap); err != nil {
		t.Fatalf("columnValues is not valid JSON: %v", err)
	}
	if _, ok := colMap["name"]; ok {
		t.Error("expected no 'name' key when --name not given")
	}
	if _, ok := colMap["status"]; !ok {
		t.Error("expected 'status' key in columnValues")
	}
}

func TestItemUpdate_NothingToUpdate(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return mustMarshal(sampleItemUpdateResponse("1234567890", "x"))
	})
	installItemFactory(t, srv.URL)

	_, err := execItemUpdate(t, "1234567890", "--board", "9832181507")
	if err == nil {
		t.Fatal("expected error when neither --name nor --col given")
	}
	code := errsCode(err)
	if code != "USAGE" {
		t.Errorf("expected USAGE, got %q", code)
	}
}

func TestItemUpdate_MalformedColJSON(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return mustMarshal(sampleItemUpdateResponse("1234567890", "x"))
	})
	installItemFactory(t, srv.URL)

	_, err := execItemUpdate(t, "1234567890", "--board", "9832181507",
		"--col", `status=not-json`)
	if err == nil {
		t.Fatal("expected error for malformed JSON in --col")
	}
	code := errsCode(err)
	if code != "USAGE" {
		t.Errorf("expected USAGE, got %q", code)
	}
}

func TestItemUpdate_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"errors":[{"message":"forbidden","extensions":{"code":"Forbidden"}}]}`)
	}))
	t.Cleanup(srv.Close)
	installItemFactory(t, srv.URL)

	_, err := execItemUpdate(t, "1234567890", "--board", "9832181507",
		"--col", `status={"label":"Done"}`)
	if err == nil {
		t.Fatal("expected error from API error response")
	}
}

// ---- item move tests ----

func execItemMove(t *testing.T, args ...string) (string, error) {
	t.Helper()
	globals = GlobalFlags{JSON: true}
	defer func() { globals = GlobalFlags{} }()

	var buf bytes.Buffer
	cmd := newItemCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(append([]string{"move"}, args...))
	err := cmd.Execute()
	return buf.String(), err
}

func sampleMoveToGroupResponse(id, name, groupID, groupTitle string) map[string]any {
	return map[string]any{
		"move_item_to_group": map[string]any{
			"id":    id,
			"name":  name,
			"state": "active",
			"board": map[string]any{"id": "9832181507", "name": "Dev Board"},
			"group": map[string]any{"id": groupID, "title": groupTitle},
		},
	}
}

func sampleMoveToBoardResponse(id, name, boardID, boardName, groupID, groupTitle string) map[string]any {
	return map[string]any{
		"move_item_to_board": map[string]any{
			"id":    id,
			"name":  name,
			"state": "active",
			"board": map[string]any{"id": boardID, "name": boardName},
			"group": map[string]any{"id": groupID, "title": groupTitle},
		},
	}
}

func TestItemMove_ToGroup(t *testing.T) {
	var capturedVars map[string]any

	srv := newTestServer(t, func(body map[string]any) string {
		opName, _ := body["operationName"].(string)
		if opName != "ItemMoveToGroup" {
			t.Errorf("expected ItemMoveToGroup op, got %q", opName)
		}
		if vars, ok := body["variables"].(map[string]any); ok {
			capturedVars = vars
		}
		return mustMarshal(sampleMoveToGroupResponse("111", "My Item", "new_group", "Sprint 2"))
	})
	installItemFactory(t, srv.URL)

	out, err := execItemMove(t, "111", "--to-group", "new_group")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedVars["itemId"] != "111" {
		t.Errorf("expected itemId='111', got %v", capturedVars["itemId"])
	}
	if capturedVars["groupId"] != "new_group" {
		t.Errorf("expected groupId='new_group', got %v", capturedVars["groupId"])
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	if result["id"] != "111" {
		t.Errorf("expected id='111', got %v", result["id"])
	}
	group, _ := result["group"].(map[string]any)
	if group == nil || group["id"] != "new_group" {
		t.Errorf("expected group.id='new_group', got %v", group)
	}
}

func TestItemMove_ToBoard(t *testing.T) {
	var capturedVars map[string]any

	srv := newTestServer(t, func(body map[string]any) string {
		opName, _ := body["operationName"].(string)
		if opName != "ItemMoveToBoard" {
			t.Errorf("expected ItemMoveToBoard op, got %q", opName)
		}
		if vars, ok := body["variables"].(map[string]any); ok {
			capturedVars = vars
		}
		return mustMarshal(sampleMoveToBoardResponse("222", "Task", "8888", "Other Board", "grp1", "Target Group"))
	})
	installItemFactory(t, srv.URL)

	out, err := execItemMove(t, "222", "--to-board", "8888", "--group", "grp1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedVars["itemId"] != "222" {
		t.Errorf("expected itemId='222', got %v", capturedVars["itemId"])
	}
	if capturedVars["boardId"] != "8888" {
		t.Errorf("expected boardId='8888', got %v", capturedVars["boardId"])
	}
	if capturedVars["groupId"] != "grp1" {
		t.Errorf("expected groupId='grp1', got %v", capturedVars["groupId"])
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	board, _ := result["board"].(map[string]any)
	if board == nil || board["id"] != "8888" {
		t.Errorf("expected board.id='8888', got %v", board)
	}
}

func TestItemMove_ToBoardMissingGroup(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return mustMarshal(sampleMoveToBoardResponse("222", "x", "8888", "x", "g", "x"))
	})
	installItemFactory(t, srv.URL)

	_, err := execItemMove(t, "222", "--to-board", "8888")
	if err == nil {
		t.Fatal("expected error when --group is missing with --to-board")
	}
	code := errsCode(err)
	if code != "USAGE" {
		t.Errorf("expected USAGE, got %q", code)
	}
}

func TestItemMove_NeitherFlag(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return mustMarshal(sampleMoveToGroupResponse("1", "x", "g", "x"))
	})
	installItemFactory(t, srv.URL)

	_, err := execItemMove(t, "111")
	if err == nil {
		t.Fatal("expected error when neither --to-group nor --to-board given")
	}
	code := errsCode(err)
	if code != "USAGE" {
		t.Errorf("expected USAGE, got %q", code)
	}
}

func TestItemMove_BothFlags(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return mustMarshal(sampleMoveToGroupResponse("1", "x", "g", "x"))
	})
	installItemFactory(t, srv.URL)

	_, err := execItemMove(t, "111", "--to-group", "g", "--to-board", "b")
	if err == nil {
		t.Fatal("expected error when both --to-group and --to-board given")
	}
}

// ---- item delete tests ----

func execItemDelete(t *testing.T, args ...string) (string, error) {
	t.Helper()
	globals = GlobalFlags{JSON: true}
	defer func() { globals = GlobalFlags{} }()

	var buf bytes.Buffer
	cmd := newItemCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(append([]string{"delete"}, args...))
	err := cmd.Execute()
	return buf.String(), err
}

func sampleItemDeleteResponse(id, name string) map[string]any {
	return map[string]any{
		"delete_item": map[string]any{
			"id":    id,
			"name":  name,
			"state": "deleted",
		},
	}
}

func TestItemDelete_HappyPath(t *testing.T) {
	var capturedVars map[string]any

	srv := newTestServer(t, func(body map[string]any) string {
		opName, _ := body["operationName"].(string)
		if opName != "ItemDelete" {
			t.Errorf("expected ItemDelete op, got %q", opName)
		}
		if vars, ok := body["variables"].(map[string]any); ok {
			capturedVars = vars
		}
		return mustMarshal(sampleItemDeleteResponse("1234567890", "My Task"))
	})
	installItemFactory(t, srv.URL)

	out, err := execItemDelete(t, "1234567890")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedVars["itemId"] != "1234567890" {
		t.Errorf("expected itemId='1234567890', got %v", capturedVars["itemId"])
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	if result["id"] != "1234567890" {
		t.Errorf("expected id='1234567890', got %v", result["id"])
	}
	if result["name"] != "My Task" {
		t.Errorf("expected name='My Task', got %v", result["name"])
	}
	if result["state"] != "deleted" {
		t.Errorf("expected state='deleted', got %v", result["state"])
	}
}

func TestItemDelete_InvalidID(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return mustMarshal(sampleItemDeleteResponse("1", "x"))
	})
	installItemFactory(t, srv.URL)

	_, err := execItemDelete(t, "not-a-number")
	if err == nil {
		t.Fatal("expected error for non-numeric id")
	}
	code := errsCode(err)
	if code != "USAGE" {
		t.Errorf("expected USAGE, got %q", code)
	}
}

func TestItemDelete_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"errors":[{"message":"forbidden","extensions":{"code":"Forbidden"}}]}`)
	}))
	t.Cleanup(srv.Close)
	installItemFactory(t, srv.URL)

	_, err := execItemDelete(t, "1234567890")
	if err == nil {
		t.Fatal("expected error from API error response")
	}
}

// ---- item archive tests ----

func execItemArchive(t *testing.T, args ...string) (string, error) {
	t.Helper()
	globals = GlobalFlags{JSON: true}
	defer func() { globals = GlobalFlags{} }()

	var buf bytes.Buffer
	cmd := newItemCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(append([]string{"archive"}, args...))
	err := cmd.Execute()
	return buf.String(), err
}

func sampleItemArchiveResponse(id, name string) map[string]any {
	return map[string]any{
		"archive_item": map[string]any{
			"id":    id,
			"name":  name,
			"state": "archived",
		},
	}
}

func TestItemArchive_HappyPath(t *testing.T) {
	var capturedVars map[string]any

	srv := newTestServer(t, func(body map[string]any) string {
		opName, _ := body["operationName"].(string)
		if opName != "ItemArchive" {
			t.Errorf("expected ItemArchive op, got %q", opName)
		}
		if vars, ok := body["variables"].(map[string]any); ok {
			capturedVars = vars
		}
		return mustMarshal(sampleItemArchiveResponse("1234567890", "My Task"))
	})
	installItemFactory(t, srv.URL)

	out, err := execItemArchive(t, "1234567890")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedVars["itemId"] != "1234567890" {
		t.Errorf("expected itemId='1234567890', got %v", capturedVars["itemId"])
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	if result["id"] != "1234567890" {
		t.Errorf("expected id='1234567890', got %v", result["id"])
	}
	if result["name"] != "My Task" {
		t.Errorf("expected name='My Task', got %v", result["name"])
	}
	if result["state"] != "archived" {
		t.Errorf("expected state='archived', got %v", result["state"])
	}
}

func TestItemArchive_InvalidID(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return mustMarshal(sampleItemArchiveResponse("1", "x"))
	})
	installItemFactory(t, srv.URL)

	_, err := execItemArchive(t, "not-a-number")
	if err == nil {
		t.Fatal("expected error for non-numeric id")
	}
	code := errsCode(err)
	if code != "USAGE" {
		t.Errorf("expected USAGE, got %q", code)
	}
}

func TestItemArchive_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"errors":[{"message":"access denied","extensions":{"code":"Unauthorized"}}]}`)
	}))
	t.Cleanup(srv.Close)
	installItemFactory(t, srv.URL)

	_, err := execItemArchive(t, "1234567890")
	if err == nil {
		t.Fatal("expected error from API error response")
	}
}

// --- item post-update ---

// execItemPostUpdate runs 'item post-update <id>' with stdin optionally piped from `stdin`.
func execItemPostUpdate(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	globals = GlobalFlags{JSON: true}
	defer func() { globals = GlobalFlags{} }()

	var buf bytes.Buffer
	cmd := newItemCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	if stdin != "" {
		cmd.SetIn(strings.NewReader(stdin))
	}
	cmd.SetArgs(append([]string{"post-update"}, args...))
	err := cmd.Execute()
	return buf.String(), err
}

func sampleItemPostUpdateResponse(id string) map[string]any {
	return map[string]any{
		"create_update": map[string]any{
			"id":         id,
			"body":       "<p>hi</p>",
			"text_body":  "hi",
			"created_at": "2026-05-16T12:00:00Z",
		},
	}
}

func TestItemPostUpdate_HappyPath(t *testing.T) {
	var capturedVars map[string]any
	srv := newTestServer(t, func(body map[string]any) string {
		opName, _ := body["operationName"].(string)
		if opName != "ItemPostUpdate" {
			t.Errorf("expected ItemPostUpdate op, got %q", opName)
		}
		if vars, ok := body["variables"].(map[string]any); ok {
			capturedVars = vars
		}
		return mustMarshal(sampleItemPostUpdateResponse("9001"))
	})
	installItemFactory(t, srv.URL)

	out, err := execItemPostUpdate(t, "", "1234567890", "--body", "Got it, looking now")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedVars["itemId"] != "1234567890" {
		t.Errorf("itemId mismatch: got %v", capturedVars["itemId"])
	}
	if capturedVars["body"] != "Got it, looking now" {
		t.Errorf("body mismatch: got %v", capturedVars["body"])
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	if result["id"] != "9001" {
		t.Errorf("expected id=9001, got %v", result["id"])
	}
}

func TestItemPostUpdate_BodyFromStdin(t *testing.T) {
	var capturedVars map[string]any
	srv := newTestServer(t, func(body map[string]any) string {
		if vars, ok := body["variables"].(map[string]any); ok {
			capturedVars = vars
		}
		return mustMarshal(sampleItemPostUpdateResponse("9002"))
	})
	installItemFactory(t, srv.URL)

	piped := "line one\nline two\nline three"
	_, err := execItemPostUpdate(t, piped, "1234567890", "--body", "-")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedVars["body"] != piped {
		t.Errorf("expected stdin body to round-trip; got %q", capturedVars["body"])
	}
}

func TestItemPostUpdate_WithParent(t *testing.T) {
	var capturedVars map[string]any
	srv := newTestServer(t, func(body map[string]any) string {
		if vars, ok := body["variables"].(map[string]any); ok {
			capturedVars = vars
		}
		return mustMarshal(sampleItemPostUpdateResponse("9003"))
	})
	installItemFactory(t, srv.URL)

	_, err := execItemPostUpdate(t, "",
		"1234567890", "--body", "thread reply", "--parent", "555")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedVars["parentId"] != "555" {
		t.Errorf("parentId mismatch: got %v", capturedVars["parentId"])
	}
}

func TestItemPostUpdate_RejectsNonNumericItemID(t *testing.T) {
	_, err := execItemPostUpdate(t, "", "not-a-number", "--body", "x")
	if err == nil {
		t.Fatal("expected error")
	}
	if errsCode(err) != "USAGE" {
		t.Errorf("expected USAGE, got %q", errsCode(err))
	}
}

func TestItemPostUpdate_RejectsEmptyBody(t *testing.T) {
	_, err := execItemPostUpdate(t, "", "1234567890", "--body", "   ")
	if err == nil {
		t.Fatal("expected error")
	}
	if errsCode(err) != "USAGE" {
		t.Errorf("expected USAGE, got %q", errsCode(err))
	}
}

func TestItemPostUpdate_RejectsNonNumericParent(t *testing.T) {
	_, err := execItemPostUpdate(t, "", "1234567890", "--body", "x", "--parent", "abc")
	if err == nil {
		t.Fatal("expected error")
	}
	if errsCode(err) != "USAGE" {
		t.Errorf("expected USAGE, got %q", errsCode(err))
	}
}

func TestItemPostUpdate_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"errors":[{"message":"access denied","extensions":{"code":"Unauthorized"}}]}`)
	}))
	t.Cleanup(srv.Close)
	installItemFactory(t, srv.URL)

	_, err := execItemPostUpdate(t, "", "1234567890", "--body", "x")
	if err == nil {
		t.Fatal("expected error from API error response")
	}
}
