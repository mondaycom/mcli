package cli

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	htmltomd "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/spf13/cobra"

	"github.com/mondaycom/mcli/internal/api/gen"
	"github.com/mondaycom/mcli/internal/errs"
)

// --- output types ---

// updateCreator is the user shape in update output.
type updateCreator struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// updateReply is the reply sub-shape in update output.
type updateReply struct {
	ID        string        `json:"id"`
	Body      string        `json:"body"`
	TextBody  string        `json:"text_body,omitempty"`
	CreatedAt string        `json:"created_at,omitempty"`
	Creator   updateCreator `json:"creator"`
}

// updateItem is the per-update shape in 'mcli item get-updates'.
type updateItem struct {
	ID        string        `json:"id"`
	Body      string        `json:"body"`
	TextBody  string        `json:"text_body,omitempty"`
	CreatedAt string        `json:"created_at,omitempty"`
	Creator   updateCreator `json:"creator"`
	Replies   []updateReply `json:"replies"`
}

// newItemGetUpdatesCmd returns the 'mcli item get-updates' subcommand.
func newItemGetUpdatesCmd() *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:   "get-updates <id>",
		Short: "Fetch updates (comments) on an item",
		Long: `Fetch the updates feed (comments and replies) for a monday.com item.

Use --limit to control the number of updates returned (default 25).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runItemGetUpdates(cmd, args[0], limit)
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 25, "max updates to return")
	return cmd
}

func runItemGetUpdates(cmd *cobra.Command, id string, limit int) error {
	if limit < 1 {
		limit = 25
	}

	gql, err := newItemClient()
	if err != nil {
		return err
	}

	resp, apiErr := gen.ItemGetUpdates(context.Background(), gql, id, limit)
	if apiErr != nil {
		return apiErr
	}

	if len(resp.Items) == 0 {
		return errs.NotFound("item %s", id)
	}

	rawUpdates := resp.Items[0].Updates
	updates := make([]updateItem, len(rawUpdates))
	for i, u := range rawUpdates {
		replies := make([]updateReply, len(u.Replies))
		for j, r := range u.Replies {
			replies[j] = updateReply{
				ID:        r.Id,
				Body:      r.Body,
				TextBody:  r.Text_body,
				CreatedAt: r.Created_at,
				Creator:   updateCreator{ID: r.Creator.Id, Name: r.Creator.Name, Email: r.Creator.Email},
			}
		}
		updates[i] = updateItem{
			ID:        u.Id,
			Body:      u.Body,
			TextBody:  u.Text_body,
			CreatedAt: u.Created_at,
			Creator:   updateCreator{ID: u.Creator.Id, Name: u.Creator.Name, Email: u.Creator.Email},
			Replies:   replies,
		}
	}

	mode, modeErr := resolveOutputMode(os.Stdout, globals, configOutputMode())
	if modeErr != nil {
		return modeErr
	}

	switch mode {
	case ModeJSON:
		data, mErr := json.Marshal(updates)
		if mErr != nil {
			return errs.Internal("marshal output: %v", mErr)
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return err

	case ModeCSV:
		return writeGetUpdatesCSV(cmd, updates)

	case ModeTerse:
		return writeGetUpdatesTerse(cmd, updates)

	default: // ModePretty
		return writeGetUpdatesPretty(cmd, updates)
	}
}

func writeGetUpdatesPretty(cmd *cobra.Command, updates []updateItem) error {
	o := cmd.OutOrStdout()
	for _, u := range updates {
		ts := u.CreatedAt
		_, _ = fmt.Fprintf(o, "@%s %s\n", u.Creator.Name, ts)
		body := strings.TrimSpace(u.TextBody)
		if body == "" {
			body = htmlToMarkdown(u.Body)
		}
		_, _ = fmt.Fprintf(o, "  %s\n", indentLines(body, "  "))
		for _, r := range u.Replies {
			rts := r.CreatedAt
			_, _ = fmt.Fprintf(o, "    @%s %s (reply)\n", r.Creator.Name, rts)
			rb := strings.TrimSpace(r.TextBody)
			if rb == "" {
				rb = htmlToMarkdown(r.Body)
			}
			_, _ = fmt.Fprintf(o, "    %s\n", indentLines(rb, "    "))
		}
		_, _ = fmt.Fprintln(o)
	}
	return nil
}

func writeGetUpdatesTerse(cmd *cobra.Command, updates []updateItem) error {
	o := cmd.OutOrStdout()
	for _, u := range updates {
		fl := firstLine(u.TextBody)
		if fl == "" {
			fl = firstLine(htmlToMarkdown(u.Body))
		}
		_, _ = fmt.Fprintf(o, "%s|%s|%s|%s\n", u.ID, u.Creator.Name, u.CreatedAt, fl)
	}
	return nil
}

func writeGetUpdatesCSV(cmd *cobra.Command, updates []updateItem) error {
	w := csv.NewWriter(cmd.OutOrStdout())
	_ = w.Write([]string{"id", "creator_name", "created_at", "text_body"})
	for _, u := range updates {
		body := strings.TrimSpace(u.TextBody)
		_ = w.Write([]string{u.ID, u.Creator.Name, u.CreatedAt, body})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return errs.Internal("write csv: %v", err)
	}
	return nil
}

// firstLine returns the first non-empty line of s, trimmed.
func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		s = s[:idx]
	}
	return strings.TrimSpace(s)
}

// indentLines adds prefix to every non-empty line in s.
func indentLines(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if strings.TrimSpace(l) != "" {
			lines[i] = prefix + l
		}
	}
	return strings.Join(lines, "\n")
}

// htmlToMarkdown converts an HTML body to markdown for LLM-friendly output.
// Falls back to the raw string on conversion errors.
func htmlToMarkdown(s string) string {
	md, err := htmltomd.ConvertString(s)
	if err != nil {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(md)
}
