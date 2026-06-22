// Package config manages mcli configuration loading, saving, and token resolution.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/mondaycom/mcli/internal/errs"
)

// APIToken is a monday.com API token.
type APIToken string

// SecretBackend identifies which secret storage backend holds the API token.
type SecretBackend string

const (
	// BackendKeychain stores the token in the OS keychain.
	BackendKeychain SecretBackend = "keychain"
	// BackendFile stores the token in an age-encrypted file.
	BackendFile SecretBackend = "file"
)

// Config holds the persistent mcli configuration.
type Config struct {
	SecretStore SecretBackend `yaml:"secret_store,omitempty"`
	OutputMode  string        `yaml:"output_mode,omitempty"`
}

// legacyConfig is used during Load to detect and migrate a legacy plaintext token.
type legacyConfig struct {
	Token       string        `yaml:"token"`
	SecretStore SecretBackend `yaml:"secret_store,omitempty"`
	OutputMode  string        `yaml:"output_mode,omitempty"`
}

// DefaultPath returns the default config file path.
// It honours $XDG_CONFIG_HOME; otherwise falls back to $HOME/.config.
func DefaultPath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		base = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(base, "mcli", "config.yaml")
}

// Load reads and unmarshals a Config from path.
// It returns a zero Config (not an error) when the file does not exist, so
// callers can treat a missing config as an empty one.
//
// If the file contains a legacy plaintext token field, Load emits a warning to
// stderr, rewrites the file without the token field, and returns a Config with
// no token information.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}

	var legacy legacyConfig
	if err := yaml.Unmarshal(data, &legacy); err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}

	cfg := Config{SecretStore: legacy.SecretStore, OutputMode: legacy.OutputMode}

	if legacy.Token != "" {
		_, _ = fmt.Fprintf(os.Stderr,
			"warning: stripping legacy plaintext token from %s; run 'mcli auth login' to re-store securely\n",
			path)
		if saveErr := Save(path, cfg); saveErr != nil {
			return cfg, fmt.Errorf("migrate config %s: %w", path, saveErr)
		}
	}

	return cfg, nil
}

// Save marshals cfg to YAML and writes it atomically to path.
// The parent directory is created with mode 0700; the file is written with
// mode 0600.
func Save(path string, cfg Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config dir %s: %w", dir, err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write config %s: %w", path, err)
	}
	return nil
}

// Store is a minimal interface satisfied by internal/secrets.Store.
// Defined here to avoid an import cycle (secrets imports config).
type Store interface {
	Get() (APIToken, error)
}

// ResolveEndpoint returns the monday.com API endpoint, reading MONDAY_API_URL
// from the environment and falling back to the production URL.
func ResolveEndpoint() string {
	if url := os.Getenv("MONDAY_API_URL"); url != "" {
		return url
	}
	return "https://api.monday.com/v2"
}

// ResolveToken returns the API token using ADR-004 precedence:
// flag > MONDAY_API_TOKEN env > configured secret store.
// store may be nil (when cfg.SecretStore is unset); in that case only
// flag and env are consulted.
func ResolveToken(cfg Config, flagToken string, store Store) (APIToken, error) {
	if flagToken != "" {
		return APIToken(flagToken), nil
	}
	if env := os.Getenv("MONDAY_API_TOKEN"); env != "" {
		return APIToken(env), nil
	}
	if store != nil {
		tok, err := store.Get()
		if err == nil {
			return tok, nil
		}
		// NOT_FOUND means no token stored — fall through to the error below.
		if e, ok := err.(*errs.Error); !ok || e.Code != errs.CodeNotFound {
			return "", fmt.Errorf("read secret store: %w", err)
		}
	}
	return "", errs.Auth("no API token: set --token, MONDAY_API_TOKEN, or run 'mcli auth login'")
}
