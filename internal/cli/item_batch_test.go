package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/mondaycom/mcli/internal/errs"
)

// gqlCall records one request the CLI made.
type gqlCall struct {
	op   string
	vars map[string]any
}

// gqlRecorder is a fake monday endpoint that records every call and lets a test
// return a full response body (data or errors) per request.
type gqlRecorder struct {
	mu    sync.Mutex
	calls []gqlCall
}

// newGQLRecorder starts a server whose handler is fn(op, vars, nthCallOfThatOp) and
// returns the raw GraphQL response body to send.
func newGQLRecorder(t *testing.T, fn func(op string, vars map[string]any, n int) string) (*gqlRecorder, *httptest.Server) {
	t.Helper()
	rec := &gqlRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read body", http.StatusBadRequest)
			return
		}
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			http.Error(w, "parse body", http.StatusBadRequest)
			return
		}
		op, _ := body["operationName"].(string)
		vars, _ := body["variables"].(map[string]any)

		rec.mu.Lock()
		n := 0
		for _, c := range rec.calls {
			if c.op == op {
				n++
			}
		}
		rec.calls = append(rec.calls, gqlCall{op: op, vars: vars})
		rec.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, fn(op, vars, n))
	}))
	t.Cleanup(srv.Close)
	installItemFactory(t, srv.URL)
	return rec, srv
}

// ops returns the recorded operation names in order.
func (r *gqlRecorder) ops() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.calls))
	for _, c := range r.calls {
		out = append(out, c.op)
	}
	return out
}

// countOp returns how many times op was called.
func (r *gqlRecorder) countOp(op string) int {
	n := 0
	for _, got := range r.ops() {
		if got == op {
			n++
		}
	}
	return n
}

// varsFor returns the variables of the nth (0-based) call to op.
func (r *gqlRecorder) varsFor(op string, n int) map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	seen := 0
	for _, c := range r.calls {
		if c.op != op {
			continue
		}
		if seen == n {
			return c.vars
		}
		seen++
	}
	return nil
}

// boardColumnsBody is a BoardColumnList response for a board with one column of each
// shorthand-addressable type, plus an archived status column that must be ignored (it
// would otherwise make --status ambiguous even though it cannot be written).
func boardColumnsBody() string {
	cols := []map[string]any{
		{"id": "name", "title": "Name", "type": "name", "settings_str": "{}", "width": 200, "archived": false},
		{"id": "status_1", "title": "Stage", "type": "status", "settings_str": testStatusSettings, "width": 150, "archived": false},
		{"id": "status_old", "title": "Old Stage", "type": "status", "settings_str": testStatusSettings, "width": 150, "archived": true},
		{"id": "date_4", "title": "Due", "type": "date", "settings_str": "{}", "width": 150, "archived": false},
		{"id": "numbers_7", "title": "Estimate", "type": "numbers", "settings_str": "{}", "width": 150, "archived": false},
		{"id": "text_9", "title": "Notes", "type": "text", "settings_str": "{}", "width": 150, "archived": false},
		{"id": "checkbox_2", "title": "Blocked", "type": "checkbox", "settings_str": "{}", "width": 150, "archived": false},
	}
	boards := []map[string]any{{"id": "9832181507", "columns": cols}}
	return fmt.Sprintf(`{"data":%s}`, mustMarshal(map[string]any{"boards": boards}))
}

// boardColumnsBodyTextOnly is a board with no column of most shorthand types, for the
// "this board has none" path.
func boardColumnsBodyTextOnly() string {
	cols := []map[string]any{
		{"id": "name", "title": "Name", "type": "name", "settings_str": "{}", "width": 200, "archived": false},
		{"id": "text_9", "title": "Notes", "type": "text", "settings_str": "{}", "width": 150, "archived": false},
	}
	boards := []map[string]any{{"id": "9832181507", "columns": cols}}
	return fmt.Sprintf(`{"data":%s}`, mustMarshal(map[string]any{"boards": boards}))
}

// itemCreateBody is a successful create_item response.
func itemCreateBody(id, name string) string {
	return fmt.Sprintf(`{"data":{"create_item":{"id":%q,"name":%q,"state":"active",`+
		`"board":{"id":"9832181507","name":"Dev Board"},"group":{"id":"topics","title":"Sprint 1"}}}}`, id, name)
}

