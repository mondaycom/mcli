package cli

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

// execItemDescription runs 'item description <id>' with given args.
func execItemDescription(t *testing.T, id string, args ...string) (string, error) {
	t.Helper()
	globals = GlobalFlags{}
	defer func() { globals = GlobalFlags{} }()

	var buf bytes.Buffer
	cmd := newItemCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(append([]string{"description", id}, args...))
	err := cmd.Execute()
	return buf.String(), err
}

// ---- ItemDescription tests ----

func TestItemDescription_Read(t *testing.T) {
	// Sequence: first call is DocGetByObjectID, second is DocExportMarkdown.
	callCount := 0
	srv := newTestServer(t, func(body map[string]any) string {
		callCount++
		opName, _ := body["operationName"].(string)
		switch opName {
		case "DocGetByObjectID":
			return mustMarshal(map[string]any{
				"docs": []any{
					map[string]any{"id": "doc99", "name": "Item desc", "object_id": "item42"},
				},
			})
		case "DocExportMarkdown":
			return mustMarshal(map[string]any{
				"export_markdown_from_doc": map[string]any{
					"success":  true,
					"markdown": "# Hello\n\nThis is the item description.",
					"error":    "",
				},
			})
		default:
			t.Errorf("unexpected operation: %q", opName)
			return `{}`
		}
	})
	installItemFactory(t, srv.URL)

	out, err := execItemDescription(t, "item42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Hello") {
		t.Errorf("expected markdown in output, got: %q", out)
	}
}

func TestItemDescription_Write(t *testing.T) {
	srv := newTestServer(t, func(body map[string]any) string {
		opName, _ := body["operationName"].(string)
		if opName != "DocSetItemDescription" {
			t.Errorf("unexpected operation: %q", opName)
		}
		return mustMarshal(map[string]any{
			"set_item_description_content": map[string]any{
				"success":   true,
				"block_ids": []string{"blk1"},
				"error":     "",
			},
		})
	})
	installItemFactory(t, srv.URL)

	out, err := execItemDescription(t, "item42", "--set", "# New content")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "item42") {
		t.Errorf("expected item id in confirmation, got: %q", out)
	}
}

func TestItemDescription_WriteFromFile(t *testing.T) {
	srv := newTestServer(t, func(body map[string]any) string {
		opName, _ := body["operationName"].(string)
		if opName != "DocSetItemDescription" {
			t.Errorf("unexpected operation: %q", opName)
		}
		return mustMarshal(map[string]any{
			"set_item_description_content": map[string]any{
				"success":   true,
				"block_ids": []string{"blk1"},
				"error":     "",
			},
		})
	})
	installItemFactory(t, srv.URL)

	// Write markdown to a temp file.
	f := t.TempDir() + "/desc.md"
	if err := writeTestFile(f, "# From File\n\nContent here."); err != nil {
		t.Fatalf("create temp file: %v", err)
	}

	out, err := execItemDescription(t, "item42", "--set-file", f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "item42") {
		t.Errorf("expected item id in confirmation, got: %q", out)
	}
}

func TestItemDescription_ReadNotFound(t *testing.T) {
	srv := newTestServer(t, func(body map[string]any) string {
		opName, _ := body["operationName"].(string)
		if opName == "DocGetByObjectID" {
			return mustMarshal(map[string]any{"docs": []any{}})
		}
		return `{}`
	})
	installItemFactory(t, srv.URL)

	_, err := execItemDescription(t, "item_missing")
	if err == nil {
		t.Fatal("expected not-found error, got nil")
	}
}

// writeTestFile writes content to path (used by test helpers only).
func writeTestFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}
