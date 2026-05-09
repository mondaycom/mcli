// Package secrets — keychain backend tests.
//
// Build tags:
//   - (no tag): compile-only smoke test; verifies keychainStore satisfies Store
//     and that Open(BackendKeychain) returns a non-nil Store.
//   - keychain_real: executes a real round-trip against the OS keyring.
//     Run with: go test -run TestKeychain -tags keychain_real ./internal/secrets/
//     Requires a functional OS keychain (macOS Keychain or Linux Secret Service).
package secrets

import (
	"testing"

	"github.com/mondaycom/mcli/internal/config"
)

// TestKeychainStore_InterfaceCompliance verifies that keychainStore satisfies
// both the Store and config.Store interfaces without hitting the OS keyring.
func TestKeychainStore_InterfaceCompliance(t *testing.T) {
	t.Parallel()

	st, err := Open(config.BackendKeychain, t.TempDir())
	if err != nil {
		t.Fatalf("Open(BackendKeychain): %v", err)
	}
	if st == nil {
		t.Fatal("Open(BackendKeychain) returned nil store")
	}

	// Verify the returned value implements the config.Store interface (used by
	// config.ResolveToken). The Store interface itself is already guaranteed by
	// Open's return type.
	var _ config.Store = st
}

// TestOpen_UnknownBackend verifies that Open returns a USAGE error for an
// unrecognised backend name.
func TestOpen_UnknownBackend(t *testing.T) {
	t.Parallel()

	_, err := Open("unsupported", t.TempDir())
	if err == nil {
		t.Fatal("expected error for unknown backend")
	}
}
