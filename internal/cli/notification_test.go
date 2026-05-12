package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mondaycom/mcli/internal/daemon"
)

// execNotification runs 'mcli notification <args>' and returns stdout + error.
func execNotification(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	orig := globals
	globals = GlobalFlags{Config: filepath.Join(dir, "config.yaml")}
	defer func() { globals = orig }()

	var buf bytes.Buffer
	cmd := newNotificationCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

// startDaemonForNotifications starts a daemon and waits until it is reachable.
// It returns the dir, the daemon instance (so tests can call d.Store()), and a
// cancel func. The daemon uses a static external URL so no cloudflared is needed.
func startDaemonForNotifications(t *testing.T) (dir string, d *daemon.Daemon, cancel func()) {
	t.Helper()
	dir = shortTempDir(t)
	port := freePort(t)

	cfg := daemon.Config{
		Port:        port,
		ConfigDir:   dir,
		ExternalURL: "https://test.example.com",
	}
	d = daemon.New(cfg)
	ctx, cancelFn := context.WithCancel(context.Background())
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
	if err := c.Ping(); err != nil {
		t.Fatalf("daemon not reachable: %v", err)
	}

	cancel = cancelFn
	return dir, d, cancel
}

// seedEvent inserts a test event into the daemon store and returns its ID.
func seedEvent(t *testing.T, d *daemon.Daemon, boardID, eventType string) daemon.EventID {
	t.Helper()
	store := d.Store()
	if store == nil {
		t.Fatal("daemon store is nil; daemon may not have started yet")
	}
	ev := &daemon.Event{
		BoardID:   boardID,
		EventType: eventType,
		WebhookID: "wh-test",
		Payload:   `{"event":{"type":"` + eventType + `"}}`,
	}
	if err := store.InsertEvent(ev); err != nil {
		t.Fatalf("seedEvent: %v", err)
	}
	return ev.ID
}

func TestNotificationCount_NoDaemon(t *testing.T) {
	dir := t.TempDir()

	_, err := execNotification(t, dir, "count")
	if err == nil {
		t.Fatal("expected error when daemon not running")
	}
	if code := errsCode(err); code != "DAEMON_REQUIRED" {
		t.Errorf("expected DAEMON_REQUIRED, got %q", code)
	}
}

func TestNotificationList_NoDaemon(t *testing.T) {
	dir := t.TempDir()

	_, err := execNotification(t, dir, "list")
	if err == nil {
		t.Fatal("expected error when daemon not running")
	}
	if code := errsCode(err); code != "DAEMON_REQUIRED" {
		t.Errorf("expected DAEMON_REQUIRED, got %q", code)
	}
}

func TestNotificationAck_NoDaemon(t *testing.T) {
	dir := t.TempDir()

	_, err := execNotification(t, dir, "ack", "some-id")
	if err == nil {
		t.Fatal("expected error when daemon not running")
	}
	if code := errsCode(err); code != "DAEMON_REQUIRED" {
		t.Errorf("expected DAEMON_REQUIRED, got %q", code)
	}
}

func TestNotificationAck_NoArgs(t *testing.T) {
	dir, _, cancel := startDaemonForNotifications(t)
	defer cancel()

	_, err := execNotification(t, dir, "ack")
	if err == nil {
		t.Fatal("expected error when neither id nor --all provided")
	}
	if code := errsCode(err); code != "USAGE" {
		t.Errorf("expected USAGE, got %q", code)
	}
}

func TestNotificationCount_Empty(t *testing.T) {
	dir, _, cancel := startDaemonForNotifications(t)
	defer cancel()

	out, err := execNotification(t, dir, "count")
	if err != nil {
		t.Fatalf("notification count: %v", err)
	}

	var result map[string]int
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	if result["unread"] != 0 {
		t.Errorf("expected unread=0, got %d", result["unread"])
	}
}

func TestNotificationCount_WithEvents(t *testing.T) {
	dir, d, cancel := startDaemonForNotifications(t)
	defer cancel()

	seedEvent(t, d, "board1", "create_item")
	seedEvent(t, d, "board1", "create_item")

	out, err := execNotification(t, dir, "count")
	if err != nil {
		t.Fatalf("notification count: %v", err)
	}

	var result map[string]int
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	if result["unread"] != 2 {
		t.Errorf("expected unread=2, got %d", result["unread"])
	}
}

func TestNotificationList_Empty(t *testing.T) {
	dir, _, cancel := startDaemonForNotifications(t)
	defer cancel()

	out, err := execNotification(t, dir, "list")
	if err != nil {
		t.Fatalf("notification list: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	items, ok := result["items"].([]any)
	if !ok || len(items) != 0 {
		t.Errorf("expected empty items, got %v", result["items"])
	}
	if result["unread_count"] != float64(0) {
		t.Errorf("expected unread_count=0, got %v", result["unread_count"])
	}
}

func TestNotificationList_AllByDefault(t *testing.T) {
	dir, d, cancel := startDaemonForNotifications(t)
	defer cancel()

	seedEvent(t, d, "board1", "create_item")
	seedEvent(t, d, "board2", "change_column_value")

	out, err := execNotification(t, dir, "list")
	if err != nil {
		t.Fatalf("notification list: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	items, ok := result["items"].([]any)
	if !ok || len(items) != 2 {
		t.Errorf("expected 2 items, got %v", result["items"])
	}
	if result["unread_count"] != float64(2) {
		t.Errorf("expected unread_count=2, got %v", result["unread_count"])
	}
}

func TestNotificationList_UnreadFilter(t *testing.T) {
	dir, d, cancel := startDaemonForNotifications(t)
	defer cancel()

	id1 := seedEvent(t, d, "board1", "create_item")
	seedEvent(t, d, "board1", "create_item")

	// Ack first event directly via store.
	if err := d.Store().AckEvent(id1); err != nil {
		t.Fatalf("ack event: %v", err)
	}

	out, err := execNotification(t, dir, "list", "--unread")
	if err != nil {
		t.Fatalf("notification list --unread: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	items, ok := result["items"].([]any)
	if !ok || len(items) != 1 {
		t.Errorf("expected 1 unread item, got %v", result["items"])
	}
}

func TestNotificationList_BoardFilter(t *testing.T) {
	dir, d, cancel := startDaemonForNotifications(t)
	defer cancel()

	seedEvent(t, d, "board1", "create_item")
	seedEvent(t, d, "board2", "create_item")
	seedEvent(t, d, "board1", "change_column_value")

	out, err := execNotification(t, dir, "list", "--board", "board1")
	if err != nil {
		t.Fatalf("notification list --board board1: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	items, ok := result["items"].([]any)
	if !ok || len(items) != 2 {
		t.Errorf("expected 2 items for board1, got %v", result["items"])
	}
}

func TestNotificationList_LimitFilter(t *testing.T) {
	dir, d, cancel := startDaemonForNotifications(t)
	defer cancel()

	for i := 0; i < 5; i++ {
		seedEvent(t, d, "board1", "create_item")
	}

	out, err := execNotification(t, dir, "list", "--limit", "3")
	if err != nil {
		t.Fatalf("notification list --limit 3: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	items, ok := result["items"].([]any)
	if !ok || len(items) != 3 {
		t.Errorf("expected 3 items with --limit 3, got %v", result["items"])
	}
}

func TestNotificationAck_Single(t *testing.T) {
	dir, d, cancel := startDaemonForNotifications(t)
	defer cancel()

	id := seedEvent(t, d, "board1", "create_item")

	out, err := execNotification(t, dir, "ack", string(id))
	if err != nil {
		t.Fatalf("notification ack: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	if result["acknowledged"] != true {
		t.Errorf("expected acknowledged=true, got %v", result["acknowledged"])
	}

	// Verify count is now 0.
	countOut, err := execNotification(t, dir, "count")
	if err != nil {
		t.Fatalf("notification count after ack: %v", err)
	}
	var countResult map[string]int
	if err := json.Unmarshal([]byte(strings.TrimSpace(countOut)), &countResult); err != nil {
		t.Fatalf("parse count output: %v", err)
	}
	if countResult["unread"] != 0 {
		t.Errorf("expected unread=0 after ack, got %d", countResult["unread"])
	}
}

func TestNotificationAck_All(t *testing.T) {
	dir, d, cancel := startDaemonForNotifications(t)
	defer cancel()

	seedEvent(t, d, "board1", "create_item")
	seedEvent(t, d, "board2", "create_item")
	seedEvent(t, d, "board3", "create_item")

	out, err := execNotification(t, dir, "ack", "--all")
	if err != nil {
		t.Fatalf("notification ack --all: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, out)
	}
	if result["acknowledged"] != true {
		t.Errorf("expected acknowledged=true, got %v", result["acknowledged"])
	}

	// Verify all are read.
	countOut, err := execNotification(t, dir, "count")
	if err != nil {
		t.Fatalf("notification count after ack --all: %v", err)
	}
	var countResult map[string]int
	if err := json.Unmarshal([]byte(strings.TrimSpace(countOut)), &countResult); err != nil {
		t.Fatalf("parse count output: %v", err)
	}
	if countResult["unread"] != 0 {
		t.Errorf("expected unread=0 after ack --all, got %d", countResult["unread"])
	}
}
