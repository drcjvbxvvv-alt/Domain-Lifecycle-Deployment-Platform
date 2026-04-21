package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Webhook sends notifications to a generic HTTP endpoint via JSON POST.
type Webhook struct {
	url    string
	client *http.Client
}

func NewWebhook(url string) *Webhook {
	return &Webhook{
		url:    url,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (w *Webhook) Name() string { return "webhook" }

func (w *Webhook) Send(ctx context.Context, msg Message) error {
	if w.url == "" {
		return fmt.Errorf("webhook: url not configured")
	}

	payload, _ := json.Marshal(map[string]string{
		"subject":  msg.Subject,
		"body":     msg.Body,
		"severity": msg.Severity,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("webhook: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook: send: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook: HTTP %d", resp.StatusCode)
	}
	return nil
}
