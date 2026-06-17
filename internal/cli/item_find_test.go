package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

// execItemFind runs 'item find' with the given args.
func execItemFind(t *testing.T, args ...string) (string, error) {
	t.Helper()
	globals = GlobalFlags{JSON: true}
	defer func() { globals = GlobalFlags{} }()

	var buf bytes.Buffer
	cmd := newItemCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(append([]string{"find"}, args...))
	err := cmd.Execute()
	return buf.String(), err
}

// ---- ItemFind tests ----

func TestItemFind_Basic(t *testing.T) {
	srv := newTestServer(t, func(body map[string]any) string {
		opName, _ := body["operationName"].(string)
		if opName != "ItemFindByColumnValue" {
			t.Errorf("unexpected operationName: %q", opName)
		}
		return mustMarshal(map[string]any{
			"items_page_by_column_values": map[string]any{
				"cursor": "",
				"items": []any{
					map[string]any{
						"id":    "42",
						"name":  "Widget A",
						"group": map[string]any{"id": "grp1", "title": "Backlog"},
					},
				},
			},
		})
	})
	installItemFactory(t, srv.URL)

	out, err := execItemFind(t,
		"--board", "99",
		"--column", "status",
		"--value", "Done",
	)
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
	it := items[0].(map[string]any)
	if it["id"] != "42" {
		t.Errorf("expected id=42, got %v", it["id"])
	}
	if it["name"] != "Widget A" {
		t.Errorf("expected name=Widget A, got %v", it["name"])
	}
	grp, _ := it["group"].(map[string]any)
	if grp["title"] != "Backlog" {
		t.Errorf("expected group title=Backlog, got %v", grp["title"])
	}
}

func TestItemFind_Empty(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return mustMarshal(map[string]any{
			"items_page_by_column_values": map[string]any{
				"cursor": "",
				"items":  []any{},
			},
		})
	})
	installItemFactory(t, srv.URL)

	out, err := execItemFind(t,
		"--board", "99",
		"--column", "status",
		"--value", "Nonexistent",
	)
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
}

func TestItemFind_MissingFlags(t *testing.T) {
	// No --board flag; should fail.
	_, err := execItemFind(t, "--column", "status", "--value", "Done")
	if err == nil {
		t.Fatal("expected error for missing --board, got nil")
	}
}

func TestItemFind_TerseOutput(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return mustMarshal(map[string]any{
			"items_page_by_column_values": map[string]any{
				"cursor": "",
				"items": []any{
					map[string]any{
						"id":    "42",
						"name":  "Widget A",
						"group": map[string]any{"id": "grp1", "title": "Backlog"},
					},
				},
			},
		})
	})
	installItemFactory(t, srv.URL)

	globals = GlobalFlags{Terse: true}
	defer func() { globals = GlobalFlags{} }()

	var buf bytes.Buffer
	cmd := newItemCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"find", "--board", "99", "--column", "status", "--value", "Done"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	line := strings.TrimSpace(buf.String())
	if !strings.HasPrefix(line, "#42:Widget A:group=Backlog") {
		t.Errorf("terse output mismatch: %q", line)
	}
}
