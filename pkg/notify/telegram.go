package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Telegram sends notifications via the Telegram Bot API.
type Telegram struct {
	botToken string
	chatID   string
	client   *http.Client
}

func NewTelegram(botToken, chatID string) *Telegram {
	return &Telegram{
		botToken: botToken,
		chatID:   chatID,
		client:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (t *Telegram) Name() string { return "telegram" }

func (t *Telegram) Send(ctx context.Context, msg Message) error {
	if t.botToken == "" || t.chatID == "" {
		return fmt.Errorf("telegram: bot_token or chat_id not configured")
	}

	text := msg.Body
	if msg.Subject != "" {
		text = fmt.Sprintf("*%s*\n\n%s", escapeMarkdown(msg.Subject), msg.Body)
	}

	payload, _ := json.Marshal(map[string]interface{}{
		"chat_id":    t.chatID,
		"text":       text,
		"parse_mode": "Markdown",
	})

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.botToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("telegram: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("telegram: send: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode >= 400 {
		return fmt.Errorf("telegram: HTTP %d", resp.StatusCode)
	}
	return nil
}

func escapeMarkdown(s string) string {
	// Minimal escape for Telegram Markdown v1
	replacer := []struct{ old, new string }{
		{"_", "\\_"}, {"*", "\\*"}, {"`", "\\`"}, {"[", "\\["},
	}
	for _, r := range replacer {
		result := ""
		for _, c := range s {
			if string(c) == r.old {
				result += r.new
			} else {
				result += string(c)
			}
		}
		s = result
	}
	return s
}
