package cli

import (
	"fmt"
	"os"
	"regexp"

	"github.com/spf13/cobra"

	apischema "github.com/mondaycom/mcli/internal/api/schema"
	"github.com/mondaycom/mcli/internal/config"
	"github.com/mondaycom/mcli/internal/errs"
)

// apiVersionRE accepts YYYY-MM calendar versions (e.g. 2026-07) or named
// versions used on non-production environments (e.g. dev, staging, latest).
var apiVersionRE = regexp.MustCompile(`^(\d{4}-\d{2}|[a-z][a-z0-9-]*)$`)

var validOutputModes = map[string]bool{
	"":        true,
	"default": true,
	"json":    true,
	"pretty":  true,
	"terse":   true,
	"csv":     true,
}

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Read and write mcli configuration",
	}
	cmd.AddCommand(newConfigSetCmd())
	cmd.AddCommand(newConfigGetCmd())
	return cmd
}

func newConfigSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a configuration value",
		Long: `Set a configuration value and persist it to config.yaml.

Supported keys:
  output-mode   Default output mode: default, json, pretty, terse, or csv
  api-url       monday.com API endpoint URL (e.g. https://api.mondaystaging.com/v2); empty to reset
  api-version   monday.com API version: YYYY-MM (e.g. 2026-07), named (e.g. dev), or "default" to reset
  routing-key   Routing key for local-api-proxy debugging (baggage: routingKey=<value>); empty to clear`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, value := args[0], args[1]
			return runConfigSet(cmd, key, value)
		},
	}
}

func runConfigSet(cmd *cobra.Command, key, value string) error {
	switch key {
	case "output-mode":
		if !validOutputModes[value] {
			return errs.Usage("invalid output-mode %q: must be default, json, pretty, terse, or csv", value)
		}
		if value == "default" {
			value = ""
		}
	case "api-url":
		// any URL string is valid; empty string resets to default
	case "api-version":
		if value != "default" && !apiVersionRE.MatchString(value) {
			return errs.Usage("invalid api-version %q: must be YYYY-MM (e.g. 2026-07), a named version (e.g. dev), or \"default\" to reset", value)
		}
		if value == "default" {
			value = ""
		}
	case "routing-key":
		// any non-empty string is valid; empty string clears the key
	default:
		return errs.Usage("unknown config key %q: supported keys: output-mode, api-url, api-version, routing-key", key)
	}

	cfgPath := resolveConfigPath()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return errs.Internal("load config: %v", err)
	}

	switch key {
	case "output-mode":
		cfg.OutputMode = value
	case "api-url":
		cfg.APIURL = value
	case "api-version":
		return runConfigSetAPIVersion(cmd, cfgPath, cfg, value)
	case "routing-key":
		cfg.RoutingKey = value
	}

	if err := config.Save(cfgPath, cfg); err != nil {
		return errs.Internal("save config: %v", err)
	}

	_, err = fmt.Fprintf(cmd.OutOrStdout(), "set %s = %q\n", key, value)
	return err
}

// runConfigSetAPIVersion handles 'config set api-version <value>'.
// value is already normalised: "" means reset to default, a YYYY-MM string means set.
// On set: saves tentatively, fetches schema, writes cache, busts in-memory cache.
// On reset: clears config and removes cached schema.
// Reverts config if the fetch fails.
func runConfigSetAPIVersion(cmd *cobra.Command, cfgPath string, cfg config.Config, value string) error {
	if value == "" {
		cfg.APIVersion = ""
		if err := config.Save(cfgPath, cfg); err != nil {
			return errs.Internal("save config: %v", err)
		}
		schemaPath := apischema.CachedSchemaPath(resolveConfigDir())
		_ = os.Remove(schemaPath)
		apischema.SetConfigDir(resolveConfigDir())
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "api-version reset to default (%s)\n", config.ResolveAPIVersion(config.Config{}))
		return err
	}

	// Tentative save so token resolution picks up any saved secret store.
	cfg.APIVersion = value
	if err := config.Save(cfgPath, cfg); err != nil {
		return errs.Internal("save config: %v", err)
	}

	// Resolve token to fetch the schema. Missing token is non-fatal: warn and return.
	token, tokenErr := resolveAPIToken(cfg)
	if tokenErr != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
			"api-version saved; run 'mcli auth login' then 'mcli config set api-version %s' to refresh schema\n",
			value)
		return nil
	}

	schemaPath, fetchErr := fetchAndCacheSchema(cmd.Context(), cfg, value, token)
	if fetchErr != nil {
		// Revert the saved version.
		cfg.APIVersion = ""
		if revertErr := config.Save(cfgPath, cfg); revertErr != nil {
			return errs.Internal("revert config: %v", revertErr)
		}
		return errs.Usage("api version %q not available: %v — reverted to default (%s)",
			value, fetchErr, config.ResolveAPIVersion(config.Config{}))
	}

	_, err := fmt.Fprintf(cmd.OutOrStdout(), "fetched schema for %s → %s\n", value, schemaPath)
	return err
}

func newConfigGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "Get a configuration value",
		Long: `Get a configuration value from config.yaml.

Supported keys:
  output-mode   Default output mode
  api-url       monday.com API endpoint URL (empty means default production URL)
  api-version   monday.com API version (empty means default: 2026-07)
  routing-key   Routing key for local-api-proxy debugging`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigGet(cmd, args[0])
		},
	}
}

func runConfigGet(cmd *cobra.Command, key string) error {
	switch key {
	case "output-mode", "api-url", "api-version", "routing-key":
	default:
		return errs.Usage("unknown config key %q: supported keys: output-mode, api-url, api-version, routing-key", key)
	}

	cfgPath := resolveConfigPath()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return errs.Internal("load config: %v", err)
	}

	var value string
	switch key {
	case "output-mode":
		value = cfg.OutputMode
	case "api-url":
		value = cfg.APIURL
	case "api-version":
		value = cfg.APIVersion
	case "routing-key":
		value = cfg.RoutingKey
	}

	_, err = fmt.Fprintln(cmd.OutOrStdout(), value)
	return err
}
