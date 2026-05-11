package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/mondaycom/mcli/internal/api/gen"
	apigraphql "github.com/mondaycom/mcli/internal/api/graphql"
	"github.com/mondaycom/mcli/internal/api/items/columns"
	"github.com/mondaycom/mcli/internal/config"
	"github.com/mondaycom/mcli/internal/errs"
	"github.com/mondaycom/mcli/internal/secrets"

	gqlclient "github.com/Khan/genqlient/graphql"
)

const (
	itemListDefaultLimit = 25
	itemListMaxLimit     = 500
)

// itemClientFactory is an unexported seam that lets tests inject a fake
// graphql.Client without touching network or environment. Production code
// leaves this nil, which causes newItemClient to build a real client.
var itemClientFactory func() (gqlclient.Client, error)

// newItemClient resolves a graphql.Client for item commands.
// If itemClientFactory is set (tests), it delegates there.
// Otherwise it builds a real authenticated client from config + secrets.
func newItemClient() (gqlclient.Client, error) {
	if itemClientFactory != nil {
		return itemClientFactory()
	}

	cfgPath := resolveConfigPath()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	var store config.Store
	if cfg.SecretStore != "" {
		st, openErr := secrets.Open(cfg.SecretStore, resolveConfigDir())
		if openErr != nil {
			return nil, openErr
		}
		store = st
	}

	token, err := config.ResolveToken(cfg, globals.Token, store)
	if err != nil {
		return nil, err
	}

	c := apigraphql.New(token, version)
	return c.GQL(), nil
}

// newItemCmd returns the 'mcli item' parent command.
func newItemCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "item",
		Short: "Manage monday.com items",
	}
	cmd.AddCommand(newItemListCmd())
	cmd.AddCommand(newItemGetCmd())
	return cmd
}

// --- output types ---

// renderedColumn is the clean output shape for a single column value.
type renderedColumn struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Value any    `json:"value"`
}

// itemGroup is the group sub-object in item output.
type itemGroup struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// itemListOutputItem is the per-item shape in 'mcli item list'.
type itemListOutputItem struct {
	ID      string           `json:"id"`
	Name    string           `json:"name"`
	State   string           `json:"state"`
	Group   itemGroup        `json:"group"`
	Columns []renderedColumn `json:"columns"`
}

// itemListOutput is the JSON envelope for 'mcli item list'.
type itemListOutput struct {
	Items  []itemListOutputItem `json:"items"`
	Cursor string               `json:"cursor"`
}

// itemSubitem is a subitem reference in item get output.
type itemSubitem struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"`
}

// itemGetBoard is the board reference in item get output.
type itemGetBoard struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// itemGetCreator is the creator reference in item get output.
type itemGetCreator struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// itemGetParent is the parent item reference in item get output.
type itemGetParent struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// itemGetOutput is the JSON shape for 'mcli item get'.
type itemGetOutput struct {
	ID         string           `json:"id"`
	Name       string           `json:"name"`
	State      string           `json:"state"`
	CreatedAt  string           `json:"created_at,omitempty"`
	UpdatedAt  string           `json:"updated_at,omitempty"`
	Creator    *itemGetCreator  `json:"creator,omitempty"`
	Group      *itemGroup       `json:"group,omitempty"`
	Board      *itemGetBoard    `json:"board,omitempty"`
	ParentItem *itemGetParent   `json:"parent_item,omitempty"`
	Subitems   []itemSubitem    `json:"subitems"`
	Columns    []renderedColumn `json:"columns"`
}

// --- column rendering helper ---

// columnEntry holds the minimal data needed to render one column value.
// We extract this from the genqlient interface types to avoid working
// around the pointer-receiver vs value-type mismatch in the generated code.
type columnEntry struct {
	id          string
	columnType  string
	settingsStr string
	rawValue    string
}

// renderColumns turns a slice of columnEntry values into the clean output shape.
// Columns whose raw value is empty or null are skipped.
// Errors from columns.Decode propagate.
func renderColumns(entries []columnEntry) ([]renderedColumn, error) {
	out := make([]renderedColumn, 0, len(entries))
	for _, ce := range entries {
		if ce.rawValue == "" || ce.rawValue == "null" {
			continue
		}
		decoded, err := columns.Decode(ce.columnType, ce.settingsStr, ce.rawValue)
		if err != nil {
			return nil, fmt.Errorf("decode column %s: %w", ce.id, err)
		}
		if decoded.Value == nil {
			continue
		}
		out = append(out, renderedColumn{
			ID:    ce.id,
			Type:  decoded.Type,
			Value: decoded.Value,
		})
	}
	return out, nil
}

// extractBoardColumnEntries converts ItemsListByBoard column values to columnEntries.
func extractBoardColumnEntries(
	vals []gen.ItemsListByBoardBoardsBoardItems_pageItemsResponseItemsItemColumn_valuesColumnValue,
) []columnEntry {
	entries := make([]columnEntry, 0, len(vals))
	for _, cv := range vals {
		col := cv.GetColumn()
		entries = append(entries, columnEntry{
			id:          cv.GetId(),
			columnType:  string(cv.GetType()),
			settingsStr: col.Settings_str,
			rawValue:    cv.GetValue(),
		})
	}
	return entries
}

