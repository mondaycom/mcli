package daemon_test

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/mondaycom/mcli/internal/daemon"
)

// freePort returns a free TCP port by binding to :0 then closing the listener.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

func TestDaemon_StartStop(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	port := freePort(t)

	cfg := daemon.Config{
		Port:      port,
		ConfigDir: dir,
	}
	d := daemon.New(cfg)

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- d.Start(ctx)
	}()

	// Wait for the daemon to be reachable via IPC.
	sockPath := filepath.Join(dir, "daemon.sock")
	c := daemon.NewClient(sockPath)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err := c.Ping(); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if err := c.Ping(); err != nil {
		t.Fatalf("daemon did not become reachable: %v", err)
	}

	// Cancel context to trigger graceful shutdown.
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Start returned unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("daemon did not stop within timeout")
	}
}

func TestDaemon_Status(t *testing.T) {
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

	sr, err := c.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !sr.Running {
		t.Error("expected running=true")
	}
	if sr.Port != port {
		t.Errorf("expected port %d, got %d", port, sr.Port)
	}
	if sr.PID == 0 {
		t.Error("expected non-zero PID")
	}
}

func TestDaemon_StopViaIPC(t *testing.T) {
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
	go func() {
		errCh <- d.Start(ctx)
	}()

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
		t.Fatalf("Stop via IPC: %v", err)
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Start returned unexpected error after IPC stop: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("daemon did not stop after IPC stop command")
	}
}
