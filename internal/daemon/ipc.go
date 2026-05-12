package daemon

import (
	"context"
	"encoding/json"
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
