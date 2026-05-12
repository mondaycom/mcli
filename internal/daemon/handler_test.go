package daemon

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func openTestHandler(t *testing.T) (*WebhookHandler, *Store) {
	t.Helper()
	s, err := OpenStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return NewWebhookHandler(s), s
}

func TestHandler_Challenge(t *testing.T) {
	t.Parallel()
	h, _ := openTestHandler(t)

	body := `{"challenge":"abc123"}`
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(body))
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["challenge"] != "abc123" {
		t.Errorf("expected challenge=abc123, got %q", resp["challenge"])
	}
}

func TestHandler_Event(t *testing.T) {
	t.Parallel()
	h, store := openTestHandler(t)

	payload := `{
		"event": {"type": "create_item", "boardId": 12345},
		"webhook": {"id": "wh-99"}
	}`
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewBufferString(payload))
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("expected status=ok, got %q", resp["status"])
	}

	events, err := store.ListEvents(EventFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 stored event, got %d", len(events))
	}
	ev := events[0]
	if ev.EventType != "create_item" {
		t.Errorf("EventType: expected create_item, got %q", ev.EventType)
	}
	if ev.BoardID != "12345" {
		t.Errorf("BoardID: expected 12345, got %q", ev.BoardID)
	}
	if ev.WebhookID != "wh-99" {
		t.Errorf("WebhookID: expected wh-99, got %q", ev.WebhookID)
	}
}

func TestHandler_MethodNotAllowed(t *testing.T) {
	t.Parallel()
	h, _ := openTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/webhook", nil)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

func TestHandler_InvalidJSON(t *testing.T) {
	t.Parallel()
	h, _ := openTestHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader("{bad json"))
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}
