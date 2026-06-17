package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/mondaycom/mcli/internal/api/gen"
	"github.com/mondaycom/mcli/internal/api/items/columns"
	"github.com/mondaycom/mcli/internal/errs"

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
	return newGQLClient()
}

// newItemCmd returns the 'mcli item' parent command.
func newItemCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "item",
		Short: "Manage monday.com items",
	}
	cmd.AddCommand(newItemListCmd())
	cmd.AddCommand(newItemGetCmd())
	cmd.AddCommand(newItemCreateCmd())
	cmd.AddCommand(newItemUpdateCmd())
	cmd.AddCommand(newItemPostUpdateCmd())
	cmd.AddCommand(newItemMoveCmd())
	cmd.AddCommand(newItemDeleteCmd())
	cmd.AddCommand(newItemArchiveCmd())
	cmd.AddCommand(newItemFindCmd())
	cmd.AddCommand(newItemGetUpdatesCmd())
	cmd.AddCommand(newItemDescriptionCmd())
	return cmd
}

// parseColFlags parses --col flags into a json.RawMessage map, last-wins on duplicates.
// Each element must be of the form "<id>=<json>". Split is on the FIRST '=' only.
// Returns errs.Usage on malformed JSON, missing '=', or empty col id.
func parseColFlags(raw []string) (map[string]json.RawMessage, error) {
	result := make(map[string]json.RawMessage, len(raw))
	for _, flag := range raw {
		colID, colJSON, ok := strings.Cut(flag, "=")
		if !ok {
			return nil, errs.Usage("--col %q: must be in <id>=<json> form", flag)
		}
		if colID == "" {
			return nil, errs.Usage("--col flag has empty column id in %q", flag)
		}
		if !json.Valid([]byte(colJSON)) {
			return nil, errs.Usage("--col %s: value is not valid JSON: %s", colID, colJSON)
		}
		// Last-wins on duplicate ids.
		result[colID] = json.RawMessage(colJSON)
	}
	return result, nil
}

// buildColumnValues marshals a column map into the monday wire shape:
// a JSON-stringified object, e.g. `{"status":{"label":"Done"}}`.
// Returns an empty string when the map is empty.
func buildColumnValues(cols map[string]json.RawMessage) (string, error) {
	if len(cols) == 0 {
		return "", nil
	}
	b, err := json.Marshal(cols)
	if err != nil {
		return "", fmt.Errorf("marshal column_values: %w", err)
	}
	return string(b), nil
}

// --- output types for create / update ---

// itemWriteBoard is the board sub-object in create/update output.
type itemWriteBoard struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// itemWriteGroup is the group sub-object in create/update output.
type itemWriteGroup struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// itemWriteParent is the parent_item sub-object in subitem create output.
type itemWriteParent struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// itemWriteOutput is the JSON shape for create and update responses.
type itemWriteOutput struct {
	ID         string           `json:"id"`
	Name       string           `json:"name"`
	State      string           `json:"state"`
	Board      *itemWriteBoard  `json:"board,omitempty"`
	Group      *itemWriteGroup  `json:"group,omitempty"`
	ParentItem *itemWriteParent `json:"parent_item,omitempty"`
}

// --- item create ---

