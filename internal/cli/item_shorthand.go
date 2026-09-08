package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	gqlclient "github.com/Khan/genqlient/graphql"
	"github.com/spf13/cobra"

	"github.com/mondaycom/mcli/internal/api/gen"
	"github.com/mondaycom/mcli/internal/api/items/columns"
	"github.com/mondaycom/mcli/internal/errs"
)

// shorthandSpec maps one typed shorthand flag to the monday column type it writes.
// There is no column id in the flag, so the column is resolved by type against the
// board's live columns; see boardColumnIndex.resolve.
type shorthandSpec struct {
	flag       string
	columnType string
	usage      string
}

// shorthandSpecs are the typed column shorthands, in the order they are applied and
// reported. Every columnType here must be encodable by the columns package
// (enforced by TestShorthandSpecs_AreEncodable).
var shorthandSpecs = []shorthandSpec{
	{flag: "text", columnType: "text", usage: `set the board's text column, e.g. --text "Follow up"`},
	{flag: "status", columnType: "status", usage: `set the board's status column by label, e.g. --status Done`},
	{flag: "date", columnType: "date", usage: `set the board's date column, e.g. --date 2026-05-10 or 2026-05-10T14:30`},
	{flag: "due", columnType: "date", usage: `alias for --date`},
	{flag: "number", columnType: "numbers", usage: `set the board's numbers column, e.g. --number 42`},
	{flag: "checkbox", columnType: "checkbox", usage: `set the board's checkbox column, e.g. --checkbox true`},
}

// shorthandValue is one shorthand the caller actually supplied.
type shorthandValue struct {
	spec  shorthandSpec
	value string
}

// encodedShorthand is a shorthand after resolution: which column it landed on and
// the wire JSON written there.
type encodedShorthand struct {
	flag  string
	colID string
	title string
	value json.RawMessage
}

// addShorthandFlags declares the typed column shorthands on cmd and returns the
// destinations they parse into, keyed by flag name.
func addShorthandFlags(cmd *cobra.Command) map[string]*string {
	vals := make(map[string]*string, len(shorthandSpecs))
	for _, sp := range shorthandSpecs {
		v := new(string)
		cmd.Flags().StringVar(v, sp.flag, "", sp.usage)
		vals[sp.flag] = v
	}
	// --due is sugar for --date; accepting both would be ambiguous about precedence.
	cmd.MarkFlagsMutuallyExclusive("date", "due")
	return vals
}

// setShorthands returns the shorthands the caller passed, in spec order. A flag left
// at its zero value is not "set an empty value" — only Changed counts, so
// `--text ""` still clears a text column.
func setShorthands(cmd *cobra.Command, vals map[string]*string) []shorthandValue {
	var out []shorthandValue
	for _, sp := range shorthandSpecs {
		if cmd.Flags().Changed(sp.flag) {
			out = append(out, shorthandValue{spec: sp, value: *vals[sp.flag]})
		}
	}
	return out
}

// writableColumn is the subset of a board column needed to write to it.
type writableColumn struct {
	id       string
	title    string
	colType  string
	settings string
}

// boardColumnIndex holds a board's live columns grouped by type.
//
// It is fetched at most once per invocation and never cached across invocations: a
// stale mapping would resolve a shorthand onto a column that has since been deleted
// or retyped, which writes the right value to the wrong place. One extra request is
// the cheaper mistake.
type boardColumnIndex struct {
	byType map[string][]writableColumn
	// types is the sorted set of types present, for "the board has none" errors.
	types []string
}

// fetchBoardColumnIndex reads the board's columns. Archived columns are skipped:
// they cannot be written and would only create phantom ambiguity.
func fetchBoardColumnIndex(ctx context.Context, gql gqlclient.Client, boardID string) (*boardColumnIndex, error) {
	resp, err := gen.BoardColumnList(ctx, gql, boardID)
	if err != nil {
		return nil, err
	}
	if len(resp.Boards) == 0 {
		return nil, errs.NotFound("board %s", boardID)
	}

	idx := &boardColumnIndex{byType: make(map[string][]writableColumn)}
	for _, c := range resp.Boards[0].Columns {
		if c.Archived {
			continue
		}
		t := string(c.Type)
		idx.byType[t] = append(idx.byType[t], writableColumn{
			id: c.Id, title: c.Title, colType: t, settings: c.Settings_str,
		})
	}
	for t := range idx.byType {
		idx.types = append(idx.types, t)
	}
	sort.Strings(idx.types)
	return idx, nil
}

