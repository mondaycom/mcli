package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"strconv"
	"strings"

	gqlclient "github.com/Khan/genqlient/graphql"
	"github.com/spf13/cobra"

	"github.com/mondaycom/mcli/internal/api/gen"
	apigraphql "github.com/mondaycom/mcli/internal/api/graphql"
	"github.com/mondaycom/mcli/internal/errs"
)

const (
	// batchStdinSentinel is the positional arg that switches create/update into
	// batch mode, reading rows from stdin. ADR-002 specifies this contract.
	batchStdinSentinel = "-"
	// batchMaxInputBytes bounds stdin so a runaway pipe cannot exhaust memory.
	batchMaxInputBytes = 8 << 20
	// batchMaxRows caps one invocation. create_item has no dedupe key, so an
	// accidental huge batch is expensive to undo; splitting is the caller's call.
	batchMaxRows = 500
)

// jsonScalar is a JSON string, number, or boolean captured as its text form, so a
// batch row may write "number": 42 or "number": "42" and mean the same thing.
type jsonScalar string

// UnmarshalJSON accepts a string, number, or boolean.
func (s *jsonScalar) UnmarshalJSON(b []byte) error {
	// null is rejected rather than read as "clear the column": encoding/json treats
	// unmarshalling null as a no-op, so it would silently mean "absent" instead. Use
	// "" to clear a column.
	if string(bytes.TrimSpace(b)) == "null" {
		return fmt.Errorf(`null is not a value; use "" to clear a column`)
	}

	var str string
	if err := json.Unmarshal(b, &str); err == nil {
		*s = jsonScalar(str)
		return nil
	}
	var num json.Number
	if err := json.Unmarshal(b, &num); err == nil {
		*s = jsonScalar(num.String())
		return nil
	}
	var boolean bool
	if err := json.Unmarshal(b, &boolean); err == nil {
		*s = jsonScalar(strconv.FormatBool(boolean))
		return nil
	}
	return fmt.Errorf("expected a string, number, or boolean, got %s", string(b))
}

// MarshalJSON keeps --dry-run echoes round-trippable.
func (s jsonScalar) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(s))
}

// batchRow is one row of a batch payload. Create rows carry name/group, update rows
// carry id, and both may carry raw cols plus the same typed shorthands as the flags.
//
// A nil shorthand pointer means "absent"; an empty one means "write empty", which is
// how a column gets cleared.
type batchRow struct {
	ID    string                     `json:"id,omitempty"`
	Name  string                     `json:"name,omitempty"`
	Group string                     `json:"group,omitempty"`
	Cols  map[string]json.RawMessage `json:"cols,omitempty"`

	Text     *jsonScalar `json:"text,omitempty"`
	Status   *jsonScalar `json:"status,omitempty"`
	Date     *jsonScalar `json:"date,omitempty"`
	Due      *jsonScalar `json:"due,omitempty"`
	Number   *jsonScalar `json:"number,omitempty"`
	Checkbox *jsonScalar `json:"checkbox,omitempty"`
}

// shorthands returns the shorthands present on the row, in spec order, so the stdin
// path and the flag path share one encoder and one resolution rule.
func (r batchRow) shorthands() []shorthandValue {
	byFlag := map[string]*jsonScalar{
		"text":     r.Text,
		"status":   r.Status,
		"date":     r.Date,
		"due":      r.Due,
		"number":   r.Number,
		"checkbox": r.Checkbox,
	}
	var out []shorthandValue
	for _, sp := range shorthandSpecs {
		if v := byFlag[sp.flag]; v != nil {
			out = append(out, shorthandValue{spec: sp, value: string(*v)})
		}
	}
	return out
}

// isBatchArg reports whether args select batch mode.
func isBatchArg(args []string) bool {
	return len(args) == 1 && args[0] == batchStdinSentinel
}

// parseBatchRows reads a JSON array or an NDJSON stream of rows.
//
// Unknown fields are rejected: a mistyped key like "columns" instead of "cols" would
// otherwise silently drop every column value in the row and report success.
func parseBatchRows(r io.Reader) ([]batchRow, error) {
	data, err := io.ReadAll(io.LimitReader(r, batchMaxInputBytes+1))
	if err != nil {
		return nil, errs.Usage("read batch rows from stdin: %v", err)
	}
	if len(data) > batchMaxInputBytes {
		return nil, errs.Usage("batch input exceeds %d bytes; split it into smaller batches", batchMaxInputBytes)
	}

	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, errs.Usage("no batch rows on stdin")
	}

	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.DisallowUnknownFields()

	var rows []batchRow
	if trimmed[0] == '[' {
		if err := dec.Decode(&rows); err != nil {
			return nil, errs.Usage("parse batch rows: %v", err)
		}
		if dec.More() {
			return nil, errs.Usage("parse batch rows: unexpected content after the JSON array")
		}
	} else {
		for {
			var row batchRow
			if err := dec.Decode(&row); err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				return nil, errs.Usage("parse batch row %d: %v", len(rows), err)
			}
			rows = append(rows, row)
		}
	}

	if len(rows) == 0 {
		return nil, errs.Usage("no batch rows on stdin")
	}
	if len(rows) > batchMaxRows {
		return nil, errs.Usage("batch has %d rows, max is %d; split it into smaller batches", len(rows), batchMaxRows)
	}
	return rows, nil
}

