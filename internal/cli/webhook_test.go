package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mondaycom/mcli/internal/daemon"
)

// shortTempDir creates a temp dir under /tmp to avoid long socket paths on macOS
// (Unix domain sockets are limited to ~104 chars).
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "mcli-wh-")
	if err != nil {
		t.Fatalf("shortTempDir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// startDaemonWithMondayMock starts a daemon whose IPC calls flow to a real
// Store, and whose monday.com API calls are redirected to mondaySrv.
// The daemon uses externalURL as its public URL so no cloudflared is needed.
// Returns the dir used, a client, and a cancel func.
func startDaemonWithMondayMock(t *testing.T, mondaySrv *httptest.Server) (dir string, c *daemon.Client, cancel func()) {
	t.Helper()
	dir = shortTempDir(t)
	port := freePort(t)

	// Override the daemon's monday API URL to point at our mock.
	origURL := daemon.MondayAPIURL()
	daemon.SetMondayAPIURL(mondaySrv.URL)
	t.Cleanup(func() { daemon.SetMondayAPIURL(origURL) })

	cfg := daemon.Config{
		Port:        port,
		ConfigDir:   dir,
		ExternalURL: "https://test.example.com",
		Token:       "test-token",
	}
	d := daemon.New(cfg)
	ctx, cancelFn := context.WithCancel(context.Background())
	go func() { _ = d.Start(ctx) }()

	sockPath := filepath.Join(dir, "daemon.sock")
	c = daemon.NewClient(sockPath)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err := c.Ping(); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := c.Ping(); err != nil {
		t.Fatalf("daemon not reachable: %v", err)
	}
	cancel = cancelFn
	return dir, c, cancel
}

// execWebhook runs 'mcli webhook <args>' and returns stdout + error.
func execWebhook(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	orig := globals
	globals = GlobalFlags{Config: filepath.Join(dir, "config.yaml")}
	defer func() { globals = orig }()

	var buf bytes.Buffer
	cmd := newWebhookCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

// mondayCreateWebhookHandler returns a handler that responds with a create_webhook response.
func mondayCreateWebhookHandler(t *testing.T, webhookID string) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST to monday API, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		resp := `{"data":{"create_webhook":{"id":"` + webhookID + `"}}}`
		_, _ = w.Write([]byte(resp))
	})
}

// TestWebhookEvents does not need a daemon.
func TestWebhookEvents(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	out, err := execWebhook(t, dir, "events")
	if err != nil {
		t.Fatalf("webhook events: %v", err)
	}

	var types []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &types); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	if len(types) != 21 {
		t.Errorf("expected 21 event types, got %d", len(types))
	}
	// Verify sorted order.
	for i := 1; i < len(types); i++ {
		if types[i] < types[i-1] {
			t.Errorf("types not sorted: %s before %s", types[i-1], types[i])
		}
	}
	// Spot-check a few.
	found := func(name string) bool {
		for _, e := range types {
			if e == name {
				return true
			}
		}
		return false
	}
	for _, want := range []string{"create_item", "change_column_value", "item_deleted"} {
		if !found(want) {
			t.Errorf("event type %q not in list", want)
		}
	}
}

func TestWebhookCreate(t *testing.T) {
	mondaySrv := httptest.NewServer(mondayCreateWebhookHandler(t, "mwh-42"))
	defer mondaySrv.Close()

	dir, _, cancel := startDaemonWithMondayMock(t, mondaySrv)
	defer cancel()

	out, err := execWebhook(t, dir, "create", "--board", "board123", "--event", "create_item")
	if err != nil {
		t.Fatalf("webhook create: %v", err)
	}

	var rec map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &rec); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	if rec["id"] != "mwh-42" {
		t.Errorf("expected id=mwh-42, got %v", rec["id"])
	}
	if rec["board_id"] != "board123" {
		t.Errorf("expected board_id=board123, got %v", rec["board_id"])
	}
	if rec["event"] != "create_item" {
		t.Errorf("expected event=create_item, got %v", rec["event"])
	}
}

func TestWebhookCreate_MissingBoard(t *testing.T) {
	dir := t.TempDir()

	_, err := execWebhook(t, dir, "create", "--event", "create_item")
	if err == nil {
		t.Fatal("expected error for missing --board")
	}
}

func TestWebhookCreate_MissingEvent(t *testing.T) {
	dir := t.TempDir()

	_, err := execWebhook(t, dir, "create", "--board", "board1")
	if err == nil {
		t.Fatal("expected error for missing --event")
	}
}

func TestWebhookCreate_InvalidEvent(t *testing.T) {
	dir := t.TempDir()

	_, err := execWebhook(t, dir, "create", "--board", "board1", "--event", "not_a_real_event")
	if err == nil {
		t.Fatal("expected error for invalid event type")
	}
	if !strings.Contains(err.Error(), "not_a_real_event") {
		t.Errorf("error should mention the invalid event type: %v", err)
	}
}

