package daemon_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/mondaycom/mcli/internal/daemon"
)

// startTestDaemon starts a daemon in a temp dir and returns the client and a
// cancel func. The caller must call cancel to shut the daemon down.
func startTestDaemon(t *testing.T) (*daemon.Client, context.CancelFunc) {
	t.Helper()
	dir := t.TempDir()
	port := freePort(t)

	cfg := daemon.Config{
		Port:      port,
		ConfigDir: dir,
	}
	d := daemon.New(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = d.Start(ctx) }()

	sockPath := filepath.Join(dir, "daemon.sock")
	c := daemon.NewClient(sockPath)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err := c.Ping(); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	return c, cancel
}

func TestClient_Ping(t *testing.T) {
	t.Parallel()
	c, cancel := startTestDaemon(t)
	defer cancel()

	if err := c.Ping(); err != nil {
		t.Errorf("Ping: %v", err)
	}
}

func TestClient_Status(t *testing.T) {
	t.Parallel()
	c, cancel := startTestDaemon(t)
	defer cancel()

	sr, err := c.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !sr.Running {
		t.Error("expected running=true")
	}
	if sr.PID == 0 {
		t.Error("expected non-zero PID in status")
	}
}

func TestClient_Stop(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	port := freePort(t)

	cfg := daemon.Config{
		Port:      port,
		ConfigDir: dir,
	}
	d := daemon.New(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- d.Start(ctx) }()

	sockPath := filepath.Join(dir, "daemon.sock")
	c := daemon.NewClient(sockPath)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err := c.Ping(); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if err := c.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	select {
	case <-errCh:
		// daemon exited — success
	case <-time.After(5 * time.Second):
		t.Fatal("daemon did not exit after Stop")
	}
}

func TestClient_Unreachable(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "no-daemon.sock")
	c := daemon.NewClient(sockPath)

	if err := c.Ping(); err == nil {
		t.Error("expected error when daemon is not running")
	}
}
