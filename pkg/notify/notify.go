// Package notify provides a multi-channel notification system.
// Implementations: Telegram, Webhook, Noop.
package notify

import "context"

// Message is the notification payload sent to any Notifier.
type Message struct {
	Subject  string // short title (used by Telegram as bold header)
	Body     string // full message body (Markdown or plain text)
	Severity string // "info", "warning", "urgent", "critical"
}

// Notifier sends notifications to a single channel.
type Notifier interface {
	// Send delivers a message. Implementations must respect context cancellation.
	Send(ctx context.Context, msg Message) error
	// Name returns the channel name for logging (e.g. "telegram", "webhook").
	Name() string
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
