// Package secrets stores and retrieves API tokens via pluggable backends.
package secrets

import (
	"github.com/mondaycom/mcli/internal/config"
	"github.com/mondaycom/mcli/internal/errs"
)

// Store stores a single API token.
type Store interface {
	// Put encrypts/stores token in the backend.
	Put(token config.APIToken) error
	// Get retrieves the stored token.
	// Returns errs.NotFound when no token has been stored yet.
	Get() (config.APIToken, error)
	// Delete removes the stored token.
	// Returns nil when no token was present (idempotent).
	Delete() error
	// Available returns nil when the backend is usable, or a descriptive
	// *errs.Error when it is not (e.g. keyring daemon missing, MCLI_PASSPHRASE unset).
	Available() error
}

// Open returns a Store for the given backend name.
// configDir is used by file-backed stores to locate credentials.age.
// Returns a *errs.Error with CodeUsage for unknown backend names.
func Open(name config.SecretBackend, configDir string) (Store, error) {
	switch name {
	case config.BackendKeychain:
		return &keychainStore{}, nil
	case config.BackendFile:
		return &fileStore{configDir: configDir}, nil
	default:
		return nil, errs.Usage("unknown secret backend %q: must be %q or %q",
			name, config.BackendKeychain, config.BackendFile)
	}
}

// Ensure keychainStore and fileStore satisfy the config.Store interface used by
// config.ResolveToken, so callers may pass them directly.
var (
	_ config.Store = (*keychainStore)(nil)
	_ config.Store = (*fileStore)(nil)
)