func TestWebhookList(t *testing.T) {
	mondaySrv := httptest.NewServer(mondayCreateWebhookHandler(t, "mwh-list"))
	defer mondaySrv.Close()

	dir, _, cancel := startDaemonWithMondayMock(t, mondaySrv)
	defer cancel()

	// Create a webhook first.
	if _, err := execWebhook(t, dir, "create", "--board", "board1", "--event", "create_item"); err != nil {
		t.Fatalf("create webhook: %v", err)
	}

	out, err := execWebhook(t, dir, "list")
	if err != nil {
		t.Fatalf("webhook list: %v", err)
	}

	var recs []map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &recs); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	if len(recs) != 1 {
		t.Fatalf("expected 1 webhook, got %d", len(recs))
	}
}

func TestWebhookList_FilterByBoard(t *testing.T) {
	// Return different IDs on successive calls to create two webhooks.
	callCount := 0
	mondaySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		id := "mwh-a"
		if callCount > 1 {
			id = "mwh-b"
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"create_webhook":{"id":"` + id + `"}}}`))
	}))
	defer mondaySrv.Close()

	dir, _, cancel := startDaemonWithMondayMock(t, mondaySrv)
	defer cancel()

	if _, err := execWebhook(t, dir, "create", "--board", "board1", "--event", "create_item"); err != nil {
		t.Fatalf("create webhook board1: %v", err)
	}
	if _, err := execWebhook(t, dir, "create", "--board", "board2", "--event", "create_update"); err != nil {
		t.Fatalf("create webhook board2: %v", err)
	}

	out, err := execWebhook(t, dir, "list", "--board", "board1")
	if err != nil {
		t.Fatalf("webhook list --board board1: %v", err)
	}

	var recs []map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &recs); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	if len(recs) != 1 {
		t.Fatalf("expected 1 webhook for board1, got %d", len(recs))
	}
	if recs[0]["board_id"] != "board1" {
		t.Errorf("expected board_id=board1, got %v", recs[0]["board_id"])
	}
}

func TestWebhookDelete(t *testing.T) {
	callCount := 0
	mondaySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			// First call: create_webhook
			_, _ = w.Write([]byte(`{"data":{"create_webhook":{"id":"mwh-del"}}}`))
		} else {
			// Second call: delete_webhook
			_, _ = w.Write([]byte(`{"data":{"delete_webhook":{"id":"mwh-del"}}}`))
		}
	}))
	defer mondaySrv.Close()

	dir, _, cancel := startDaemonWithMondayMock(t, mondaySrv)
	defer cancel()

	// Create the webhook.
	if _, err := execWebhook(t, dir, "create", "--board", "board1", "--event", "create_item"); err != nil {
		t.Fatalf("create webhook: %v", err)
	}

	// Delete it.
	out, err := execWebhook(t, dir, "delete", "mwh-del")
	if err != nil {
		t.Fatalf("webhook delete: %v", err)
	}
	if !strings.Contains(out, `"deleted"`) {
		t.Errorf("expected deleted in response: %s", out)
	}

	// Verify it's gone.
	listOut, err := execWebhook(t, dir, "list")
	if err != nil {
		t.Fatalf("webhook list after delete: %v", err)
	}
	var recs []map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(listOut)), &recs); err != nil {
		t.Fatalf("parse list output: %v", err)
	}
	if len(recs) != 0 {
		t.Fatalf("expected 0 webhooks after delete, got %d", len(recs))
	}
}

func TestWebhookCreate_NoDaemon(t *testing.T) {
	dir := t.TempDir()

	_, err := execWebhook(t, dir, "create", "--board", "board1", "--event", "create_item")
	if err == nil {
		t.Fatal("expected error when daemon not running")
	}
	code := errsCode(err)
	if code != "DAEMON_REQUIRED" {
		t.Errorf("expected DAEMON_REQUIRED error code, got %q", code)
	}
}

func TestWebhookList_NoDaemon(t *testing.T) {
	dir := t.TempDir()

	_, err := execWebhook(t, dir, "list")
	if err == nil {
		t.Fatal("expected error when daemon not running")
	}
	code := errsCode(err)
	if code != "DAEMON_REQUIRED" {
		t.Errorf("expected DAEMON_REQUIRED error code, got %q", code)
	}
}

func TestWebhookDelete_NoDaemon(t *testing.T) {
	dir := t.TempDir()

	_, err := execWebhook(t, dir, "delete", "some-id")
	if err == nil {
		t.Fatal("expected error when daemon not running")
	}
	code := errsCode(err)
	if code != "DAEMON_REQUIRED" {
		t.Errorf("expected DAEMON_REQUIRED error code, got %q", code)
	}
}