// extractGroupColumnEntries converts ItemsListByGroup column values to columnEntries.
func extractGroupColumnEntries(
	vals []gen.ItemsListByGroupBoardsBoardGroupsGroupItems_pageItemsResponseItemsItemColumn_valuesColumnValue,
) []columnEntry {
	entries := make([]columnEntry, 0, len(vals))
	for _, cv := range vals {
		col := cv.GetColumn()
		entries = append(entries, columnEntry{
			id:          cv.GetId(),
			columnType:  string(cv.GetType()),
			settingsStr: col.Settings_str,
			rawValue:    cv.GetValue(),
		})
	}
	return entries
}

// extractGetColumnEntries converts ItemGet column values to columnEntries.
func extractGetColumnEntries(
	vals []gen.ItemGetItemsItemColumn_valuesColumnValue,
) []columnEntry {
	entries := make([]columnEntry, 0, len(vals))
	for _, cv := range vals {
		col := cv.GetColumn()
		entries = append(entries, columnEntry{
			id:          cv.GetId(),
			columnType:  string(cv.GetType()),
			settingsStr: col.Settings_str,
			rawValue:    cv.GetValue(),
		})
	}
	return entries
}

// --- item list ---

func newItemListCmd() *cobra.Command {
	var (
		boardID string
		groupID string
		limit   int
		cursor  string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List items in a board",
		Long:  "List monday.com items, optionally filtered to a group.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runItemList(cmd, boardID, groupID, limit, cursor)
		},
	}

	cmd.Flags().StringVar(&boardID, "board", "", "board ID (required)")
	cmd.Flags().StringVar(&groupID, "group", "", "group ID (optional; omit for all groups)")
	cmd.Flags().IntVar(&limit, "limit", itemListDefaultLimit, "max items per page (max 500)")
	cmd.Flags().StringVar(&cursor, "cursor", "", "opaque page cursor (omit for first page)")
	_ = cmd.MarkFlagRequired("board")

	return cmd
}

func runItemList(cmd *cobra.Command, boardID, groupID string, limit int, cursor string) error {
	if _, err := strconv.ParseUint(boardID, 10, 64); err != nil {
		return errs.Usage("board id must be a numeric string, got %q", boardID)
	}

	// Clamp limit.
	if limit < 1 {
		limit = itemListDefaultLimit
	}
	if limit > itemListMaxLimit {
		limit = itemListMaxLimit
	}

	gql, err := newItemClient()
	if err != nil {
		return err
	}

	var rawItems []itemListOutputItem
	var nextCursor string

	if groupID != "" {
		resp, apiErr := gen.ItemsListByGroup(context.Background(), gql, boardID, groupID, limit, cursor)
		if apiErr != nil {
			return apiErr
		}
		if len(resp.Boards) > 0 && len(resp.Boards[0].Groups) > 0 {
			page := resp.Boards[0].Groups[0].Items_page
			nextCursor = page.Cursor
			rawItems, err = convertItemsListByGroup(page.Items)
			if err != nil {
				return err
			}
		}
	} else {
		resp, apiErr := gen.ItemsListByBoard(context.Background(), gql, boardID, limit, cursor)
		if apiErr != nil {
			return apiErr
		}
		if len(resp.Boards) > 0 {
			page := resp.Boards[0].Items_page
			nextCursor = page.Cursor
			rawItems, err = convertItemsListByBoard(page.Items)
			if err != nil {
				return err
			}
		}
	}

	if rawItems == nil {
		rawItems = []itemListOutputItem{}
	}

	out := itemListOutput{
		Items:  rawItems,
		Cursor: nextCursor,
	}

	mode, modeErr := resolveOutputMode(os.Stdout, globals)
	if modeErr != nil {
		return modeErr
	}

	if mode == ModeJSON {
		data, mErr := json.Marshal(out)
		if mErr != nil {
			return fmt.Errorf("marshal output: %w", mErr)
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return err
	}

	// Pretty output: tab-aligned table.
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tNAME\tSTATE\tGROUP\tCOLUMNS")
	for _, item := range rawItems {
		colSummary := fmt.Sprintf("%d column(s)", len(item.Columns))
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			item.ID, item.Name, item.State, item.Group.Title, colSummary)
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("flush table: %w", err)
	}
	if nextCursor != "" {
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "\nnext cursor: %s\n", nextCursor)
	}
	return err
}

// convertItemsListByBoard converts genqlient board-query items to output items.
func convertItemsListByBoard(
	items []gen.ItemsListByBoardBoardsBoardItems_pageItemsResponseItemsItem,
) ([]itemListOutputItem, error) {
	out := make([]itemListOutputItem, 0, len(items))
	for _, it := range items {
		cols, err := renderColumns(extractBoardColumnEntries(it.Column_values))
		if err != nil {
			return nil, err
		}
		out = append(out, itemListOutputItem{
			ID:      it.Id,
			Name:    it.Name,
			State:   string(it.State),
			Group:   itemGroup{ID: it.Group.Id, Title: it.Group.Title},
			Columns: cols,
		})
	}
	return out, nil
}

