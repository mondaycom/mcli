package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mondaycom/mcli/internal/api/gen"
	apigraphql "github.com/mondaycom/mcli/internal/api/graphql"
	"github.com/mondaycom/mcli/internal/config"
	"github.com/mondaycom/mcli/internal/errs"
	"github.com/mondaycom/mcli/internal/secrets"

	gqlclient "github.com/Khan/genqlient/graphql"
)

// docClientFactory is a test seam for injecting a fake graphql.Client.
var docClientFactory func() (gqlclient.Client, error)

// newDocClient resolves a graphql.Client for doc commands.
func newDocClient() (gqlclient.Client, error) {
	if docClientFactory != nil {
		return docClientFactory()
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

// newDocCmd returns the 'mcli doc' parent command.
func newDocCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doc",
		Short: "Read and write monday.com documents",
	}
	cmd.AddCommand(newDocReadCmd())
	cmd.AddCommand(newDocWriteCmd())
	return cmd
}

// --- doc read ---

func newDocReadCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "read <id>",
		Short: "Export a document as markdown",
		Long:  "Export the full content of a monday.com document as markdown text.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDocRead(cmd, args[0])
		},
	}
}

func runDocRead(cmd *cobra.Command, docID string) error {
	gql, err := newDocClient()
	if err != nil {
		return err
	}

	resp, apiErr := gen.DocExportMarkdown(context.Background(), gql, docID)
	if apiErr != nil {
		return apiErr
	}

	result := resp.Export_markdown_from_doc
	if !result.Success {
		msg := result.Error
		if msg == "" {
			msg = "export_markdown_from_doc returned success=false"
		}
		return errs.API("%s", msg)
	}

	_, err = fmt.Fprint(cmd.OutOrStdout(), result.Markdown)
	return err
}

// --- doc write ---

func newDocWriteCmd() *cobra.Command {
	var (
		content  string
		filePath string
	)

	cmd := &cobra.Command{
		Use:   "write <id>",
		Short: "Replace a document's content with markdown",
		Long: `Replace all content in a monday.com document with markdown.

Provide --content <markdown> or --file <path> (use - for stdin).
The existing blocks are deleted and new ones are created from the markdown.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDocWrite(cmd, args[0], content, filePath)
		},
	}

	cmd.Flags().StringVar(&content, "content", "", "markdown content to write")
	cmd.Flags().StringVar(&filePath, "file", "", "path to markdown file (use - for stdin)")
	cmd.MarkFlagsMutuallyExclusive("content", "file")

	return cmd
}

func runDocWrite(cmd *cobra.Command, docID, content, filePath string) error {
	if content == "" && filePath == "" {
		return errs.Usage("one of --content or --file is required")
	}

	md, err := resolveMarkdownInput(cmd, content, filePath)
	if err != nil {
		return err
	}
	if md == "" {
		return errs.Usage("markdown content must not be empty")
	}

	gql, err := newDocClient()
	if err != nil {
		return err
	}

	ctx := context.Background()

	// Step 1: collect all existing block IDs.
	blockResp, apiErr := gen.DocGetBlockIDs(ctx, gql, docID)
	if apiErr != nil {
		return apiErr
	}

	if len(blockResp.Docs) == 0 {
		return errs.NotFound("doc %s", docID)
	}

	blockIDs := make([]string, 0, len(blockResp.Docs[0].Blocks))
	for _, b := range blockResp.Docs[0].Blocks {
		blockIDs = append(blockIDs, b.Id)
	}

	// Step 2: delete all existing blocks (if any).
	if len(blockIDs) > 0 {
		if _, delErr := gen.DocDeleteBlocks(ctx, gql, blockIDs); delErr != nil {
			return delErr
		}
	}

	// Step 3: add new content from markdown.
	addResp, apiErr := gen.DocAddMarkdown(ctx, gql, docID, md)
	if apiErr != nil {
		return apiErr
	}
	result := addResp.Add_content_to_doc_from_markdown
	if !result.Success {
		msg := result.Error
		if msg == "" {
			msg = "add_content_to_doc_from_markdown returned success=false"
		}
		return errs.API("%s", msg)
	}

	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Updated doc %s (%d block(s) created)\n",
		docID, len(result.Block_ids))
	return err
}
