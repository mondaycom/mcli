package cli

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/mondaycom/mcli/internal/api/gen"
	"github.com/mondaycom/mcli/internal/errs"

	gqlclient "github.com/Khan/genqlient/graphql"
)

const (
	searchDefaultLimit = 10
	searchMaxLimit     = 20
)

// searchClientFactory is a test seam for injecting a fake graphql.Client.
var searchClientFactory func() (gqlclient.Client, error)

// newSearchClient resolves a graphql.Client for search commands.
func newSearchClient() (gqlclient.Client, error) {
	if searchClientFactory != nil {
		return searchClientFactory()
	}
	return newGQLClient()
}

// --- output types ---

// searchBoardItem is the JSON shape for a single board search result.
type searchBoardItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	URL         string `json:"url"`
}

// searchItemItem is the JSON shape for a single item search result.
type searchItemItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	BoardID     string `json:"board_id,omitempty"`
	WorkspaceID string `json:"workspace_id,omitempty"`
}

// searchDocItem is the JSON shape for a single doc search result.
type searchDocItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	WorkspaceID string `json:"workspace_id,omitempty"`
}

// searchOutput is the JSON envelope for 'mcli search'.
type searchOutput struct {
	Boards []searchBoardItem `json:"boards"`
	Items  []searchItemItem  `json:"items"`
	Docs   []searchDocItem   `json:"docs"`
}

