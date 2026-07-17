package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mondaycom/mcli/internal/api/gen"
	"github.com/mondaycom/mcli/internal/errs"
)

const itemDescMaxBytes = 1 << 20 // 1 MB

// newItemDescriptionCmd returns the 'mcli item description' subcommand.
func newItemDescriptionCmd() *cobra.Command {
	var (
		setMarkdown string
		setFile     string
	)

	cmd := &cobra.Command{
		Use:   "description <id>",
		Short: "Read or write an item's description doc",
		Long: `Read or write the description document of a monday.com item.

Without --set or --set-file the command reads and prints the item's
description as markdown.

With --set <markdown> or --set-file <path> the description is replaced
with the provided markdown content.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runItemDescription(cmd, args[0], setMarkdown, setFile)
		},
	}

	cmd.Flags().StringVar(&setMarkdown, "set", "", "replace description with this markdown string")
	cmd.Flags().StringVar(&setFile, "set-file", "", "replace description with markdown read from this file")
	cmd.MarkFlagsMutuallyExclusive("set", "set-file")

	return cmd
}

func runItemDescription(cmd *cobra.Command, itemID, setMarkdown, setFile string) error {
	// Write path.
	if setMarkdown != "" || setFile != "" {
		md, readErr := resolveMarkdownInput(cmd, setMarkdown, setFile)
		if readErr != nil {
			return readErr
		}

		gql, err := newItemClient()
		if err != nil {
			return err
		}

		resp, apiErr := gen.DocSetItemDescription(cmd.Context(), gql, itemID, md)
		if apiErr != nil {
			return apiErr
		}
		result := resp.Set_item_description_content
		if !result.Success {
			msg := result.Error
			if msg == "" {
				msg = "set_item_description_content returned success=false"
			}
			return errs.API("%s", msg)
		}

		_, err = fmt.Fprintf(cmd.OutOrStdout(), "Updated description for item %s\n", itemID)
		return err
	}

	// Read path: discover the item's description doc via object_id, then export markdown.
	gql, err := newItemClient()
	if err != nil {
		return err
	}

	docsResp, apiErr := gen.DocGetByObjectID(cmd.Context(), gql, itemID)
	if apiErr != nil {
		return apiErr
	}
	if len(docsResp.Docs) == 0 {
		return errs.NotFound("description doc for item %s", itemID)
	}

	docID := docsResp.Docs[0].Id

	exportResp, apiErr := gen.DocExportMarkdown(cmd.Context(), gql, docID)
	if apiErr != nil {
		return apiErr
	}
	result := exportResp.Export_markdown_from_doc
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

// resolveMarkdownInput returns the markdown string from either the inline flag
// or a file path. When setFile is "-", it reads from cmd's stdin.
func resolveMarkdownInput(cmd *cobra.Command, inline, filePath string) (string, error) {
	if inline != "" {
		return strings.TrimSpace(inline), nil
	}

	var r io.Reader
	if filePath == "-" {
		r = cmd.InOrStdin()
	} else {
		f, err := os.Open(filePath)
		if err != nil {
			return "", fmt.Errorf("open %s: %w", filePath, err)
		}
		defer func() { _ = f.Close() }()
		r = f
	}

	limited := io.LimitReader(r, itemDescMaxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return "", fmt.Errorf("read markdown: %w", err)
	}
	if int64(len(data)) > itemDescMaxBytes {
		return "", errs.Usage("markdown input exceeds %d bytes", itemDescMaxBytes)
	}
	return strings.TrimSpace(string(data)), nil
}
