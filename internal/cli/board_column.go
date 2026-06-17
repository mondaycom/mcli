package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/mondaycom/mcli/internal/api/gen"
	"github.com/mondaycom/mcli/internal/errs"
)

// validColumnTypes is the set of column types accepted by the CLI for create.
// Restricted to user-creatable types (excludes system/deprecated types).
var validColumnTypes = map[string]gen.ColumnType{
	"auto_number":    gen.ColumnTypeAutoNumber,
	"board_relation": gen.ColumnTypeBoardRelation,
	"button":         gen.ColumnTypeButton,
	"checkbox":       gen.ColumnTypeCheckbox,
	"color_picker":   gen.ColumnTypeColorPicker,
	"country":        gen.ColumnTypeCountry,
	"creation_log":   gen.ColumnTypeCreationLog,
	"date":           gen.ColumnTypeDate,
	"dependency":     gen.ColumnTypeDependency,
	"doc":            gen.ColumnTypeDoc,
	"dropdown":       gen.ColumnTypeDropdown,
	"email":          gen.ColumnTypeEmail,
	"file":           gen.ColumnTypeFile,
	"formula":        gen.ColumnTypeFormula,
	"hour":           gen.ColumnTypeHour,
	"item_id":        gen.ColumnTypeItemId,
	"last_updated":   gen.ColumnTypeLastUpdated,
	"link":           gen.ColumnTypeLink,
	"location":       gen.ColumnTypeLocation,
	"long_text":      gen.ColumnTypeLongText,
	"mirror":         gen.ColumnTypeMirror,
	"name":           gen.ColumnTypeName,
	"numbers":        gen.ColumnTypeNumbers,
	"people":         gen.ColumnTypePeople,
	"phone":          gen.ColumnTypePhone,
	"progress":       gen.ColumnTypeProgress,
	"rating":         gen.ColumnTypeRating,
	"status":         gen.ColumnTypeStatus,
	"tags":           gen.ColumnTypeTags,
	"team":           gen.ColumnTypeTeam,
	"text":           gen.ColumnTypeText,
	"timeline":       gen.ColumnTypeTimeline,
	"time_tracking":  gen.ColumnTypeTimeTracking,
	"vote":           gen.ColumnTypeVote,
	"week":           gen.ColumnTypeWeek,
	"world_clock":    gen.ColumnTypeWorldClock,
	"unsupported":    gen.ColumnTypeUnsupported,
}

// validColumnTypeNames returns a sorted comma-separated list of valid column type names for help text.
func validColumnTypeNames() string {
	names := make([]string, 0, len(validColumnTypes))
	for k := range validColumnTypes {
		names = append(names, k)
	}
	// Sort for deterministic output.
	for i := 0; i < len(names)-1; i++ {
		for j := i + 1; j < len(names); j++ {
			if names[i] > names[j] {
				names[i], names[j] = names[j], names[i]
			}
		}
	}
	return strings.Join(names, ", ")
}

// newBoardColumnCmd returns the 'mcli board column' parent command.
func newBoardColumnCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "column",
		Short: "Manage board columns",
	}
	cmd.AddCommand(newBoardColumnListCmd())
	cmd.AddCommand(newBoardColumnCreateCmd())
	cmd.AddCommand(newBoardColumnDeleteCmd())
	cmd.AddCommand(newBoardColumnRenameCmd())
	cmd.AddCommand(newBoardColumnDescribeCmd())
	return cmd
}

// columnOutput is the JSON shape for column objects returned by column commands.
type columnOutput struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Type        string `json:"type"`
	SettingsStr string `json:"settings_str"`
	Width       int    `json:"width"`
	Archived    bool   `json:"archived"`
}

// columnListOutput is the JSON shape for 'mcli board column list'.
type columnListOutput struct {
	BoardID string         `json:"board_id"`
	Items   []columnOutput `json:"items"`
}

func newBoardColumnListCmd() *cobra.Command {
	var boardID string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List columns in a board",
		Long:  "List all columns in a monday.com board.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runBoardColumnList(cmd, boardID)
		},
	}

	cmd.Flags().StringVar(&boardID, "board", "", "board ID (required)")
	_ = cmd.MarkFlagRequired("board")

	return cmd
}