// resolve returns the board's single column of the spec's type.
//
// It never guesses. Silently taking the first of three status columns would move the
// wrong field on a CRM board, and an LLM caller would have no way to notice.
func (idx *boardColumnIndex) resolve(sp shorthandSpec) (writableColumn, error) {
	candidates := idx.byType[sp.columnType]
	switch len(candidates) {
	case 1:
		return candidates[0], nil
	case 0:
		types := "none"
		if len(idx.types) > 0 {
			types = strings.Join(idx.types, ", ")
		}
		return writableColumn{}, errs.Usage("--%s needs a %s column, but this board has none; its column types are: %s",
			sp.flag, sp.columnType, types)
	default:
		parts := make([]string, 0, len(candidates))
		for _, c := range candidates {
			parts = append(parts, fmt.Sprintf("%s (%q)", c.id, c.title))
		}
		return writableColumn{}, errs.Usage("--%s is ambiguous: this board has %d %s columns: %s — use --col <id>=<json> to pick one",
			sp.flag, len(candidates), sp.columnType, strings.Join(parts, ", "))
	}
}

// encodeShorthands resolves each shorthand against the board's columns and encodes
// its value into monday's wire shape.
func encodeShorthands(idx *boardColumnIndex, set []shorthandValue) ([]encodedShorthand, error) {
	out := make([]encodedShorthand, 0, len(set))
	seen := make(map[string]string, len(set))

	for _, sv := range set {
		col, err := idx.resolve(sv.spec)
		if err != nil {
			return nil, err
		}
		if prev, dup := seen[col.id]; dup {
			return nil, errs.Usage("--%s and --%s both target column %s; use only one", prev, sv.spec.flag, col.id)
		}
		seen[col.id] = sv.spec.flag

		value, err := columns.Encode(sv.spec.columnType, col.settings, sv.value)
		if err != nil {
			return nil, annotateShorthandError(sv.spec.flag, col, err)
		}

		out = append(out, encodedShorthand{flag: sv.spec.flag, colID: col.id, title: col.title, value: value})
	}
	return out, nil
}

// annotateShorthandError re-points an encoder error at the flag and column the caller
// can actually see. The encoder only knows the column type.
func annotateShorthandError(flag string, col writableColumn, err error) error {
	msg := err.Error()
	if e, ok := errors.AsType[*errs.Error](err); ok {
		msg = e.Message
	}
	return errs.Usage("--%s (column %s %q): %s", flag, col.id, col.title, msg)
}

// mergeShorthands folds resolved shorthands into a --col map.
//
// A collision is an error rather than a precedence rule: if --status and
// --col status_1=… both target one column, silently letting either win is how you
// ship a write that does not match what the caller wrote.
func mergeShorthands(cols map[string]json.RawMessage, enc []encodedShorthand) error {
	for _, e := range enc {
		if _, exists := cols[e.colID]; exists {
			return errs.Usage("--%s and --col %s both target column %s (%q); use only one",
				e.flag, e.colID, e.colID, e.title)
		}
		cols[e.colID] = e.value
	}
	return nil
}

// applyShorthands resolves and merges shorthands into cols, fetching the board's
// columns only when at least one shorthand is set — a pure --col invocation stays a
// single request, exactly as before.
func applyShorthands(
	ctx context.Context, gql gqlclient.Client, boardID string,
	cols map[string]json.RawMessage, set []shorthandValue,
) ([]encodedShorthand, error) {
	if len(set) == 0 {
		return nil, nil
	}
	idx, err := fetchBoardColumnIndex(ctx, gql, boardID)
	if err != nil {
		return nil, err
	}
	enc, err := encodeShorthands(idx, set)
	if err != nil {
		return nil, err
	}
	if err := mergeShorthands(cols, enc); err != nil {
		return nil, err
	}
	return enc, nil
}
