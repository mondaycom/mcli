package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
)

// Client is an IPC client that speaks to a running daemon over a Unix socket.
type Client struct {
	sockPath string
	http     *http.Client
}

// NewClient returns a Client that connects to the daemon socket at sockPath.
func NewClient(sockPath string) *Client {
	transport := &http.Transport{
		DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", sockPath)
		},
	}
	return &Client{
		sockPath: sockPath,
		http: &http.Client{
			Transport: transport,
		},
	}
}

// Status queries the daemon for its current status.
func (c *Client) Status() (*StatusResponse, error) {
	resp, err := c.http.Get("http://daemon/status")
	if err != nil {
		return nil, fmt.Errorf("Status: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Status: unexpected status %d", resp.StatusCode)
	}
	var sr StatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, fmt.Errorf("Status: decode: %w", err)
	}
	return &sr, nil
}

// Stop asks the daemon to shut down gracefully.
func (c *Client) Stop() error {
	resp, err := c.http.Post("http://daemon/stop", "application/json", nil)
	if err != nil {
		return fmt.Errorf("Stop: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Stop: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// Ping checks whether the daemon is reachable by calling /status.
func (c *Client) Ping() error {
	_, err := c.Status()
	if err != nil {
		return fmt.Errorf("Ping: %w", err)
	}
	return nil
}
