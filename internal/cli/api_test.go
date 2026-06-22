package cli

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// execAPI runs 'mcli api' with args and returns stdout + error.
func execAPI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	cmd := newAPICmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func TestAPIDryRun(t *testing.T) {
	out, err := execAPI(t, "me", "--dry-run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should contain 'me' field reference.
	if !strings.Contains(out, "me") {
		t.Errorf("dry-run output missing 'me': %s", out)
	}
	// Should look like a query.
	if !strings.HasPrefix(strings.TrimSpace(out), "query") {
		t.Errorf("dry-run output should start with 'query': %s", out)
	}
	// Should have a selection set.
	if !strings.Contains(out, "{") {
		t.Errorf("dry-run output missing selection set: %s", out)
	}
}

func TestAPIListOutput(t *testing.T) {
	out, err := execAPI(t, "list")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should contain known operation names.
	if !strings.Contains(out, "me") {
		t.Errorf("list output missing 'me': %s", out)
	}
	if !strings.Contains(out, "boards") {
		t.Errorf("list output missing 'boards': %s", out)
	}
}

func TestAPIListJSON(t *testing.T) {
	origJSON := globals.JSON
	globals.JSON = true
	t.Cleanup(func() { globals.JSON = origJSON })

	out, err := execAPI(t, "list")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(out), "[") {
		t.Errorf("expected JSON array, got: %s", out)
	}
	if !strings.Contains(out, `"name"`) {
		t.Errorf("JSON output missing 'name' key: %s", out)
	}
}

func TestAPIDescribeOperation(t *testing.T) {
	out, err := execAPI(t, "describe", "me")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "me") {
		t.Errorf("describe output missing 'me': %s", out)
	}
	if !strings.Contains(out, "query") {
		t.Errorf("describe output missing type 'query': %s", out)
	}
}

func TestAPIDescribeType(t *testing.T) {
	out, err := execAPI(t, "describe", "Board")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Board") {
		t.Errorf("describe Board output missing 'Board': %s", out)
	}
	// Board type should have fields.
	if !strings.Contains(out, "id") {
		t.Errorf("describe Board output missing 'id' field: %s", out)
	}
}

func TestAPIMissingRequiredArg(t *testing.T) {
	// change_column_value requires board_id, item_id, column_id, value
	_, err := execAPI(t, "change_column_value")
	if err == nil {
		t.Fatal("expected error for missing required args")
	}
	if !strings.Contains(err.Error(), "requires") {
		t.Errorf("error should mention missing args: %v", err)
	}
}

func TestAPIExecute_Success(t *testing.T) {
	const respBody = `{"data":{"me":{"id":"1","name":"Test User"}}}`
	srv := httptest.NewServer(mondayHandler(t, respBody, http.StatusOK))
	defer srv.Close()
	installQueryFactory(t, srv)

	out, err := execAPI(t, "me")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, `"me"`) {
		t.Errorf("output missing 'me': %s", out)
	}
}

func TestAPIUnknownOperation(t *testing.T) {
	_, err := execAPI(t, "nonexistent_operation_xyz")
	if err == nil {
		t.Fatal("expected error for unknown operation")
	}
}

func TestAPISelectFlag(t *testing.T) {
	out, err := execAPI(t, "me", "--dry-run", "--select", "id,name,email")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "id") || !strings.Contains(out, "name") || !strings.Contains(out, "email") {
		t.Errorf("dry-run with --select missing expected fields: %s", out)
	}
}

func TestAPIConvertSelectFlag(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input string
		want  string
	}{
		{"id,name,state", "{ id name state }"},
		{"id,name,column{id,text}", "{ id name column { id text } }"},
		{"id", "{ id }"},
	}
	for _, tc := range cases {
		got := convertSelectFlag(tc.input)
		if got != tc.want {
			t.Errorf("convertSelectFlag(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestAPIListNoBuiltins(t *testing.T) {
	out, err := execAPI(t, "list", "--no-builtins")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 'me' has a builtin equivalent, so it should be absent.
	if strings.Contains(out, " me ") || strings.HasPrefix(out, "me ") {
		// Check that "me" as a standalone operation name doesn't appear.
		// (It might appear in descriptions or equivalents text, so we check lines.)
		for line := range strings.SplitSeq(out, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "me ") ||
				strings.TrimSpace(line) == "me" {
				t.Errorf("'me' should be hidden with --no-builtins, found: %s", line)
			}
		}
	}
}
