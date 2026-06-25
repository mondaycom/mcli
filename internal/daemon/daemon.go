// Package daemon implements the mcli background daemon: an HTTP server for
// receiving monday.com webhooks and a Unix socket IPC server for CLI control.
package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/mondaycom/mcli/internal/config"
)

// Config holds the parameters needed to construct a Daemon.
type Config struct {
	Port        int
	ConfigDir   string
	ExternalURL string
	Token       string // monday.com API token for webhook CRUD operations
}

// DefaultPort is the default TCP port the webhook HTTP server listens on.
const DefaultPort = 8420

// Daemon is the mcli background service. It owns an HTTP server (webhook
// ingestion) and a Unix socket IPC server (CLI control).
type Daemon struct {
	cfg        Config
	httpServer *http.Server
	ipcServer  *IPCServer
	tunnel     *Tunnel
	store      *Store
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

	d.httpServer = &http.Server{
		Addr: fmt.Sprintf(":%d", cfg.Port),
	}

	d.ipcServer = NewIPCServer(d.sockPath(), d)

	// Pre-create the Tunnel so TunnelURLChanges() is available before Start.
	if cfg.ExternalURL == "" {
		d.tunnel = NewTunnel(cfg.Port)
	}

	return d
}

func (d *Daemon) sockPath() string {
	return filepath.Join(d.cfg.ConfigDir, "daemon.sock")
}

func (d *Daemon) pidPath() string {
	return filepath.Join(d.cfg.ConfigDir, "daemon.pid")
}