func newItemCreateCmd() *cobra.Command {
	var (
		boardID  string
		parentID string
		name     string
		groupID  string
		colFlags []string
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an item (or subitem with --parent)",
		Long: `Create a monday.com item on a board, or a subitem under a parent item.

Exactly one of --board or --parent must be provided.
Use --col <id>=<json> to set column values; the JSON must match monday's
column-value wire shape for the column type. Repeated --col flags are
order-preserving; if the same column id appears twice, the last value wins.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runItemCreate(cmd, boardID, parentID, name, groupID, colFlags)
		},
	}

	cmd.Flags().StringVar(&boardID, "board", "", "board ID (required unless --parent is given)")
	cmd.Flags().StringVar(&parentID, "parent", "", "parent item ID; creates a subitem when set")
	cmd.Flags().StringVar(&name, "name", "", "item name (required)")
	cmd.Flags().StringVar(&groupID, "group", "", "group ID (optional; ignored when --parent is given)")
	cmd.Flags().StringArrayVar(&colFlags, "col", nil, "column value: <id>=<json> (repeatable)")

	cmd.MarkFlagsMutuallyExclusive("board", "parent")
	_ = cmd.MarkFlagRequired("name")

	return cmd
}

func runItemCreate(cmd *cobra.Command, boardID, parentID, name, groupID string, colFlags []string) error {
	if boardID == "" && parentID == "" {
		return errs.Usage("one of --board or --parent is required")
	}

	if boardID != "" {
		if _, err := strconv.ParseUint(boardID, 10, 64); err != nil {
			return errs.Usage("board id must be a numeric string, got %q", boardID)
		}
	}
	if parentID != "" {
		if _, err := strconv.ParseUint(parentID, 10, 64); err != nil {
			return errs.Usage("parent id must be a numeric string, got %q", parentID)
		}
	}

	cols, err := parseColFlags(colFlags)
	if err != nil {
		return err
	}
	colValuesStr, err := buildColumnValues(cols)
	if err != nil {
		return err
	}

	gql, err := newItemClient()
	if err != nil {
		return err
	}

	var out itemWriteOutput

	if parentID != "" {
		resp, apiErr := gen.SubitemCreate(context.Background(), gql, parentID, name, colValuesStr)
		if apiErr != nil {
			return apiErr
		}
		it := resp.Create_subitem
		out = itemWriteOutput{
			ID:    it.Id,
			Name:  it.Name,
			State: string(it.State),
		}
		if it.Board.Id != "" {
			out.Board = &itemWriteBoard{ID: it.Board.Id, Name: it.Board.Name}
		}
		if it.Parent_item.Id != "" {
			out.ParentItem = &itemWriteParent{ID: it.Parent_item.Id, Name: it.Parent_item.Name}
		}
	} else {
		resp, apiErr := gen.ItemCreate(context.Background(), gql, boardID, name, groupID, colValuesStr)
		if apiErr != nil {
			return apiErr
		}
		it := resp.Create_item
		out = itemWriteOutput{
			ID:    it.Id,
			Name:  it.Name,
			State: string(it.State),
		}
		if it.Board.Id != "" {
			out.Board = &itemWriteBoard{ID: it.Board.Id, Name: it.Board.Name}
		}
		if it.Group.Id != "" {
			out.Group = &itemWriteGroup{ID: it.Group.Id, Title: it.Group.Title}
		}
	}

	mode, modeErr := resolveOutputMode(os.Stdout, globals, configOutputMode())
	if modeErr != nil {
		return modeErr
	}

	if mode == ModeJSON {
		data, mErr := json.Marshal(out)
		if mErr != nil {
			return errs.Internal("marshal output: %v", mErr)
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return err
	}

	kind := "item"
	if parentID != "" {
		kind = "subitem"
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Created %s %s: %s\n", kind, out.ID, out.Name)
	return err
}

// --- item update ---

func newItemUpdateCmd() *cobra.Command {
	var (
		boardID  string
		name     string
		colFlags []string
	)

	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update column values on an item",
		Long: `Update one or more column values on a monday.com item.

Provide --name to rename the item. Use --col <id>=<json> to update columns;
the JSON must match monday's column-value wire shape. At least one of --name
or --col must be given.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runItemUpdate(cmd, args[0], boardID, name, colFlags)
		},
	}

	cmd.Flags().StringVar(&boardID, "board", "", "board ID (required)")
	cmd.Flags().StringVar(&name, "name", "", "new item name (optional)")
	cmd.Flags().StringArrayVar(&colFlags, "col", nil, "column value: <id>=<json> (repeatable)")
	_ = cmd.MarkFlagRequired("board")

	return cmd
}