// itemUpdateBody is a successful change_multiple_column_values response.
func itemUpdateBody(id, name string) string {
	return fmt.Sprintf(`{"data":{"change_multiple_column_values":{"id":%q,"name":%q,"state":"active",`+
		`"board":{"id":"9832181507","name":"Dev Board"},"group":{"id":"topics","title":"Sprint 1"}}}}`, id, name)
}

// gqlErrorBody is a GraphQL error response.
func gqlErrorBody(message, code string) string {
	return fmt.Sprintf(`{"errors":[{"message":%q,"extensions":{"code":%q}}]}`, message, code)
}

// execItemCreateStdin runs 'item create' with stdin wired to in.
func execItemCreateStdin(t *testing.T, in string, args ...string) (string, error) {
	t.Helper()
	globals = GlobalFlags{JSON: true}
	defer func() { globals = GlobalFlags{} }()

	var buf bytes.Buffer
	cmd := newItemCmd()
	// Mirror the production root command: usage text must not land on stdout, which
	// batch mode uses for its JSON result even when it exits non-zero.
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetIn(strings.NewReader(in))
	cmd.SetArgs(append([]string{"create"}, args...))
	err := cmd.Execute()
	return buf.String(), err
}

// execItemUpdateStdin runs 'item update' with stdin wired to in.
func execItemUpdateStdin(t *testing.T, in string, args ...string) (string, error) {
	t.Helper()
	globals = GlobalFlags{JSON: true}
	defer func() { globals = GlobalFlags{} }()

	var buf bytes.Buffer
	cmd := newItemCmd()
	// Mirror the production root command: usage text must not land on stdout, which
	// batch mode uses for its JSON result even when it exits non-zero.
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetIn(strings.NewReader(in))
	cmd.SetArgs(append([]string{"update"}, args...))
	err := cmd.Execute()
	return buf.String(), err
}

// decodeBatch parses batch output.
func decodeBatch(t *testing.T, out string) batchWriteOutput {
	t.Helper()
	var got batchWriteOutput
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &got); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	return got
}

// ---- jsonScalar ----

// TestJSONScalar accepts the three JSON scalar forms, so a row may write
// "number": 42 without quoting and "checkbox": true without stringifying.
func TestJSONScalar(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: `"Done"`, want: "Done"},
		{in: `42`, want: "42"},
		{in: `42.5`, want: "42.5"},
		{in: `true`, want: "true"},
		{in: `false`, want: "false"},
		{in: `""`, want: ""},
	}
	for _, tt := range tests {
		var got jsonScalar
		if err := json.Unmarshal([]byte(tt.in), &got); err != nil {
			t.Errorf("Unmarshal(%s): %v", tt.in, err)
			continue
		}
		if string(got) != tt.want {
			t.Errorf("Unmarshal(%s) = %q, want %q", tt.in, got, tt.want)
		}
	}

	for _, bad := range []string{`null`, `[1]`, `{"a":1}`} {
		var got jsonScalar
		if err := json.Unmarshal([]byte(bad), &got); err == nil {
			t.Errorf("Unmarshal(%s) = %q, want an error", bad, got)
		}
	}
}

// ---- parsing ----

func TestParseBatchRows_arrayAndNDJSON(t *testing.T) {
	array := `[{"name":"a"},{"name":"b"}]`
	ndjson := "{\"name\":\"a\"}\n{\"name\":\"b\"}\n"

	for _, in := range []string{array, ndjson} {
		rows, err := parseBatchRows(strings.NewReader(in))
		if err != nil {
			t.Fatalf("parseBatchRows(%q): %v", in, err)
		}
		if len(rows) != 2 || rows[0].Name != "a" || rows[1].Name != "b" {
			t.Errorf("parseBatchRows(%q) = %+v", in, rows)
		}
	}
}

// TestParseBatchRows_rejectsUnknownField is why DisallowUnknownFields is set: a
// mistyped key would otherwise drop the caller's data and report success.
func TestParseBatchRows_rejectsUnknownField(t *testing.T) {
	_, err := parseBatchRows(strings.NewReader(`[{"name":"a","columns":{"x":1}}]`))
	if err == nil {
		t.Fatal("want an error for an unknown row field")
	}
	if codeOf(err) != errs.CodeUsage {
		t.Errorf("code = %s, want %s", codeOf(err), errs.CodeUsage)
	}
}

