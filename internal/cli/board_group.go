package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/mondaycom/mcli/internal/api/gen"
	"github.com/mondaycom/mcli/internal/errs"
)

// newBoardGroupCmd returns the 'mcli board group' parent command.
func newBoardGroupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "group",
		Short: "Manage board groups",
	}
	cmd.AddCommand(newBoardGroupListCmd())
	cmd.AddCommand(newBoardGroupCreateCmd())
	cmd.AddCommand(newBoardGroupDeleteCmd())
	cmd.AddCommand(newBoardGroupArchiveCmd())
	cmd.AddCommand(newBoardGroupRenameCmd())
	return cmd
}

// groupOutput is the JSON shape for group objects returned by group commands.
type groupOutput struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Color    string `json:"color"`
	Position string `json:"position"`
}

// groupListOutput is the JSON shape for 'mcli board group list'.
type groupListOutput struct {
	BoardID string        `json:"board_id"`
	Items   []groupOutput `json:"items"`
}

func newBoardGroupListCmd() *cobra.Command {
	var boardID string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List groups in a board",
		Long:  "List all visible groups in a monday.com board.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runBoardGroupList(cmd, boardID)
		},
	}

	cmd.Flags().StringVar(&boardID, "board", "", "board ID (required)")
	_ = cmd.MarkFlagRequired("board")

	return cmd
}

func runBoardGroupList(cmd *cobra.Command, boardID string) error {
	if boardID == "" {
		return errs.Usage("--board is required")
	}

	gql, err := newBoardClient()
	if err != nil {
		return err
	}

	resp, err := gen.BoardGroupList(context.Background(), gql, boardID)
	if err != nil {
		return err
	}

	if len(resp.Boards) == 0 {
		return errs.NotFound("board %s", boardID)
	}

	b := resp.Boards[0]
	items := make([]groupOutput, len(b.Groups))
	for i, g := range b.Groups {
		items[i] = groupOutput{
			ID:       g.Id,
			Title:    g.Title,
			Color:    g.Color,
			Position: g.Position,
		}
	}

	out := groupListOutput{BoardID: boardID, Items: items}

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
	_, _ = fmt.Fprintln(w, "ID\tTITLE\tCOLOR\tPOSITION")
	for _, g := range items {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", g.ID, g.Title, g.Color, g.Position)
	}
	return w.Flush()
}

func newBoardGroupCreateCmd() *cobra.Command {
	var (
		boardID string
		name    string
		color   string
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a group in a board",
		Long:  "Create a new group in a monday.com board.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runBoardGroupCreate(cmd, boardID, name, color)
		},
	}

	cmd.Flags().StringVar(&boardID, "board", "", "board ID (required)")
	cmd.Flags().StringVar(&name, "name", "", "group name (required)")
	cmd.Flags().StringVar(&color, "color", "", "group color as hex (optional)")
	_ = cmd.MarkFlagRequired("board")
	_ = cmd.MarkFlagRequired("name")

	return cmd
}

func runBoardGroupCreate(cmd *cobra.Command, boardID, name, color string) error {
	if boardID == "" {
		return errs.Usage("--board is required")
	}
	if name == "" {
		return errs.Usage("--name is required")
	}

	gql, err := newBoardClient()
	if err != nil {
		return err
	}

	resp, err := gen.BoardGroupCreate(context.Background(), gql, boardID, name, color)
	if err != nil {
		return err
	}

	g := resp.Create_group
	out := groupOutput{
		ID:       g.Id,
		Title:    g.Title,
		Color:    g.Color,
		Position: g.Position,
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

	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Created group %s: %s\n", out.ID, out.Title)
	return err
}

// groupDeleteOutput is the JSON shape for group delete/archive.
type groupDeleteOutput struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

func newBoardGroupDeleteCmd() *cobra.Command {
	var (
		boardID string
		groupID string
	)

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a group from a board",
		Long:  "Permanently delete a group from a monday.com board.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runBoardGroupDelete(cmd, boardID, groupID)
		},
	}

	cmd.Flags().StringVar(&boardID, "board", "", "board ID (required)")
	cmd.Flags().StringVar(&groupID, "group", "", "group ID (required)")
	_ = cmd.MarkFlagRequired("board")
	_ = cmd.MarkFlagRequired("group")

	return cmd
}

func runBoardGroupDelete(cmd *cobra.Command, boardID, groupID string) error {
	if boardID == "" {
		return errs.Usage("--board is required")
	}
	if groupID == "" {
		return errs.Usage("--group is required")
	}

	gql, err := newBoardClient()
	if err != nil {
		return err
	}

	resp, err := gen.BoardGroupDelete(context.Background(), gql, boardID, groupID)
	if err != nil {
		return err
	}

	g := resp.Delete_group
	out := groupDeleteOutput{ID: g.Id, Title: g.Title}

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

	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Deleted group %s: %s\n", out.ID, out.Title)
	return err
}

func newBoardGroupArchiveCmd() *cobra.Command {
	var (
		boardID string
		groupID string
	)

	cmd := &cobra.Command{
		Use:   "archive",
		Short: "Archive a group in a board",
		Long:  "Archive a group in a monday.com board.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runBoardGroupArchive(cmd, boardID, groupID)
		},
	}

	cmd.Flags().StringVar(&boardID, "board", "", "board ID (required)")
	cmd.Flags().StringVar(&groupID, "group", "", "group ID (required)")
	_ = cmd.MarkFlagRequired("board")
	_ = cmd.MarkFlagRequired("group")

	return cmd
}

func runBoardGroupArchive(cmd *cobra.Command, boardID, groupID string) error {
	if boardID == "" {
		return errs.Usage("--board is required")
	}
	if groupID == "" {
		return errs.Usage("--group is required")
	}

	gql, err := newBoardClient()
	if err != nil {
		return err
	}

	resp, err := gen.BoardGroupArchive(context.Background(), gql, boardID, groupID)
	if err != nil {
		return err
	}

	g := resp.Archive_group
	out := groupDeleteOutput{ID: g.Id, Title: g.Title}

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

	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Archived group %s: %s\n", out.ID, out.Title)
	return err
}

func newBoardGroupRenameCmd() *cobra.Command {
	var (
		boardID string
		groupID string
		name    string
	)

	cmd := &cobra.Command{
		Use:   "rename",
		Short: "Rename a group in a board",
		Long:  "Rename a group in a monday.com board.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runBoardGroupRename(cmd, boardID, groupID, name)
		},
	}

	cmd.Flags().StringVar(&boardID, "board", "", "board ID (required)")
	cmd.Flags().StringVar(&groupID, "group", "", "group ID (required)")
	cmd.Flags().StringVar(&name, "name", "", "new group name (required)")
	_ = cmd.MarkFlagRequired("board")
	_ = cmd.MarkFlagRequired("group")
	_ = cmd.MarkFlagRequired("name")

	return cmd
}

func runBoardGroupRename(cmd *cobra.Command, boardID, groupID, name string) error {
	if boardID == "" {
		return errs.Usage("--board is required")
	}
	if groupID == "" {
		return errs.Usage("--group is required")
	}
	if name == "" {
		return errs.Usage("--name is required")
	}

	gql, err := newBoardClient()
	if err != nil {
		return err
	}

	resp, err := gen.BoardGroupRename(context.Background(), gql, boardID, groupID, name)
	if err != nil {
		return err
	}

	g := resp.Update_group
	out := groupOutput{
		ID:       g.Id,
		Title:    g.Title,
		Color:    g.Color,
		Position: g.Position,
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

	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Renamed group %s: %s\n", out.ID, out.Title)
	return err
}