func runItemUpdate(cmd *cobra.Command, itemID, boardID, name string, colFlags []string) error {
	if _, err := strconv.ParseUint(itemID, 10, 64); err != nil {
		return errs.Usage("item id must be a numeric string, got %q", itemID)
	}
	if _, err := strconv.ParseUint(boardID, 10, 64); err != nil {
		return errs.Usage("board id must be a numeric string, got %q", boardID)
	}

	if name == "" && len(colFlags) == 0 {
		return errs.Usage("nothing to update: provide --name and/or at least one --col")
	}

	cols, err := parseColFlags(colFlags)
	if err != nil {
		return err
	}

	// If --name is provided, inject the name column. Monday expects a bare
	// JSON string for the "name" column in change_multiple_column_values.
	if name != "" {
		nameVal, _ := json.Marshal(name)
		cols["name"] = json.RawMessage(nameVal)
	}

	colValuesStr, err := buildColumnValues(cols)
	if err != nil {
		return err
	}

	gql, err := newItemClient()
	if err != nil {
		return err
	}

	resp, apiErr := gen.ItemUpdate(context.Background(), gql, boardID, itemID, colValuesStr)
	if apiErr != nil {
		return apiErr
	}

	it := resp.Change_multiple_column_values
	out := itemWriteOutput{
		ID:    it.Id,
		Name:  it.Name,
		State: string(it.State),
	}
	if it.Board.Id != "" {
		out.Board = &itemWriteBoard{ID: it.Board.Id, Name: it.Board.Name}
	}
	if it.Group.Id != "" {
		out.Group = &itemWriteGroup{ID: it.Group.Id, Title: it.Group.Title}
	}

	mode, modeErr := resolveOutputMode(os.Stdout, globals, configOutputMode())
	if modeErr != nil {
		return modeErr
	}

	if mode == ModeJSON {
		data, mErr := json.Marshal(out)
		if mErr != nil {
			return errs.Internal("marshal output: %v", mErr)
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return err
	}

	o := cmd.OutOrStdout()
	_, err = fmt.Fprintf(o, "Updated item %s\n", out.ID)
	if name != "" {
		_, _ = fmt.Fprintf(o, "  name → %s\n", name)
	}
	for _, f := range colFlags {
		_, _ = fmt.Fprintf(o, "  col  → %s\n", f)
	}
	return err
}

// --- item post-update ---

// itemPostUpdateOutput is the JSON shape for post-update responses.
type itemPostUpdateOutput struct {
	ID        string `json:"id"`
	Body      string `json:"body,omitempty"`
	TextBody  string `json:"text_body,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

const postUpdateMaxBodyBytes = 1 << 20 // 1MB — guard against pathological input on stdin

func newItemPostUpdateCmd() *cobra.Command {
	var (
		body     string
		parentID string
	)

	cmd := &cobra.Command{
		Use:   "post-update <item-id>",
		Short: "Post a new update (comment) to an item's Updates feed",
		Long: `Post a new update on a monday.com item.

The body comes from --body. Pass "--body -" to read the body from stdin
(useful for multi-line text that would otherwise need shell escaping).

To reply to an existing update post, pass --parent <update-id>.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runItemPostUpdate(cmd, args[0], body, parentID)
		},
	}

	cmd.Flags().StringVar(&body, "body", "", "update text; use - to read from stdin")
	cmd.Flags().StringVar(&parentID, "parent", "", "parent update ID, when replying to an existing post")
	_ = cmd.MarkFlagRequired("body")

	return cmd
}

