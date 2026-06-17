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
	"github.com/mondaycom/mcli/internal/errs"

	gqlclient "github.com/Khan/genqlient/graphql"
)

const (
	folderListDefaultLimit = 25
	folderListMaxLimit     = 100
)

// folderClientFactory is an unexported seam that lets tests inject a fake
// graphql.Client without touching network or environment. Production code
// leaves this nil, which causes newFolderClient to build a real client.
var folderClientFactory func() (gqlclient.Client, error)

// newFolderClient resolves a graphql.Client for folder commands.
// If folderClientFactory is set (tests), it delegates there.
// Otherwise it builds a real authenticated client from config + secrets.
func newFolderClient() (gqlclient.Client, error) {
	if folderClientFactory != nil {
		return folderClientFactory()
	}
	return newGQLClient()
}

// newFolderCmd returns the 'mcli folder' parent command.
func newFolderCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "folder",
		Short: "Manage monday.com folders",
	}
	cmd.AddCommand(newFolderListCmd())
	cmd.AddCommand(newFolderCreateCmd())
	cmd.AddCommand(newFolderRenameCmd())
	cmd.AddCommand(newFolderDeleteCmd())
	return cmd
}

// folderItem is the per-folder shape used in list and other outputs.
type folderItem struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Color     string `json:"color,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	OwnerID   string `json:"owner_id,omitempty"`
}

// folderListOutput is the JSON shape for 'mcli folder list'.
type folderListOutput struct {
	Items  []folderItem `json:"items"`
	Cursor string       `json:"cursor"`
}

func newFolderListCmd() *cobra.Command {
	var (
		workspaceID string
		limit       int
		cursor      string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List folders",
		Long:  "List monday.com folders, optionally filtered by workspace.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runFolderList(cmd, workspaceID, limit, cursor)
		},
	}

	cmd.Flags().StringVar(&workspaceID, "workspace", "", "filter by workspace ID")
	cmd.Flags().IntVar(&limit, "limit", folderListDefaultLimit, "number of folders to return (max 100)")
	cmd.Flags().StringVar(&cursor, "cursor", "", "page cursor (omit for first page)")

	return cmd
}

func runFolderList(cmd *cobra.Command, workspaceID string, limit int, cursor string) error {
	// Clamp limit.
	if limit < 1 {
		limit = folderListDefaultLimit
	}
	if limit > folderListMaxLimit {
		limit = folderListMaxLimit
	}

	// Decode cursor → page number.
	page := 1
	if cursor != "" {
		p, err := strconv.Atoi(cursor)
		if err != nil || p < 1 {
			return errs.Usage("invalid cursor %q: must be a positive integer page number", cursor)
		}
		page = p
	}

	var workspaceIDs []string
	if workspaceID != "" {
		workspaceIDs = []string{workspaceID}
	}

	gql, err := newFolderClient()
	if err != nil {
		return err
	}

	resp, err := gen.FoldersList(context.Background(), gql, limit, page, workspaceIDs)
	if err != nil {
		return err
	}

	items := make([]folderItem, len(resp.Folders))
	for i, f := range resp.Folders {
		items[i] = folderItem{
			ID:        f.Id,
			Name:      f.Name,
			Color:     string(f.Color),
			CreatedAt: f.Created_at,
			OwnerID:   f.Owner_id,
		}
	}

	// If we got exactly limit results, there may be more.
	nextCursor := ""
	if len(resp.Folders) == limit {
		nextCursor = strconv.Itoa(page + 1)
	}

	mode, modeErr := resolveOutputMode(os.Stdout, globals, configOutputMode())
	if modeErr != nil {
		return modeErr
	}

	if mode == ModeJSON {
		out := folderListOutput{Items: items, Cursor: nextCursor}
		data, mErr := json.Marshal(out)
		if mErr != nil {
			return errs.Internal("marshal output: %v", mErr)
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return err
	}

	// Pretty output: tab-aligned table.
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tNAME\tCOLOR\tOWNER")
	for _, item := range items {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", item.ID, item.Name, item.Color, item.OwnerID)
	}
	if err := w.Flush(); err != nil {
		return errs.Internal("flush table: %v", err)
	}
	if nextCursor != "" {
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "\nnext cursor: %s\n", nextCursor)
	}
	return err
}

func newFolderCreateCmd() *cobra.Command {
	var (
		name        string
		workspaceID string
		parentID    string
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new folder",
		Long:  "Create a new monday.com folder in a workspace or under a parent folder.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runFolderCreate(cmd, name, workspaceID, parentID)
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "folder name (required)")
	cmd.Flags().StringVar(&workspaceID, "workspace", "", "workspace ID (optional)")
	cmd.Flags().StringVar(&parentID, "parent", "", "parent folder ID (optional)")
	_ = cmd.MarkFlagRequired("name")

	return cmd
}

func runFolderCreate(cmd *cobra.Command, name, workspaceID, parentID string) error {
	gql, err := newFolderClient()
	if err != nil {
		return err
	}

	resp, err := gen.FolderCreate(context.Background(), gql, name, workspaceID, parentID)
	if err != nil {
		return err
	}

	f := resp.Create_folder
	if f.Id == "" {
		return errs.API("create_folder returned no folder")
	}

	out := folderItem{
		ID:        f.Id,
		Name:      f.Name,
		Color:     string(f.Color),
		CreatedAt: f.Created_at,
		OwnerID:   f.Owner_id,
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

	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Created folder %s: %s\n", out.ID, out.Name)
	return err
}

func newFolderRenameCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rename <id> <name>",
		Short: "Rename a folder",
		Long:  "Rename a monday.com folder by its ID.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFolderRename(cmd, args[0], args[1])
		},
	}
}

func runFolderRename(cmd *cobra.Command, id, name string) error {
	if _, err := strconv.ParseUint(id, 10, 64); err != nil {
		return errs.Usage("folder id must be a numeric string, got %q", id)
	}

	gql, err := newFolderClient()
	if err != nil {
		return err
	}

	resp, err := gen.FolderRename(context.Background(), gql, id, name)
	if err != nil {
		return err
	}

	f := resp.Update_folder
	out := folderItem{
		ID:        f.Id,
		Name:      f.Name,
		Color:     string(f.Color),
		CreatedAt: f.Created_at,
		OwnerID:   f.Owner_id,
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

	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Renamed folder %s: %s\n", out.ID, out.Name)
	return err
}

// folderDeleteOutput is the JSON shape for 'mcli folder delete'.
type folderDeleteOutput struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func newFolderDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a folder",
		Long:  "Delete a monday.com folder by its numeric ID.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFolderDelete(cmd, args[0])
		},
	}
}

func runFolderDelete(cmd *cobra.Command, id string) error {
	if _, err := strconv.ParseUint(id, 10, 64); err != nil {
		return errs.Usage("folder id must be a numeric string, got %q", id)
	}

	gql, err := newFolderClient()
	if err != nil {
		return err
	}

	resp, err := gen.FolderDelete(context.Background(), gql, id)
	if err != nil {
		return err
	}

	f := resp.Delete_folder
	out := folderDeleteOutput{
		ID:   f.Id,
		Name: f.Name,
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

	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Deleted folder %s: %s\n", out.ID, out.Name)
	return err
}