// newSearchCmd returns the 'mcli search' command.
func newSearchCmd() *cobra.Command {
	var (
		searchType string
		limit      int
	)

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search monday.com boards, items, and docs",
		Long: `Search monday.com for boards, items, and/or docs matching a query string.

Use -t to restrict the search to a single entity type.
Without -t, all three types are searched and results combined.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSearch(cmd, args[0], searchType, limit)
		},
	}

	cmd.Flags().StringVarP(&searchType, "type", "t", "", "entity type to search: boards|items|docs (default: all)")
	cmd.Flags().IntVar(&limit, "limit", searchDefaultLimit, "max results per entity type (max 20)")

	return cmd
}

func runSearch(cmd *cobra.Command, query, searchType string, limit int) error {
	if query == "" {
		return errs.Usage("query must not be empty")
	}

	// Validate type flag.
	switch searchType {
	case "", "boards", "items", "docs":
	default:
		return errs.Usage("invalid -t %q: must be one of boards, items, docs", searchType)
	}

	// Clamp limit.
	if limit < 1 {
		limit = searchDefaultLimit
	}
	if limit > searchMaxLimit {
		limit = searchMaxLimit
	}

	gql, err := newSearchClient()
	if err != nil {
		return err
	}

	ctx := context.Background()
	out := searchOutput{
		Boards: []searchBoardItem{},
		Items:  []searchItemItem{},
		Docs:   []searchDocItem{},
	}

	if searchType == "" || searchType == "boards" {
		resp, apiErr := gen.SearchBoards(ctx, gql, query, limit)
		if apiErr != nil {
			return apiErr
		}
		for _, r := range resp.Search.Boards.Results {
			d := r.Indexed_data
			out.Boards = append(out.Boards, searchBoardItem{
				ID:          d.Id,
				Name:        d.Name,
				Description: d.Description,
				WorkspaceID: d.Workspace_id,
				URL:         d.Url,
			})
		}
	}

	if searchType == "" || searchType == "items" {
		resp, apiErr := gen.SearchItems(ctx, gql, query, limit)
		if apiErr != nil {
			return apiErr
		}
		for _, r := range resp.Search.Items.Results {
			d := r.Indexed_data
			out.Items = append(out.Items, searchItemItem{
				ID:          d.Id,
				Name:        d.Name,
				URL:         d.Url,
				BoardID:     d.Board_id,
				WorkspaceID: d.Workspace_id,
			})
		}
	}

	if searchType == "" || searchType == "docs" {
		resp, apiErr := gen.SearchDocs(ctx, gql, query, limit)
		if apiErr != nil {
			return apiErr
		}
		for _, r := range resp.Search.Docs.Results {
			d := r.Indexed_data
			out.Docs = append(out.Docs, searchDocItem{
				ID:          d.Id,
				Name:        d.Name,
				WorkspaceID: d.Workspace_id,
			})
		}
	}

	mode, modeErr := resolveOutputMode(os.Stdout, globals, configOutputMode())
	if modeErr != nil {
		return modeErr
	}

	switch mode {
	case ModeJSON:
		data, mErr := json.Marshal(out)
		if mErr != nil {
			return fmt.Errorf("marshal output: %w", mErr)
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return err

	case ModeCSV:
		return writeSearchCSV(cmd, out)

	case ModeTerse:
		return writeSearchTerse(cmd, out)

	default: // ModePretty
		return writeSearchPretty(cmd, out)
	}
}

func writeSearchPretty(cmd *cobra.Command, out searchOutput) error {
	o := cmd.OutOrStdout()

	if len(out.Boards) > 0 {
		_, _ = fmt.Fprintf(o, "Boards (%d):\n", len(out.Boards))
		w := tabwriter.NewWriter(o, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(w, "  ID\tNAME\tWORKSPACE")
		for _, b := range out.Boards {
			_, _ = fmt.Fprintf(w, "  %s\t%s\t%s\n", b.ID, b.Name, b.WorkspaceID)
		}
		_ = w.Flush()
	}

	if len(out.Items) > 0 {
		_, _ = fmt.Fprintf(o, "Items (%d):\n", len(out.Items))
		w := tabwriter.NewWriter(o, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(w, "  ID\tNAME\tBOARD")
		for _, it := range out.Items {
			_, _ = fmt.Fprintf(w, "  %s\t%s\t%s\n", it.ID, it.Name, it.BoardID)
		}
		_ = w.Flush()
	}

	if len(out.Docs) > 0 {
		_, _ = fmt.Fprintf(o, "Docs (%d):\n", len(out.Docs))
		w := tabwriter.NewWriter(o, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(w, "  ID\tNAME\tWORKSPACE")
		for _, d := range out.Docs {
			_, _ = fmt.Fprintf(w, "  %s\t%s\t%s\n", d.ID, d.Name, d.WorkspaceID)
		}
		_ = w.Flush()
	}

	total := len(out.Boards) + len(out.Items) + len(out.Docs)
	if total == 0 {
		_, _ = fmt.Fprintln(o, "No results found.")
	}
	return nil
}

func writeSearchTerse(cmd *cobra.Command, out searchOutput) error {
	o := cmd.OutOrStdout()
	for _, b := range out.Boards {
		ctx := b.WorkspaceID
		if ctx == "" {
			ctx = "-"
		}
		_, _ = fmt.Fprintf(o, "board:%s:%s:ws=%s\n", b.ID, b.Name, ctx)
	}
	for _, it := range out.Items {
		ctx := it.BoardID
		if ctx == "" {
			ctx = "-"
		}
		_, _ = fmt.Fprintf(o, "item:%s:%s:board=%s\n", it.ID, it.Name, ctx)
	}
	for _, d := range out.Docs {
		ctx := d.WorkspaceID
		if ctx == "" {
			ctx = "-"
		}
		_, _ = fmt.Fprintf(o, "doc:%s:%s:ws=%s\n", d.ID, d.Name, ctx)
	}
	return nil
}

func writeSearchCSV(cmd *cobra.Command, out searchOutput) error {
	w := csv.NewWriter(cmd.OutOrStdout())
	_ = w.Write([]string{"type", "id", "name", "context_id"})
	for _, b := range out.Boards {
		_ = w.Write([]string{"board", b.ID, b.Name, b.WorkspaceID})
	}
	for _, it := range out.Items {
		_ = w.Write([]string{"item", it.ID, it.Name, it.BoardID})
	}
	for _, d := range out.Docs {
		_ = w.Write([]string{"doc", d.ID, d.Name, d.WorkspaceID})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return fmt.Errorf("write csv: %w", err)
	}
	return nil
}
