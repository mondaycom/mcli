package config_test

import (
	"path/filepath"
	"testing"

	"github.com/mondaycom/mcli/internal/config"
	"github.com/mondaycom/mcli/internal/errs"
)

func TestSaveLoad_RoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "mcli", "config.yaml")

	want := config.Config{Token: "test-token-abc"}
	if err := config.Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Token != want.Token {
		t.Errorf("round-trip token: got %q, want %q", got.Token, want.Token)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfg, err := config.Load(filepath.Join(dir, "does-not-exist.yaml"))
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	if cfg.Token != "" {
		t.Errorf("expected empty token for missing file, got %q", cfg.Token)
	}
}

func TestDefaultPath(t *testing.T) {
	t.Parallel()

	path := config.DefaultPath()
	if path == "" {
		t.Error("DefaultPath() must not be empty")
	}
}

func TestResolveToken_FlagPrecedence(t *testing.T) {
	t.Setenv("MONDAY_API_TOKEN", "env-token")
	cfg := config.Config{Token: "config-token"}
	tok, err := config.ResolveToken(cfg, "flag-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok != "flag-token" {
		t.Errorf("flag should take precedence, got %q", tok)
	}
}

func TestResolveToken_EnvPrecedence(t *testing.T) {
	t.Setenv("MONDAY_API_TOKEN", "env-token")
	cfg := config.Config{Token: "config-token"}
	tok, err := config.ResolveToken(cfg, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok != "env-token" {
		t.Errorf("env should take precedence over config, got %q", tok)
	}
}

func TestResolveToken_ConfigFallback(t *testing.T) {
	t.Setenv("MONDAY_API_TOKEN", "")
	cfg := config.Config{Token: "config-token"}
	tok, err := config.ResolveToken(cfg, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok != "config-token" {
		t.Errorf("config should be used when flag and env are empty, got %q", tok)
	}
}

func TestResolveToken_AllEmpty(t *testing.T) {
	t.Setenv("MONDAY_API_TOKEN", "")
	cfg := config.Config{}
	_, err := config.ResolveToken(cfg, "")
	if err == nil {
		t.Fatal("expected error when all sources are empty")
	}
	authErr, ok := err.(*errs.Error)
	if !ok {
		t.Fatalf("expected *errs.Error, got %T", err)
	}
	if authErr.Code != errs.CodeAuth {
		t.Errorf("expected AUTH code, got %q", authErr.Code)
	}
}
