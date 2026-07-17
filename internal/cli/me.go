package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mondaycom/mcli/internal/api/gen"
	apigraphql "github.com/mondaycom/mcli/internal/api/graphql"
	"github.com/mondaycom/mcli/internal/config"
	"github.com/mondaycom/mcli/internal/errs"
	"github.com/mondaycom/mcli/internal/secrets"
)

// newMeCmd returns the 'mcli me' command for printing current user info.
func newMeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "me",
		Short: "Print the authenticated user's details",
		Args:  cobra.NoArgs,
		RunE:  runMe,
	}
	return cmd
}

// meAccount is the account sub-object in me output.
type meAccount struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// meTeam is a team entry in me output.
type meTeam struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// meOutput is the JSON shape for 'mcli me'.
type meOutput struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Email   string    `json:"email"`
	Account meAccount `json:"account"`
	Teams   []meTeam  `json:"teams"`
}

func runMe(cmd *cobra.Command, _ []string) error {
	cfgPath := resolveConfigPath()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return errs.Internal("load config: %v", err)
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

	c := apigraphql.New(token, version)

	resp, err := gen.Me(cmd.Context(), c.GQL())
	if err != nil {
		return err
	}

	u := resp.Me
	teams := make([]meTeam, len(u.Teams))
	for i, t := range u.Teams {
		teams[i] = meTeam{ID: t.Id, Name: t.Name}
	}
	out := meOutput{
		ID:      u.Id,
		Name:    u.Name,
		Email:   u.Email,
		Account: meAccount{ID: u.Account.Id, Name: u.Account.Name},
		Teams:   teams,
	}

	mode, modeErr := resolveOutputMode(os.Stdout, globals, configOutputMode())
	if modeErr != nil {
		return modeErr
	}

	switch mode {
	case ModeJSON:
		data, mErr := json.Marshal(out)
		if mErr != nil {
			return errs.Internal("marshal output: %v", mErr)
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return err

	case ModeTerse:
		teamNames := make([]string, len(out.Teams))
		for i, t := range out.Teams {
			teamNames[i] = t.Name
		}
		ts := strings.Join(teamNames, ",")
		if ts == "" {
			ts = "-"
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(),
			"@%s %s <%s> account=%s teams=%s\n",
			out.ID, out.Name, out.Email, out.Account.Name, ts)
		return err

	default: // ModePretty and ModeCSV both use pretty here (CSV not meaningful for a single user)
		o := cmd.OutOrStdout()
		_, _ = fmt.Fprintf(o, "id:      %s\n", out.ID)
		_, _ = fmt.Fprintf(o, "name:    %s\n", out.Name)
		_, _ = fmt.Fprintf(o, "email:   %s\n", out.Email)
		_, _ = fmt.Fprintf(o, "account: %s (%s)\n", out.Account.Name, out.Account.ID)
		if len(out.Teams) > 0 {
			_, _ = fmt.Fprintf(o, "teams:\n")
			for _, t := range out.Teams {
				_, _ = fmt.Fprintf(o, "  - %s (%s)\n", t.Name, t.ID)
			}
		}
		return nil
	}
}