func TestParseBatchRows_errors(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{name: "empty", in: ""},
		{name: "whitespace only", in: "  \n\t\n"},
		{name: "empty array", in: "[]"},
		{name: "truncated array", in: `[{"name":"a"}`},
		{name: "trailing content after array", in: `[{"name":"a"}] {"name":"b"}`},
		{name: "not an object", in: `"a string"`},
		{name: "bad ndjson second row", in: "{\"name\":\"a\"}\nnope\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows, err := parseBatchRows(strings.NewReader(tt.in))
			if err == nil {
				t.Fatalf("parseBatchRows(%q) = %+v, want an error", tt.in, rows)
			}
			if codeOf(err) != errs.CodeUsage {
				t.Errorf("code = %s, want %s", codeOf(err), errs.CodeUsage)
			}
		})
	}
}

func TestParseBatchRows_maxRows(t *testing.T) {
	rows := make([]string, batchMaxRows)
	for i := range rows {
		rows[i] = fmt.Sprintf(`{"name":"item %d"}`, i)
	}
	atMax := "[" + strings.Join(rows, ",") + "]"

	got, err := parseBatchRows(strings.NewReader(atMax))
	if err != nil {
		t.Fatalf("parseBatchRows at max (%d rows): %v", batchMaxRows, err)
	}
	if len(got) != batchMaxRows {
		t.Errorf("parsed %d rows, want %d", len(got), batchMaxRows)
	}

	overMax := "[" + strings.Join(append(rows, `{"name":"one too many"}`), ",") + "]"
	if _, err := parseBatchRows(strings.NewReader(overMax)); err == nil {
		t.Errorf("parseBatchRows accepted %d rows, want a cap at %d", batchMaxRows+1, batchMaxRows)
	}
}

// ---- create batch ----

func TestItemCreateBatch_happyPath(t *testing.T) {
	rec, _ := newGQLRecorder(t, func(op string, vars map[string]any, n int) string {
		if op != "ItemCreate" {
			t.Errorf("unexpected op %q", op)
		}
		name, _ := vars["name"].(string)
		return itemCreateBody(fmt.Sprintf("100%d", n), name)
	})

	in := `[{"name":"a","group":"topics"},{"name":"b"},{"name":"c","cols":{"text_9":"hi"}}]`
	out, err := execItemCreateStdin(t, in, "--board", "9832181507", "-")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := decodeBatch(t, out)
	if got.Written != 3 || got.Failed != 0 {
		t.Errorf("written=%d failed=%d, want 3/0", got.Written, got.Failed)
	}
	if got.Verb != "created" {
		t.Errorf("verb = %q, want created", got.Verb)
	}
	if len(got.Items) != 3 {
		t.Fatalf("items = %d, want 3", len(got.Items))
	}
	if got.Errors == nil {
		t.Error("errors is null; it must always be present as an array")
	}
	if rec.countOp("ItemCreate") != 3 {
		t.Errorf("made %d ItemCreate calls, want 3", rec.countOp("ItemCreate"))
	}
	// No shorthand in the payload, so the board's columns must not be fetched.
	if rec.countOp("BoardColumnList") != 0 {
		t.Errorf("fetched board columns %d times for a --col-only batch, want 0", rec.countOp("BoardColumnList"))
	}

	if cv, _ := rec.varsFor("ItemCreate", 2)["columnValues"].(string); cv != `{"text_9":"hi"}` {
		t.Errorf("row 2 columnValues = %q, want the row's cols", cv)
	}
	if gid, _ := rec.varsFor("ItemCreate", 0)["groupId"].(string); gid != "topics" {
		t.Errorf("row 0 groupId = %q, want topics", gid)
	}
}

func TestItemCreateBatch_ndjson(t *testing.T) {
	rec, _ := newGQLRecorder(t, func(_ string, vars map[string]any, n int) string {
		name, _ := vars["name"].(string)
		return itemCreateBody(fmt.Sprintf("200%d", n), name)
	})

	in := "{\"name\":\"a\"}\n{\"name\":\"b\"}\n"
	out, err := execItemCreateStdin(t, in, "--board", "9832181507", "-")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := decodeBatch(t, out); got.Written != 2 {
		t.Errorf("written = %d, want 2", got.Written)
	}
	if rec.countOp("ItemCreate") != 2 {
		t.Errorf("made %d ItemCreate calls, want 2", rec.countOp("ItemCreate"))
	}
}

