package cli

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

// webhookEventTypes lists all supported monday.com WebhookEventType values,
// sorted alphabetically.
var webhookEventTypes = []string{
	"change_column_value",
	"change_name",
	"change_specific_column_value",
	"change_status_column_value",
	"change_subitem_column_value",
	"change_subitem_name",
	"create_column",
	"create_item",
	"create_subitem",
	"create_subitem_update",
	"create_update",
	"delete_update",
	"edit_update",
	"item_archived",
	"item_deleted",
	"item_moved_to_any_group",
	"item_moved_to_specific_group",
	"item_restored",
	"move_subitem",
	"subitem_archived",
	"subitem_deleted",
}

// newWebhookCmd returns the 'mcli webhook' parent command.
func newWebhookCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "webhook",
		Short: "Manage monday.com webhooks (requires daemon)",
	}
	cmd.AddCommand(newWebhookCreateCmd())
	cmd.AddCommand(newWebhookListCmd())
	cmd.AddCommand(newWebhookDeleteCmd())
	cmd.AddCommand(newWebhookEventsCmd())
	return cmd
}

// newWebhookCreateCmd returns the 'mcli webhook create' command.
func newWebhookCreateCmd() *cobra.Command {
	var (
		boardID string
		event   string
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Register a new webhook with monday.com via the daemon",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if boardID == "" {
				return fmt.Errorf("--board is required")
			}
			if event == "" {
				return fmt.Errorf("--event is required")
			}
			if !validWebhookEvent(event) {
				return fmt.Errorf("unknown event type %q; run 'mcli webhook events' to list valid types", event)
			}

			c, err := requireDaemon()
			if err != nil {
				return err
			}

			rec, err := c.RegisterWebhook(boardID, event)
			if err != nil {
				return fmt.Errorf("register webhook: %w", err)
			}

			data, err := json.Marshal(rec)
			if err != nil {
				return fmt.Errorf("marshal response: %w", err)
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return err
		},
	}

	cmd.Flags().StringVar(&boardID, "board", "", "board ID to register the webhook for (required)")
	cmd.Flags().StringVar(&event, "event", "", "webhook event type (required; see 'mcli webhook events')")
	return cmd
}

// newWebhookListCmd returns the 'mcli webhook list' command.
func newWebhookListCmd() *cobra.Command {
	var boardID string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List registered webhooks",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := requireDaemon()
			if err != nil {
				return err
			}

			records, err := c.ListWebhooks(boardID)
			if err != nil {
				return fmt.Errorf("list webhooks: %w", err)
			}

			data, err := json.Marshal(records)
			if err != nil {
				return fmt.Errorf("marshal response: %w", err)
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return err
		},
	}

	cmd.Flags().StringVar(&boardID, "board", "", "filter by board ID")
	return cmd
}

// newWebhookDeleteCmd returns the 'mcli webhook delete' command.
func newWebhookDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a registered webhook",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]

			c, err := requireDaemon()
			if err != nil {
				return err
			}

			if err := c.DeleteWebhook(id); err != nil {
				return fmt.Errorf("delete webhook: %w", err)
			}

			_, err = fmt.Fprintln(cmd.OutOrStdout(), `{"deleted":true}`)
			return err
		},
	}
}

// newWebhookEventsCmd returns the 'mcli webhook events' command.
// It prints all valid WebhookEventType values (no daemon required).
func newWebhookEventsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "events",
		Short: "List available webhook event types",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Return a sorted copy to guarantee order.
			types := make([]string, len(webhookEventTypes))
			copy(types, webhookEventTypes)
			sort.Strings(types)

			data, err := json.Marshal(types)
			if err != nil {
				return fmt.Errorf("marshal event types: %w", err)
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return err
		},
	}
}

// validWebhookEvent reports whether event is a known WebhookEventType.
func validWebhookEvent(event string) bool {
	for _, e := range webhookEventTypes {
		if e == event {
			return true
		}
	}
	return false
}
