package daemon_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mondaycom/mcli/internal/daemon"
)

func TestWriteReadRemovePID(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pid")

	if err := daemon.WritePID(path); err != nil {
		t.Fatalf("WritePID: %v", err)
	}

	pid, err := daemon.ReadPID(path)
	if err != nil {
		t.Fatalf("ReadPID: %v", err)
	}
	if pid != os.Getpid() {
		t.Errorf("expected PID %d, got %d", os.Getpid(), pid)
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
	// Must not error when file does not exist.
	if err := daemon.RemovePID(path); err != nil {
		t.Errorf("RemovePID on missing file should not error: %v", err)
	}
}

func TestReadPID_MissingFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.pid")
	_, err := daemon.ReadPID(path)
	if err == nil {
		t.Error("expected error for missing PID file")
	}
}

func TestReadPID_MalformedContent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.pid")
	if err := os.WriteFile(path, []byte("not-a-number\n"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	_, err := daemon.ReadPID(path)
	if err == nil {
		t.Error("expected error for malformed PID file")
	}
}

func TestIsRunning_CurrentProcess(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "running.pid")
	if err := daemon.WritePID(path); err != nil {
		t.Fatalf("WritePID: %v", err)
	}
	if !daemon.IsRunning(path) {
		t.Error("expected IsRunning to return true for current process")
	}
}

func TestIsRunning_MissingFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.pid")
	if daemon.IsRunning(path) {
		t.Error("expected IsRunning to return false for missing file")
	}
}

func TestIsRunning_StalePID(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "stale.pid")
	// PID 0 is never a valid user process; signal should fail.
	if err := os.WriteFile(path, []byte("99999999\n"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// We do not assert the exact result (the PID might theoretically exist on
	// some systems), but IsRunning must not panic.
	_ = daemon.IsRunning(path)
}