// TestItemCreateBatch_shorthandsFetchColumnsOnce is the rate-limit point of the
// design: N rows using shorthands cost one column lookup, not N.
func TestItemCreateBatch_shorthandsFetchColumnsOnce(t *testing.T) {
	rec, _ := newGQLRecorder(t, func(op string, vars map[string]any, n int) string {
		switch op {
		case "BoardColumnList":
			return boardColumnsBody()
		case "ItemCreate":
			name, _ := vars["name"].(string)
			return itemCreateBody(fmt.Sprintf("300%d", n), name)
		default:
			t.Errorf("unexpected op %q", op)
			return `{"data":{}}`
		}
	})

	in := `[{"name":"a","status":"done","due":"2026-05-10"},{"name":"b","number":42,"checkbox":true},{"name":"c"}]`
	out, err := execItemCreateStdin(t, in, "--board", "9832181507", "-")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := decodeBatch(t, out); got.Written != 3 {
		t.Errorf("written = %d, want 3", got.Written)
	}
	if n := rec.countOp("BoardColumnList"); n != 1 {
		t.Errorf("fetched board columns %d times, want exactly 1 for the whole batch", n)
	}

	cv0, _ := rec.varsFor("ItemCreate", 0)["columnValues"].(string)
	var parsed0 map[string]json.RawMessage
	if err := json.Unmarshal([]byte(cv0), &parsed0); err != nil {
		t.Fatalf("row 0 columnValues is not JSON: %q", cv0)
	}
	if string(parsed0["status_1"]) != `{"label":"Done"}` {
		t.Errorf("row 0 status_1 = %s, want the board's own casing", parsed0["status_1"])
	}
	if string(parsed0["date_4"]) != `{"date":"2026-05-10"}` {
		t.Errorf("row 0 date_4 = %s", parsed0["date_4"])
	}

	cv1, _ := rec.varsFor("ItemCreate", 1)["columnValues"].(string)
	var parsed1 map[string]json.RawMessage
	if err := json.Unmarshal([]byte(cv1), &parsed1); err != nil {
		t.Fatalf("row 1 columnValues is not JSON: %q", cv1)
	}
	if string(parsed1["numbers_7"]) != `"42"` {
		t.Errorf("row 1 numbers_7 = %s, want a bare JSON number to be accepted", parsed1["numbers_7"])
	}
	if string(parsed1["checkbox_2"]) != `{"checked":"true"}` {
		t.Errorf("row 1 checkbox_2 = %s, want a bare JSON bool to be accepted", parsed1["checkbox_2"])
	}

	// Row 2 sets nothing, so it must send an empty column_values rather than "{}".
	if cv2, _ := rec.varsFor("ItemCreate", 2)["columnValues"].(string); cv2 != "" {
		t.Errorf("row 2 columnValues = %q, want empty", cv2)
	}
}

// TestItemCreateBatch_badLabelFailsBeforeAnyWrite: one bad status label must not
// leave the first N items created. Validation happens before the first mutation.
func TestItemCreateBatch_badLabelFailsBeforeAnyWrite(t *testing.T) {
	rec, _ := newGQLRecorder(t, func(op string, _ map[string]any, _ int) string {
		if op == "BoardColumnList" {
			return boardColumnsBody()
		}
		t.Errorf("unexpected write %q: the batch should have failed validation first", op)
		return `{"data":{}}`
	})

	in := `[{"name":"a","status":"done"},{"name":"b","status":"Dunn"}]`
	_, err := execItemCreateStdin(t, in, "--board", "9832181507", "-")
	if err == nil {
		t.Fatal("want an error for an unknown status label")
	}
	if codeOf(err) != errs.CodeUsage {
		t.Errorf("code = %s, want %s", codeOf(err), errs.CodeUsage)
	}
	if !strings.Contains(err.Error(), "row 1") {
		t.Errorf("error %q does not name the offending row", err.Error())
	}
	if rec.countOp("ItemCreate") != 0 {
		t.Errorf("created %d items before failing, want 0", rec.countOp("ItemCreate"))
	}
}

