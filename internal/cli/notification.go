package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mondaycom/mcli/internal/daemon"
	"github.com/mondaycom/mcli/internal/errs"
)

// newNotificationCmd returns the 'mcli notification' parent command.
func newNotificationCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "notification",
		Short: "Manage daemon event notifications (requires daemon)",
	}
	cmd.AddCommand(newNotificationListCmd())
	cmd.AddCommand(newNotificationCountCmd())
	cmd.AddCommand(newNotificationAckCmd())
	return cmd
}

// newNotificationListCmd returns the 'mcli notification list' command.
func newNotificationListCmd() *cobra.Command {
	var (
		unread    bool
		boardID   string
		eventType string
		since     string
		limit     int
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List events from the daemon inbox",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := requireDaemon()
			if err != nil {
				return err
			}

			filter := daemon.EventFilter{
				Unread:    unread,
				BoardID:   boardID,
				EventType: eventType,
				Since:     since,
				Limit:     limit,
			}

			result, err := c.ListNotifications(filter)
			if err != nil {
				return fmt.Errorf("list notifications: %w", err)
			}

			data, err := json.Marshal(result)
			if err != nil {
				return fmt.Errorf("marshal response: %w", err)
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return err
		},
	}

	cmd.Flags().BoolVar(&unread, "unread", false, "only show unread events")
	cmd.Flags().StringVar(&boardID, "board", "", "filter by board ID")
	cmd.Flags().StringVar(&eventType, "event", "", "filter by event type")
	cmd.Flags().StringVar(&since, "since", "", "only show events received at or after this RFC3339 timestamp")
	cmd.Flags().IntVar(&limit, "limit", 0, "maximum number of events to return (default 100)")
	return cmd
}

// newNotificationCountCmd returns the 'mcli notification count' command.
func newNotificationCountCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "count",
		Short: "Show the number of unread notifications",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := requireDaemon()
			if err != nil {
				return err
			}

			n, err := c.NotificationCount()
			if err != nil {
				return fmt.Errorf("notification count: %w", err)
			}

			data, err := json.Marshal(map[string]int{"unread": n})
			if err != nil {
				return fmt.Errorf("marshal response: %w", err)
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return err
		},
	}
}

// newNotificationAckCmd returns the 'mcli notification ack' command.
func newNotificationAckCmd() *cobra.Command {
	var all bool

	cmd := &cobra.Command{
		Use:   "ack [<id>]",
		Short: "Mark one or all notifications as read",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && !all {
				_ = cmd.Usage()
				return errs.Usage("provide an event <id> or pass --all")
			}

			c, err := requireDaemon()
			if err != nil {
				return err
			}

			if all {
				if err := c.AckAllNotifications(); err != nil {
					return fmt.Errorf("ack all notifications: %w", err)
				}
			} else {
				if err := c.AckNotification(daemon.EventID(args[0])); err != nil {
					return fmt.Errorf("ack notification: %w", err)
				}
			}

			_, err = fmt.Fprintln(cmd.OutOrStdout(), `{"acknowledged":true}`)
			return err
		},
	}

	cmd.Flags().BoolVar(&all, "all", false, "acknowledge all unread notifications")
	return cmd
}
