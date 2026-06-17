package cli

import (
	"fmt"

	apigraphql "github.com/mondaycom/mcli/internal/api/graphql"
	"github.com/mondaycom/mcli/internal/config"
	"github.com/mondaycom/mcli/internal/secrets"

	gqlclient "github.com/Khan/genqlient/graphql"
)

// newGQLClient builds a real authenticated GQL client from config + secrets.
// It is the shared implementation used by all per-domain newXClient helpers.
func newGQLClient() (gqlclient.Client, error) {
	cfgPath := resolveConfigPath()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	var store config.Store
	if cfg.SecretStore != "" {
		st, openErr := secrets.Open(cfg.SecretStore, resolveConfigDir())
		if openErr != nil {
			return nil, openErr
		}
		store = st
	}

	token, err := config.ResolveToken(cfg, globals.Token, store)
	if err != nil {
		return nil, err
	}

	c := apigraphql.New(token, version)
	return c.GQL(), nil
}
