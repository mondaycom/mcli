package secrets

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"filippo.io/age"

	"github.com/mondaycom/mcli/internal/config"
	"github.com/mondaycom/mcli/internal/errs"
)

const credentialsFile = "credentials.age"

// fileStore implements Store using age passphrase encryption.
// The passphrase is read exclusively from the MCLI_PASSPHRASE environment variable.
type fileStore struct {
	configDir string
}

func (s *fileStore) credPath() string {
	return filepath.Join(s.configDir, credentialsFile)
}

// Available returns nil when MCLI_PASSPHRASE is set, otherwise an AUTH error
// naming the env var.
func (s *fileStore) Available() error {
	if os.Getenv("MCLI_PASSPHRASE") == "" {
		return errs.Auth("file backend requires MCLI_PASSPHRASE env var to be set")
	}
	return nil
}

// Put encrypts the token with the MCLI_PASSPHRASE passphrase and writes it
// atomically to <configDir>/credentials.age (mode 0600).
func (s *fileStore) Put(token config.APIToken) error {
	passphrase := os.Getenv("MCLI_PASSPHRASE")
	if passphrase == "" {
		return errs.Auth("file backend requires MCLI_PASSPHRASE env var to be set")
	}

	recipient, err := age.NewScryptRecipient(passphrase)
	if err != nil {
		return fmt.Errorf("create age recipient: %w", err)
	}

	var buf bytes.Buffer
	w, err := age.Encrypt(&buf, recipient)
	if err != nil {
		return fmt.Errorf("begin age encrypt: %w", err)
	}
	if _, err := io.WriteString(w, string(token)); err != nil {
		return fmt.Errorf("encrypt token: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("finalise age encrypt: %w", err)
	}

	dir := s.configDir
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config dir %s: %w", dir, err)
	}

	tmp := s.credPath() + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o600); err != nil {
		return fmt.Errorf("write credentials.age.tmp: %w", err)
	}
	if err := os.Rename(tmp, s.credPath()); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename credentials.age.tmp: %w", err)
	}
	return nil
}

// Get decrypts and returns the stored token.
// Returns errs.NotFound when the file does not exist, or errs.Auth on wrong passphrase.
func (s *fileStore) Get() (config.APIToken, error) {
	passphrase := os.Getenv("MCLI_PASSPHRASE")
	if passphrase == "" {
		return "", errs.Auth("file backend requires MCLI_PASSPHRASE env var to be set")
	}

	data, err := os.ReadFile(s.credPath())
	if err != nil {
		if os.IsNotExist(err) {
			return "", errs.NotFound("no credential stored: run 'mcli auth login'")
		}
		return "", fmt.Errorf("read credentials.age: %w", err)
	}

	identity, err := age.NewScryptIdentity(passphrase)
	if err != nil {
		return "", fmt.Errorf("create age identity: %w", err)
	}

	r, err := age.Decrypt(bytes.NewReader(data), identity)
	if err != nil {
		if _, ok := errors.AsType[*age.NoIdentityMatchError](err); ok {
			return "", errs.Auth("decrypt credentials.age: wrong passphrase")
		}
		return "", fmt.Errorf("decrypt credentials.age: %w", err)
	}

	plain, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("read decrypted token: %w", err)
	}
	return config.APIToken(plain), nil
}

// Delete removes the credentials file.
// Missing file is treated as success (idempotent).
func (s *fileStore) Delete() error {
	err := os.Remove(s.credPath())
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete credentials.age: %w", err)
	}
	return nil
}
