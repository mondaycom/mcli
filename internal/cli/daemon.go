package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/mondaycom/mcli/internal/daemon"
	"github.com/mondaycom/mcli/internal/errs"
)

// newDaemonCmd returns the 'mcli daemon' parent command.
func newDaemonCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Manage the mcli background daemon",
	}
	cmd.AddCommand(newDaemonStartCmd())
	cmd.AddCommand(newDaemonStopCmd())
	cmd.AddCommand(newDaemonStatusCmd())
	return cmd
}

// newDaemonStartCmd returns the 'mcli daemon start' command.
func newDaemonStartCmd() *cobra.Command {
	var (
		port   int
		detach bool
		url    string
	)

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the mcli daemon in the foreground",
		Long:  "Start the mcli daemon. It listens for webhooks on --port and accepts IPC commands over a Unix socket.",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if detach {
				return errs.Usage("detach mode not yet supported")
			}

			cfg := daemon.Config{
				Port:        port,
				ConfigDir:   resolveConfigDir(),
				ExternalURL: url,
			}
			d := daemon.New(cfg)

			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			return d.Start(ctx)
		},
	}

	cmd.Flags().IntVar(&port, "port", daemon.DefaultPort, "TCP port for the webhook HTTP server")
	cmd.Flags().BoolVar(&detach, "detach", false, "run daemon in the background (not yet supported)")
	cmd.Flags().StringVar(&url, "url", "", "external URL to register webhooks (skips tunnel setup)")

	return cmd
}

// newDaemonStopCmd returns the 'mcli daemon stop' command.
func newDaemonStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the running mcli daemon",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := requireDaemon()
			if err != nil {
				return err
			}
			if err := c.Stop(); err != nil {
				return fmt.Errorf("stop daemon: %w", err)
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), `{"stopping":true}`)
			return err
		},
	}
}

// newDaemonStatusCmd returns the 'mcli daemon status' command.
func newDaemonStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the status of the mcli daemon",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := requireDaemon()
			if err != nil {
				// Daemon not running is not a fatal error for status — report it.
				out := map[string]any{"running": false, "error": err.Error()}
				data, mErr := json.Marshal(out)
				if mErr != nil {
					return fmt.Errorf("marshal status: %w", mErr)
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
				return nil
			}

			sr, err := c.Status()
			if err != nil {
				return fmt.Errorf("query daemon status: %w", err)
			}

			data, err := json.Marshal(sr)
			if err != nil {
				return fmt.Errorf("marshal status: %w", err)
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return err
		},
	}
}

// newNotificationCmd is a stub that requires the daemon to be running.
func newNotificationCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "notification",
		Short: "Manage notifications (requires daemon)",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if _, err := requireDaemon(); err != nil {
				return err
			}
			return errs.Usage("notification subcommands not yet implemented")
		},
	}
}

// newWebhookCmd is a stub that requires the daemon to be running.
func newWebhookCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "webhook",
		Short: "Manage webhooks (requires daemon)",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if _, err := requireDaemon(); err != nil {
				return err
			}
			return errs.Usage("webhook subcommands not yet implemented")
		},
	}
}

// printDaemonError writes a structured error to stderr in a format consistent
// with ADR-002. Kept here to avoid scattering os.Stderr calls.
func printDaemonError(err error) {
	_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
}