func runBoardColumnList(cmd *cobra.Command, boardID string) error {
	if boardID == "" {
		return errs.Usage("--board is required")
	}

	gql, err := newBoardClient()
	if err != nil {
		return err
	}

	resp, err := gen.BoardColumnList(context.Background(), gql, boardID)
	if err != nil {
		return err
	}

	if len(resp.Boards) == 0 {
		return errs.NotFound("board %s", boardID)
	}

	b := resp.Boards[0]
	items := make([]columnOutput, len(b.Columns))
	for i, c := range b.Columns {
		items[i] = columnOutput{
			ID:          c.Id,
			Title:       c.Title,
			Type:        string(c.Type),
			SettingsStr: c.Settings_str,
			Width:       c.Width,
			Archived:    c.Archived,
		}
	}

	out := columnListOutput{BoardID: boardID, Items: items}

	mode, modeErr := resolveOutputMode(os.Stdout, globals, configOutputMode())
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

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tTITLE\tTYPE\tARCHIVED")
	for _, c := range items {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%v\n", c.ID, c.Title, c.Type, c.Archived)
	}
	return w.Flush()
}

func newBoardColumnCreateCmd() *cobra.Command {
	var (
		boardID     string
		title       string
		columnType  string
		description string
		defaults    string
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a column in a board",
		Long: fmt.Sprintf(
			"Create a new column in a monday.com board.\n\nValid types: %s",
			validColumnTypeNames(),
		),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runBoardColumnCreate(cmd, boardID, title, columnType, description, defaults)
		},
	}

	cmd.Flags().StringVar(&boardID, "board", "", "board ID (required)")
	cmd.Flags().StringVar(&title, "title", "", "column title (required)")
	cmd.Flags().StringVar(&columnType, "type", "", "column type (required)")
	cmd.Flags().StringVar(&description, "description", "", "column description (optional)")
	cmd.Flags().StringVar(&defaults, "defaults", "", "column defaults as JSON (optional)")
	_ = cmd.MarkFlagRequired("board")
	_ = cmd.MarkFlagRequired("title")
	_ = cmd.MarkFlagRequired("type")

	return cmd
}

func runBoardColumnCreate(cmd *cobra.Command, boardID, title, columnType, description, defaults string) error {
	if boardID == "" {
		return errs.Usage("--board is required")
	}
	if title == "" {
		return errs.Usage("--title is required")
	}
	if columnType == "" {
		return errs.Usage("--type is required")
	}

	ct, ok := validColumnTypes[columnType]
	if !ok {
		return errs.Usage("invalid --type %q: must be one of: %s", columnType, validColumnTypeNames())
	}

	// Validate defaults JSON if provided.
	if defaults != "" {
		if !json.Valid([]byte(defaults)) {
			return errs.Usage("--defaults must be valid JSON")
		}
	}

	gql, err := newBoardClient()
	if err != nil {
		return err
	}

	resp, err := gen.BoardColumnCreate(context.Background(), gql, boardID, title, ct, description, defaults)
	if err != nil {
		return err
	}

	c := resp.Create_column
	out := columnOutput{
		ID:          c.Id,
		Title:       c.Title,
		Type:        string(c.Type),
		SettingsStr: c.Settings_str,
		Width:       c.Width,
		Archived:    c.Archived,
	}

	mode, modeErr := resolveOutputMode(os.Stdout, globals, configOutputMode())
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

	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Created column %s: %s (%s)\n", out.ID, out.Title, out.Type)
	return err
}

// columnDeleteOutput is the JSON shape for column delete.
type columnDeleteOutput struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

func newBoardColumnDeleteCmd() *cobra.Command {
	var (
		boardID  string
		columnID string
	)

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a column from a board",
		Long:  "Permanently delete a column from a monday.com board.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runBoardColumnDelete(cmd, boardID, columnID)
		},
	}

	cmd.Flags().StringVar(&boardID, "board", "", "board ID (required)")
	cmd.Flags().StringVar(&columnID, "column", "", "column ID (required)")
	_ = cmd.MarkFlagRequired("board")
	_ = cmd.MarkFlagRequired("column")

	return cmd
}

