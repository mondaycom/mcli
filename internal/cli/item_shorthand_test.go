package cli

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/mondaycom/mcli/internal/api/items/columns"
	"github.com/mondaycom/mcli/internal/errs"
)

// statusSettings is a status column's settings_str as monday reports it.
const testStatusSettings = `{"labels":{"0":"Not Started","1":"Working on it","2":"Done"}}`

// codeOf returns an error's errs code, or "" if it is not an *errs.Error.
func codeOf(err error) errs.Code {
	if e, ok := errors.AsType[*errs.Error](err); ok {
		return e.Code
	}
	return ""
}

// testIndex builds a boardColumnIndex without going through the API.
func testIndex(cols ...writableColumn) *boardColumnIndex {
	idx := &boardColumnIndex{byType: map[string][]writableColumn{}}
	for _, c := range cols {
		idx.byType[c.colType] = append(idx.byType[c.colType], c)
	}
	for t := range idx.byType {
		idx.types = append(idx.types, t)
	}
	return idx
}

// specFor returns the shorthandSpec for a flag name.
func specFor(t *testing.T, flag string) shorthandSpec {
	t.Helper()
	for _, sp := range shorthandSpecs {
		if sp.flag == flag {
			return sp
		}
	}
	t.Fatalf("no shorthand spec for --%s", flag)
	return shorthandSpec{}
}

// TestShorthandSpecs_AreEncodable guards the contract in shorthandSpecs' doc comment:
// a flag whose column type has no encoder would fail at runtime, not at build time.
func TestShorthandSpecs_AreEncodable(t *testing.T) {
	encodable := map[string]bool{}
	for _, ct := range columns.EncodableTypes() {
		encodable[ct] = true
	}
	for _, sp := range shorthandSpecs {
		if !encodable[sp.columnType] {
			t.Errorf("--%s writes column type %q, which columns.Encode does not support", sp.flag, sp.columnType)
		}
	}
}

func TestBoardColumnIndex_resolveOne(t *testing.T) {
	idx := testIndex(
		writableColumn{id: "status_1", title: "Stage", colType: "status", settings: testStatusSettings},
		writableColumn{id: "text_9", title: "Notes", colType: "text"},
	)
	got, err := idx.resolve(specFor(t, "status"))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got.id != "status_1" {
		t.Errorf("resolved column = %q, want status_1", got.id)
	}
}

// TestBoardColumnIndex_resolveNone checks the error names the types the board does
// have, so a caller can pick a real column instead of guessing again.
func TestBoardColumnIndex_resolveNone(t *testing.T) {
	idx := testIndex(
		writableColumn{id: "text_9", title: "Notes", colType: "text"},
		writableColumn{id: "people_2", title: "Owner", colType: "people"},
	)
	_, err := idx.resolve(specFor(t, "status"))
	if err == nil {
		t.Fatal("want an error when the board has no status column")
	}
	if codeOf(err) != errs.CodeUsage {
		t.Errorf("code = %s, want %s", codeOf(err), errs.CodeUsage)
	}
	for _, want := range []string{"text", "people"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name existing type %q", err.Error(), want)
		}
	}
}

// TestBoardColumnIndex_resolveAmbiguous is the important one: guessing here would
// write a correct value to the wrong column, silently.
func TestBoardColumnIndex_resolveAmbiguous(t *testing.T) {
	idx := testIndex(
		writableColumn{id: "status_1", title: "Stage", colType: "status", settings: testStatusSettings},
		writableColumn{id: "status_2", title: "Priority", colType: "status", settings: testStatusSettings},
	)
	_, err := idx.resolve(specFor(t, "status"))
	if err == nil {
		t.Fatal("want an error when the board has two status columns")
	}
	if codeOf(err) != errs.CodeUsage {
		t.Errorf("code = %s, want %s", codeOf(err), errs.CodeUsage)
	}
	for _, want := range []string{"status_1", "Stage", "status_2", "Priority", "--col"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err.Error(), want)
		}
	}
}

