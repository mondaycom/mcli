package daemon

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// EventID is a unique identifier for a stored webhook event.
type EventID string

// Event represents a single webhook event persisted in the store.
type Event struct {
	ID         EventID `json:"id"`
	ReceivedAt string  `json:"received_at"` // RFC3339
	BoardID    string  `json:"board_id"`
	EventType  string  `json:"event_type"`
	WebhookID  string  `json:"webhook_id"`
	Payload    string  `json:"payload"` // raw JSON
	Read       bool    `json:"read"`
}

// EventFilter controls which events are returned by ListEvents.
type EventFilter struct {
	Unread    bool
	BoardID   string
	EventType string
	Since     string // RFC3339
	Limit     int
}

// Store is a SQLite-backed event store.
type Store struct {
	db *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS events (
    id TEXT PRIMARY KEY,
    received_at TEXT NOT NULL,
    board_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    webhook_id TEXT NOT NULL,
    payload TEXT NOT NULL,
    read INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_events_unread ON events(read, received_at);
CREATE INDEX IF NOT EXISTS idx_events_board ON events(board_id);
`

// OpenStore opens (or creates) the SQLite database at dbPath and applies the
// schema migration. Callers must call Close when done.
func OpenStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("OpenStore: open db: %w", err)
	}
	// SQLite is single-writer; keep one connection in the pool to avoid locking.
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("OpenStore: migrate events: %w", err)
	}
	if _, err := db.Exec(webhookSchema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("OpenStore: migrate webhooks: %w", err)
	}
	return &Store{db: db}, nil
}

// Close releases all database resources.
func (s *Store) Close() error {
	return s.db.Close()
}

// newEventID generates a random 16-byte hex event ID.
func newEventID() (EventID, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("newEventID: %w", err)
	}
	return EventID(hex.EncodeToString(b)), nil
}

// InsertEvent persists e into the store. If e.ID is empty a new ID is
// generated; if e.ReceivedAt is empty the current UTC time is used.
func (s *Store) InsertEvent(e *Event) error {
	if e.ID == "" {
		id, err := newEventID()
		if err != nil {
			return err
		}
		e.ID = id
	}
	if e.ReceivedAt == "" {
		e.ReceivedAt = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := s.db.Exec(
		`INSERT INTO events (id, received_at, board_id, event_type, webhook_id, payload, read)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		string(e.ID), e.ReceivedAt, e.BoardID, e.EventType, e.WebhookID, e.Payload, boolToInt(e.Read),
	)
	if err != nil {
		return fmt.Errorf("InsertEvent: %w", err)
	}
	return nil
}

// ListEvents returns events matching f, ordered newest-first.
func (s *Store) ListEvents(f EventFilter) ([]Event, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = 100
	}

	var conditions []string
	var args []any

	if f.Unread {
		conditions = append(conditions, "read = 0")
	}
	if f.BoardID != "" {
		conditions = append(conditions, "board_id = ?")
		args = append(args, f.BoardID)
	}
	if f.EventType != "" {
		conditions = append(conditions, "event_type = ?")
		args = append(args, f.EventType)
	}
	if f.Since != "" {
		conditions = append(conditions, "received_at >= ?")
		args = append(args, f.Since)
	}

	q := "SELECT id, received_at, board_id, event_type, webhook_id, payload, read FROM events"
	if len(conditions) > 0 {
		q += " WHERE " + strings.Join(conditions, " AND ")
	}
	q += " ORDER BY received_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("ListEvents: query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var events []Event
	for rows.Next() {
		var ev Event
		var readInt int
		if err := rows.Scan(&ev.ID, &ev.ReceivedAt, &ev.BoardID, &ev.EventType,
			&ev.WebhookID, &ev.Payload, &readInt); err != nil {
			return nil, fmt.Errorf("ListEvents: scan: %w", err)
		}
		ev.Read = readInt != 0
		events = append(events, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ListEvents: rows: %w", err)
	}
	return events, nil
}

// CountUnread returns the number of events with read=false.
func (s *Store) CountUnread() (int, error) {
	var n int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM events WHERE read = 0").Scan(&n); err != nil {
		return 0, fmt.Errorf("CountUnread: %w", err)
	}
	return n, nil
}

// AckEvent marks the event with the given id as read.
func (s *Store) AckEvent(id EventID) error {
	_, err := s.db.Exec("UPDATE events SET read = 1 WHERE id = ?", string(id))
	if err != nil {
		return fmt.Errorf("AckEvent: %w", err)
	}
	return nil
}

// AckAll marks all events as read.
func (s *Store) AckAll() error {
	_, err := s.db.Exec("UPDATE events SET read = 1")
	if err != nil {
		return fmt.Errorf("AckAll: %w", err)
	}
	return nil
}

// Prune deletes the oldest events so that at most maxEvents remain.
func (s *Store) Prune(maxEvents int) error {
	_, err := s.db.Exec(
		`DELETE FROM events WHERE id NOT IN (
			SELECT id FROM events ORDER BY received_at DESC LIMIT ?
		)`, maxEvents,
	)
	if err != nil {
		return fmt.Errorf("Prune: %w", err)
	}
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
