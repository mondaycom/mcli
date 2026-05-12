package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
)

// StatusResponse is the JSON shape returned by GET /status.
type StatusResponse struct {
	Running bool   `json:"running"`
	Port    int    `json:"port"`
	PID     int    `json:"pid"`
	Version string `json:"version,omitempty"`
	URL     string `json:"url,omitempty"`
}

// StopResponse is the JSON shape returned by POST /stop.
type StopResponse struct {
	Stopping bool `json:"stopping"`
}

// registerWebhookRequest is the body for POST /webhooks/register.
type registerWebhookRequest struct {
	BoardID string `json:"board_id"`
	Event   string `json:"event"`
}

// IPCServer serves the Unix socket control API for a running Daemon.
type IPCServer struct {
	sockPath string
	daemon   *Daemon
	server   *http.Server
}

// NewIPCServer creates an IPCServer that will listen on sockPath and delegate
// control operations to d.
func NewIPCServer(sockPath string, d *Daemon) *IPCServer {
	s := &IPCServer{sockPath: sockPath, daemon: d}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", s.handleStatus)
	mux.HandleFunc("POST /stop", s.handleStop)
	mux.HandleFunc("GET /webhooks", s.handleListWebhooks)
	mux.HandleFunc("POST /webhooks/register", s.handleRegisterWebhook)
	mux.HandleFunc("DELETE /webhooks/{id}", s.handleDeleteWebhook)
	// Legacy method-agnostic routes for backward compatibility with older clients.
	mux.HandleFunc("/status", s.handleStatus)
	mux.HandleFunc("/stop", s.handleStop)
	s.server = &http.Server{Handler: mux}
	return s
}

// Serve starts accepting connections on the Unix socket. It returns when the
// server is shut down.
func (s *IPCServer) Serve() error {
	ln, err := net.Listen("unix", s.sockPath)
	if err != nil {
		return err
	}
	return s.server.Serve(ln)
}

// Shutdown gracefully stops the IPC server.
func (s *IPCServer) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func (s *IPCServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	resp := s.daemon.Status()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *IPCServer) handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(StopResponse{Stopping: true})
	// Trigger shutdown asynchronously so we can flush the response first.
	go s.daemon.triggerStop()
}

func (s *IPCServer) handleListWebhooks(w http.ResponseWriter, r *http.Request) {
	store := s.daemon.store
	if store == nil {
		http.Error(w, "store not ready", http.StatusServiceUnavailable)
		return
	}

	boardID := r.URL.Query().Get("board_id")
	var (
		records []WebhookRecord
		err     error
	)
	if boardID != "" {
		records, err = store.ListWebhooksByBoard(boardID)
	} else {
		records, err = store.ListWebhooks()
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if records == nil {
		records = []WebhookRecord{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(records)
}

func (s *IPCServer) handleRegisterWebhook(w http.ResponseWriter, r *http.Request) {
	store := s.daemon.store
	if store == nil {
		http.Error(w, "store not ready", http.StatusServiceUnavailable)
		return
	}

	var req registerWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}
	if req.BoardID == "" || req.Event == "" {
		http.Error(w, "board_id and event are required", http.StatusBadRequest)
		return
	}

	daemonURL := s.daemon.URL()
	if daemonURL == "" {
		http.Error(w, "daemon has no external URL", http.StatusServiceUnavailable)
		return
	}

	webhookURL := daemonURL + "/webhook"
	mondayID, err := s.daemon.createMondayWebhook(r.Context(), req.BoardID, req.Event, webhookURL)
	if err != nil {
		http.Error(w, "failed to register webhook with monday.com", http.StatusBadGateway)
		return
	}

	record := &WebhookRecord{
		ID:      mondayID,
		BoardID: req.BoardID,
		Event:   req.Event,
		URL:     webhookURL,
	}
	if err := store.InsertWebhook(record); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(record)
}

func (s *IPCServer) handleDeleteWebhook(w http.ResponseWriter, r *http.Request) {
	store := s.daemon.store
	if store == nil {
		http.Error(w, "store not ready", http.StatusServiceUnavailable)
		return
	}

	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "webhook id is required", http.StatusBadRequest)
		return
	}

	if err := s.daemon.deleteMondayWebhook(r.Context(), id); err != nil {
		http.Error(w, "failed to delete webhook from monday.com", http.StatusBadGateway)
		return
	}

	if err := store.DeleteWebhook(id); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
