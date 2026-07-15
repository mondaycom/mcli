package cli

import (
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
	workspaceListDefaultLimit = 25
	workspaceListMaxLimit     = 100
)

// workspaceClientFactory is an unexported seam that lets tests inject a fake
// graphql.Client without touching network or environment. Production code
// leaves this nil, which causes newWorkspaceClient to build a real client.
var workspaceClientFactory func() (gqlclient.Client, error)

// newWorkspaceClient resolves a graphql.Client for workspace commands.
// If workspaceClientFactory is set (tests), it delegates there.
// Otherwise it builds a real authenticated client from config + secrets.
func newWorkspaceClient() (gqlclient.Client, error) {
	if workspaceClientFactory != nil {
		return workspaceClientFactory()
	}
	return newGQLClient()
}

// newWorkspaceCmd returns the 'mcli workspace' parent command.
func newWorkspaceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workspace",
		Short: "Manage monday.com workspaces",
	}
	cmd.AddCommand(newWorkspaceListCmd())
	cmd.AddCommand(newWorkspaceGetCmd())
	cmd.AddCommand(newWorkspaceCreateCmd())
	cmd.AddCommand(newWorkspaceUpdateCmd())
	cmd.AddCommand(newWorkspaceDeleteCmd())
	return cmd
}

// workspaceItem is the per-workspace shape used in list and other outputs.
type workspaceItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Description string `json:"description"`
	CreatedAt   string `json:"created_at,omitempty"`
	State       string `json:"state"`
}

// workspaceListOutput is the JSON shape for 'mcli workspace list'.
// cursor is the next page number as a string, or empty when no more pages.
type workspaceListOutput struct {
	Items  []workspaceItem `json:"items"`
	Cursor string          `json:"cursor"`
}

// validWorkspaceKinds maps the lowercase CLI-accepted kind strings to their
// gen.WorkspaceKind constants.
var validWorkspaceKinds = map[string]gen.WorkspaceKind{
	"open":     gen.WorkspaceKindOpen,
	"closed":   gen.WorkspaceKindClosed,
	"template": gen.WorkspaceKindTemplate,
}

func newWorkspaceListCmd() *cobra.Command {
	var (
		kind   string
		limit  int
		cursor string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List workspaces",
		Long:  "List monday.com workspaces, optionally filtered by kind.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runWorkspaceList(cmd, kind, limit, cursor)
		},
	}

	cmd.Flags().StringVar(&kind, "kind", "", "filter by workspace kind: open|closed|template")
	cmd.Flags().IntVar(&limit, "limit", workspaceListDefaultLimit, "number of workspaces to return (max 100)")
	cmd.Flags().StringVar(&cursor, "cursor", "", "page cursor (omit for first page)")

	return cmd
}

func runWorkspaceList(cmd *cobra.Command, kind string, limit int, cursor string) error {
	// Clamp limit.
	if limit < 1 {
		limit = workspaceListDefaultLimit
	}
	if limit > workspaceListMaxLimit {
		limit = workspaceListMaxLimit
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

	var wsKind gen.WorkspaceKind
	if kind != "" {
		k, ok := validWorkspaceKinds[kind]
		if !ok {
			return errs.Usage("invalid --kind %q: must be one of open, closed, template", kind)
		}
		wsKind = k
	}

	gql, err := newWorkspaceClient()
	if err != nil {
		return err
	}

	resp, err := gen.WorkspacesList(cmd.Context(), gql, wsKind, limit, page)
	if err != nil {
		return err
	}

	items := make([]workspaceItem, len(resp.Workspaces))
	for i, w := range resp.Workspaces {
		items[i] = workspaceItem{
			ID:          w.Id,
			Name:        w.Name,
			Kind:        string(w.Kind),
			Description: w.Description,
			CreatedAt:   w.Created_at,
			State:       string(w.State),
		}
	}

	// If we got exactly limit results, there may be more.
	nextCursor := ""
	if len(resp.Workspaces) == limit {
		nextCursor = strconv.Itoa(page + 1)
	}

	mode, modeErr := resolveOutputMode(os.Stdout, globals, configOutputMode())
	if modeErr != nil {
		return modeErr
	}

	if mode == ModeJSON {
		out := workspaceListOutput{Items: items, Cursor: nextCursor}
		data, mErr := json.Marshal(out)
		if mErr != nil {
			return errs.Internal("marshal output: %v", mErr)
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return err
	}

	// Pretty output: tab-aligned table.
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tNAME\tKIND\tSTATE")
	for _, item := range items {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", item.ID, item.Name, item.Kind, item.State)
	}
	if err := w.Flush(); err != nil {
		return errs.Internal("flush table: %v", err)
	}
	if nextCursor != "" {
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "\nnext cursor: %s\n", nextCursor)
	}
	return err
}

func newWorkspaceGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get a workspace by ID",
		Long:  "Fetch details for a monday.com workspace by its numeric ID.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkspaceGet(cmd, args[0])
		},
	}
}