// validateBatchRow checks what is common to create and update rows.
func validateBatchRow(index int, row batchRow) error {
	if row.Date != nil && row.Due != nil {
		return errs.Usage("row %d: set either \"date\" or \"due\", not both", index)
	}
	for id, raw := range row.Cols {
		if id == "" {
			return errs.Usage("row %d: cols has an empty column id", index)
		}
		if !json.Valid(raw) {
			return errs.Usage("row %d: cols.%s is not valid JSON", index, id)
		}
	}
	return nil
}

// validateCreateRows rejects the whole payload before any request fires: a batch that
// dies halfway through leaves items behind that a retry would duplicate.
func validateCreateRows(rows []batchRow) error {
	for i, row := range rows {
		if err := validateBatchRow(i, row); err != nil {
			return err
		}
		if strings.TrimSpace(row.Name) == "" {
			return errs.Usage("row %d: \"name\" is required to create an item", i)
		}
		if row.ID != "" {
			return errs.Usage("row %d: \"id\" is not valid when creating items", i)
		}
	}
	return nil
}

// validateUpdateRows rejects the whole payload before any request fires.
func validateUpdateRows(rows []batchRow) error {
	for i, row := range rows {
		if err := validateBatchRow(i, row); err != nil {
			return err
		}
		if row.ID == "" {
			return errs.Usage("row %d: \"id\" is required to update an item", i)
		}
		if _, err := strconv.ParseUint(row.ID, 10, 64); err != nil {
			return errs.Usage("row %d: id must be a numeric string, got %q", i, row.ID)
		}
		if row.Name == "" && len(row.Cols) == 0 && len(row.shorthands()) == 0 {
			return errs.Usage("row %d: nothing to update: provide \"name\", \"cols\", or a shorthand", i)
		}
	}
	return nil
}

// batchNeedsColumns reports whether any row uses a shorthand, i.e. whether the
// board's columns have to be fetched at all.
func batchNeedsColumns(rows []batchRow) bool {
	for _, row := range rows {
		if len(row.shorthands()) > 0 {
			return true
		}
	}
	return false
}

// buildBatchPayloads turns every row into the exact column_values string that will be
// sent for it, resolving the board's columns at most once for the whole batch.
//
// All rows are built up front so a bad status label fails the batch as a usage error
// instead of surfacing after the first N items are already created.
func buildBatchPayloads(
	ctx context.Context, gql gqlclient.Client, boardID string, rows []batchRow,
) ([]string, error) {
	var idx *boardColumnIndex
	if batchNeedsColumns(rows) {
		var err error
		idx, err = fetchBoardColumnIndex(ctx, gql, boardID)
		if err != nil {
			return nil, err
		}
	}

	payloads := make([]string, len(rows))
	for i, row := range rows {
		// Copied rather than used in place, so encoding a shorthand never mutates the
		// caller's parsed row (--dry-run echoes it back).
		cols := make(map[string]json.RawMessage, len(row.Cols))
		maps.Copy(cols, row.Cols)

		if short := row.shorthands(); len(short) > 0 {
			enc, err := encodeShorthands(idx, short)
			if err != nil {
				return nil, errs.Usage("row %d: %s", i, errMessage(err))
			}
			if err := mergeShorthands(cols, enc); err != nil {
				return nil, errs.Usage("row %d: %s", i, errMessage(err))
			}
		}

		// change_multiple_column_values takes the item name as a "name" column.
		if row.Name != "" && row.ID != "" {
			nameVal, _ := json.Marshal(row.Name)
			cols["name"] = json.RawMessage(nameVal)
		}

		payload, err := buildColumnValues(cols)
		if err != nil {
			return nil, errs.Usage("row %d: %v", i, err)
		}
		payloads[i] = payload
	}
	return payloads, nil
}