// TestItemCreateBatch_partialFailure is the contract that makes a batch retryable:
// exit 2, and errors[].index names exactly which rows to re-send.
func TestItemCreateBatch_partialFailure(t *testing.T) {
	rec, _ := newGQLRecorder(t, func(_ string, vars map[string]any, n int) string {
		if n == 1 {
			return gqlErrorBody("column not found", "InvalidColumnIdException")
		}
		name, _ := vars["name"].(string)
		return itemCreateBody(fmt.Sprintf("400%d", n), name)
	})

	in := `[{"name":"a"},{"name":"b"},{"name":"c"}]`
	out, err := execItemCreateStdin(t, in, "--board", "9832181507", "-")
	if err == nil {
		t.Fatal("want an error so a partial failure does not exit 0")
	}
	if got := errs.ToExitCode(err); got != 2 {
		t.Errorf("exit code = %d, want 2", got)
	}

	got := decodeBatch(t, out)
	if got.Written != 2 || got.Failed != 1 {
		t.Errorf("written=%d failed=%d, want 2/1", got.Written, got.Failed)
	}
	if len(got.Errors) != 1 {
		t.Fatalf("errors = %d, want 1", len(got.Errors))
	}
	if got.Errors[0].Index != 1 {
		t.Errorf("failed row index = %d, want 1", got.Errors[0].Index)
	}
	if got.Errors[0].Code == "" {
		t.Error("failed row has no code")
	}
	if got.Errors[0].Code == string(errs.CodeInternal) {
		t.Errorf("failed row code = %s; an API rejection should not be reported as internal", got.Errors[0].Code)
	}
	if !strings.Contains(got.Errors[0].Message, "column not found") {
		t.Errorf("failed row message = %q, want the API's message", got.Errors[0].Message)
	}
	// All three rows are attempted: one bad row does not abandon the rest.
	if rec.countOp("ItemCreate") != 3 {
		t.Errorf("made %d ItemCreate calls, want 3", rec.countOp("ItemCreate"))
	}
}

