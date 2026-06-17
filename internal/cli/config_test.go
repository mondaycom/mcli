package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/mondaycom/mcli/internal/config"
)

// runConfigCmd executes a config sub-command in an isolated temp dir and
// returns the captured stdout output.
func runConfigCmd(t *testing.T, cfgPath string, args ...string) (string, error) {
	t.Helper()
	origGlobals := globals
	t.Cleanup(func() { globals = origGlobals })
	globals.Config = cfgPath

	root := &cobra.Command{Use: "mcli", SilenceErrors: true, SilenceUsage: true}
	root.AddCommand(newConfigCmd())

	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs(args)

	err := root.Execute()
	return buf.String(), err
}

func TestConfigSetGet_OutputModeRoundTrip(t *testing.T) {
	for _, mode := range []string{"json", "pretty", "terse", "csv"} {
		mode := mode
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			cfgPath := filepath.Join(dir, "config.yaml")

			if _, err := runConfigCmd(t, cfgPath, "config", "set", "output-mode", mode); err != nil {
				t.Fatalf("set output-mode %s: %v", mode, err)
			}

			out, err := runConfigCmd(t, cfgPath, "config", "get", "output-mode")
			if err != nil {
				t.Fatalf("get output-mode after set %s: %v", mode, err)
			}
			got := strings.TrimSpace(out)
			if got != mode {
				t.Errorf("round-trip output-mode: got %q, want %q", got, mode)
			}
		})
	}
}

func TestConfigSet_UnsetOutputMode(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	if _, err := runConfigCmd(t, cfgPath, "config", "set", "output-mode", "json"); err != nil {
		t.Fatalf("set output-mode json: %v", err)
	}
	if _, err := runConfigCmd(t, cfgPath, "config", "set", "output-mode", ""); err != nil {
		t.Fatalf("unset output-mode: %v", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.OutputMode != "" {
		t.Errorf("output-mode should be empty after unset, got %q", cfg.OutputMode)
	}
}

func TestConfigSet_DefaultAliasClearsMode(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	if _, err := runConfigCmd(t, cfgPath, "config", "set", "output-mode", "terse"); err != nil {
		t.Fatalf("set output-mode terse: %v", err)
	}
	if _, err := runConfigCmd(t, cfgPath, "config", "set", "output-mode", "default"); err != nil {
		t.Fatalf("set output-mode default: %v", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.OutputMode != "" {
		t.Errorf("'default' should clear output-mode, got %q", cfg.OutputMode)
	}
}

func TestConfigSet_InvalidOutputMode(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	if _, err := runConfigCmd(t, cfgPath, "config", "set", "output-mode", "bogus"); err == nil {
		t.Fatal("expected error for invalid output-mode value, got nil")
	}
}

func TestConfigSet_UnknownKey(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	if _, err := runConfigCmd(t, cfgPath, "config", "set", "no-such-key", "value"); err == nil {
		t.Fatal("expected error for unknown key, got nil")
	}
}

func TestConfigGet_UnknownKey(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	if _, err := runConfigCmd(t, cfgPath, "config", "get", "no-such-key"); err == nil {
		t.Fatal("expected error for unknown key, got nil")
	}
}

func TestConfigSet_PreservesExistingFields(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	// Prime the config with a secret_store value.
	if err := config.Save(cfgPath, config.Config{SecretStore: config.BackendKeychain}); err != nil {
		t.Fatalf("save initial config: %v", err)
	}

	if _, err := runConfigCmd(t, cfgPath, "config", "set", "output-mode", "terse"); err != nil {
		t.Fatalf("set output-mode: %v", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.SecretStore != config.BackendKeychain {
		t.Errorf("SecretStore was overwritten: got %q", cfg.SecretStore)
	}
	if cfg.OutputMode != "terse" {
		t.Errorf("OutputMode not set: got %q", cfg.OutputMode)
	}
}
