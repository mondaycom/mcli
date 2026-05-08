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

// Config holds the persistent mcli configuration.
type Config struct {
	Token APIToken `yaml:"token"`
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
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	return cfg, nil
}

// Save marshals cfg to YAML and writes it atomically to path.
// The parent directory is created with mode 0700; the file is written with
// mode 0600 to protect the token.
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

// ResolveToken returns the API token using the precedence order documented in
// ADR-002: flag > MONDAY_API_TOKEN env > config file.
// Returns an AUTH error when all sources are empty.
func ResolveToken(cfg Config, flagToken string) (APIToken, error) {
	if flagToken != "" {
		return APIToken(flagToken), nil
	}
	if env := os.Getenv("MONDAY_API_TOKEN"); env != "" {
		return APIToken(env), nil
	}
	if cfg.Token != "" {
		return cfg.Token, nil
	}
	return "", errs.Auth("no API token found: set --token, MONDAY_API_TOKEN env, or run 'mcli auth login'")
}
