package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	gqlclient "github.com/Khan/genqlient/graphql"
)

// installSearchFactory sets up searchClientFactory to use the given httptest
// server URL for the duration of the test, then restores the original value.
func installSearchFactory(t *testing.T, srvURL string) {
	t.Helper()
	orig := searchClientFactory
	searchClientFactory = func() (gqlclient.Client, error) {
		return gqlclient.NewClient(srvURL, http.DefaultClient), nil
	}
	t.Cleanup(func() { searchClientFactory = orig })
}

// execSearch runs 'mcli search <query>' with the given extra args.
func execSearch(t *testing.T, query string, args ...string) (string, error) {
	t.Helper()
	globals = GlobalFlags{JSON: true}
	defer func() { globals = GlobalFlags{} }()

	var buf bytes.Buffer
	cmd := newSearchCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(append([]string{query}, args...))
	err := cmd.Execute()
	return buf.String(), err
}

// ---- Search tests ----

func TestSearch_Boards(t *testing.T) {
	srv := newTestServer(t, func(body map[string]any) string {
		opName, _ := body["operationName"].(string)
		if opName != "SearchBoards" {
			t.Errorf("unexpected operationName: %q", opName)
		}
		return mustMarshal(map[string]any{
			"search": map[string]any{
				"boards": map[string]any{
					"results": []any{
						map[string]any{
							"id": "111",
							"indexed_data": map[string]any{
								"id":           "111",
								"name":         "Project Alpha",
								"description":  "Main project board",
								"workspace_id": "999",
								"url":          "https://monday.com/boards/111",
							},
						},
					},
				},
			},
		})
	})
	installSearchFactory(t, srv.URL)

	out, err := execSearch(t, "Alpha", "-t", "boards")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	boards, _ := result["boards"].([]any)
	if len(boards) != 1 {
		t.Fatalf("expected 1 board, got %d", len(boards))
	}
	b := boards[0].(map[string]any)
	if b["id"] != "111" {
		t.Errorf("expected id=111, got %v", b["id"])
	}
	if b["name"] != "Project Alpha" {
		t.Errorf("expected name=Project Alpha, got %v", b["name"])
	}
}

func TestSearch_Items(t *testing.T) {
	srv := newTestServer(t, func(body map[string]any) string {
		opName, _ := body["operationName"].(string)
		if opName != "SearchItems" {
			t.Errorf("unexpected operationName: %q", opName)
		}
		return mustMarshal(map[string]any{
			"search": map[string]any{
				"items": map[string]any{
					"results": []any{
						map[string]any{
							"id": "222",
							"indexed_data": map[string]any{
								"id":           "222",
								"name":         "Deploy backend",
								"url":          "https://monday.com/items/222",
								"board_id":     "111",
								"workspace_id": "999",
							},
						},
					},
				},
			},
		})
	})
	installSearchFactory(t, srv.URL)

	out, err := execSearch(t, "Deploy", "-t", "items")
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
	if it["id"] != "222" {
		t.Errorf("expected id=222, got %v", it["id"])
	}
	if it["board_id"] != "111" {
		t.Errorf("expected board_id=111, got %v", it["board_id"])
	}
}

func TestSearch_Docs(t *testing.T) {
	srv := newTestServer(t, func(body map[string]any) string {
		opName, _ := body["operationName"].(string)
		if opName != "SearchDocs" {
			t.Errorf("unexpected operationName: %q", opName)
		}
		return mustMarshal(map[string]any{
			"search": map[string]any{
				"docs": map[string]any{
					"results": []any{
						map[string]any{
							"id": "333",
							"indexed_data": map[string]any{
								"id":           "333",
								"name":         "Architecture notes",
								"workspace_id": "999",
							},
						},
					},
				},
			},
		})
	})
	installSearchFactory(t, srv.URL)

	out, err := execSearch(t, "Architecture", "-t", "docs")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	docs, _ := result["docs"].([]any)
	if len(docs) != 1 {
		t.Fatalf("expected 1 doc, got %d", len(docs))
	}
	d := docs[0].(map[string]any)
	if d["id"] != "333" {
		t.Errorf("expected id=333, got %v", d["id"])
	}
}

func TestSearch_InvalidType(t *testing.T) {
	_, err := execSearch(t, "query", "-t", "users")
	if err == nil {
		t.Fatal("expected error for invalid type, got nil")
	}
}

func TestSearch_EmptyQuery(t *testing.T) {
	_, err := execSearch(t, "")
	if err == nil {
		t.Fatal("expected error for empty query, got nil")
	}
}

func TestSearch_TerseOutput(t *testing.T) {
	srv := newTestServer(t, func(body map[string]any) string {
		opName, _ := body["operationName"].(string)
		switch opName {
		case "SearchBoards":
			return mustMarshal(map[string]any{
				"search": map[string]any{
					"boards": map[string]any{
						"results": []any{
							map[string]any{
								"id": "111",
								"indexed_data": map[string]any{
									"id": "111", "name": "Alpha", "workspace_id": "999",
									"url": "https://monday.com/boards/111",
								},
							},
						},
					},
				},
			})
		case "SearchItems":
			return mustMarshal(map[string]any{
				"search": map[string]any{
					"items": map[string]any{"results": []any{}},
				},
			})
		case "SearchDocs":
			return mustMarshal(map[string]any{
				"search": map[string]any{
					"docs": map[string]any{"results": []any{}},
				},
			})
		}
		return `{}`
	})
	installSearchFactory(t, srv.URL)

	globals = GlobalFlags{Terse: true}
	defer func() { globals = GlobalFlags{} }()

	var buf bytes.Buffer
	cmd := newSearchCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"Alpha"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	line := strings.TrimSpace(buf.String())
	if !strings.HasPrefix(line, "board:111:Alpha:ws=999") {
		t.Errorf("terse output mismatch: %q", line)
	}
}
