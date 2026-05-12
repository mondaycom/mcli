package daemon

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os/exec"
	"regexp"
	"sync"
	"time"
)

const (
	tunnelMaxRetries   = 5
	tunnelRestartDelay = 2 * time.Second
)

var reCloudflaredURL = regexp.MustCompile(`https://[^\s]+\.trycloudflare\.com`)

// ParseCloudflaredURL extracts the trycloudflare.com URL from a cloudflared
// log line, returning "" if none is found.
func ParseCloudflaredURL(line string) string {
	return reCloudflaredURL.FindString(line)
}

// Tunnel manages a cloudflared quick-tunnel subprocess.
type Tunnel struct {
	port    int
	mu      sync.RWMutex
	url     string
	urlCh   chan string
	done    chan struct{}
	process *exec.Cmd
}

// NewTunnel creates a Tunnel that will forward traffic to localhost:<port>.
func NewTunnel(port int) *Tunnel {
	return &Tunnel{
		port:  port,
		urlCh: make(chan string, 4),
		done:  make(chan struct{}),
	}
}

// Start launches the cloudflared subprocess and begins parsing its output for
// the assigned tunnel URL. It blocks until the first URL is assigned or an
// error occurs. Restart logic runs in the background until ctx is done or the
// maximum retry count is exhausted.
func (t *Tunnel) Start(ctx context.Context) error {
	if _, err := exec.LookPath("cloudflared"); err != nil {
		return fmt.Errorf("cloudflared not found in PATH. Install: https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/")
	}

	urlReady := make(chan error, 1)
	go t.runLoop(ctx, urlReady)

	select {
	case err := <-urlReady:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Stop kills the cloudflared subprocess if it is running.
func (t *Tunnel) Stop() error {
	t.mu.Lock()
	proc := t.process
	t.mu.Unlock()
	if proc != nil && proc.Process != nil {
		return proc.Process.Kill()
	}
	return nil
}

// URL returns the currently assigned tunnel URL, or "" if not yet available.
func (t *Tunnel) URL() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.url
}

// URLChanges returns a channel that receives tunnel URL values whenever the
// URL is assigned or changes. The channel is closed when the tunnel stops.
func (t *Tunnel) URLChanges() <-chan string {
	return t.urlCh
}

// setURL stores the URL and notifies listeners non-blocking.
func (t *Tunnel) setURL(u string) {
	t.mu.Lock()
	t.url = u
	t.mu.Unlock()
	select {
	case t.urlCh <- u:
	default:
	}
}

// runLoop starts and restarts cloudflared up to tunnelMaxRetries times.
// urlReady receives nil after the first successful URL assignment, or an error
// if the first attempt fails without producing a URL.
func (t *Tunnel) runLoop(ctx context.Context, urlReady chan<- error) {
	defer close(t.done)
	defer close(t.urlCh)

	firstRun := true

	for attempt := 0; attempt < tunnelMaxRetries; attempt++ {
		if ctx.Err() != nil {
			if firstRun {
				urlReady <- ctx.Err()
			}
			return
		}

		url, err := t.runOnce(ctx)

		if firstRun {
			firstRun = false
			switch {
			case url != "":
				t.setURL(url)
				urlReady <- nil
			case err != nil:
				urlReady <- err
				return
			default:
				urlReady <- fmt.Errorf("cloudflared exited without providing a URL")
				return
			}
		} else if url != "" && url != t.URL() {
			t.setURL(url)
		}

		if ctx.Err() != nil {
			return
		}

		log.Printf("tunnel: cloudflared exited (attempt %d/%d), restarting in %s",
			attempt+1, tunnelMaxRetries, tunnelRestartDelay)

		select {
		case <-time.After(tunnelRestartDelay):
		case <-ctx.Done():
			return
		}
	}

	log.Printf("tunnel: cloudflared failed %d times, giving up", tunnelMaxRetries)
}

// runOnce spawns a single cloudflared process, reads its stderr for the URL,
// and blocks until the process exits. It returns the URL found (if any) and
// any error.
func (t *Tunnel) runOnce(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "cloudflared", "tunnel",
		"--url", fmt.Sprintf("http://localhost:%d", t.port),
		"--no-autoupdate",
	)

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("tunnel: pipe: %w", err)
	}
	cmd.Stdout = io.Discard

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("tunnel: start: %w", err)
	}

	t.mu.Lock()
	t.process = cmd
	t.mu.Unlock()

	// Parse stderr line-by-line for the tunnel URL.
	urlFound := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			if u := ParseCloudflaredURL(scanner.Text()); u != "" {
				select {
				case urlFound <- u:
				default:
				}
			}
		}
	}()

	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()

	var foundURL string
	select {
	case u := <-urlFound:
		foundURL = u
	case <-waitCh:
		// Process exited before producing a URL.
		return "", nil
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		<-waitCh
		return "", ctx.Err()
	}

	// URL acquired; wait for the process to finish (ctx cancellation will
	// cause exec.CommandContext to kill it).
	select {
	case <-waitCh:
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		<-waitCh
	}
	return foundURL, nil
}
