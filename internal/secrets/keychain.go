package secrets

import (
	"errors"

	"github.com/zalando/go-keyring"

	"github.com/mondaycom/mcli/internal/config"
	"github.com/mondaycom/mcli/internal/errs"
)

const (
	keychainService = "mcli"
	keychainAccount = "monday-api-token"
)

// keychainStore implements Store using the OS keyring via zalando/go-keyring.
type keychainStore struct{}

// Available probes the keyring by attempting a sentinel read.
// If the error is keyring.ErrNotFound the keyring is reachable (return nil).
// Any other error means the keyring is unavailable.
func (s *keychainStore) Available() error {
	_, err := keyring.Get(keychainService, "__mcli_probe__")
	if err == nil || errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return errs.Auth("keychain unavailable: %v", err)
}

// Put stores the token in the OS keychain.
func (s *keychainStore) Put(token config.APIToken) error {
	if err := keyring.Set(keychainService, keychainAccount, string(token)); err != nil {
		return errs.Auth("store token in keychain: %v", err)
	}
	return nil
}

// Get retrieves the token from the OS keychain.
// Returns errs.NotFound when no token has been stored.
func (s *keychainStore) Get() (config.APIToken, error) {
	val, err := keyring.Get(keychainService, keychainAccount)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", errs.NotFound("no credential stored: run 'mcli auth login'")
		}
		return "", errs.Auth("read token from keychain: %v", err)
	}
	return config.APIToken(val), nil
}

// Delete removes the token from the OS keychain.
// Missing entry is treated as success (idempotent).
func (s *keychainStore) Delete() error {
	err := keyring.Delete(keychainService, keychainAccount)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return errs.Auth("delete token from keychain: %v", err)
	}
	return nil
}