// convertItemsListByGroup converts genqlient group-query items to output items.
func convertItemsListByGroup(
	items []gen.ItemsListByGroupBoardsBoardGroupsGroupItems_pageItemsResponseItemsItem,
) ([]itemListOutputItem, error) {
	out := make([]itemListOutputItem, 0, len(items))
	for _, it := range items {
		cols, err := renderColumns(extractGroupColumnEntries(it.Column_values))
		if err != nil {
			return nil, err
		}
		out = append(out, itemListOutputItem{
			ID:      it.Id,
			Name:    it.Name,
			State:   string(it.State),
			Group:   itemGroup{ID: it.Group.Id, Title: it.Group.Title},
			Columns: cols,
		})
	}
	return out, nil
}

// --- item get ---

func newItemGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get an item by ID",
		Long:  "Fetch full details for a monday.com item by its numeric ID.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runItemGet(cmd, args[0])
		},
	}
}

func runItemGet(cmd *cobra.Command, id string) error {
	if _, err := strconv.ParseUint(id, 10, 64); err != nil {
		return errs.Usage("item id must be a numeric string, got %q", id)
	}

	gql, err := newItemClient()
	if err != nil {
		return err
	}

	resp, err := gen.ItemGet(context.Background(), gql, id)
	if err != nil {
		return err
	}

	if len(resp.Items) == 0 {
		return errs.NotFound("item %s", id)
	}

	it := resp.Items[0]

	cols, err := renderColumns(extractGetColumnEntries(it.Column_values))
	if err != nil {
		return err
	}

	subitems := make([]itemSubitem, len(it.Subitems))
	for i, s := range it.Subitems {
		subitems[i] = itemSubitem{ID: s.Id, Name: s.Name, State: string(s.State)}
	}

	out := itemGetOutput{
		ID:        it.Id,
		Name:      it.Name,
		State:     string(it.State),
		CreatedAt: it.Created_at,
		UpdatedAt: it.Updated_at,
		Subitems:  subitems,
		Columns:   cols,
	}

	if it.Creator.Id != "" {
		out.Creator = &itemGetCreator{ID: it.Creator.Id, Name: it.Creator.Name}
	}
	if it.Group.Id != "" {
		out.Group = &itemGroup{ID: it.Group.Id, Title: it.Group.Title}
	}
	if it.Board.Id != "" {
		out.Board = &itemGetBoard{ID: it.Board.Id, Name: it.Board.Name}
	}
	if it.Parent_item.Id != "" {
		out.ParentItem = &itemGetParent{ID: it.Parent_item.Id, Name: it.Parent_item.Name}
	}

	mode, modeErr := resolveOutputMode(os.Stdout, globals)
	if modeErr != nil {
		return modeErr
	}

	if mode == ModeJSON {
		data, mErr := json.Marshal(out)
		if mErr != nil {
			return fmt.Errorf("marshal output: %w", mErr)
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return err
	}

	// Pretty output.
	o := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(o, "ID:         %s\n", out.ID)
	_, _ = fmt.Fprintf(o, "Name:       %s\n", out.Name)
	_, _ = fmt.Fprintf(o, "State:      %s\n", out.State)
	if out.CreatedAt != "" {
		_, _ = fmt.Fprintf(o, "Created:    %s\n", out.CreatedAt)
	}
	if out.UpdatedAt != "" {
		_, _ = fmt.Fprintf(o, "Updated:    %s\n", out.UpdatedAt)
	}
	if out.Creator != nil {
		_, _ = fmt.Fprintf(o, "Creator:    %s (%s)\n", out.Creator.Name, out.Creator.ID)
	}
	if out.Group != nil {
		_, _ = fmt.Fprintf(o, "Group:      %s (%s)\n", out.Group.Title, out.Group.ID)
	}
	if out.Board != nil {
		_, _ = fmt.Fprintf(o, "Board:      %s (%s)\n", out.Board.Name, out.Board.ID)
	}
	if out.ParentItem != nil {
		_, _ = fmt.Fprintf(o, "Parent:     %s (%s)\n", out.ParentItem.Name, out.ParentItem.ID)
	}

	if len(out.Subitems) > 0 {
		_, _ = fmt.Fprintf(o, "\nSubitems (%d):\n", len(out.Subitems))
		tw := tabwriter.NewWriter(o, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "  ID\tNAME\tSTATE")
		for _, s := range out.Subitems {
			_, _ = fmt.Fprintf(tw, "  %s\t%s\t%s\n", s.ID, s.Name, s.State)
		}
		_ = tw.Flush()
	}

	if len(out.Columns) > 0 {
		_, _ = fmt.Fprintf(o, "\nColumns (%d):\n", len(out.Columns))
		tw := tabwriter.NewWriter(o, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "  ID\tTYPE\tVALUE")
		for _, c := range out.Columns {
			valStr := fmt.Sprintf("%v", c.Value)
			_, _ = fmt.Fprintf(tw, "  %s\t%s\t%s\n", c.ID, c.Type, valStr)
		}
		_ = tw.Flush()
	}

	return nil
}
