package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

// execItemGetUpdates runs 'item get-updates <id>' with the given args.
func execItemGetUpdates(t *testing.T, id string, args ...string) (string, error) {
	t.Helper()
	globals = GlobalFlags{JSON: true}
	defer func() { globals = GlobalFlags{} }()

	var buf bytes.Buffer
	cmd := newItemCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(append([]string{"get-updates", id}, args...))
	err := cmd.Execute()
	return buf.String(), err
}

// ---- ItemGetUpdates tests ----

func TestItemGetUpdates_Basic(t *testing.T) {
	srv := newTestServer(t, func(body map[string]any) string {
		opName, _ := body["operationName"].(string)
		if opName != "ItemGetUpdates" {
			t.Errorf("unexpected operationName: %q", opName)
		}
		return mustMarshal(map[string]any{
			"items": []any{
				map[string]any{
					"id": "777",
					"updates": []any{
						map[string]any{
							"id":         "upd1",
							"body":       "<p>Great progress!</p>",
							"text_body":  "Great progress!",
							"created_at": "2026-06-01",
							"creator":    map[string]any{"id": "u1", "name": "Alice", "email": "alice@example.com"},
							"replies":    []any{},
						},
					},
				},
			},
		})
	})
	installItemFactory(t, srv.URL)

	out, err := execItemGetUpdates(t, "777")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result []map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 update, got %d", len(result))
	}
	u := result[0]
	if u["id"] != "upd1" {
		t.Errorf("expected id=upd1, got %v", u["id"])
	}
	if u["text_body"] != "Great progress!" {
		t.Errorf("expected text_body, got %v", u["text_body"])
	}
	creator, _ := u["creator"].(map[string]any)
	if creator["name"] != "Alice" {
		t.Errorf("expected creator name=Alice, got %v", creator["name"])
	}
}

func TestItemGetUpdates_WithReplies(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return mustMarshal(map[string]any{
			"items": []any{
				map[string]any{
					"id": "888",
					"updates": []any{
						map[string]any{
							"id":         "upd2",
							"body":       "Top update",
							"text_body":  "Top update",
							"created_at": "2026-06-02",
							"creator":    map[string]any{"id": "u1", "name": "Alice", "email": "alice@example.com"},
							"replies": []any{
								map[string]any{
									"id":         "rep1",
									"body":       "Reply here",
									"text_body":  "Reply here",
									"created_at": "2026-06-02",
									"creator":    map[string]any{"id": "u2", "name": "Bob", "email": "bob@example.com"},
								},
							},
						},
					},
				},
			},
		})
	})
	installItemFactory(t, srv.URL)

	out, err := execItemGetUpdates(t, "888")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result []map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	replies, _ := result[0]["replies"].([]any)
	if len(replies) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(replies))
	}
	r := replies[0].(map[string]any)
	if r["id"] != "rep1" {
		t.Errorf("expected reply id=rep1, got %v", r["id"])
	}
}

func TestItemGetUpdates_NotFound(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return mustMarshal(map[string]any{
			"items": []any{},
		})
	})
	installItemFactory(t, srv.URL)

	_, err := execItemGetUpdates(t, "9999")
	if err == nil {
		t.Fatal("expected not-found error, got nil")
	}
}

func TestItemGetUpdates_TerseOutput(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return mustMarshal(map[string]any{
			"items": []any{
				map[string]any{
					"id": "777",
					"updates": []any{
						map[string]any{
							"id":         "upd1",
							"body":       "Good work",
							"text_body":  "Good work",
							"created_at": "2026-06-01",
							"creator":    map[string]any{"id": "u1", "name": "Alice", "email": "alice@example.com"},
							"replies":    []any{},
						},
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
	cmd.SetArgs([]string{"get-updates", "777"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	line := strings.TrimSpace(buf.String())
	if !strings.HasPrefix(line, "upd1|Alice|2026-06-01|Good work") {
		t.Errorf("terse output mismatch: %q", line)
	}
}
