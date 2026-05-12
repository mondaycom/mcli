package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func execMutation(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	cmd := newMutationCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func execMutationSub(t *testing.T, sub string, args ...string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	cmd := newMutationCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(append([]string{sub}, args...))
	err := cmd.Execute()
	return buf.String(), err
}

func TestMutationCmd_Integration(t *testing.T) {
	dir := t.TempDir()

	origConf := globals.Config
	globals.Config = filepath.Join(dir, "config.yaml")
	t.Cleanup(func() { globals.Config = origConf })

	origJSON := globals.JSON
	globals.JSON = true
	t.Cleanup(func() { globals.JSON = origJSON })

	t.Run("Inline_Success", func(t *testing.T) {
		const respBody = `{"data":{"create_board":{"id":"123"}}}`
		srv := httptest.NewServer(mondayHandler(t, respBody, http.StatusOK))
		defer srv.Close()
		installQueryFactory(t, srv)

		out, err := execMutation(t, `mutation { create_board(board_name: "x", board_kind: public) { id } }`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, `"create_board"`) {
			t.Errorf("output missing 'create_board': %s", out)
		}
	})

	t.Run("GraphQLErrors_ExitCode2", func(t *testing.T) {
		const respBody = `{"errors":[{"message":"invalid mutation"}]}`
		srv := httptest.NewServer(mondayHandler(t, respBody, http.StatusOK))
		defer srv.Close()
		installQueryFactory(t, srv)

		out, err := execMutation(t, `mutation { bad }`)
		if err == nil {
			t.Fatal("expected error for GraphQL errors")
		}
		if !strings.Contains(out, `"errors"`) {
			t.Errorf("output missing 'errors': %s", out)
		}
	})

	t.Run("WithVars", func(t *testing.T) {
		var capturedBody []byte
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			capturedBody = b
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintln(w, `{"data":null}`)
		}))
		defer srv.Close()
		installQueryFactory(t, srv)

		_, _ = execMutation(t, `mutation($name: String!) { create_board(board_name: $name, board_kind: public) { id } }`, "--var", "name=TestBoard")

		var payload map[string]any
		if err := json.Unmarshal(capturedBody, &payload); err != nil {
			t.Fatalf("parse captured body: %v", err)
		}
		vars, ok := payload["variables"].(map[string]any)
		if !ok {
			t.Fatalf("variables not present: %v", payload)
		}
		if vars["name"] != "TestBoard" {
			t.Errorf("vars[name] = %v, want 'TestBoard'", vars["name"])
		}
	})

	t.Run("NoArgsNoFile_Error", func(t *testing.T) {
		_, err := execMutation(t)
		if err == nil {
			t.Fatal("expected error when no mutation provided")
		}
	})
}