func runItemPostUpdate(cmd *cobra.Command, itemID, bodyFlag, parentID string) error {
	if _, err := strconv.ParseUint(itemID, 10, 64); err != nil {
		return errs.Usage("item id must be a numeric string, got %q", itemID)
	}
	if parentID != "" {
		if _, err := strconv.ParseUint(parentID, 10, 64); err != nil {
			return errs.Usage("parent id must be a numeric string, got %q", parentID)
		}
	}

	resolved, err := resolvePostUpdateBody(cmd, bodyFlag)
	if err != nil {
		return err
	}
	if resolved == "" {
		return errs.Usage("--body must not be empty")
	}

	gql, err := newItemClient()
	if err != nil {
		return err
	}

	resp, apiErr := gen.ItemPostUpdate(context.Background(), gql, itemID, resolved, parentID)
	if apiErr != nil {
		return apiErr
	}

	out := itemPostUpdateOutput{
		ID:        resp.Create_update.Id,
		Body:      resp.Create_update.Body,
		TextBody:  resp.Create_update.Text_body,
		CreatedAt: resp.Create_update.Created_at,
	}

	mode, modeErr := resolveOutputMode(os.Stdout, globals, configOutputMode())
	if modeErr != nil {
		return modeErr
	}

	if mode == ModeJSON {
		data, mErr := json.Marshal(out)
		if mErr != nil {
			return errs.Internal("marshal output: %v", mErr)
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return err
	}

	o := cmd.OutOrStdout()
	_, err = fmt.Fprintf(o, "Posted update %s on item %s\n", out.ID, itemID)
	return err
}

// resolvePostUpdateBody returns the trimmed body. When bodyFlag is "-", it
// reads from cmd's stdin (cobra wires this to os.Stdin in production and
// allows tests to inject a buffer via cmd.SetIn).
func resolvePostUpdateBody(cmd *cobra.Command, bodyFlag string) (string, error) {
	if bodyFlag != "-" {
		return strings.TrimSpace(bodyFlag), nil
	}
	in := cmd.InOrStdin()
	limited := io.LimitReader(in, postUpdateMaxBodyBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return "", fmt.Errorf("read body from stdin: %w", err)
	}
	if int64(len(data)) > postUpdateMaxBodyBytes {
		return "", errs.Usage("body on stdin exceeds %d bytes", postUpdateMaxBodyBytes)
	}
	return strings.TrimSpace(string(data)), nil
}

// --- item move ---

func newItemMoveCmd() *cobra.Command {
	var toGroup string
	var toBoard string
	var groupID string

	cmd := &cobra.Command{
		Use:   "move <item-id>",
		Short: "Move an item to a different group or board",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runItemMove(cmd, args[0], toGroup, toBoard, groupID)
		},
	}
	cmd.Flags().StringVar(&toGroup, "to-group", "", "Target group ID (within same board)")
	cmd.Flags().StringVar(&toBoard, "to-board", "", "Target board ID")
	cmd.Flags().StringVar(&groupID, "group", "", "Target group ID on target board (required with --to-board)")
	cmd.MarkFlagsMutuallyExclusive("to-group", "to-board")
	return cmd
}

func runItemMove(cmd *cobra.Command, itemID, toGroup, toBoard, groupID string) error {
	if _, err := strconv.ParseUint(itemID, 10, 64); err != nil {
		return errs.Usage("item id must be a numeric string, got %q", itemID)
	}
	if toGroup == "" && toBoard == "" {
		return errs.Usage("must specify --to-group or --to-board")
	}

	gql, err := newItemClient()
	if err != nil {
		return err
	}

	var out itemWriteOutput

	if toGroup != "" {
		resp, apiErr := gen.ItemMoveToGroup(context.Background(), gql, itemID, toGroup)
		if apiErr != nil {
			return apiErr
		}
		it := resp.Move_item_to_group
		out = itemWriteOutput{ID: it.Id, Name: it.Name, State: string(it.State)}
		if it.Board.Id != "" {
			out.Board = &itemWriteBoard{ID: it.Board.Id, Name: it.Board.Name}
		}
		if it.Group.Id != "" {
			out.Group = &itemWriteGroup{ID: it.Group.Id, Title: it.Group.Title}
		}
	} else {
		if groupID == "" {
			return errs.Usage("--group is required when using --to-board")
		}
		resp, apiErr := gen.ItemMoveToBoard(context.Background(), gql, itemID, toBoard, groupID)
		if apiErr != nil {
			return apiErr
		}
		it := resp.Move_item_to_board
		out = itemWriteOutput{ID: it.Id, Name: it.Name, State: string(it.State)}
		if it.Board.Id != "" {
			out.Board = &itemWriteBoard{ID: it.Board.Id, Name: it.Board.Name}
		}
		if it.Group.Id != "" {
			out.Group = &itemWriteGroup{ID: it.Group.Id, Title: it.Group.Title}
		}
	}

	mode, modeErr := resolveOutputMode(os.Stdout, globals, configOutputMode())
	if modeErr != nil {
		return modeErr
	}

	if mode == ModeJSON {
		data, mErr := json.Marshal(out)
		if mErr != nil {
			return errs.Internal("marshal output: %v", mErr)
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return err
	}

	o := cmd.OutOrStdout()
	_, err = fmt.Fprintf(o, "Moved item %s", out.ID)
	if out.Group != nil {
		_, _ = fmt.Fprintf(o, " → group %q", out.Group.Title)
	}
	if out.Board != nil {
		_, _ = fmt.Fprintf(o, " on board %q", out.Board.Name)
	}
	_, _ = fmt.Fprintln(o)
	return err
}

// --- item delete ---

// itemDeleteOutput is the JSON shape for delete and archive responses.
type itemDeleteOutput struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"`
}

func newItemDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Permanently delete an item",
		Long:  "Permanently delete a monday.com item by its numeric ID.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runItemDelete(cmd, args[0])
		},
	}
}

func runItemDelete(cmd *cobra.Command, id string) error {
	if _, err := strconv.ParseUint(id, 10, 64); err != nil {
		return errs.Usage("item id must be a numeric string, got %q", id)
	}

	gql, err := newItemClient()
	if err != nil {
		return err
	}

	resp, err := gen.ItemDelete(context.Background(), gql, id)
	if err != nil {
		return err
	}

	it := resp.Delete_item
	out := itemDeleteOutput{ID: it.Id, Name: it.Name, State: string(it.State)}

	mode, modeErr := resolveOutputMode(os.Stdout, globals, configOutputMode())
	if modeErr != nil {
		return modeErr
	}

	if mode == ModeJSON {
		data, mErr := json.Marshal(out)
		if mErr != nil {
			return errs.Internal("marshal output: %v", mErr)
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return err
	}

	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Deleted item %s: %s\n", out.ID, out.Name)
	return err
}

// --- item archive ---

func newItemArchiveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "archive <id>",
		Short: "Archive an item",
		Long:  "Archive a monday.com item by its numeric ID.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runItemArchive(cmd, args[0])
		},
	}
}

func runItemArchive(cmd *cobra.Command, id string) error {
	if _, err := strconv.ParseUint(id, 10, 64); err != nil {
		return errs.Usage("item id must be a numeric string, got %q", id)
	}

	gql, err := newItemClient()
	if err != nil {
		return err
	}

	resp, err := gen.ItemArchive(context.Background(), gql, id)
	if err != nil {
		return err
	}

	it := resp.Archive_item
	out := itemDeleteOutput{ID: it.Id, Name: it.Name, State: string(it.State)}

	mode, modeErr := resolveOutputMode(os.Stdout, globals, configOutputMode())
	if modeErr != nil {
		return modeErr
	}

	if mode == ModeJSON {
		data, mErr := json.Marshal(out)
		if mErr != nil {
			return errs.Internal("marshal output: %v", mErr)
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return err
	}

	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Archived item %s: %s\n", out.ID, out.Name)
	return err
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

	mode, modeErr := resolveOutputMode(os.Stdout, globals, configOutputMode())
	if modeErr != nil {
		return modeErr
	}

	if mode == ModeJSON {
		data, mErr := json.Marshal(out)
		if mErr != nil {
			return errs.Internal("marshal output: %v", mErr)
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
		return errs.Internal("flush table: %v", err)
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

	mode, modeErr := resolveOutputMode(os.Stdout, globals, configOutputMode())
	if modeErr != nil {
		return modeErr
	}

	if mode == ModeJSON {
		data, mErr := json.Marshal(out)
		if mErr != nil {
			return errs.Internal("marshal output: %v", mErr)
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