func TestEncodeShorthands(t *testing.T) {
	idx := testIndex(
		writableColumn{id: "status_1", title: "Stage", colType: "status", settings: testStatusSettings},
		writableColumn{id: "date_4", title: "Due", colType: "date"},
		writableColumn{id: "numbers_7", title: "Estimate", colType: "numbers"},
	)

	set := []shorthandValue{
		{spec: specFor(t, "status"), value: "done"},
		{spec: specFor(t, "due"), value: "2026-05-10"},
		{spec: specFor(t, "number"), value: "3.5"},
	}

	enc, err := encodeShorthands(idx, set)
	if err != nil {
		t.Fatalf("encodeShorthands: %v", err)
	}
	if len(enc) != 3 {
		t.Fatalf("encoded %d shorthands, want 3", len(enc))
	}

	byCol := map[string]string{}
	for _, e := range enc {
		byCol[e.colID] = string(e.value)
	}
	want := map[string]string{
		"status_1":  `{"label":"Done"}`,
		"date_4":    `{"date":"2026-05-10"}`,
		"numbers_7": `"3.5"`,
	}
	for col, wantVal := range want {
		if byCol[col] != wantVal {
			t.Errorf("column %s = %s, want %s", col, byCol[col], wantVal)
		}
	}
}

// TestEncodeShorthands_badLabelNamesFlagAndColumn checks the encoder's error is
// re-pointed at what the caller typed. "status: 'Dunn' is not a label" is much less
// useful than naming the flag and the column it resolved to.
func TestEncodeShorthands_badLabelNamesFlagAndColumn(t *testing.T) {
	idx := testIndex(writableColumn{id: "status_1", title: "Stage", colType: "status", settings: testStatusSettings})
	_, err := encodeShorthands(idx, []shorthandValue{{spec: specFor(t, "status"), value: "Dunn"}})
	if err == nil {
		t.Fatal("want an error for an unknown status label")
	}
	if codeOf(err) != errs.CodeUsage {
		t.Errorf("code = %s, want %s", codeOf(err), errs.CodeUsage)
	}
	for _, want := range []string{"--status", "status_1", "Stage", "Done"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err.Error(), want)
		}
	}
}

// TestEncodeShorthands_twoFlagsOneColumn covers --date and --due both landing on the
// board's only date column when the mutual-exclusion check is bypassed (batch rows go
// through their own validation, so this layer must still refuse).
func TestEncodeShorthands_twoFlagsOneColumn(t *testing.T) {
	idx := testIndex(writableColumn{id: "date_4", title: "Due", colType: "date"})
	_, err := encodeShorthands(idx, []shorthandValue{
		{spec: specFor(t, "date"), value: "2026-05-10"},
		{spec: specFor(t, "due"), value: "2026-06-01"},
	})
	if err == nil {
		t.Fatal("want an error when two shorthands target one column")
	}
	if codeOf(err) != errs.CodeUsage {
		t.Errorf("code = %s, want %s", codeOf(err), errs.CodeUsage)
	}
}

func TestMergeShorthands(t *testing.T) {
	cols := map[string]json.RawMessage{"text_9": json.RawMessage(`"hi"`)}
	err := mergeShorthands(cols, []encodedShorthand{
		{flag: "status", colID: "status_1", title: "Stage", value: json.RawMessage(`{"label":"Done"}`)},
	})
	if err != nil {
		t.Fatalf("mergeShorthands: %v", err)
	}
	if string(cols["status_1"]) != `{"label":"Done"}` {
		t.Errorf("status_1 = %s, want the encoded label", cols["status_1"])
	}
	if string(cols["text_9"]) != `"hi"` {
		t.Errorf("mergeShorthands clobbered an existing --col value")
	}
}

// TestMergeShorthands_collision: --status and --col status_1=... in one command is
// ambiguous. Picking a winner would send a value the caller did not ask for.
func TestMergeShorthands_collision(t *testing.T) {
	cols := map[string]json.RawMessage{"status_1": json.RawMessage(`{"index":2}`)}
	err := mergeShorthands(cols, []encodedShorthand{
		{flag: "status", colID: "status_1", title: "Stage", value: json.RawMessage(`{"label":"Done"}`)},
	})
	if err == nil {
		t.Fatal("want an error when a shorthand and --col target one column")
	}
	if codeOf(err) != errs.CodeUsage {
		t.Errorf("code = %s, want %s", codeOf(err), errs.CodeUsage)
	}
	for _, want := range []string{"--status", "--col", "status_1"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err.Error(), want)
		}
	}
}

// TestApplyShorthands_noneSkipsFetch: a pure --col command must not gain a request.
// A nil client would panic if applyShorthands tried to use it.
func TestApplyShorthands_noneSkipsFetch(t *testing.T) {
	cols := map[string]json.RawMessage{"text_9": json.RawMessage(`"hi"`)}
	enc, err := applyShorthands(t.Context(), nil, "123", cols, nil)
	if err != nil {
		t.Fatalf("applyShorthands: %v", err)
	}
	if len(enc) != 0 {
		t.Errorf("encoded %d shorthands, want 0", len(enc))
	}
}
