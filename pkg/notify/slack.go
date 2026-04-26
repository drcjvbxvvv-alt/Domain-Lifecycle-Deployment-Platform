package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// SlackConfig is the JSON structure stored in notification_channels.config
// for a Slack channel.
type SlackConfig struct {
	WebhookURL string `json:"webhook_url"`
	// Optional: override the username shown in Slack.
	Username string `json:"username,omitempty"`
	// Optional: override the channel (e.g. "#alerts"). Webhook already has a
	// default channel, so this is only needed for incoming webhook overrides.
	Channel string `json:"channel,omitempty"`
}

// SlackSender implements Sender for Slack incoming webhooks.
type SlackSender struct {
	client *http.Client
}

func NewSlackSender() *SlackSender {
	return &SlackSender{
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *SlackSender) Send(ctx context.Context, config json.RawMessage, msg Message) error {
	cfg, err := parseSlackConfig(config)
	if err != nil {
		return err
	}
	return s.send(ctx, cfg, msg.Subject, msg.Body)
}

func (s *SlackSender) Test(ctx context.Context, config json.RawMessage) error {
	cfg, err := parseSlackConfig(config)
	if err != nil {
		return err
	}
	return s.send(ctx, cfg, "Test notification", "This is a test message from Domain Platform.")
}

func (s *SlackSender) send(ctx context.Context, cfg *SlackConfig, subject, body string) error {
	text := body
	if subject != "" {
		text = fmt.Sprintf("*%s*\n%s", subject, body)
	}

	payload := map[string]interface{}{
		"text": text,
	}
	if cfg.Username != "" {
		payload["username"] = cfg.Username
	}
	if cfg.Channel != "" {
		payload["channel"] = cfg.Channel
	}

	raw, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.WebhookURL, bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("slack: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("slack: send: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode >= 400 {
		return fmt.Errorf("slack: HTTP %d", resp.StatusCode)
	}
	return nil
}

func parseSlackConfig(raw json.RawMessage) (*SlackConfig, error) {
	var cfg SlackConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("slack: parse config: %w", err)
	}
	if cfg.WebhookURL == "" {
		return nil, fmt.Errorf("slack: webhook_url is required")
	}
	return &cfg, nil
}
