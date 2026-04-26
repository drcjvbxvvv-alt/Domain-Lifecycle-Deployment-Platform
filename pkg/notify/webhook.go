package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// WebhookConfig is the JSON structure stored in notification_channels.config.
type WebhookConfig struct {
	URL string `json:"url"`
}

// WebhookSender implements the PC.6 Sender interface for generic webhooks.
type WebhookSender struct {
	client *http.Client
}

func NewWebhookSender() *WebhookSender {
	return &WebhookSender{client: &http.Client{Timeout: 10 * time.Second}}
}

func (s *WebhookSender) Send(ctx context.Context, config json.RawMessage, msg Message) error {
	var cfg WebhookConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("webhook: parse config: %w", err)
	}
	n := NewWebhook(cfg.URL)
	n.client = s.client
	return n.Send(ctx, msg)
}

func (s *WebhookSender) Test(ctx context.Context, config json.RawMessage) error {
	return s.Send(ctx, config, Message{
		Subject:  "Test notification",
		Body:     "This is a test message from Domain Platform.",
		Severity: "info",
	})
}

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
