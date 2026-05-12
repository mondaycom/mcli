package daemon

import (
	"testing"
	"time"
)

func makeWebhook(id, boardID, event, url string) *WebhookRecord {
	return &WebhookRecord{
		ID:        id,
		BoardID:   boardID,
		Event:     event,
		URL:       url,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
}

func TestWebhookStore_InsertAndList(t *testing.T) {
	t.Parallel()
	s := openTestStore(t)

	recs := []*WebhookRecord{
		makeWebhook("wh1", "board1", "create_item", "https://example.com/webhook"),
		makeWebhook("wh2", "board1", "create_update", "https://example.com/webhook"),
		makeWebhook("wh3", "board2", "create_item", "https://example.com/webhook"),
	}
	for _, r := range recs {
		if err := s.InsertWebhook(r); err != nil {
			t.Fatalf("InsertWebhook %s: %v", r.ID, err)
		}
	}

	all, err := s.ListWebhooks()
	if err != nil {
		t.Fatalf("ListWebhooks: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 webhooks, got %d", len(all))
	}
}

func TestWebhookStore_ListByBoard(t *testing.T) {
	t.Parallel()
	s := openTestStore(t)

	if err := s.InsertWebhook(makeWebhook("wh1", "board1", "create_item", "https://a.com/webhook")); err != nil {
		t.Fatalf("InsertWebhook: %v", err)
	}
	if err := s.InsertWebhook(makeWebhook("wh2", "board1", "create_update", "https://a.com/webhook")); err != nil {
		t.Fatalf("InsertWebhook: %v", err)
	}
	if err := s.InsertWebhook(makeWebhook("wh3", "board2", "create_item", "https://a.com/webhook")); err != nil {
		t.Fatalf("InsertWebhook: %v", err)
	}

	board1, err := s.ListWebhooksByBoard("board1")
	if err != nil {
		t.Fatalf("ListWebhooksByBoard: %v", err)
	}
	if len(board1) != 2 {
		t.Fatalf("expected 2 webhooks for board1, got %d", len(board1))
	}

	board2, err := s.ListWebhooksByBoard("board2")
	if err != nil {
		t.Fatalf("ListWebhooksByBoard board2: %v", err)
	}
	if len(board2) != 1 {
		t.Fatalf("expected 1 webhook for board2, got %d", len(board2))
	}

	empty, err := s.ListWebhooksByBoard("board99")
	if err != nil {
		t.Fatalf("ListWebhooksByBoard empty: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected 0 webhooks for unknown board, got %d", len(empty))
	}
}

func TestWebhookStore_Delete(t *testing.T) {
	t.Parallel()
	s := openTestStore(t)

	if err := s.InsertWebhook(makeWebhook("wh1", "board1", "create_item", "https://a.com/webhook")); err != nil {
		t.Fatalf("InsertWebhook: %v", err)
	}
	if err := s.InsertWebhook(makeWebhook("wh2", "board1", "create_update", "https://a.com/webhook")); err != nil {
		t.Fatalf("InsertWebhook: %v", err)
	}

	if err := s.DeleteWebhook("wh1"); err != nil {
		t.Fatalf("DeleteWebhook: %v", err)
	}

	all, err := s.ListWebhooks()
	if err != nil {
		t.Fatalf("ListWebhooks after delete: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 webhook after delete, got %d", len(all))
	}
	if all[0].ID != "wh2" {
		t.Errorf("expected remaining webhook wh2, got %s", all[0].ID)
	}
}

func TestWebhookStore_UpdateURL(t *testing.T) {
	t.Parallel()
	s := openTestStore(t)

	oldURL := "https://old.example.com/webhook"
	newURL := "https://new.example.com/webhook"

	if err := s.InsertWebhook(makeWebhook("wh1", "board1", "create_item", oldURL)); err != nil {
		t.Fatalf("InsertWebhook: %v", err)
	}
	if err := s.InsertWebhook(makeWebhook("wh2", "board1", "create_update", oldURL)); err != nil {
		t.Fatalf("InsertWebhook: %v", err)
	}
	if err := s.InsertWebhook(makeWebhook("wh3", "board2", "create_item", "https://other.com/webhook")); err != nil {
		t.Fatalf("InsertWebhook: %v", err)
	}

	if err := s.UpdateWebhookURL(oldURL, newURL); err != nil {
		t.Fatalf("UpdateWebhookURL: %v", err)
	}

	// wh1 and wh2 should have new URL.
	updated, err := s.AllWebhooksByURL(newURL)
	if err != nil {
		t.Fatalf("AllWebhooksByURL: %v", err)
	}
	if len(updated) != 2 {
		t.Fatalf("expected 2 webhooks with new URL, got %d", len(updated))
	}

	// wh3 should keep its old URL.
	unchanged, err := s.AllWebhooksByURL("https://other.com/webhook")
	if err != nil {
		t.Fatalf("AllWebhooksByURL other: %v", err)
	}
	if len(unchanged) != 1 {
		t.Fatalf("expected 1 webhook at other URL, got %d", len(unchanged))
	}
}

func TestWebhookStore_AllWebhooksByURL(t *testing.T) {
	t.Parallel()
	s := openTestStore(t)

	url1 := "https://url1.com/webhook"
	url2 := "https://url2.com/webhook"

	if err := s.InsertWebhook(makeWebhook("wh1", "board1", "create_item", url1)); err != nil {
		t.Fatalf("InsertWebhook: %v", err)
	}
	if err := s.InsertWebhook(makeWebhook("wh2", "board1", "create_update", url1)); err != nil {
		t.Fatalf("InsertWebhook: %v", err)
	}
	if err := s.InsertWebhook(makeWebhook("wh3", "board2", "create_item", url2)); err != nil {
		t.Fatalf("InsertWebhook: %v", err)
	}

	byURL1, err := s.AllWebhooksByURL(url1)
	if err != nil {
		t.Fatalf("AllWebhooksByURL url1: %v", err)
	}
	if len(byURL1) != 2 {
		t.Fatalf("expected 2 webhooks at url1, got %d", len(byURL1))
	}

	byURL2, err := s.AllWebhooksByURL(url2)
	if err != nil {
		t.Fatalf("AllWebhooksByURL url2: %v", err)
	}
	if len(byURL2) != 1 {
		t.Fatalf("expected 1 webhook at url2, got %d", len(byURL2))
	}

	byMissing, err := s.AllWebhooksByURL("https://missing.com/webhook")
	if err != nil {
		t.Fatalf("AllWebhooksByURL missing: %v", err)
	}
	if len(byMissing) != 0 {
		t.Fatalf("expected 0 webhooks at missing URL, got %d", len(byMissing))
	}
}

func TestWebhookStore_CreatedAtDefaulted(t *testing.T) {
	t.Parallel()
	s := openTestStore(t)

	r := &WebhookRecord{
		ID:      "wh1",
		BoardID: "b1",
		Event:   "create_item",
		URL:     "https://a.com/webhook",
		// CreatedAt intentionally left empty.
	}
	if err := s.InsertWebhook(r); err != nil {
		t.Fatalf("InsertWebhook: %v", err)
	}
	if r.CreatedAt == "" {
		t.Error("expected CreatedAt to be populated")
	}

	all, err := s.ListWebhooks()
	if err != nil {
		t.Fatalf("ListWebhooks: %v", err)
	}
	if len(all) != 1 || all[0].CreatedAt == "" {
		t.Errorf("persisted webhook missing created_at: %+v", all)
	}
}
