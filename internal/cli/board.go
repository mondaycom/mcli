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
	"github.com/mondaycom/mcli/internal/config"
	"github.com/mondaycom/mcli/internal/errs"
	"github.com/mondaycom/mcli/internal/secrets"

	gqlclient "github.com/Khan/genqlient/graphql"
)

const (
	boardListDefaultLimit = 25
	boardListMaxLimit     = 100
)

// boardClientFactory is an unexported seam that lets tests inject a fake
// graphql.Client without touching network or environment. Production code
// leaves this nil, which causes newBoardClient to build a real client.
var boardClientFactory func() (gqlclient.Client, error)

// newBoardClient resolves a graphql.Client for board commands.
// If boardClientFactory is set (tests), it delegates there.
// Otherwise it builds a real authenticated client from config + secrets.
func newBoardClient() (gqlclient.Client, error) {
	if boardClientFactory != nil {
		return boardClientFactory()
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

// newBoardCmd returns the 'mcli board' parent command.
func newBoardCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "board",
		Short: "Manage monday.com boards",
	}
	cmd.AddCommand(newBoardListCmd())
	cmd.AddCommand(newBoardGetCmd())
	cmd.AddCommand(newBoardCreateCmd())
	return cmd
}

// boardListOutput is the JSON shape for 'mcli board list' per ADR-002.
// cursor is the next page number as a string, or empty when no more pages.
type boardListOutput struct {
	Items  []boardListItem `json:"items"`
	Cursor string          `json:"cursor"`
}

type boardListItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	State       string `json:"state"`
	WorkspaceID string `json:"workspace_id"`
}

func newBoardListCmd() *cobra.Command {
	var (
		workspaceID string
		limit       int
		cursor      string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List boards",
		Long:  "List monday.com boards, optionally filtered by workspace.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runBoardList(cmd, workspaceID, limit, cursor)
		},
	}

	cmd.Flags().StringVar(&workspaceID, "workspace", "", "filter by workspace ID")
	cmd.Flags().IntVar(&limit, "limit", boardListDefaultLimit, "number of boards to return (max 100)")
	cmd.Flags().StringVar(&cursor, "cursor", "", "page cursor (omit for first page)")

	return cmd
}

func runBoardList(cmd *cobra.Command, workspaceID string, limit int, cursor string) error {
	// Clamp limit.
	if limit < 1 {
		limit = boardListDefaultLimit
	}
	if limit > boardListMaxLimit {
		limit = boardListMaxLimit
	}

	// Decode cursor → page number.
	// monday.com boards use page/limit pagination, not cursor-based.
	// We encode the cursor as the decimal string of the page number so the
	// CLI surface stays uniform with other paginated commands. Page 1 is first.
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

	gql, err := newBoardClient()
	if err != nil {
		return err
	}

	resp, err := gen.BoardsList(context.Background(), gql, limit, page, workspaceIDs)
	if err != nil {
		return err
	}

	// Build items.
	items := make([]boardListItem, len(resp.Boards))
	for i, b := range resp.Boards {
		items[i] = boardListItem{
			ID:          b.Id,
			Name:        b.Name,
			Kind:        string(b.Board_kind),
			State:       string(b.State),
			WorkspaceID: b.Workspace_id,
		}
	}

	// Determine next cursor: if we got exactly limit results, there may be more.
	// Encode the next page number as the cursor string. If we got fewer, no more.
	nextCursor := ""
	if len(resp.Boards) == limit {
		nextCursor = strconv.Itoa(page + 1)
	}

	mode, modeErr := resolveOutputMode(os.Stdout, globals)
	if modeErr != nil {
		return modeErr
	}

	if mode == ModeJSON {
		out := boardListOutput{Items: items, Cursor: nextCursor}
		data, mErr := json.Marshal(out)
		if mErr != nil {
			return fmt.Errorf("marshal output: %w", mErr)
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return err
	}

	// Pretty output: tab-aligned table.
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tNAME\tKIND\tSTATE\tWORKSPACE")
	for _, item := range items {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			item.ID, item.Name, item.Kind, item.State, item.WorkspaceID)
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("flush table: %w", err)
	}
	if nextCursor != "" {
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "\nnext cursor: %s\n", nextCursor)
	}
	return err
}

// boardGetOutput is the JSON shape for 'mcli board get'.
type boardGetOutput struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Kind        string          `json:"kind"`
	State       string          `json:"state"`
	Description string          `json:"description"`
	WorkspaceID string          `json:"workspace_id"`
	Workspace   *boardWorkspace `json:"workspace,omitempty"`
	Owners      []boardOwner    `json:"owners"`
	Groups      []boardGroup    `json:"groups"`
	Columns     []boardColumn   `json:"columns"`
}

type boardWorkspace struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type boardOwner struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type boardGroup struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Color    string `json:"color"`
	Position string `json:"position"`
}

type boardColumn struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Type        string `json:"type"`
	SettingsStr string `json:"settings_str"`
	Width       int    `json:"width"`
	Archived    bool   `json:"archived"`
}

func newBoardGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get a board by ID",
		Long:  "Fetch full details for a monday.com board by its numeric ID.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBoardGet(cmd, args[0])
		},
	}
}

// boardCreateOutput is the JSON shape for 'mcli board create'.
// It mirrors the minimal slice returned by 'mcli board get'.
type boardCreateOutput struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	State       string `json:"state"`
	WorkspaceID string `json:"workspace_id"`
	Description string `json:"description"`
}

// validBoardKinds maps the lowercase CLI-accepted kind strings to their
// gen.BoardKind constants. Only the three canonical values are accepted.
var validBoardKinds = map[string]gen.BoardKind{
	"public":  gen.BoardKindPublic,
	"private": gen.BoardKindPrivate,
	"share":   gen.BoardKindShare,
}