// errMessage returns an *errs.Error's bare message, so nesting a row index in front
// of it does not produce a doubled "[USAGE] … [USAGE] …" string.
func errMessage(err error) string {
	if e, ok := errors.AsType[*errs.Error](err); ok {
		return e.Message
	}
	return err.Error()
}

// batchError reports one failed row.
//
// Index is the row's position in the input, which is what makes a partial failure
// recoverable: create_item has no dedupe key, so re-running the whole payload
// duplicates every row that succeeded. Retry the listed indexes, not the batch.
type batchError struct {
	Index   int    `json:"index"`
	ID      string `json:"id,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// batchWriteOutput is the JSON shape for a batch create or update. It is one
// top-level object, never a stream (ADR-002), and every key is always present so a
// caller can read counts without checking for absence.
type batchWriteOutput struct {
	// Written is the number of rows monday accepted.
	Written int `json:"written"`
	Failed  int `json:"failed"`
	// Verb is "created" or "updated", so a caller need not infer it from the command.
	Verb   string            `json:"verb"`
	Items  []itemWriteOutput `json:"items"`
	Errors []batchError      `json:"errors"`
}

// batchDryRunRow echoes one row exactly as it would be sent.
type batchDryRunRow struct {
	Index        int    `json:"index"`
	ID           string `json:"id,omitempty"`
	Name         string `json:"name,omitempty"`
	Group        string `json:"group,omitempty"`
	ColumnValues string `json:"column_values,omitempty"`
}

// batchDryRunOutput is the JSON shape for a batch --dry-run.
type batchDryRunOutput struct {
	DryRun bool             `json:"dry_run"`
	Verb   string           `json:"verb"`
	Rows   int              `json:"rows"`
	Items  []batchDryRunRow `json:"items"`
}

// classifyBatchErr maps a per-row error to a stable code. Errors from the production
// client are already *errs.Error; anything else is normalised so a raw transport
// failure still reports a real code rather than a bare INTERNAL.
func classifyBatchErr(err error) errs.Code {
	if e, ok := errors.AsType[*errs.Error](err); ok {
		return e.Code
	}
	if e, ok := errors.AsType[*errs.Error](apigraphql.Normalize(err)); ok {
		return e.Code
	}
	return errs.CodeInternal
}

// batchRowFn performs the write for one row, returning the item monday reported.
type batchRowFn func(ctx context.Context, index int, row batchRow, columnValues string) (itemWriteOutput, error)

// runBatch executes rows sequentially and collects per-row outcomes.
//
// Sequential is deliberate: monday's limit is complexity-per-minute, so parallelism
// does not raise throughput, it only reaches the ceiling sooner and makes pacing
// impossible to reason about.
func runBatch(ctx context.Context, verb string, rows []batchRow, payloads []string, write batchRowFn) batchWriteOutput {
	out := batchWriteOutput{
		Verb:   verb,
		Items:  make([]itemWriteOutput, 0, len(rows)),
		Errors: []batchError{},
	}

	for i, row := range rows {
		if ctxErr := ctx.Err(); ctxErr != nil {
			out.Errors = append(out.Errors, batchError{
				Index:   i,
				ID:      row.ID,
				Code:    string(errs.CodeInterrupted),
				Message: fmt.Sprintf("cancelled before row %d of %d", i, len(rows)),
			})
			out.Failed++
			break
		}

		item, err := write(ctx, i, row, payloads[i])
		if err != nil {
			out.Errors = append(out.Errors, batchError{
				Index:   i,
				ID:      row.ID,
				Code:    string(classifyBatchErr(err)),
				Message: errMessage(err),
			})
			out.Failed++
			continue
		}
		out.Items = append(out.Items, item)
		out.Written++
	}

	return out
}

// writeBatchOutput renders the batch result and returns the error that sets the exit
// code: any failed row is exit 2, so a caller can tell "all fine" from "mostly fine"
// without diffing counts.
func writeBatchOutput(cmd *cobra.Command, out batchWriteOutput) error {
	mode, modeErr := resolveOutputMode(os.Stdout, globals, configOutputMode())
	if modeErr != nil {
		return modeErr
	}

	o := cmd.OutOrStdout()
	if mode == ModeJSON {
		data, mErr := json.Marshal(out)
		if mErr != nil {
			return errs.Internal("marshal output: %v", mErr)
		}
		if _, err := fmt.Fprintln(o, string(data)); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintf(o, "%s %s, %d failed\n", out.Verb, pluralItems(out.Written), out.Failed); err != nil {
			return err
		}
		for _, e := range out.Errors {
			_, _ = fmt.Fprintf(o, "  row %d: [%s] %s\n", e.Index, e.Code, e.Message)
		}
	}

	if out.Failed > 0 {
		return errs.API("%d of %d rows failed; see errors[].index to retry only those",
			out.Failed, out.Written+out.Failed)
	}
	return nil
}

// writeBatchDryRun renders what a batch would send, without sending it.
func writeBatchDryRun(cmd *cobra.Command, verb string, rows []batchRow, payloads []string) error {
	out := batchDryRunOutput{DryRun: true, Verb: verb, Rows: len(rows), Items: make([]batchDryRunRow, 0, len(rows))}
	for i, row := range rows {
		out.Items = append(out.Items, batchDryRunRow{
			Index:        i,
			ID:           row.ID,
			Name:         row.Name,
			Group:        row.Group,
			ColumnValues: payloads[i],
		})
	}

	mode, modeErr := resolveOutputMode(os.Stdout, globals, configOutputMode())
	if modeErr != nil {
		return modeErr
	}

	o := cmd.OutOrStdout()
	if mode == ModeJSON {
		data, mErr := json.Marshal(out)
		if mErr != nil {
			return errs.Internal("marshal output: %v", mErr)
		}
		_, err := fmt.Fprintln(o, string(data))
		return err
	}

	if _, err := fmt.Fprintf(o, "dry run: would %s %s\n", verbInfinitive(verb), pluralItems(len(rows))); err != nil {
		return err
	}
	for _, it := range out.Items {
		label := it.Name
		if it.ID != "" {
			label = it.ID
		}
		line := fmt.Sprintf("  row %d: %s", it.Index, label)
		if it.ColumnValues != "" {
			line += " " + it.ColumnValues
		}
		_, _ = fmt.Fprintln(o, line)
	}
	return nil
}

// verbInfinitive turns the output verb into the form a sentence needs.
func verbInfinitive(verb string) string {
	switch verb {
	case "created":
		return "create"
	case "updated":
		return "update"
	default:
		return verb
	}
}

// pluralItems renders an item count without the "1 items" tell.
func pluralItems(n int) string {
	if n == 1 {
		return "1 item"
	}
	return fmt.Sprintf("%d items", n)
}

// --- batch entry points ---

// runItemCreateBatch implements 'mcli item create --board <id> -'.
func runItemCreateBatch(cmd *cobra.Command, boardID string, dryRun bool) error {
	rows, err := parseBatchRows(cmd.InOrStdin())
	if err != nil {
		return err
	}
	if err := validateCreateRows(rows); err != nil {
		return err
	}

	// A dry run that uses no shorthand needs no API access at all, so it stays
	// usable as a pure parse check without a token.
	var gql gqlclient.Client
	if !dryRun || batchNeedsColumns(rows) {
		gql, err = newItemClient()
		if err != nil {
			return err
		}
	}

	payloads, err := buildBatchPayloads(cmd.Context(), gql, boardID, rows)
	if err != nil {
		return err
	}

	if dryRun {
		return writeBatchDryRun(cmd, "created", rows, payloads)
	}

	out := runBatch(cmd.Context(), "created", rows, payloads,
		func(ctx context.Context, _ int, row batchRow, columnValues string) (itemWriteOutput, error) {
			resp, apiErr := gen.ItemCreate(ctx, gql, boardID, row.Name, row.Group, columnValues)
			if apiErr != nil {
				return itemWriteOutput{}, apiErr
			}
			return itemCreateToOutput(resp), nil
		})

	return writeBatchOutput(cmd, out)
}

// runItemUpdateBatch implements 'mcli item update --board <id> -'.
func runItemUpdateBatch(cmd *cobra.Command, boardID string, dryRun bool) error {
	rows, err := parseBatchRows(cmd.InOrStdin())
	if err != nil {
		return err
	}
	if err := validateUpdateRows(rows); err != nil {
		return err
	}

	var gql gqlclient.Client
	if !dryRun || batchNeedsColumns(rows) {
		gql, err = newItemClient()
		if err != nil {
			return err
		}
	}

	payloads, err := buildBatchPayloads(cmd.Context(), gql, boardID, rows)
	if err != nil {
		return err
	}

	if dryRun {
		return writeBatchDryRun(cmd, "updated", rows, payloads)
	}

	out := runBatch(cmd.Context(), "updated", rows, payloads,
		func(ctx context.Context, _ int, row batchRow, columnValues string) (itemWriteOutput, error) {
			resp, apiErr := gen.ItemUpdate(ctx, gql, boardID, row.ID, columnValues)
			if apiErr != nil {
				return itemWriteOutput{}, apiErr
			}
			return itemUpdateToOutput(resp), nil
		})

	return writeBatchOutput(cmd, out)
}
