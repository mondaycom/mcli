package daemon_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mondaycom/mcli/internal/daemon"
)

func TestWriteRemovePID(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pid")

	if err := daemon.WritePID(path); err != nil {
		t.Fatalf("WritePID: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected PID file to exist: %v", err)
	}

	if err := daemon.RemovePID(path); err != nil {
		t.Fatalf("RemovePID: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("expected PID file to be removed")
	}
}

func TestRemovePID_NotExist(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.pid")
	if err := daemon.RemovePID(path); err != nil {
		t.Errorf("RemovePID on missing file should not error: %v", err)
	}
}