func newBoardCreateCmd() *cobra.Command {
	var (
		name        string
		workspaceID string
		kind        string
		description string
		empty       bool
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new board",
		Long:  "Create a new monday.com board with the given name and options.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runBoardCreate(cmd, name, workspaceID, kind, description, empty)
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "board name (required)")
	cmd.Flags().StringVar(&workspaceID, "workspace", "", "workspace ID (optional)")
	cmd.Flags().StringVar(&kind, "kind", "public", "board kind: public|private|share")
	cmd.Flags().StringVar(&description, "description", "", "board description (optional)")
	cmd.Flags().BoolVar(&empty, "empty", false, "create board without default items")
	_ = cmd.MarkFlagRequired("name")

	return cmd
}

func runBoardCreate(cmd *cobra.Command, name, workspaceID, kind, description string, empty bool) error {
	if name == "" {
		return errs.Usage("--name is required")
	}

	boardKind, ok := validBoardKinds[kind]
	if !ok {
		return errs.Usage("invalid --kind %q: must be one of public, private, share", kind)
	}

	gql, err := newBoardClient()
	if err != nil {
		return err
	}

	resp, err := gen.BoardCreate(context.Background(), gql, name, boardKind, workspaceID, description, empty)
	if err != nil {
		return err
	}

	b := resp.Create_board
	if b.Id == "" {
		return errs.API("create_board returned no board")
	}

	out := boardCreateOutput{
		ID:          b.Id,
		Name:        b.Name,
		Kind:        string(b.Board_kind),
		State:       string(b.State),
		WorkspaceID: b.Workspace_id,
		Description: b.Description,
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

	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Created board %s: %s\n", out.ID, out.Name)
	return err
}

func runBoardGet(cmd *cobra.Command, id string) error {
	// Monday IDs are numeric strings; validate before hitting the API.
	if _, err := strconv.ParseUint(id, 10, 64); err != nil {
		return errs.Usage("board id must be a numeric string, got %q", id)
	}

	gql, err := newBoardClient()
	if err != nil {
		return err
	}

	resp, err := gen.BoardGet(context.Background(), gql, id)
	if err != nil {
		return err
	}

	if len(resp.Boards) == 0 {
		return errs.NotFound("board %s", id)
	}

	b := resp.Boards[0]

	// Build output struct.
	out := boardGetOutput{
		ID:          b.Id,
		Name:        b.Name,
		Kind:        string(b.Board_kind),
		State:       string(b.State),
		Description: b.Description,
		WorkspaceID: b.Workspace_id,
		Owners:      make([]boardOwner, len(b.Owners)),
		Groups:      make([]boardGroup, len(b.Groups)),
		Columns:     make([]boardColumn, len(b.Columns)),
	}

	if b.Workspace.Id != "" {
		out.Workspace = &boardWorkspace{
			ID:   b.Workspace.Id,
			Name: b.Workspace.Name,
			Kind: string(b.Workspace.Kind),
		}
	}

	for i, o := range b.Owners {
		out.Owners[i] = boardOwner{ID: o.Id, Name: o.Name}
	}
	for i, g := range b.Groups {
		out.Groups[i] = boardGroup{
			ID:       g.Id,
			Title:    g.Title,
			Color:    g.Color,
			Position: g.Position,
		}
	}
	for i, c := range b.Columns {
		out.Columns[i] = boardColumn{
			ID:          c.Id,
			Title:       c.Title,
			Type:        string(c.Type),
			SettingsStr: c.Settings_str,
			Width:       c.Width,
			Archived:    c.Archived,
		}
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
	_, _ = fmt.Fprintf(o, "ID:          %s\n", out.ID)
	_, _ = fmt.Fprintf(o, "Name:        %s\n", out.Name)
	_, _ = fmt.Fprintf(o, "Kind:        %s\n", out.Kind)
	_, _ = fmt.Fprintf(o, "State:       %s\n", out.State)
	if out.Description != "" {
		_, _ = fmt.Fprintf(o, "Description: %s\n", out.Description)
	}
	_, _ = fmt.Fprintf(o, "WorkspaceID: %s\n", out.WorkspaceID)
	if out.Workspace != nil {
		_, _ = fmt.Fprintf(o, "Workspace:   %s (%s, %s)\n",
			out.Workspace.Name, out.Workspace.ID, out.Workspace.Kind)
	}
	_, _ = fmt.Fprintf(o, "Owners:      %d\n", len(out.Owners))
	for _, owner := range out.Owners {
		_, _ = fmt.Fprintf(o, "  - %s (%s)\n", owner.Name, owner.ID)
	}

	if len(out.Groups) > 0 {
		_, _ = fmt.Fprintf(o, "\nGroups (%d):\n", len(out.Groups))
		tw := tabwriter.NewWriter(o, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "  ID\tTITLE\tCOLOR\tPOSITION")
		for _, g := range out.Groups {
			_, _ = fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n", g.ID, g.Title, g.Color, g.Position)
		}
		_ = tw.Flush()
	}

	if len(out.Columns) > 0 {
		_, _ = fmt.Fprintf(o, "\nColumns (%d):\n", len(out.Columns))
		tw := tabwriter.NewWriter(o, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "  ID\tTITLE\tTYPE\tARCHIVED")
		for _, c := range out.Columns {
			_, _ = fmt.Fprintf(tw, "  %s\t%s\t%s\t%v\n", c.ID, c.Title, c.Type, c.Archived)
		}
		_ = tw.Flush()
	}

	return nil
}
