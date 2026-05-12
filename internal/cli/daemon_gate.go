package cli

import (
	"github.com/mondaycom/mcli/internal/daemon"
	"github.com/mondaycom/mcli/internal/errs"
)

// requireDaemon verifies the daemon is reachable and returns a connected client.
// Returns errs.DaemonRequired when the socket does not exist or the daemon does
// not respond to a ping.
func requireDaemon() (*daemon.Client, error) {
	sockPath := resolveConfigDir() + "/daemon.sock"
	c := daemon.NewClient(sockPath)
	if err := c.Ping(); err != nil {
		return nil, errs.DaemonRequired("daemon is not running (start with 'mcli daemon start')")
	}
	return c, nil
}
