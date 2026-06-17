package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/mondaycom/mcli/internal/config"
	"github.com/mondaycom/mcli/internal/daemon"
	"github.com/mondaycom/mcli/internal/errs"
	"github.com/mondaycom/mcli/internal/secrets"
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

			// Resolve API token so the daemon can make monday.com API calls.
			token, err := resolveToken()
			if err != nil {
				// Token resolution failure is non-fatal for daemon start; the
				// daemon will return errors when webhook operations are attempted.
				token = ""
			}

			cfg := daemon.Config{
				Port:        port,
				ConfigDir:   resolveConfigDir(),
				ExternalURL: url,
				Token:       token,
			}
			d := daemon.New(cfg)

			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			fmt.Fprintf(os.Stderr, "daemon listening on port %d\n", port)

			// Listen for tunnel URL changes in the background so we can log
			// them once the daemon has started.
			go func() {
				for u := range d.TunnelURLChanges() {
					fmt.Fprintf(os.Stderr, "tunnel URL: %s\n", u)
				}
			}()

			return d.Start(ctx)
		},
	}

	cmd.Flags().IntVar(&port, "port", daemon.DefaultPort, "TCP port for the webhook HTTP server")
	cmd.Flags().BoolVar(&detach, "detach", false, "run daemon in the background (not yet supported)")
	cmd.Flags().StringVar(&url, "url", "", "external URL to register webhooks (skips tunnel setup)")

	return cmd
}

// resolveToken loads the monday.com API token from config/secrets using the
// same mechanism as newQueryHTTPClient.
func resolveToken() (string, error) {
	cfgPath := resolveConfigPath()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return "", errs.Internal("load config: %v", err)
	}

	var store config.Store
	if cfg.SecretStore != "" {
		st, openErr := secrets.Open(cfg.SecretStore, resolveConfigDir())
		if openErr != nil {
			return "", openErr
		}
		store = st
	}

	token, err := config.ResolveToken(cfg, globals.Token, store)
	if err != nil {
		return "", err
	}
	return string(token), nil
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
				return errs.API("stop daemon: %v", err)
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
					return errs.Internal("marshal status: %v", mErr)
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
				return nil
			}

			sr, err := c.Status()
			if err != nil {
				return errs.API("query daemon status: %v", err)
			}

			data, err := json.Marshal(sr)
			if err != nil {
				return errs.Internal("marshal status: %v", err)
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return err
		},
	}
}
