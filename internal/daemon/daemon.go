// Package daemon implements the mcli background daemon: an HTTP server for
// receiving monday.com webhooks and a Unix socket IPC server for CLI control.
package daemon

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync/atomic"
)

// Config holds the parameters needed to construct a Daemon.
type Config struct {
	Port        int
	ConfigDir   string
	ExternalURL string
}

// DefaultPort is the default TCP port the webhook HTTP server listens on.
const DefaultPort = 8420

// Daemon is the mcli background service. It owns an HTTP server (webhook
// ingestion) and a Unix socket IPC server (CLI control).
type Daemon struct {
	cfg        Config
	httpServer *http.Server
	ipcServer  *IPCServer
	stopCh     chan struct{}
	stopped    atomic.Bool
}

// New constructs a Daemon from cfg. It does not start any servers.
func New(cfg Config) *Daemon {
	if cfg.Port == 0 {
		cfg.Port = DefaultPort
	}
	d := &Daemon{
		cfg:    cfg,
		stopCh: make(chan struct{}),
	}

	whMux := http.NewServeMux()
	whMux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	d.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: whMux,
	}

	d.ipcServer = NewIPCServer(d.sockPath(), d)
	return d
}

func (d *Daemon) sockPath() string {
	return d.cfg.ConfigDir + "/daemon.sock"
}

func (d *Daemon) pidPath() string {
	return d.cfg.ConfigDir + "/daemon.pid"
}

// Start launches the HTTP webhook server and IPC server, then blocks until
// Stop is called (or ctx is cancelled).
func (d *Daemon) Start(ctx context.Context) error {
	if err := os.MkdirAll(d.cfg.ConfigDir, 0o700); err != nil {
		return fmt.Errorf("Start: create config dir: %w", err)
	}

	if err := WritePID(d.pidPath()); err != nil {
		return fmt.Errorf("Start: %w", err)
	}

	// Remove stale socket if present.
	_ = os.Remove(d.sockPath())

	httpErrCh := make(chan error, 1)
	go func() {
		ln, err := net.Listen("tcp", d.httpServer.Addr)
		if err != nil {
			httpErrCh <- fmt.Errorf("Start: listen HTTP: %w", err)
			return
		}
		if err := d.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			httpErrCh <- fmt.Errorf("Start: HTTP server: %w", err)
		}
	}()

	ipcErrCh := make(chan error, 1)
	go func() {
		if err := d.ipcServer.Serve(); err != nil && err != http.ErrServerClosed {
			ipcErrCh <- fmt.Errorf("Start: IPC server: %w", err)
		}
	}()

	select {
	case <-ctx.Done():
		return d.Stop()
	case <-d.stopCh:
		return d.shutdown()
	case err := <-httpErrCh:
		_ = d.Stop()
		return err
	case err := <-ipcErrCh:
		_ = d.Stop()
		return err
	}
}

// triggerStop signals the daemon to stop without blocking.
func (d *Daemon) triggerStop() {
	if d.stopped.CompareAndSwap(false, true) {
		close(d.stopCh)
	}
}

// Stop initiates a graceful shutdown and blocks until complete.
func (d *Daemon) Stop() error {
	d.triggerStop()
	return d.shutdown()
}

func (d *Daemon) shutdown() error {
	ctx := context.Background()

	var httpErr, ipcErr error
	httpErr = d.httpServer.Shutdown(ctx)
	ipcErr = d.ipcServer.Shutdown(ctx)

	_ = RemovePID(d.pidPath())
	_ = os.Remove(d.sockPath())

	if httpErr != nil {
		return fmt.Errorf("shutdown HTTP: %w", httpErr)
	}
	if ipcErr != nil {
		return fmt.Errorf("shutdown IPC: %w", ipcErr)
	}
	return nil
}

// Status returns the current status of the daemon.
func (d *Daemon) Status() StatusResponse {
	return StatusResponse{
		Running: true,
		Port:    d.cfg.Port,
		PID:     os.Getpid(),
	}
}
