// Package notify provides a multi-channel notification system.
// Implementations: Telegram, Slack, Webhook, Email, Noop.
//
// Two interfaces are provided:
//   - Notifier: config embedded in the struct at construction time (legacy, used by the alert engine task handler)
//   - Sender:   config supplied per-call as raw JSON (used by the PC.6 Dispatcher with notification_channels)
package notify

import (
	"context"
	"encoding/json"
)

// Message is the notification payload sent to any Notifier or Sender.
type Message struct {
	Subject  string // short title (used as bold header where supported)
	Body     string // full message body (Markdown or plain text)
	Severity string // "info", "warning", "critical" etc.
}

// Notifier sends notifications to a single channel.
// Config is embedded at construction time (legacy interface).
type Notifier interface {
	// Send delivers a message. Implementations must respect context cancellation.
	Send(ctx context.Context, msg Message) error
	// Name returns the channel name for logging (e.g. "telegram", "webhook").
	Name() string
}

// Sender is the PC.6 config-driven interface.
// The caller supplies the channel-specific config as raw JSON on every call,
// allowing the Dispatcher to work with runtime-loaded notification_channels rows.
type Sender interface {
	// Send delivers a message using the channel config supplied as raw JSON.
	Send(ctx context.Context, config json.RawMessage, msg Message) error
	// Test sends a test/ping message to verify the channel config is valid.
	Test(ctx context.Context, config json.RawMessage) error
}

// Multi fans out to multiple notifiers. All errors are collected.
type Multi struct {
	notifiers []Notifier
}

func NewMulti(notifiers ...Notifier) *Multi {
	return &Multi{notifiers: notifiers}
}

func (m *Multi) Send(ctx context.Context, msg Message) error {
	var firstErr error
	for _, n := range m.notifiers {
		if err := n.Send(ctx, msg); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (m *Multi) Name() string { return "multi" }