func runBoardColumnDelete(cmd *cobra.Command, boardID, columnID string) error {
	if boardID == "" {
		return errs.Usage("--board is required")
	}
	if columnID == "" {
		return errs.Usage("--column is required")
	}

	gql, err := newBoardClient()
	if err != nil {
		return err
	}

	resp, err := gen.BoardColumnDelete(context.Background(), gql, boardID, columnID)
	if err != nil {
		return err
	}

	c := resp.Delete_column
	out := columnDeleteOutput{ID: c.Id, Title: c.Title}

	mode, modeErr := resolveOutputMode(os.Stdout, globals, configOutputMode())
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

	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Deleted column %s: %s\n", out.ID, out.Title)
	return err
}

func newBoardColumnRenameCmd() *cobra.Command {
	var (
		boardID  string
		columnID string
		title    string
	)

	cmd := &cobra.Command{
		Use:   "rename",
		Short: "Rename a column in a board",
		Long:  "Rename a column in a monday.com board.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runBoardColumnRename(cmd, boardID, columnID, title)
		},
	}

	cmd.Flags().StringVar(&boardID, "board", "", "board ID (required)")
	cmd.Flags().StringVar(&columnID, "column", "", "column ID (required)")
	cmd.Flags().StringVar(&title, "title", "", "new column title (required)")
	_ = cmd.MarkFlagRequired("board")
	_ = cmd.MarkFlagRequired("column")
	_ = cmd.MarkFlagRequired("title")

	return cmd
}

func runBoardColumnRename(cmd *cobra.Command, boardID, columnID, title string) error {
	if boardID == "" {
		return errs.Usage("--board is required")
	}
	if columnID == "" {
		return errs.Usage("--column is required")
	}
	if title == "" {
		return errs.Usage("--title is required")
	}

	gql, err := newBoardClient()
	if err != nil {
		return err
	}

	resp, err := gen.BoardColumnRename(context.Background(), gql, boardID, columnID, title)
	if err != nil {
		return err
	}

	c := resp.Change_column_title
	out := columnOutput{
		ID:          c.Id,
		Title:       c.Title,
		Type:        string(c.Type),
		SettingsStr: c.Settings_str,
		Width:       c.Width,
		Archived:    c.Archived,
	}

	mode, modeErr := resolveOutputMode(os.Stdout, globals, configOutputMode())
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

	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Renamed column %s: %s\n", out.ID, out.Title)
	return err
}

func newBoardColumnDescribeCmd() *cobra.Command {
	var (
		boardID     string
		columnID    string
		description string
	)

	cmd := &cobra.Command{
		Use:   "describe",
		Short: "Set description of a column",
		Long:  "Update the description of a column in a monday.com board.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runBoardColumnDescribe(cmd, boardID, columnID, description)
		},
	}

	cmd.Flags().StringVar(&boardID, "board", "", "board ID (required)")
	cmd.Flags().StringVar(&columnID, "column", "", "column ID (required)")
	cmd.Flags().StringVar(&description, "text", "", "column description text (required)")
	_ = cmd.MarkFlagRequired("board")
	_ = cmd.MarkFlagRequired("column")
	_ = cmd.MarkFlagRequired("text")

	return cmd
}

func runBoardColumnDescribe(cmd *cobra.Command, boardID, columnID, description string) error {
	if boardID == "" {
		return errs.Usage("--board is required")
	}
	if columnID == "" {
		return errs.Usage("--column is required")
	}
	if description == "" {
		return errs.Usage("--text is required")
	}

	gql, err := newBoardClient()
	if err != nil {
		return err
	}

	resp, err := gen.BoardColumnDescribe(context.Background(), gql, boardID, columnID, description)
	if err != nil {
		return err
	}

	c := resp.Change_column_metadata
	out := columnOutput{
		ID:          c.Id,
		Title:       c.Title,
		Type:        string(c.Type),
		SettingsStr: c.Settings_str,
		Width:       c.Width,
		Archived:    c.Archived,
	}

	mode, modeErr := resolveOutputMode(os.Stdout, globals, configOutputMode())
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

	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Updated description for column %s: %s\n", out.ID, out.Title)
	return err
}
