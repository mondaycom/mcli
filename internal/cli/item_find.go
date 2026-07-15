package cli

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/mondaycom/mcli/internal/api/gen"
	"github.com/mondaycom/mcli/internal/errs"
)

// --- output types ---

// itemFindOutputItem is the per-item shape in 'mcli item find'.
type itemFindOutputItem struct {
	ID    string    `json:"id"`
	Name  string    `json:"name"`
	Group itemGroup `json:"group"`
}

// itemFindOutput is the JSON envelope for 'mcli item find'.
type itemFindOutput struct {
	Items  []itemFindOutputItem `json:"items"`
	Cursor string               `json:"cursor"`
}

// newItemFindCmd returns the 'mcli item find' subcommand.
func newItemFindCmd() *cobra.Command {
	var (
		boardID  string
		columnID string
		value    string
		limit    int
		cursor   string
	)

	cmd := &cobra.Command{
		Use:   "find",
		Short: "Find items by column value",
		Long: `Find monday.com items on a board by matching a column value.

Provide --board, --column (the column ID), and --value to match.
Multiple matches may be returned when several items share the same value.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runItemFind(cmd, boardID, columnID, value, limit, cursor)
		},
	}

	cmd.Flags().StringVar(&boardID, "board", "", "board ID (required)")
	cmd.Flags().StringVar(&columnID, "column", "", "column ID to filter by (required)")
	cmd.Flags().StringVar(&value, "value", "", "column value to match (required)")
	cmd.Flags().IntVar(&limit, "limit", 25, "max items to return (max 500)")
	cmd.Flags().StringVar(&cursor, "cursor", "", "page cursor (omit for first page)")

	_ = cmd.MarkFlagRequired("board")
	_ = cmd.MarkFlagRequired("column")
	_ = cmd.MarkFlagRequired("value")

	return cmd
}

func runItemFind(cmd *cobra.Command, boardID, columnID, value string, limit int, cursor string) error {
	if boardID == "" {
		return errs.Usage("--board is required")
	}
	if columnID == "" {
		return errs.Usage("--column is required")
	}
	if value == "" {
		return errs.Usage("--value is required")
	}

	if limit < 1 {
		limit = 25
	}
	if limit > 500 {
		limit = 500
	}

	gql, err := newItemClient()
	if err != nil {
		return err
	}

	cols := []gen.ItemsPageByColumnValuesQuery{
		{Column_id: columnID, Column_values: []string{value}},
	}

	resp, apiErr := gen.ItemFindByColumnValue(cmd.Context(), gql, boardID, cols, limit, cursor)
	if apiErr != nil {
		return apiErr
	}

	page := resp.Items_page_by_column_values
	items := make([]itemFindOutputItem, len(page.Items))
	for i, it := range page.Items {
		items[i] = itemFindOutputItem{
			ID:   it.Id,
			Name: it.Name,
			Group: itemGroup{
				ID:    it.Group.Id,
				Title: it.Group.Title,
			},
		}
	}

	out := itemFindOutput{
		Items:  items,
		Cursor: page.Cursor,
	}

	mode, modeErr := resolveOutputMode(os.Stdout, globals, configOutputMode())
	if modeErr != nil {
		return modeErr
	}

	switch mode {
	case ModeJSON:
		data, mErr := json.Marshal(out)
		if mErr != nil {
			return errs.Internal("marshal output: %v", mErr)
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return err

	case ModeCSV:
		return writeItemFindCSV(cmd, out)

	case ModeTerse:
		return writeItemFindTerse(cmd, out)

	default: // ModePretty
		return writeItemFindPretty(cmd, out)
	}
}

func writeItemFindPretty(cmd *cobra.Command, out itemFindOutput) error {
	o := cmd.OutOrStdout()
	w := tabwriter.NewWriter(o, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tNAME\tGROUP")
	for _, it := range out.Items {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", it.ID, it.Name, it.Group.Title)
	}
	if err := w.Flush(); err != nil {
		return errs.Internal("flush table: %v", err)
	}
	if out.Cursor != "" {
		_, _ = fmt.Fprintf(o, "\nnext cursor: %s\n", out.Cursor)
	}
	return nil
}

func writeItemFindTerse(cmd *cobra.Command, out itemFindOutput) error {
	o := cmd.OutOrStdout()
	for _, it := range out.Items {
		_, _ = fmt.Fprintf(o, "#%s:%s:group=%s\n", it.ID, it.Name, it.Group.Title)
	}
	return nil
}

func writeItemFindCSV(cmd *cobra.Command, out itemFindOutput) error {
	w := csv.NewWriter(cmd.OutOrStdout())
	_ = w.Write([]string{"id", "name", "group_id", "group_title"})
	for _, it := range out.Items {
		_ = w.Write([]string{it.ID, it.Name, it.Group.ID, it.Group.Title})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return errs.Internal("write csv: %v", err)
	}
	return nil
}