func TestMutationSavedCmds(t *testing.T) {
	dir := t.TempDir()

	origConf := globals.Config
	globals.Config = filepath.Join(dir, "config.yaml")
	t.Cleanup(func() { globals.Config = origConf })

	origJSON := globals.JSON
	globals.JSON = true
	t.Cleanup(func() { globals.JSON = origJSON })

	origWD, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir to %s: %v", dir, err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })

	t.Run("Save_Local", func(t *testing.T) {
		out, err := execMutationSub(t, "save", "mymut", "--query", `mutation { create_board(board_name: "x", board_kind: public) { id } }`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "mymut") {
			t.Errorf("output missing mutation name: %s", out)
		}
		mPath := filepath.Join(dir, ".mcli", "mutations", "mymut.graphql")
		if _, statErr := os.Stat(mPath); statErr != nil {
			t.Errorf("expected file %s to exist: %v", mPath, statErr)
		}
	})

	t.Run("Save_Global", func(t *testing.T) {
		out, err := execMutationSub(t, "save", "gmut", "--query", `mutation { delete_board(board_id: 1) { id } }`, "--global")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "global") {
			t.Errorf("output missing 'global': %s", out)
		}
	})

	t.Run("List", func(t *testing.T) {
		out, err := execMutationSub(t, "list")
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		var result struct {
			Items []savedMutationEntry `json:"items"`
		}
		if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); jsonErr != nil {
			t.Fatalf("parse output JSON: %v (output: %s)", jsonErr, out)
		}
		if len(result.Items) != 2 {
			t.Fatalf("expected 2 items, got %d: %v", len(result.Items), result.Items)
		}
		if result.Items[0].Name != "mymut" || result.Items[0].Source != "local" {
			t.Errorf("item[0] = %+v, want {mymut local}", result.Items[0])
		}
		if result.Items[1].Name != "gmut" || result.Items[1].Source != "global" {
			t.Errorf("item[1] = %+v, want {gmut global}", result.Items[1])
		}
	})

	t.Run("Delete_Local", func(t *testing.T) {
		if _, err := execMutationSub(t, "save", "todel", "--query", `mutation { x { id } }`); err != nil {
			t.Fatalf("save: %v", err)
		}
		out, err := execMutationSub(t, "delete", "todel")
		if err != nil {
			t.Fatalf("delete: %v", err)
		}
		if !strings.Contains(out, "todel") {
			t.Errorf("output missing name: %s", out)
		}
		mPath := filepath.Join(dir, ".mcli", "mutations", "todel.graphql")
		if _, statErr := os.Stat(mPath); !os.IsNotExist(statErr) {
			t.Errorf("file %s should not exist after delete", mPath)
		}
	})

	t.Run("Delete_NotFound", func(t *testing.T) {
		_, err := execMutationSub(t, "delete", "nonexistent")
		if err == nil {
			t.Fatal("expected error for nonexistent mutation")
		}
	})

	t.Run("Run_LocalFirst", func(t *testing.T) {
		const respBody = `{"data":{"create_board":{"id":"999"}}}`
		srv := httptest.NewServer(mondayHandler(t, respBody, http.StatusOK))
		defer srv.Close()
		installQueryFactory(t, srv)

		out, err := execMutationSub(t, "run", "mymut")
		if err != nil {
			t.Fatalf("run: %v", err)
		}
		if !strings.Contains(out, `"create_board"`) {
			t.Errorf("output missing 'create_board': %s", out)
		}
	})

	t.Run("Run_GlobalFallback", func(t *testing.T) {
		const respBody = `{"data":{"delete_board":{"id":"1"}}}`
		srv := httptest.NewServer(mondayHandler(t, respBody, http.StatusOK))
		defer srv.Close()
		installQueryFactory(t, srv)

		out, err := execMutationSub(t, "run", "gmut")
		if err != nil {
			t.Fatalf("run: %v", err)
		}
		if !strings.Contains(out, `"delete_board"`) {
			t.Errorf("output missing 'delete_board': %s", out)
		}
	})

	t.Run("Run_NotFound", func(t *testing.T) {
		_, err := execMutationSub(t, "run", "doesnotexist")
		if err == nil {
			t.Fatal("expected error for nonexistent mutation")
		}
	})

	t.Run("Run_WithVars", func(t *testing.T) {
		var capturedBody []byte
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			capturedBody = b
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintln(w, `{"data":null}`)
		}))
		defer srv.Close()
		installQueryFactory(t, srv)

		if _, err := execMutationSub(t, "save", "pm", "--query", `mutation($name: String!) { create_board(board_name: $name, board_kind: public) { id } }`); err != nil {
			t.Fatalf("save: %v", err)
		}
		_, _ = execMutationSub(t, "run", "pm", "--var", "name=Hello")

		var payload map[string]any
		if err := json.Unmarshal(capturedBody, &payload); err != nil {
			t.Fatalf("parse body: %v", err)
		}
		vars, _ := payload["variables"].(map[string]any)
		if vars["name"] != "Hello" {
			t.Errorf("vars[name] = %v, want 'Hello'", vars["name"])
		}
	})
}