func TestItemCreateBatch_validationErrors(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{name: "missing name", in: `[{"cols":{"text_9":"hi"}}]`},
		{name: "blank name", in: `[{"name":"   "}]`},
		{name: "id not allowed on create", in: `[{"name":"a","id":"456"}]`},
		{name: "date and due together", in: `[{"name":"a","date":"2026-05-10","due":"2026-06-01"}]`},
		{name: "empty col id", in: `[{"name":"a","cols":{"":"hi"}}]`},
		{name: "second row invalid", in: `[{"name":"a"},{"name":""}]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, _ := newGQLRecorder(t, func(op string, _ map[string]any, _ int) string {
				t.Errorf("unexpected request %q for invalid input", op)
				return `{"data":{}}`
			})
			_, err := execItemCreateStdin(t, tt.in, "--board", "9832181507", "-")
			if err == nil {
				t.Fatalf("want an error for %s", tt.name)
			}
			if codeOf(err) != errs.CodeUsage {
				t.Errorf("code = %s, want %s", codeOf(err), errs.CodeUsage)
			}
			if len(rec.ops()) != 0 {
				t.Errorf("made %d requests for invalid input, want 0", len(rec.ops()))
			}
		})
	}
}

// TestItemCreateBatch_flagsRejected: silently ignoring --name in batch mode would
// create N items that all disagree with what the caller typed.
func TestItemCreateBatch_flagsRejected(t *testing.T) {
	for _, extra := range [][]string{
		{"--name", "x"},
		{"--col", "text_9=\"hi\""},
		{"--status", "Done"},
	} {
		args := append([]string{"--board", "9832181507", "-"}, extra...)
		_, err := execItemCreateStdin(t, `[{"name":"a"}]`, args...)
		if err == nil {
			t.Errorf("want an error for batch mode with %v", extra)
			continue
		}
		if codeOf(err) != errs.CodeUsage {
			t.Errorf("code for %v = %s, want %s", extra, codeOf(err), errs.CodeUsage)
		}
	}
}

func TestItemCreateBatch_parentRejected(t *testing.T) {
	_, err := execItemCreateStdin(t, `[{"name":"a"}]`, "--parent", "456", "-")
	if err == nil {
		t.Fatal("want an error: subitems cannot be created in batch")
	}
	if codeOf(err) != errs.CodeUsage {
		t.Errorf("code = %s, want %s", codeOf(err), errs.CodeUsage)
	}
}

// TestItemCreateBatch_dryRunSendsNothing keeps --dry-run honest: it must not need a
// token and must not write.
func TestItemCreateBatch_dryRunSendsNothing(t *testing.T) {
	rec, _ := newGQLRecorder(t, func(op string, _ map[string]any, _ int) string {
		t.Errorf("dry run made a request: %q", op)
		return `{"data":{}}`
	})

	in := `[{"name":"a","cols":{"text_9":"hi"}},{"name":"b"}]`
	out, err := execItemCreateStdin(t, in, "--board", "9832181507", "-", "--dry-run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rec.ops()) != 0 {
		t.Errorf("dry run made %d requests, want 0", len(rec.ops()))
	}

	var got batchDryRunOutput
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &got); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	if !got.DryRun || got.Rows != 2 || len(got.Items) != 2 {
		t.Fatalf("dry run output = %+v", got)
	}
	if got.Items[0].ColumnValues != `{"text_9":"hi"}` {
		t.Errorf("row 0 column_values = %q", got.Items[0].ColumnValues)
	}
	if got.Items[1].Name != "b" {
		t.Errorf("row 1 name = %q, want b", got.Items[1].Name)
	}
}

// TestItemCreateBatch_dryRunWithShorthandsResolvesColumns: a dry run is the way to
// check a shorthand resolves, so it does fetch columns — but still writes nothing.
func TestItemCreateBatch_dryRunWithShorthandsResolvesColumns(t *testing.T) {
	rec, _ := newGQLRecorder(t, func(op string, _ map[string]any, _ int) string {
		if op == "BoardColumnList" {
			return boardColumnsBody()
		}
		t.Errorf("dry run made a write request: %q", op)
		return `{"data":{}}`
	})

	out, err := execItemCreateStdin(t, `[{"name":"a","status":"Done"}]`, "--board", "9832181507", "-", "--dry-run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n := rec.countOp("BoardColumnList"); n != 1 {
		t.Errorf("fetched board columns %d times, want 1", n)
	}
	if rec.countOp("ItemCreate") != 0 {
		t.Error("dry run created an item")
	}
	if !strings.Contains(out, `status_1`) {
		t.Errorf("dry run output does not show the resolved column: %s", out)
	}
}

// TestItemCreateBatch_positionalArgRejected: a bare item name as a positional arg is
// an easy mistake, and silently ignoring it would create nothing useful.
func TestItemCreateBatch_positionalArgRejected(t *testing.T) {
	_, err := execItemCreateStdin(t, "", "--board", "9832181507", "My Item")
	if err == nil {
		t.Fatal("want an error for an unexpected positional argument")
	}
	if codeOf(err) != errs.CodeUsage {
		t.Errorf("code = %s, want %s", codeOf(err), errs.CodeUsage)
	}
}

// ---- update batch ----

func TestItemUpdateBatch_happyPath(t *testing.T) {
	rec, _ := newGQLRecorder(t, func(op string, vars map[string]any, _ int) string {
		if op != "ItemUpdate" {
			t.Errorf("unexpected op %q", op)
		}
		id, _ := vars["itemId"].(string)
		return itemUpdateBody(id, "renamed")
	})

	in := "{\"id\":\"111\",\"cols\":{\"text_9\":\"hi\"}}\n{\"id\":\"222\",\"name\":\"renamed\"}\n"
	out, err := execItemUpdateStdin(t, in, "--board", "9832181507", "-")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := decodeBatch(t, out)
	if got.Written != 2 || got.Failed != 0 {
		t.Errorf("written=%d failed=%d, want 2/0", got.Written, got.Failed)
	}
	if got.Verb != "updated" {
		t.Errorf("verb = %q, want updated", got.Verb)
	}
	if id, _ := rec.varsFor("ItemUpdate", 0)["itemId"].(string); id != "111" {
		t.Errorf("row 0 itemId = %q, want 111", id)
	}
	// A row's "name" is written through the name column, as the single path does.
	if cv, _ := rec.varsFor("ItemUpdate", 1)["columnValues"].(string); cv != `{"name":"renamed"}` {
		t.Errorf("row 1 columnValues = %q, want the name column", cv)
	}
}

func TestItemUpdateBatch_validationErrors(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{name: "missing id", in: `[{"name":"a"}]`},
		{name: "non-numeric id", in: `[{"id":"abc","name":"a"}]`},
		{name: "nothing to update", in: `[{"id":"111"}]`},
		{name: "second row has no id", in: `[{"id":"111","name":"a"},{"name":"b"}]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, _ := newGQLRecorder(t, func(op string, _ map[string]any, _ int) string {
				t.Errorf("unexpected request %q for invalid input", op)
				return `{"data":{}}`
			})
			_, err := execItemUpdateStdin(t, tt.in, "--board", "9832181507", "-")
			if err == nil {
				t.Fatalf("want an error for %s", tt.name)
			}
			if codeOf(err) != errs.CodeUsage {
				t.Errorf("code = %s, want %s", codeOf(err), errs.CodeUsage)
			}
			if len(rec.ops()) != 0 {
				t.Errorf("made %d requests for invalid input, want 0", len(rec.ops()))
			}
		})
	}
}