func runWorkspaceGet(cmd *cobra.Command, id string) error {
	if _, err := strconv.ParseUint(id, 10, 64); err != nil {
		return errs.Usage("workspace id must be a numeric string, got %q", id)
	}

	gql, err := newWorkspaceClient()
	if err != nil {
		return err
	}

	resp, err := gen.WorkspaceGet(cmd.Context(), gql, id)
	if err != nil {
		return err
	}

	if len(resp.Workspaces) == 0 {
		return errs.NotFound("workspace %s", id)
	}

	w := resp.Workspaces[0]
	out := workspaceItem{
		ID:          w.Id,
		Name:        w.Name,
		Kind:        string(w.Kind),
		Description: w.Description,
		CreatedAt:   w.Created_at,
		State:       string(w.State),
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
	_, _ = fmt.Fprintf(o, "ID:          %s\n", out.ID)
	_, _ = fmt.Fprintf(o, "Name:        %s\n", out.Name)
	_, _ = fmt.Fprintf(o, "Kind:        %s\n", out.Kind)
	_, _ = fmt.Fprintf(o, "State:       %s\n", out.State)
	if out.Description != "" {
		_, _ = fmt.Fprintf(o, "Description: %s\n", out.Description)
	}
	if out.CreatedAt != "" {
		_, _ = fmt.Fprintf(o, "Created:     %s\n", out.CreatedAt)
	}
	return nil
}

func newWorkspaceCreateCmd() *cobra.Command {
	var (
		name        string
		kind        string
		description string
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new workspace",
		Long:  "Create a new monday.com workspace with the given name and kind.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runWorkspaceCreate(cmd, name, kind, description)
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "workspace name (required)")
	cmd.Flags().StringVar(&kind, "kind", "", "workspace kind: open|closed (required)")
	cmd.Flags().StringVar(&description, "description", "", "workspace description (optional)")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("kind")

	return cmd
}

func runWorkspaceCreate(cmd *cobra.Command, name, kind, description string) error {
	wsKind, ok := validWorkspaceKinds[kind]
	if !ok {
		return errs.Usage("invalid --kind %q: must be one of open, closed, template", kind)
	}

	gql, err := newWorkspaceClient()
	if err != nil {
		return err
	}

	resp, err := gen.WorkspaceCreate(cmd.Context(), gql, name, wsKind, description)
	if err != nil {
		return err
	}

	w := resp.Create_workspace
	if w.Id == "" {
		return errs.API("create_workspace returned no workspace")
	}

	out := workspaceItem{
		ID:          w.Id,
		Name:        w.Name,
		Kind:        string(w.Kind),
		Description: w.Description,
		CreatedAt:   w.Created_at,
		State:       string(w.State),
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

	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Created workspace %s: %s\n", out.ID, out.Name)
	return err
}

func newWorkspaceUpdateCmd() *cobra.Command {
	var (
		name        string
		description string
		kind        string
	)

	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update a workspace",
		Long:  "Update name, description, or kind of a monday.com workspace. At least one flag required.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkspaceUpdate(cmd, args[0], name, description, kind)
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "new workspace name")
	cmd.Flags().StringVar(&description, "description", "", "new workspace description")
	cmd.Flags().StringVar(&kind, "kind", "", "new workspace kind: open|closed|template")

	return cmd
}

func runWorkspaceUpdate(cmd *cobra.Command, id, name, description, kind string) error {
	if _, err := strconv.ParseUint(id, 10, 64); err != nil {
		return errs.Usage("workspace id must be a numeric string, got %q", id)
	}

	if name == "" && description == "" && kind == "" {
		return errs.Usage("nothing to update: provide at least one of --name, --description, --kind")
	}

	attrs := gen.UpdateWorkspaceAttributesInput{
		Name:        name,
		Description: description,
	}

	if kind != "" {
		wsKind, ok := validWorkspaceKinds[kind]
		if !ok {
			return errs.Usage("invalid --kind %q: must be one of open, closed, template", kind)
		}
		attrs.Kind = wsKind
	}

	gql, err := newWorkspaceClient()
	if err != nil {
		return err
	}

	resp, err := gen.WorkspaceUpdate(cmd.Context(), gql, id, attrs)
	if err != nil {
		return err
	}

	w := resp.Update_workspace
	out := workspaceItem{
		ID:          w.Id,
		Name:        w.Name,
		Kind:        string(w.Kind),
		Description: w.Description,
		CreatedAt:   w.Created_at,
		State:       string(w.State),
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

	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Updated workspace %s: %s\n", out.ID, out.Name)
	return err
}

// workspaceDeleteOutput is the JSON shape for 'mcli workspace delete'.
type workspaceDeleteOutput struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func newWorkspaceDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a workspace",
		Long:  "Delete a monday.com workspace by its numeric ID.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkspaceDelete(cmd, args[0])
		},
	}
}

func runWorkspaceDelete(cmd *cobra.Command, id string) error {
	if _, err := strconv.ParseUint(id, 10, 64); err != nil {
		return errs.Usage("workspace id must be a numeric string, got %q", id)
	}

	gql, err := newWorkspaceClient()
	if err != nil {
		return err
	}

	resp, err := gen.WorkspaceDelete(cmd.Context(), gql, id)
	if err != nil {
		return err
	}

	w := resp.Delete_workspace
	out := workspaceDeleteOutput{
		ID:   w.Id,
		Name: w.Name,
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

	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Deleted workspace %s: %s\n", out.ID, out.Name)
	return err
}
