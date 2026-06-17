package daemon

import (
	"fmt"
	"time"
)

// WebhookRecord represents a monday.com webhook registered and persisted locally.
type WebhookRecord struct {
	ID        string `json:"id"` // monday's webhook ID
	BoardID   string `json:"board_id"`
	Event     string `json:"event"`      // WebhookEventType value
	URL       string `json:"url"`        // the URL registered with monday
	CreatedAt string `json:"created_at"` // RFC3339
}

const webhookSchema = `
CREATE TABLE IF NOT EXISTS webhooks (
    id TEXT PRIMARY KEY,
    board_id TEXT NOT NULL,
    event TEXT NOT NULL,
    url TEXT NOT NULL,
    created_at TEXT NOT NULL
);
`

// InsertWebhook persists w into the webhooks table. If w.CreatedAt is empty
// the current UTC time is used.
func (s *Store) InsertWebhook(w *WebhookRecord) error {
	if w.CreatedAt == "" {
		w.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := s.db.Exec(
		`INSERT INTO webhooks (id, board_id, event, url, created_at) VALUES (?, ?, ?, ?, ?)`,
		w.ID, w.BoardID, w.Event, w.URL, w.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("InsertWebhook: %w", err)
	}
	return nil
}

// ListWebhooks returns all registered webhooks.
func (s *Store) ListWebhooks() ([]WebhookRecord, error) {
	rows, err := s.db.Query(
		`SELECT id, board_id, event, url, created_at FROM webhooks ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("ListWebhooks: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanWebhookRows(rows)
}

// ListWebhooksByBoard returns webhooks registered for a specific board.
func (s *Store) ListWebhooksByBoard(boardID string) ([]WebhookRecord, error) {
	rows, err := s.db.Query(
		`SELECT id, board_id, event, url, created_at FROM webhooks WHERE board_id = ? ORDER BY created_at DESC`,
		boardID,
	)
	if err != nil {
		return nil, fmt.Errorf("ListWebhooksByBoard: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanWebhookRows(rows)
}

// DeleteWebhook removes the webhook with the given id from the local store.
func (s *Store) DeleteWebhook(id string) error {
	_, err := s.db.Exec(`DELETE FROM webhooks WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("DeleteWebhook: %w", err)
	}
	return nil
}

// UpdateWebhookURL replaces all webhook URLs matching oldURL with newURL.
func (s *Store) UpdateWebhookURL(oldURL, newURL string) error {
	_, err := s.db.Exec(`UPDATE webhooks SET url = ? WHERE url = ?`, newURL, oldURL)
	if err != nil {
		return fmt.Errorf("UpdateWebhookURL: %w", err)
	}
	return nil
}

// AllWebhooksByURL returns all webhooks registered with the given URL.
func (s *Store) AllWebhooksByURL(url string) ([]WebhookRecord, error) {
	rows, err := s.db.Query(
		`SELECT id, board_id, event, url, created_at FROM webhooks WHERE url = ? ORDER BY created_at DESC`,
		url,
	)
	if err != nil {
		return nil, fmt.Errorf("AllWebhooksByURL: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanWebhookRows(rows)
}

// scanWebhookRows scans *sql.Rows into a slice of WebhookRecord. The caller
// must defer rows.Close() before calling this function.
func scanWebhookRows(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]WebhookRecord, error) {
	var records []WebhookRecord
	for rows.Next() {
		var r WebhookRecord
		if err := rows.Scan(&r.ID, &r.BoardID, &r.Event, &r.URL, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan webhook row: %w", err)
		}
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("webhook rows error: %w", err)
	}
	return records, nil
}
