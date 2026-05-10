package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/mondaycom/mcli/internal/api/gen"
	apigraphql "github.com/mondaycom/mcli/internal/api/graphql"
	"github.com/mondaycom/mcli/internal/config"
	"github.com/mondaycom/mcli/internal/secrets"
)

// newMeCmd returns the hidden 'mcli me' command used for smoke-testing the
// GraphQL client end-to-end.
func newMeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "me",
		Short: "Print the authenticated user's id and name",
		Args:  cobra.NoArgs,
		RunE:  runMe,
	}
	return cmd
}

func runMe(cmd *cobra.Command, _ []string) error {
	cfgPath := resolveConfigPath()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	var store config.Store
	if cfg.SecretStore != "" {
		st, openErr := secrets.Open(cfg.SecretStore, resolveConfigDir())
		if openErr != nil {
			return openErr
		}
		store = st
	}

	token, err := config.ResolveToken(cfg, globals.Token, store)
	if err != nil {
		return err
	}

	// Build version from the injected ldflags value exposed via version.go.
	c := apigraphql.New(token, version)

	resp, err := gen.Me(context.Background(), c.GQL())
	if err != nil {
		return err
	}

	mode, modeErr := resolveOutputMode(os.Stdout, globals)
	if modeErr != nil {
		return modeErr
	}

	if mode == ModeJSON {
		payload := map[string]any{
			"id":   resp.Me.Id,
			"name": resp.Me.Name,
		}
		data, mErr := json.Marshal(payload)
		if mErr != nil {
			return fmt.Errorf("marshal output: %w", mErr)
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return err
	}

	_, err = fmt.Fprintf(cmd.OutOrStdout(), "id:   %s\nname: %s\n", resp.Me.Id, resp.Me.Name)
	return err
}