// Start launches the HTTP webhook server and IPC server, then blocks until
// Stop is called (or ctx is cancelled). If cfg.ExternalURL is empty, a
// Cloudflare Quick Tunnel is started to provide a public URL.
func (d *Daemon) Start(ctx context.Context) error {
	if err := os.MkdirAll(d.cfg.ConfigDir, 0o700); err != nil {
		return fmt.Errorf("Start: create config dir: %w", err)
	}

	if err := WritePID(d.pidPath()); err != nil {
		return fmt.Errorf("Start: %w", err)
	}

	// Open the event store and wire the webhook handler.
	store, err := OpenStore(filepath.Join(d.cfg.ConfigDir, "events.db"))
	if err != nil {
		return fmt.Errorf("Start: open event store: %w", err)
	}
	d.store = store

	whMux := http.NewServeMux()
	whMux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"status":"ok"}`)
	})
	whMux.Handle("/webhook", NewWebhookHandler(d.store))
	d.httpServer.Handler = whMux

	// Remove stale socket if present.
	_ = os.Remove(d.sockPath())

	if d.tunnel != nil {
		if err := d.tunnel.Start(ctx); err != nil {
			// Log the error but continue: the daemon is still useful for local
			// development even without a public tunnel URL.
			log.Printf("daemon: tunnel unavailable: %v", err)
			d.tunnel = nil
		}
	}

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

	// Watch for tunnel URL changes and re-register all existing webhooks.
	go d.watchTunnelURLChanges(ctx)

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

	if d.tunnel != nil {
		_ = d.tunnel.Stop()
	}

	if d.store != nil {
		_ = d.store.Close()
	}

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

// Store returns the daemon's event store. It is nil before Start is called.
func (d *Daemon) Store() *Store {
	return d.store
}

// URL returns the external URL for this daemon instance. If an ExternalURL was
// configured, that is returned. Otherwise the active tunnel URL is returned.
func (d *Daemon) URL() string {
	if d.cfg.ExternalURL != "" {
		return d.cfg.ExternalURL
	}
	if d.tunnel != nil {
		return d.tunnel.URL()
	}
	return ""
}

// TunnelURLChanges returns the tunnel's URL-change channel, or a closed
// channel if no tunnel is configured (e.g. ExternalURL was provided).
func (d *Daemon) TunnelURLChanges() <-chan string {
	if d.tunnel != nil {
		return d.tunnel.URLChanges()
	}
	ch := make(chan string)
	close(ch)
	return ch
}

// Status returns the current status of the daemon.
func (d *Daemon) Status() StatusResponse {
	return StatusResponse{
		Running: true,
		Port:    d.cfg.Port,
		PID:     os.Getpid(),
		URL:     d.URL(),
	}
}

// watchTunnelURLChanges listens for tunnel URL changes and re-registers all
// webhooks that were registered with the old URL.
func (d *Daemon) watchTunnelURLChanges(ctx context.Context) {
	var prevURL string
	for {
		select {
		case newURL, ok := <-d.TunnelURLChanges():
			if !ok {
				return
			}
			if prevURL == "" || d.store == nil {
				prevURL = newURL
				continue
			}
			oldURL := prevURL
			prevURL = newURL
			d.reRegisterWebhooks(ctx, oldURL, newURL)
		case <-ctx.Done():
			return
		case <-d.stopCh:
			return
		}
	}
}

// reRegisterWebhooks deletes and re-creates all webhooks that used oldURL,
// updating the local store with the new URL. Errors are logged but not fatal.
func (d *Daemon) reRegisterWebhooks(ctx context.Context, oldURL, newURL string) {
	if d.store == nil {
		return
	}
	records, err := d.store.AllWebhooksByURL(oldURL)
	if err != nil {
		log.Printf("daemon: reRegisterWebhooks: list by URL: %v", err)
		return
	}
	for _, r := range records {
		// Delete old webhook from monday.
		if err := d.deleteMondayWebhook(ctx, r.ID); err != nil {
			log.Printf("daemon: reRegisterWebhooks: delete %s: %v", r.ID, err)
		}
		// Create new webhook with updated URL.
		webhookURL := newURL + "/webhook"
		newID, err := d.createMondayWebhook(ctx, r.BoardID, r.Event, webhookURL)
		if err != nil {
			log.Printf("daemon: reRegisterWebhooks: create for board %s: %v", r.BoardID, err)
			// Remove stale record from local store.
			if delErr := d.store.DeleteWebhook(r.ID); delErr != nil {
				log.Printf("daemon: reRegisterWebhooks: delete local %s: %v", r.ID, delErr)
			}
			continue
		}
		// Remove old record and insert updated one.
		if err := d.store.DeleteWebhook(r.ID); err != nil {
			log.Printf("daemon: reRegisterWebhooks: delete local %s: %v", r.ID, err)
		}
		newRec := &WebhookRecord{
			ID:      newID,
			BoardID: r.BoardID,
			Event:   r.Event,
			URL:     webhookURL,
		}
		if err := d.store.InsertWebhook(newRec); err != nil {
			log.Printf("daemon: reRegisterWebhooks: insert new record: %v", err)
		}
	}
}

// mondayAPIURLMu guards mondayAPIURL for test overrides.
var mondayAPIURLMu sync.RWMutex

// mondayAPIURL is the monday.com GraphQL endpoint. It is a variable so tests
// can override it to point at a local test server.
var mondayAPIURL = config.ResolveEndpoint(config.Config{})

// getMondayAPIURL returns the current monday.com API endpoint URL.
func getMondayAPIURL() string {
	mondayAPIURLMu.RLock()
	defer mondayAPIURLMu.RUnlock()
	return mondayAPIURL
}

// MondayAPIURL returns the current monday.com API endpoint URL.
func MondayAPIURL() string { return getMondayAPIURL() }

// SetMondayAPIURL sets the monday.com API endpoint URL. Intended for tests.
func SetMondayAPIURL(url string) {
	mondayAPIURLMu.Lock()
	mondayAPIURL = url
	mondayAPIURLMu.Unlock()
}

// createMondayWebhook registers a webhook with monday.com and returns the
// monday webhook ID.
func (d *Daemon) createMondayWebhook(ctx context.Context, boardID, event, url string) (string, error) {
	mutation := `mutation($boardId: ID!, $event: WebhookEventType!, $url: String!) {
		create_webhook(board_id: $boardId, event: $event, url: $url) { id }
	}`
	vars := map[string]any{
		"boardId": boardID,
		"event":   event,
		"url":     url,
	}
	body, err := json.Marshal(map[string]any{"query": mutation, "variables": vars})
	if err != nil {
		return "", fmt.Errorf("createMondayWebhook: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, getMondayAPIURL(), bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("createMondayWebhook: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", d.cfg.Token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("createMondayWebhook: request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("createMondayWebhook: read response: %w", err)
	}

	var result struct {
		Data struct {
			CreateWebhook struct {
				ID string `json:"id"`
			} `json:"create_webhook"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("createMondayWebhook: decode response: %w", err)
	}
	if len(result.Errors) > 0 {
		return "", fmt.Errorf("createMondayWebhook: API error: %s", result.Errors[0].Message)
	}
	if result.Data.CreateWebhook.ID == "" {
		return "", fmt.Errorf("createMondayWebhook: empty webhook ID in response")
	}
	return result.Data.CreateWebhook.ID, nil
}

// deleteMondayWebhook removes a webhook from monday.com by its ID.
func (d *Daemon) deleteMondayWebhook(ctx context.Context, id string) error {
	mutation := `mutation($id: ID!) { delete_webhook(id: $id) { id } }`
	vars := map[string]any{"id": id}
	body, err := json.Marshal(map[string]any{"query": mutation, "variables": vars})
	if err != nil {
		return fmt.Errorf("deleteMondayWebhook: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, getMondayAPIURL(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("deleteMondayWebhook: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", d.cfg.Token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("deleteMondayWebhook: request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("deleteMondayWebhook: read response: %w", err)
	}

	var result struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("deleteMondayWebhook: decode response: %w", err)
	}
	if len(result.Errors) > 0 {
		return fmt.Errorf("deleteMondayWebhook: API error: %s", result.Errors[0].Message)
	}
	return nil
}
