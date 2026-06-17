package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/mondaycom/mcli/internal/config"
	"github.com/mondaycom/mcli/internal/errs"
	"github.com/mondaycom/mcli/internal/secrets"
)

// newAuthCmd returns the 'mcli auth' parent command.
func newAuthCmd() *cobra.Command {
	authCmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage mcli authentication",
	}
	authCmd.AddCommand(newAuthLoginCmd())
	authCmd.AddCommand(newAuthLogoutCmd())
	authCmd.AddCommand(newAuthStatusCmd())
	return authCmd
}

// resolveConfigDir returns the config directory to use.
// When globals.Config is set it uses the directory that contains the config file;
// otherwise it returns the directory of config.DefaultPath().
func resolveConfigDir() string {
	if globals.Config != "" {
		return filepath.Dir(globals.Config)
	}
	return filepath.Dir(config.DefaultPath())
}

// resolveConfigPath returns the effective config file path.
func resolveConfigPath() string {
	if globals.Config != "" {
		return globals.Config
	}
	return config.DefaultPath()
}

// printAuthResult writes an auth operation result per ADR-002 output rules.
// In JSON mode (or non-TTY) it writes JSON to stdout; in pretty mode it writes
// msg to cmd.OutOrStdout().
func printAuthResult(cmd *cobra.Command, payload any, msg string) error {
	mode, err := resolveOutputMode(os.Stdout, globals, configOutputMode())
	if err != nil {
		return err
	}
	if mode == ModeJSON {
		data, mErr := json.Marshal(payload)
		if mErr != nil {
			return errs.Internal("marshal output: %v", mErr)
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return err
	}
	_, err = fmt.Fprintln(cmd.OutOrStdout(), msg)
	return err
}

// newAuthLoginCmd returns the 'mcli auth login' subcommand.
func newAuthLoginCmd() *cobra.Command {
	var tokenFlag string
	var storeFlag string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Store an API token in the configured secret backend",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if tokenFlag == "" {
				return errs.Usage("--token is required for 'auth login'")
			}

			backend := config.SecretBackend(storeFlag)
			switch backend {
			case config.BackendKeychain, config.BackendFile:
				// valid
			default:
				return errs.Usage("--store must be %q or %q, got %q",
					config.BackendKeychain, config.BackendFile, storeFlag)
			}

			configDir := resolveConfigDir()
			st, err := secrets.Open(backend, configDir)
			if err != nil {
				return err
			}
			if err := st.Available(); err != nil {
				return err
			}
			if err := st.Put(config.APIToken(tokenFlag)); err != nil {
				return errs.Internal("store token: %v", err)
			}

			cfgPath := resolveConfigPath()
			cfg, loadErr := config.Load(cfgPath)
			if loadErr != nil {
				// Best-effort: carry on with an empty config.
				cfg = config.Config{}
			}
			cfg.SecretStore = backend
			if err := config.Save(cfgPath, cfg); err != nil {
				return errs.Internal("save config: %v", err)
			}

			return printAuthResult(cmd,
				map[string]any{"backend": string(backend), "stored": true},
				fmt.Sprintf("Token stored in %s", backend),
			)
		},
	}

	cmd.Flags().StringVar(&tokenFlag, "token", "", "monday.com API token to store (required)")
	cmd.Flags().StringVar(&storeFlag, "store", string(config.BackendKeychain),
		"secret backend: keychain or file")
	return cmd
}

// newAuthLogoutCmd returns the 'mcli auth logout' subcommand.
func newAuthLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove the stored API token",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfgPath := resolveConfigPath()
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return errs.Internal("load config: %v", err)
			}
			if cfg.SecretStore == "" {
				return errs.NotFound("no token stored")
			}

			st, err := secrets.Open(cfg.SecretStore, resolveConfigDir())
			if err != nil {
				return err
			}
			if err := st.Delete(); err != nil {
				return errs.Internal("delete token: %v", err)
			}

			cfg.SecretStore = ""
			if err := config.Save(cfgPath, cfg); err != nil {
				return errs.Internal("save config: %v", err)
			}

			return printAuthResult(cmd,
				map[string]any{"stored": false},
				"Token removed",
			)
		},
	}
}

// newAuthStatusCmd returns the 'mcli auth status' subcommand.
// This command MUST NEVER print or log the token value.
func newAuthStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the active secret backend and whether a token is stored",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfgPath := resolveConfigPath()
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return errs.Internal("load config: %v", err)
			}

			if cfg.SecretStore == "" {
				return printAuthResult(cmd,
					map[string]any{"backend": nil, "stored": false},
					"No token stored",
				)
			}

			st, err := secrets.Open(cfg.SecretStore, resolveConfigDir())
			if err != nil {
				return err
			}

			_, getErr := st.Get()
			stored := getErr == nil
			if getErr != nil {
				e, ok := errors.AsType[*errs.Error](getErr)
				if ok && e.Code == errs.CodeNotFound {
					stored = false
				} else {
					return errs.Auth("check token: %v", getErr)
				}
			}

			return printAuthResult(cmd,
				map[string]any{"backend": string(cfg.SecretStore), "stored": stored},
				fmt.Sprintf("Backend: %s, stored: %v", cfg.SecretStore, stored),
			)
		},
	}
}
