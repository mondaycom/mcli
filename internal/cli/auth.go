package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mondaycom/mcli/internal/config"
	"github.com/mondaycom/mcli/internal/errs"
)

// newAuthCmd returns the 'mcli auth' parent command.
func newAuthCmd() *cobra.Command {
	authCmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage mcli authentication",
	}
	authCmd.AddCommand(newAuthLoginCmd())
	return authCmd
}

// newAuthLoginCmd returns the 'mcli auth login' subcommand.
func newAuthLoginCmd() *cobra.Command {
	var tokenFlag string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Save an API token to the config file",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if tokenFlag == "" {
				return errs.Usage("--token is required for 'auth login'")
			}

			cfgPath := globals.Config
			if cfgPath == "" {
				cfgPath = config.DefaultPath()
			}

			cfg := config.Config{Token: config.APIToken(tokenFlag)}
			if err := config.Save(cfgPath, cfg); err != nil {
				return fmt.Errorf("save config: %w", err)
			}

			_, err := fmt.Fprintf(cmd.OutOrStdout(), "Token saved to %s\n", cfgPath)
			return err
		},
	}

	cmd.Flags().StringVar(&tokenFlag, "token", "", "monday.com API token to store")
	return cmd
}
