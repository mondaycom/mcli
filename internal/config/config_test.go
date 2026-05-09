package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mondaycom/mcli/internal/config"
	"github.com/mondaycom/mcli/internal/errs"
)

// fakeStore is a simple in-memory secrets.Store substitute for unit tests.
type fakeStore struct {
	token config.APIToken
	empty bool
}

func (f *fakeStore) Get() (config.APIToken, error) {
	if f.empty {
		return "", errs.NotFound("no credential stored")
	}
	return f.token, nil
}

func TestSaveLoad_RoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "mcli", "config.yaml")

	want := config.Config{SecretStore: config.BackendKeychain}
	if err := config.Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.SecretStore != want.SecretStore {
		t.Errorf("round-trip SecretStore: got %q, want %q", got.SecretStore, want.SecretStore)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfg, err := config.Load(filepath.Join(dir, "does-not-exist.yaml"))
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	if cfg.SecretStore != "" {
		t.Errorf("expected empty SecretStore for missing file, got %q", cfg.SecretStore)
	}
}

func TestDefaultPath(t *testing.T) {
	t.Parallel()

	path := config.DefaultPath()
	if path == "" {
		t.Error("DefaultPath() must not be empty")
	}
}

func TestLoad_LegacyMigration(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Write a legacy config with a plaintext token and secret_store.
	legacy := "token: legacy-tok\nsecret_store: keychain\n"
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	// Capture stderr for the warning.
	origStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = w

	cfg, loadErr := config.Load(path)

	_ = w.Close()
	os.Stderr = origStderr

	var warnBuf strings.Builder
	buf := make([]byte, 4096)
	for {
		n, readErr := r.Read(buf)
		if n > 0 {
			warnBuf.Write(buf[:n])
		}
		if readErr != nil {
			break
		}
	}
	_ = r.Close()

	if loadErr != nil {
		t.Fatalf("Load: %v", loadErr)
	}
	// In-memory config should still have SecretStore but no token.
	if cfg.SecretStore != config.BackendKeychain {
		t.Errorf("SecretStore after migration: got %q, want %q", cfg.SecretStore, config.BackendKeychain)
	}

	// Warning must have been emitted.
	warning := warnBuf.String()
	if !strings.Contains(warning, "legacy plaintext token") {
		t.Errorf("expected migration warning on stderr, got: %q", warning)
	}

	// Re-read the file to confirm the token field is gone.
	reloaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("re-Load after migration: %v", err)
	}
	if reloaded.SecretStore != config.BackendKeychain {
		t.Errorf("re-loaded SecretStore: got %q", reloaded.SecretStore)
	}

	// Raw file must not contain the legacy token.
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "legacy-tok") {
		t.Errorf("migrated config file still contains legacy token")
	}
	if strings.Contains(string(raw), "token:") {
		t.Errorf("migrated config file still contains token: field")
	}
}

func TestResolveToken_FlagPrecedence(t *testing.T) {
	t.Setenv("MONDAY_API_TOKEN", "env-token")
	store := &fakeStore{token: "store-token"}
	cfg := config.Config{SecretStore: config.BackendKeychain}

	tok, err := config.ResolveToken(cfg, "flag-token", store)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok != "flag-token" {
		t.Errorf("flag should take precedence, got %q", tok)
	}
}

func TestResolveToken_EnvPrecedence(t *testing.T) {
	t.Setenv("MONDAY_API_TOKEN", "env-token")
	store := &fakeStore{token: "store-token"}
	cfg := config.Config{SecretStore: config.BackendKeychain}

	tok, err := config.ResolveToken(cfg, "", store)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok != "env-token" {
		t.Errorf("env should take precedence over store, got %q", tok)
	}
}

func TestResolveToken_StoreFallback(t *testing.T) {
	t.Setenv("MONDAY_API_TOKEN", "")
	store := &fakeStore{token: "store-token"}
	cfg := config.Config{SecretStore: config.BackendKeychain}

	tok, err := config.ResolveToken(cfg, "", store)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok != "store-token" {
		t.Errorf("store should be used when flag and env are empty, got %q", tok)
	}
}

func TestResolveToken_AllEmpty(t *testing.T) {
	t.Setenv("MONDAY_API_TOKEN", "")
	store := &fakeStore{empty: true}
	cfg := config.Config{}

	_, err := config.ResolveToken(cfg, "", store)
	if err == nil {
		t.Fatal("expected error when all sources are empty")
	}
	var authErr *errs.Error
	if !errors.As(err, &authErr) {
		t.Fatalf("expected *errs.Error, got %T", err)
	}
	if authErr.Code != errs.CodeAuth {
		t.Errorf("expected AUTH code, got %q", authErr.Code)
	}
}

func TestResolveToken_NilStore(t *testing.T) {
	t.Setenv("MONDAY_API_TOKEN", "")
	cfg := config.Config{}

	_, err := config.ResolveToken(cfg, "", nil)
	if err == nil {
		t.Fatal("expected error when store is nil and all sources empty")
	}
	var authErr *errs.Error
	if !errors.As(err, &authErr) {
		t.Fatalf("expected *errs.Error, got %T", err)
	}
	if authErr.Code != errs.CodeAuth {
		t.Errorf("expected AUTH code, got %q", authErr.Code)
	}
}