func TestItemUpdateBatch_partialFailure(t *testing.T) {
	newGQLRecorder(t, func(_ string, vars map[string]any, n int) string {
		if n == 0 {
			return gqlErrorBody("item not found", "ResourceNotFoundException")
		}
		id, _ := vars["itemId"].(string)
		return itemUpdateBody(id, "ok")
	})

	in := `[{"id":"111","name":"a"},{"id":"222","name":"b"}]`
	out, err := execItemUpdateStdin(t, in, "--board", "9832181507", "-")
	if err == nil {
		t.Fatal("want an error so a partial failure does not exit 0")
	}
	if got := errs.ToExitCode(err); got != 2 {
		t.Errorf("exit code = %d, want 2", got)
	}

	got := decodeBatch(t, out)
	if got.Written != 1 || got.Failed != 1 {
		t.Errorf("written=%d failed=%d, want 1/1", got.Written, got.Failed)
	}
	if len(got.Errors) != 1 {
		t.Fatalf("errors = %d, want 1", len(got.Errors))
	}
	// The item id is echoed on the failure so a caller can retry by id, not position.
	if got.Errors[0].ID != "111" {
		t.Errorf("failed row id = %q, want 111", got.Errors[0].ID)
	}
	if got.Errors[0].Index != 0 {
		t.Errorf("failed row index = %d, want 0", got.Errors[0].Index)
	}
}

func TestItemUpdateBatch_flagsRejected(t *testing.T) {
	_, err := execItemUpdateStdin(t, `[{"id":"111","name":"a"}]`, "--board", "9832181507", "-", "--name", "x")
	if err == nil {
		t.Fatal("want an error for batch mode with --name")
	}
	if codeOf(err) != errs.CodeUsage {
		t.Errorf("code = %s, want %s", codeOf(err), errs.CodeUsage)
	}
}

// ---- single-item shorthands ----

func TestItemCreate_shorthandFlags(t *testing.T) {
	rec, _ := newGQLRecorder(t, func(op string, vars map[string]any, _ int) string {
		if op == "BoardColumnList" {
			return boardColumnsBody()
		}
		name, _ := vars["name"].(string)
		return itemCreateBody("5001", name)
	})

	_, err := execItemCreateStdin(t, "", "--board", "9832181507", "--name", "Ship v1",
		"--status", "working on it", "--due", "2026-05-10T09:00", "--number", "3", "--checkbox", "yes")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cv, _ := rec.varsFor("ItemCreate", 0)["columnValues"].(string)
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal([]byte(cv), &parsed); err != nil {
		t.Fatalf("columnValues is not JSON: %q", cv)
	}
	want := map[string]string{
		"status_1":   `{"label":"Working on it"}`,
		"date_4":     `{"date":"2026-05-10","time":"09:00:00"}`,
		"numbers_7":  `"3"`,
		"checkbox_2": `{"checked":"true"}`,
	}
	for col, wantVal := range want {
		if string(parsed[col]) != wantVal {
			t.Errorf("%s = %s, want %s", col, parsed[col], wantVal)
		}
	}
	// The archived status column must not have been written to.
	if _, ok := parsed["status_old"]; ok {
		t.Error("wrote to an archived column")
	}
}

// TestItemCreate_shorthandNoColumnOfType: the board fixture has no checkbox column,
// so --checkbox must fail loudly rather than being dropped.
func TestItemCreate_shorthandNoColumnOfType(t *testing.T) {
	rec, _ := newGQLRecorder(t, func(op string, _ map[string]any, _ int) string {
		if op == "BoardColumnList" {
			return boardColumnsBodyTextOnly()
		}
		t.Errorf("unexpected write %q", op)
		return `{"data":{}}`
	})

	_, err := execItemCreateStdin(t, "", "--board", "9832181507", "--name", "x", "--checkbox", "true")
	if err == nil {
		t.Fatal("want an error: the board has no checkbox column")
	}
	if codeOf(err) != errs.CodeUsage {
		t.Errorf("code = %s, want %s", codeOf(err), errs.CodeUsage)
	}
	if rec.countOp("ItemCreate") != 0 {
		t.Error("created an item despite an unresolvable shorthand")
	}
}

