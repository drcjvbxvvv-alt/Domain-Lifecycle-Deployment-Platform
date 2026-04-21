package notify

import "context"

// Noop is a no-op notifier for development and testing.
type Noop struct{}

func NewNoop() *Noop            { return &Noop{} }
func (n *Noop) Name() string    { return "noop" }
func (n *Noop) Send(_ context.Context, _ Message) error { return nil }
