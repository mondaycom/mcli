package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/mondaycom/mcli/internal/api/gen"
	"github.com/mondaycom/mcli/internal/errs"

	gqlclient "github.com/Khan/genqlient/graphql"
)

// docClientFactory is a test seam for injecting a fake graphql.Client.
var docClientFactory func() (gqlclient.Client, error)

// newDocClient resolves a graphql.Client for doc commands.
func newDocClient() (gqlclient.Client, error) {
	if docClientFactory != nil {
		return docClientFactory()
	}
	return newGQLClient()
}

// newDocCmd returns the 'mcli doc' parent command.
func newDocCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doc",
		Short: "Create, read and write monday.com documents",
	}
	cmd.AddCommand(newDocCreateCmd())
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

// --- doc create ---

// docCreateOutput is the JSON shape for 'mcli doc create'.
type docCreateOutput struct {
	ID       string `json:"id"`
	ObjectID string `json:"object_id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	URL      string `json:"url,omitempty"`
}

func newDocCreateCmd() *cobra.Command {
	var (
		workspace string
		name      string
		kind      string
		folderID  string
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new document in a workspace",
		Long: `Create a new monday.com document in the specified workspace.

The document is created empty. Use 'mcli doc write <id>' to add content.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDocCreate(cmd, workspace, name, kind, folderID)
		},
	}

	cmd.Flags().StringVar(&workspace, "workspace", "", "workspace ID (required)")
	cmd.Flags().StringVar(&name, "name", "", "document name (required)")
	cmd.Flags().StringVar(&kind, "kind", "public", "document kind: public, private, or share")
	cmd.Flags().StringVar(&folderID, "folder", "", "folder ID (optional)")

	_ = cmd.MarkFlagRequired("workspace")
	_ = cmd.MarkFlagRequired("name")

	return cmd
}

func runDocCreate(cmd *cobra.Command, workspace, name, kind, folderID string) error {
	var bk gen.BoardKind
	switch kind {
	case "public":
		bk = gen.BoardKindPublic
	case "private":
		bk = gen.BoardKindPrivate
	case "share":
		bk = gen.BoardKindShare
	default:
		return errs.Usage("invalid --kind %q: must be public, private, or share", kind)
	}

	gql, err := newDocClient()
	if err != nil {
		return err
	}

	resp, apiErr := gen.DocCreateInWorkspace(context.Background(), gql, workspace, name, bk, folderID)
	if apiErr != nil {
		return apiErr
	}

	doc := resp.Create_doc
	out := docCreateOutput{
		ID:       doc.Id,
		ObjectID: doc.Object_id,
		Name:     doc.Name,
		Kind:     string(doc.Doc_kind),
		URL:      doc.Relative_url,
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

	case ModeTerse:
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "doc:%s:%s:kind=%s\n", out.ID, out.Name, out.Kind)
		return err

	default:
		o := cmd.OutOrStdout()
		_, _ = fmt.Fprintf(o, "id:        %s\n", out.ID)
		_, _ = fmt.Fprintf(o, "object_id: %s\n", out.ObjectID)
		_, _ = fmt.Fprintf(o, "name:      %s\n", out.Name)
		_, _ = fmt.Fprintf(o, "kind:      %s\n", out.Kind)
		if out.URL != "" {
			_, _ = fmt.Fprintf(o, "url:       %s\n", out.URL)
		}
		return nil
	}
}
