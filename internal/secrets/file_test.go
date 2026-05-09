package secrets

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/mondaycom/mcli/internal/config"
	"github.com/mondaycom/mcli/internal/errs"
)

func newFileStore(t *testing.T) *fileStore {
	t.Helper()
	return &fileStore{configDir: t.TempDir()}
}

func TestFileStore_RoundTrip(t *testing.T) {
	t.Setenv("MCLI_PASSPHRASE", "test-pass")

	s := newFileStore(t)
	const token = "tok-123"

	if err := s.Put(config.APIToken(token)); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := s.Get()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != config.APIToken(token) {
		t.Errorf("Get: got %q, want %q", got, token)
	}
}

func TestFileStore_WrongPassphrase(t *testing.T) {
	s := newFileStore(t)

	t.Setenv("MCLI_PASSPHRASE", "correct-passphrase")
	if err := s.Put("secret-token"); err != nil {
		t.Fatalf("Put: %v", err)
	}

	t.Setenv("MCLI_PASSPHRASE", "wrong-passphrase")
	_, err := s.Get()
	if err == nil {
		t.Fatal("expected error for wrong passphrase")
	}
	e, ok := err.(*errs.Error)
	if !ok {
		t.Fatalf("expected *errs.Error, got %T: %v", err, err)
	}
	if e.Code != errs.CodeAuth {
		t.Errorf("expected AUTH code, got %q", e.Code)
	}
}

func TestFileStore_MissingPassphrase_Available(t *testing.T) {
	t.Setenv("MCLI_PASSPHRASE", "")

	s := newFileStore(t)

	err := s.Available()
	if err == nil {
		t.Fatal("expected error when MCLI_PASSPHRASE is unset")
	}
	e, ok := err.(*errs.Error)
	if !ok {
		t.Fatalf("expected *errs.Error, got %T", err)
	}
	if e.Code != errs.CodeAuth {
		t.Errorf("expected AUTH code, got %q", e.Code)
	}
	if !bytes.Contains([]byte(e.Message), []byte("MCLI_PASSPHRASE")) {
		t.Errorf("error message should mention MCLI_PASSPHRASE, got %q", e.Message)
	}
}

func TestFileStore_MissingPassphrase_Get(t *testing.T) {
	t.Setenv("MCLI_PASSPHRASE", "")

	s := newFileStore(t)

	_, err := s.Get()
	if err == nil {
		t.Fatal("expected error when MCLI_PASSPHRASE is unset")
	}
	e, ok := err.(*errs.Error)
	if !ok {
		t.Fatalf("expected *errs.Error, got %T", err)
	}
	if e.Code != errs.CodeAuth {
		t.Errorf("expected AUTH code, got %q", e.Code)
	}
	if !bytes.Contains([]byte(e.Message), []byte("MCLI_PASSPHRASE")) {
		t.Errorf("error message should mention MCLI_PASSPHRASE, got %q", e.Message)
	}
}

func TestFileStore_MissingPassphrase_Put(t *testing.T) {
	t.Setenv("MCLI_PASSPHRASE", "")

	s := newFileStore(t)

	err := s.Put("some-token")
	if err == nil {
		t.Fatal("expected error when MCLI_PASSPHRASE is unset")
	}
	e, ok := err.(*errs.Error)
	if !ok {
		t.Fatalf("expected *errs.Error, got %T", err)
	}
	if e.Code != errs.CodeAuth {
		t.Errorf("expected AUTH code, got %q", e.Code)
	}
	if !bytes.Contains([]byte(e.Message), []byte("MCLI_PASSPHRASE")) {
		t.Errorf("error message should mention MCLI_PASSPHRASE, got %q", e.Message)
	}
}

func TestFileStore_MissingFile(t *testing.T) {
	t.Setenv("MCLI_PASSPHRASE", "some-pass")

	s := newFileStore(t)

	_, err := s.Get()
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	e, ok := err.(*errs.Error)
	if !ok {
		t.Fatalf("expected *errs.Error, got %T: %v", err, err)
	}
	if e.Code != errs.CodeNotFound {
		t.Errorf("expected NOT_FOUND code, got %q", e.Code)
	}
}

func TestFileStore_DeleteIdempotent(t *testing.T) {
	t.Parallel()

	s := newFileStore(t)

	if err := s.Delete(); err != nil {
		t.Errorf("Delete on non-existent file should return nil, got %v", err)
	}
}

func TestFileStore_FileMode(t *testing.T) {
	t.Setenv("MCLI_PASSPHRASE", "mode-test-pass")

	s := newFileStore(t)
	if err := s.Put("mode-test-token"); err != nil {
		t.Fatalf("Put: %v", err)
	}

	info, err := os.Stat(s.credPath())
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	perm := info.Mode().Perm()
	if perm != 0o600 {
		t.Errorf("file mode: got %04o, want 0600", perm)
	}
}

func TestFileStore_EncryptedBytesDoNotContainToken(t *testing.T) {
	t.Setenv("MCLI_PASSPHRASE", "enc-test-pass")

	s := newFileStore(t)
	const token = "super-secret-api-key-12345"

	if err := s.Put(config.APIToken(token)); err != nil {
		t.Fatalf("Put: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(s.configDir, credentialsFile))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if bytes.Contains(raw, []byte(token)) {
		t.Errorf("encrypted file should NOT contain plaintext token %q", token)
	}
}
