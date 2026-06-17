package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
)

// Client is an IPC client that speaks to a running daemon over a Unix socket.
type Client struct {
	sockPath string
	http     *http.Client
}

// NewClient returns a Client that connects to the daemon socket at sockPath.
func NewClient(sockPath string) *Client {
	transport := &http.Transport{
		DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", sockPath)
		},
	}
	return &Client{
		sockPath: sockPath,
		http: &http.Client{
			Transport: transport,
		},
	}
}

// Status queries the daemon for its current status.
func (c *Client) Status() (*StatusResponse, error) {
	resp, err := c.http.Get("http://daemon/status")
	if err != nil {
		return nil, fmt.Errorf("Status: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Status: unexpected status %d", resp.StatusCode)
	}
	var sr StatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, fmt.Errorf("Status: decode: %w", err)
	}
	return &sr, nil
}

// Stop asks the daemon to shut down gracefully.
func (c *Client) Stop() error {
	resp, err := c.http.Post("http://daemon/stop", "application/json", nil)
	if err != nil {
		return fmt.Errorf("Stop: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Stop: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// Ping checks whether the daemon is reachable by calling /status.
func (c *Client) Ping() error {
	_, err := c.Status()
	if err != nil {
		return fmt.Errorf("Ping: %w", err)
	}
	return nil
}

// RegisterWebhook asks the daemon to register a webhook with monday.com for
// the given board and event. Returns the created WebhookRecord.
func (c *Client) RegisterWebhook(boardID, event string) (*WebhookRecord, error) {
	body, err := json.Marshal(registerWebhookRequest{BoardID: boardID, Event: event})
	if err != nil {
		return nil, fmt.Errorf("RegisterWebhook: marshal: %w", err)
	}
	resp, err := c.http.Post("http://daemon/webhooks/register", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("RegisterWebhook: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("RegisterWebhook: unexpected status %d", resp.StatusCode)
	}
	var rec WebhookRecord
	if err := json.NewDecoder(resp.Body).Decode(&rec); err != nil {
		return nil, fmt.Errorf("RegisterWebhook: decode: %w", err)
	}
	return &rec, nil
}

// ListWebhooks returns all registered webhooks. If boardID is non-empty, only
// webhooks for that board are returned.
func (c *Client) ListWebhooks(boardID string) ([]WebhookRecord, error) {
	endpoint := "http://daemon/webhooks"
	if boardID != "" {
		endpoint += "?board_id=" + url.QueryEscape(boardID)
	}
	resp, err := c.http.Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("ListWebhooks: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ListWebhooks: unexpected status %d", resp.StatusCode)
	}
	var records []WebhookRecord
	if err := json.NewDecoder(resp.Body).Decode(&records); err != nil {
		return nil, fmt.Errorf("ListWebhooks: decode: %w", err)
	}
	return records, nil
}

// DeleteWebhook asks the daemon to delete the webhook with the given monday ID.
func (c *Client) DeleteWebhook(id string) error {
	req, err := http.NewRequest(http.MethodDelete, "http://daemon/webhooks/"+url.PathEscape(id), nil)
	if err != nil {
		return fmt.Errorf("DeleteWebhook: build request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("DeleteWebhook: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("DeleteWebhook: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// ListNotifications returns events matching filter along with the current
// unread count.
func (c *Client) ListNotifications(filter EventFilter) (*NotificationListResponse, error) {
	q := url.Values{}
	if filter.Unread {
		q.Set("unread", "true")
	}
	if filter.BoardID != "" {
		q.Set("board_id", filter.BoardID)
	}
	if filter.EventType != "" {
		q.Set("event_type", filter.EventType)
	}
	if filter.Since != "" {
		q.Set("since", filter.Since)
	}
	if filter.Limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", filter.Limit))
	}
	endpoint := "http://daemon/notifications"
	if len(q) > 0 {
		endpoint += "?" + q.Encode()
	}
	resp, err := c.http.Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("ListNotifications: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ListNotifications: unexpected status %d", resp.StatusCode)
	}
	var result NotificationListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("ListNotifications: decode: %w", err)
	}
	return &result, nil
}

// AckNotification marks the event with the given id as read.
func (c *Client) AckNotification(id EventID) error {
	body, err := json.Marshal(notificationAckRequest{ID: id})
	if err != nil {
		return fmt.Errorf("AckNotification: marshal: %w", err)
	}
	resp, err := c.http.Post("http://daemon/notifications/ack", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("AckNotification: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("AckNotification: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// AckAllNotifications marks all events as read.
func (c *Client) AckAllNotifications() error {
	body, err := json.Marshal(notificationAckRequest{All: true})
	if err != nil {
		return fmt.Errorf("AckAllNotifications: marshal: %w", err)
	}
	resp, err := c.http.Post("http://daemon/notifications/ack", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("AckAllNotifications: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("AckAllNotifications: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// NotificationCount returns the current number of unread notifications.
func (c *Client) NotificationCount() (int, error) {
	resp, err := c.http.Get("http://daemon/notifications/count")
	if err != nil {
		return 0, fmt.Errorf("NotificationCount: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("NotificationCount: unexpected status %d", resp.StatusCode)
	}
	var result NotificationCountResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("NotificationCount: decode: %w", err)
	}
	return result.Unread, nil
}
