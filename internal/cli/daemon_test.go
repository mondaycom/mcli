package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mondaycom/mcli/internal/daemon"
)

// freePort returns an ephemeral TCP port.
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

// startDaemonForTest starts a daemon in dir and waits until it is reachable.
// Returns a cancel func that the caller must invoke to stop it.
func startDaemonForTest(t *testing.T, dir string, port int) context.CancelFunc {
	t.Helper()
	cfg := daemon.Config{Port: port, ConfigDir: dir}
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
	return cancel
}

// execDaemon runs a daemon sub-command with the given args. It temporarily
// overrides globals.Config so resolveConfigDir() points to dir.
func execDaemon(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	orig := globals
	globals = GlobalFlags{Config: filepath.Join(dir, "config.yaml")}
	defer func() { globals = orig }()

	var buf bytes.Buffer
	cmd := newDaemonCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func TestDaemonStatus_Running(t *testing.T) {
	dir := t.TempDir()
	port := freePort(t)
	cancel := startDaemonForTest(t, dir, port)
	defer cancel()

	out, err := execDaemon(t, dir, "status")
	if err != nil {
		t.Fatalf("daemon status: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	if result["running"] != true {
		t.Errorf("expected running=true, got %v", result["running"])
	}
}

func TestDaemonStatus_NotRunning(t *testing.T) {
	dir := t.TempDir()

	out, err := execDaemon(t, dir, "status")
	// status command should not error — it reports state.
	if err != nil {
		t.Fatalf("daemon status returned unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	if result["running"] != false {
		t.Errorf("expected running=false, got %v", result["running"])
	}
}

func TestDaemonStop_Running(t *testing.T) {
	dir := t.TempDir()
	port := freePort(t)
	cancel := startDaemonForTest(t, dir, port)
	defer cancel()

	out, err := execDaemon(t, dir, "stop")
	if err != nil {
		t.Fatalf("daemon stop: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	if result["stopping"] != true {
		t.Errorf("expected stopping=true, got %v", result["stopping"])
	}
}

func TestDaemonStop_NotRunning(t *testing.T) {
	dir := t.TempDir()

	_, err := execDaemon(t, dir, "stop")
	if err == nil {
		t.Error("expected error when stopping a non-running daemon")
	}
	code := errsCode(err)
	if code != "DAEMON_REQUIRED" {
		t.Errorf("expected DAEMON_REQUIRED error code, got %q", code)
	}
}

func TestDaemonStart_DetachNotSupported(t *testing.T) {
	dir := t.TempDir()

	_, err := execDaemon(t, dir, "start", "--detach")
	if err == nil {
		t.Fatal("expected error for --detach flag")
	}
	code := errsCode(err)
	if code != "USAGE" {
		t.Errorf("expected USAGE error, got %q", code)
	}
}
