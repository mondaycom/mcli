package cli

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"

	gqlclient "github.com/Khan/genqlient/graphql"
)

// installDocFactory sets up docClientFactory for the duration of the test.
func installDocFactory(t *testing.T, srvURL string) {
	t.Helper()
	orig := docClientFactory
	docClientFactory = func() (gqlclient.Client, error) {
		return gqlclient.NewClient(srvURL, http.DefaultClient), nil
	}
	t.Cleanup(func() { docClientFactory = orig })
}

// execDocRead runs 'doc read <id>'.
func execDocRead(t *testing.T, id string, args ...string) (string, error) {
	t.Helper()
	globals = GlobalFlags{}
	defer func() { globals = GlobalFlags{} }()

	var buf bytes.Buffer
	cmd := newDocCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(append([]string{"read", id}, args...))
	err := cmd.Execute()
	return buf.String(), err
}

// execDocWrite runs 'doc write <id>' with given args.
func execDocWrite(t *testing.T, id string, args ...string) (string, error) {
	t.Helper()
	globals = GlobalFlags{}
	defer func() { globals = GlobalFlags{} }()

	var buf bytes.Buffer
	cmd := newDocCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(append([]string{"write", id}, args...))
	err := cmd.Execute()
	return buf.String(), err
}

// ---- DocRead tests ----

func TestDocRead_Basic(t *testing.T) {
	srv := newTestServer(t, func(body map[string]any) string {
		opName, _ := body["operationName"].(string)
		if opName != "DocExportMarkdown" {
			t.Errorf("unexpected operation: %q", opName)
		}
		return mustMarshal(map[string]any{
			"export_markdown_from_doc": map[string]any{
				"success":  true,
				"markdown": "# My Doc\n\nContent here.",
				"error":    "",
			},
		})
	})
	installDocFactory(t, srv.URL)

	out, err := execDocRead(t, "doc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "My Doc") {
		t.Errorf("expected markdown in output, got: %q", out)
	}
}

func TestDocRead_ExportFailure(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return mustMarshal(map[string]any{
			"export_markdown_from_doc": map[string]any{
				"success":  false,
				"markdown": "",
				"error":    "doc not accessible",
			},
		})
	})
	installDocFactory(t, srv.URL)

	_, err := execDocRead(t, "doc_bad")
	if err == nil {
		t.Fatal("expected error for failed export, got nil")
	}
}

// ---- DocWrite tests ----

func TestDocWrite_Basic(t *testing.T) {
	callSeq := []string{}
	srv := newTestServer(t, func(body map[string]any) string {
		opName, _ := body["operationName"].(string)
		callSeq = append(callSeq, opName)
		switch opName {
		case "DocGetBlockIDs":
			return mustMarshal(map[string]any{
				"docs": []any{
					map[string]any{
						"id": "doc123",
						"blocks": []any{
							map[string]any{"id": "blk1"},
							map[string]any{"id": "blk2"},
						},
					},
				},
			})
		case "DocDeleteBlocks":
			return mustMarshal(map[string]any{
				"delete_doc_blocks": []any{
					map[string]any{"id": "blk1"},
					map[string]any{"id": "blk2"},
				},
			})
		case "DocAddMarkdown":
			return mustMarshal(map[string]any{
				"add_content_to_doc_from_markdown": map[string]any{
					"success":   true,
					"block_ids": []string{"new1", "new2"},
					"error":     "",
				},
			})
		default:
			t.Errorf("unexpected operation: %q", opName)
			return `{}`
		}
	})
	installDocFactory(t, srv.URL)

	out, err := execDocWrite(t, "doc123", "--content", "# Updated\n\nNew content.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "doc123") {
		t.Errorf("expected doc id in output, got: %q", out)
	}

	// Verify correct sequence of operations.
	if len(callSeq) != 3 {
		t.Errorf("expected 3 API calls, got %d: %v", len(callSeq), callSeq)
	}
}

func TestDocWrite_NoContent(t *testing.T) {
	_, err := execDocWrite(t, "doc123")
	if err == nil {
		t.Fatal("expected error for missing --content/--file, got nil")
	}
}

func TestDocWrite_NotFound(t *testing.T) {
	srv := newTestServer(t, func(_ map[string]any) string {
		return mustMarshal(map[string]any{
			"docs": []any{},
		})
	})
	installDocFactory(t, srv.URL)

	_, err := execDocWrite(t, "doc_missing", "--content", "hello")
	if err == nil {
		t.Fatal("expected not-found error, got nil")
	}
}
