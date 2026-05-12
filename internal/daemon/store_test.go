package daemon

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func makeEvent(boardID, eventType, webhookID string) *Event {
	return &Event{
		BoardID:   boardID,
		EventType: eventType,
		WebhookID: webhookID,
		Payload:   `{"raw":true}`,
	}
}

func TestStore_InsertAndList(t *testing.T) {
	t.Parallel()
	s := openTestStore(t)

	for i := range 3 {
		ev := makeEvent("board1", "type1", "wh1")
		// Spread timestamps so ordering is deterministic.
		ev.ReceivedAt = time.Now().UTC().Add(time.Duration(i) * time.Second).Format(time.RFC3339)
		if err := s.InsertEvent(ev); err != nil {
			t.Fatalf("InsertEvent[%d]: %v", i, err)
		}
	}

	events, err := s.ListEvents(EventFilter{})
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	// Verify newest-first ordering.
	for i := 1; i < len(events); i++ {
		if events[i-1].ReceivedAt < events[i].ReceivedAt {
			t.Errorf("events not in descending order: %s < %s",
				events[i-1].ReceivedAt, events[i].ReceivedAt)
		}
	}
}

func TestStore_ListWithFilters(t *testing.T) {
	t.Parallel()
	s := openTestStore(t)

	base := time.Now().UTC()

	insert := func(boardID, eventType string, readFlag bool, offset time.Duration) *Event {
		ev := &Event{
			BoardID:    boardID,
			EventType:  eventType,
			WebhookID:  "wh1",
			Payload:    `{}`,
			Read:       readFlag,
			ReceivedAt: base.Add(offset).Format(time.RFC3339),
		}
		if err := s.InsertEvent(ev); err != nil {
			t.Fatalf("InsertEvent: %v", err)
		}
		return ev
	}

	insert("board1", "create", false, 0)
	insert("board1", "update", true, time.Second)
	insert("board2", "create", false, 2*time.Second)
	insert("board2", "delete", false, 3*time.Second)

	t.Run("unread", func(t *testing.T) {
		evs, err := s.ListEvents(EventFilter{Unread: true})
		if err != nil {
			t.Fatal(err)
		}
		if len(evs) != 3 {
			t.Errorf("expected 3 unread, got %d", len(evs))
		}
	})

	t.Run("board_id", func(t *testing.T) {
		evs, err := s.ListEvents(EventFilter{BoardID: "board1"})
		if err != nil {
			t.Fatal(err)
		}
		if len(evs) != 2 {
			t.Errorf("expected 2 for board1, got %d", len(evs))
		}
	})

	t.Run("event_type", func(t *testing.T) {
		evs, err := s.ListEvents(EventFilter{EventType: "create"})
		if err != nil {
			t.Fatal(err)
		}
		if len(evs) != 2 {
			t.Errorf("expected 2 create events, got %d", len(evs))
		}
	})

	t.Run("since", func(t *testing.T) {
		since := base.Add(2 * time.Second).Format(time.RFC3339)
		evs, err := s.ListEvents(EventFilter{Since: since})
		if err != nil {
			t.Fatal(err)
		}
		if len(evs) != 2 {
			t.Errorf("expected 2 events since +2s, got %d", len(evs))
		}
	})

	t.Run("limit", func(t *testing.T) {
		evs, err := s.ListEvents(EventFilter{Limit: 2})
		if err != nil {
			t.Fatal(err)
		}
		if len(evs) != 2 {
			t.Errorf("expected 2 with limit=2, got %d", len(evs))
		}
	})
}

func TestStore_Ack(t *testing.T) {
	t.Parallel()
	s := openTestStore(t)

	ev := makeEvent("b1", "t1", "w1")
	if err := s.InsertEvent(ev); err != nil {
		t.Fatal(err)
	}

	n, err := s.CountUnread()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 unread before ack, got %d", n)
	}

	if err := s.AckEvent(ev.ID); err != nil {
		t.Fatalf("AckEvent: %v", err)
	}

	n, err = s.CountUnread()
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("expected 0 unread after ack, got %d", n)
	}
}

func TestStore_AckAll(t *testing.T) {
	t.Parallel()
	s := openTestStore(t)

	for i := range 5 {
		ev := makeEvent(fmt.Sprintf("b%d", i), "t1", "w1")
		if err := s.InsertEvent(ev); err != nil {
			t.Fatalf("InsertEvent[%d]: %v", i, err)
		}
	}

	if err := s.AckAll(); err != nil {
		t.Fatalf("AckAll: %v", err)
	}

	n, err := s.CountUnread()
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("expected 0 unread after AckAll, got %d", n)
	}
}

func TestStore_Prune(t *testing.T) {
	t.Parallel()
	s := openTestStore(t)

	base := time.Now().UTC()
	var inserted []EventID
	for i := range 5 {
		ev := &Event{
			BoardID:    "b1",
			EventType:  "t1",
			WebhookID:  "w1",
			Payload:    `{}`,
			ReceivedAt: base.Add(time.Duration(i) * time.Second).Format(time.RFC3339),
		}
		if err := s.InsertEvent(ev); err != nil {
			t.Fatalf("InsertEvent[%d]: %v", i, err)
		}
		inserted = append(inserted, ev.ID)
	}

	if err := s.Prune(3); err != nil {
		t.Fatalf("Prune: %v", err)
	}

	evs, err := s.ListEvents(EventFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 3 {
		t.Fatalf("expected 3 events after prune, got %d", len(evs))
	}

	// Verify the 2 oldest (inserted[0], inserted[1]) are gone.
	remaining := make(map[EventID]bool)
	for _, ev := range evs {
		remaining[ev.ID] = true
	}
	for _, id := range inserted[:2] {
		if remaining[id] {
			t.Errorf("event %s should have been pruned", id)
		}
	}
}

func TestStore_CountUnread(t *testing.T) {
	t.Parallel()
	s := openTestStore(t)

	n, err := s.CountUnread()
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("expected 0 unread on empty store, got %d", n)
	}

	for i := range 4 {
		ev := makeEvent("b1", "t1", "w1")
		ev.Read = i%2 == 0 // alternate read/unread
		if err := s.InsertEvent(ev); err != nil {
			t.Fatal(err)
		}
	}

	n, err = s.CountUnread()
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("expected 2 unread, got %d", n)
	}
}
