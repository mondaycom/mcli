package daemon

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// WebhookHandler handles inbound monday.com webhook HTTP requests.
type WebhookHandler struct {
	store *Store
}

// NewWebhookHandler constructs a WebhookHandler backed by store.
func NewWebhookHandler(store *Store) *WebhookHandler {
	return &WebhookHandler{store: store}
}

// ServeHTTP implements http.Handler.
func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	// Parse into a generic map to inspect keys.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Monday.com webhook verification challenge.
	if challengeRaw, ok := raw["challenge"]; ok {
		var token string
		if err := json.Unmarshal(challengeRaw, &token); err == nil {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"challenge": token})
			return
		}
	}

	// Parse event fields defensively.
	var boardID, eventType, webhookID string

	if eventRaw, ok := raw["event"]; ok {
		var eventObj struct {
			Type    string  `json:"type"`
			BoardID float64 `json:"boardId"`
		}
		if err := json.Unmarshal(eventRaw, &eventObj); err == nil {
			eventType = eventObj.Type
			if eventObj.BoardID != 0 {
				boardID = fmt.Sprintf("%.0f", eventObj.BoardID)
			}
		}
	}

	if webhookRaw, ok := raw["webhook"]; ok {
		var webhookObj struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(webhookRaw, &webhookObj); err == nil {
			webhookID = webhookObj.ID
		}
	}

	ev := &Event{
		BoardID:   boardID,
		EventType: eventType,
		WebhookID: webhookID,
		Payload:   string(body),
	}

	if err := h.store.InsertEvent(ev); err != nil {
		http.Error(w, "failed to store event", http.StatusInternalServerError)
		return
	}

	go func() { _ = h.store.Prune(1000) }()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