// TestItemCreate_shorthandCollidesWithCol keeps an ambiguous write from happening.
func TestItemCreate_shorthandCollidesWithCol(t *testing.T) {
	newGQLRecorder(t, func(op string, _ map[string]any, _ int) string {
		if op == "BoardColumnList" {
			return boardColumnsBody()
		}
		t.Errorf("unexpected write %q", op)
		return `{"data":{}}`
	})

	_, err := execItemCreateStdin(t, "", "--board", "9832181507", "--name", "x",
		"--status", "Done", "--col", `status_1={"index":2}`)
	if err == nil {
		t.Fatal("want an error when --status and --col target one column")
	}
	if codeOf(err) != errs.CodeUsage {
		t.Errorf("code = %s, want %s", codeOf(err), errs.CodeUsage)
	}
}

func TestItemCreate_shorthandWithParentRejected(t *testing.T) {
	_, err := execItemCreateStdin(t, "", "--parent", "456", "--name", "x", "--status", "Done")
	if err == nil {
		t.Fatal("want an error: shorthands need --board")
	}
	if codeOf(err) != errs.CodeUsage {
		t.Errorf("code = %s, want %s", codeOf(err), errs.CodeUsage)
	}
}

// TestItemCreate_dateAndDueMutuallyExclusive: cobra enforces this, and it must stay
// enforced — the two flags write the same column.
func TestItemCreate_dateAndDueMutuallyExclusive(t *testing.T) {
	_, err := execItemCreateStdin(t, "", "--board", "9832181507", "--name", "x",
		"--date", "2026-05-10", "--due", "2026-06-01")
	if err == nil {
		t.Fatal("want an error when both --date and --due are given")
	}
}

// TestItemUpdate_shorthandFlags checks the update path resolves shorthands too, and
// that --name still travels as the name column alongside them.
func TestItemUpdate_shorthandFlags(t *testing.T) {
	rec, _ := newGQLRecorder(t, func(op string, vars map[string]any, _ int) string {
		if op == "BoardColumnList" {
			return boardColumnsBody()
		}
		id, _ := vars["itemId"].(string)
		return itemUpdateBody(id, "x")
	})

	_, err := execItemUpdateStdin(t, "", "111", "--board", "9832181507", "--name", "x", "--status", "Done")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cv, _ := rec.varsFor("ItemUpdate", 0)["columnValues"].(string)
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal([]byte(cv), &parsed); err != nil {
		t.Fatalf("columnValues is not JSON: %q", cv)
	}
	if string(parsed["status_1"]) != `{"label":"Done"}` {
		t.Errorf("status_1 = %s", parsed["status_1"])
	}
	if string(parsed["name"]) != `"x"` {
		t.Errorf("name = %s, want \"x\"", parsed["name"])
	}
}

// TestItemUpdate_shorthandOnlyIsEnough: a shorthand alone must satisfy the
// "nothing to update" guard, which predates shorthands.
func TestItemUpdate_shorthandOnlyIsEnough(t *testing.T) {
	rec, _ := newGQLRecorder(t, func(op string, vars map[string]any, _ int) string {
		if op == "BoardColumnList" {
			return boardColumnsBody()
		}
		id, _ := vars["itemId"].(string)
		return itemUpdateBody(id, "x")
	})

	if _, err := execItemUpdateStdin(t, "", "111", "--board", "9832181507", "--status", "Done"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.countOp("ItemUpdate") != 1 {
		t.Errorf("made %d ItemUpdate calls, want 1", rec.countOp("ItemUpdate"))
	}
}

// TestItemCreate_clearTextWithEmptyShorthand: --text "" must reach the API as an
// empty value, not be treated as "flag not set".
func TestItemCreate_clearTextWithEmptyShorthand(t *testing.T) {
	rec, _ := newGQLRecorder(t, func(op string, vars map[string]any, _ int) string {
		if op == "BoardColumnList" {
			return boardColumnsBody()
		}
		name, _ := vars["name"].(string)
		return itemCreateBody("6001", name)
	})

	if _, err := execItemCreateStdin(t, "", "--board", "9832181507", "--name", "x", "--text", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cv, _ := rec.varsFor("ItemCreate", 0)["columnValues"].(string)
	if cv != `{"text_9":""}` {
		t.Errorf("columnValues = %q, want the text column cleared", cv)
	}
}
