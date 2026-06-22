package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mondaycom/mcli/internal/config"
	"github.com/mondaycom/mcli/internal/errs"
)

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
  output-mode   Default output mode: default, json, pretty, terse, or csv`,
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
	default:
		return errs.Usage("unknown config key %q: supported keys: output-mode", key)
	}

	cfgPath := resolveConfigPath()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return errs.Internal("load config: %v", err)
	}

	switch key {
	case "output-mode":
		cfg.OutputMode = value
	}

	if err := config.Save(cfgPath, cfg); err != nil {
		return errs.Internal("save config: %v", err)
	}

	_, err = fmt.Fprintf(cmd.OutOrStdout(), "set %s = %q\n", key, value)
	return err
}

func newConfigGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "Get a configuration value",
		Long: `Get a configuration value from config.yaml.

Supported keys:
  output-mode   Default output mode`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigGet(cmd, args[0])
		},
	}
}

func runConfigGet(cmd *cobra.Command, key string) error {
	switch key {
	case "output-mode":
	default:
		return errs.Usage("unknown config key %q: supported keys: output-mode", key)
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
	}

	_, err = fmt.Fprintln(cmd.OutOrStdout(), value)
	return err
}
