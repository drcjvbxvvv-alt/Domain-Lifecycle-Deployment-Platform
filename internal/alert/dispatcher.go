package alert

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"

	"domain-platform/pkg/notify"
	"domain-platform/store/postgres"
)

// Dispatcher routes an alert event to all matching notification channels,
// records history, and handles failures gracefully.
//
// It is the PC.6 replacement for the inline buildNotifier path in task.go.
// The Engine.Fire method enqueues a TypeNotifySend asynq task; the worker
// processes it via Dispatcher.DispatchByChannelID.
type Dispatcher struct {
	channels *postgres.NotificationStore
	rules    *postgres.AlertStore
	senders  map[string]notify.Sender // keyed by channel_type
	logger   *zap.Logger
}

// NewDispatcher constructs a Dispatcher with all built-in senders registered.
func NewDispatcher(
	channels *postgres.NotificationStore,
	rules *postgres.AlertStore,
	logger *zap.Logger,
) *Dispatcher {
	return &Dispatcher{
		channels: channels,
		rules:    rules,
		senders: map[string]notify.Sender{
			"telegram": notify.NewTelegramSender(),
			"slack":    notify.NewSlackSender(),
			"webhook":  notify.NewWebhookSender(),
			"email":    notify.NewEmailSender(),
		},
		logger: logger,
	}
}

// DispatchEvent finds all matching rules for the alert event and sends to each
// matching channel. History is recorded for every dispatch attempt.
//
// Failures are logged and recorded in history but do NOT return an error —
// one bad channel must not block others.
func (d *Dispatcher) DispatchEvent(ctx context.Context, ev *postgres.AlertEvent) {
	alertType := ev.Source // use source as alert_type for rule matching
	targetKind := ev.TargetKind
	rules, err := d.rules.ListMatchingRules(ctx, ev.Severity, alertType, &targetKind, ev.TargetID)
	if err != nil {
		d.logger.Error("dispatcher: list matching rules",
			zap.Int64("alert_id", ev.ID),
			zap.Error(err),
		)
		return
	}

	if len(rules) == 0 {
		d.logger.Debug("dispatcher: no matching rules", zap.Int64("alert_id", ev.ID))
		return
	}

	// Deduplicate: dispatch at most once per channel for this event.
	seen := make(map[int64]struct{}, len(rules))
	for _, rule := range rules {
		if _, dup := seen[rule.ChannelID]; dup {
			continue
		}
		seen[rule.ChannelID] = struct{}{}

		ch, err := d.channels.GetChannel(ctx, rule.ChannelID)
		if err != nil {
			d.logger.Warn("dispatcher: get channel",
				zap.Int64("channel_id", rule.ChannelID),
				zap.Error(err),
			)
			continue
		}
		if !ch.Enabled {
			continue
		}

		msg := buildMessage(ev)
		d.sendToChannel(ctx, ch, ev.ID, msg)
	}
}

// DispatchByChannelID sends directly to a specific channel. Used by the
// asynq TypeNotifySend task handler.
func (d *Dispatcher) DispatchByChannelID(ctx context.Context, channelID int64, alertEventID *int64, msg notify.Message) {
	ch, err := d.channels.GetChannel(ctx, channelID)
	if err != nil {
		d.logger.Error("dispatcher: get channel by id",
			zap.Int64("channel_id", channelID),
			zap.Error(err),
		)
		return
	}
	if !ch.Enabled {
		d.recordHistory(ctx, channelID, alertEventID, "suppressed", msg.Subject, "channel disabled")
		return
	}
	d.sendToChannel(ctx, ch, alertEventID, msg)
}

// TestChannel sends a test message to a channel. Returns any send error.
func (d *Dispatcher) TestChannel(ctx context.Context, ch *postgres.NotificationChannel) error {
	sender, ok := d.senders[ch.ChannelType]
	if !ok {
		return fmt.Errorf("dispatcher: unsupported channel type %q", ch.ChannelType)
	}
	return sender.Test(ctx, ch.Config)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func (d *Dispatcher) sendToChannel(ctx context.Context, ch *postgres.NotificationChannel, alertEventID interface{}, msg notify.Message) {
	sender, ok := d.senders[ch.ChannelType]
	if !ok {
		d.logger.Warn("dispatcher: unsupported channel type",
			zap.String("type", ch.ChannelType),
			zap.Int64("channel_id", ch.ID),
		)
		return
	}

	// Normalise alertEventID to *int64 for history recording.
	var evID *int64
	switch v := alertEventID.(type) {
	case int64:
		if v != 0 {
			evID = &v
		}
	case *int64:
		evID = v
	}

	err := sender.Send(ctx, ch.Config, msg)
	if err != nil {
		d.logger.Error("dispatcher: send failed",
			zap.Int64("channel_id", ch.ID),
			zap.String("channel_type", ch.ChannelType),
			zap.String("channel_name", ch.Name),
			zap.Error(err),
		)
		d.recordHistory(ctx, ch.ID, evID, "failed", msg.Subject, err.Error())
		return
	}

	d.logger.Info("dispatcher: sent",
		zap.Int64("channel_id", ch.ID),
		zap.String("channel_name", ch.Name),
		zap.String("subject", msg.Subject),
	)
	d.recordHistory(ctx, ch.ID, evID, "sent", msg.Subject, "")
}

func (d *Dispatcher) recordHistory(ctx context.Context, channelID int64, alertEventID *int64, status, message, errStr string) {
	h := &postgres.NotificationHistory{
		ChannelID:    channelID,
		AlertEventID: alertEventID,
		Status:       status,
	}
	if message != "" {
		h.Message = &message
	}
	if errStr != "" {
		h.Error = &errStr
	}
	if err := d.channels.InsertHistory(ctx, h); err != nil {
		d.logger.Warn("dispatcher: record history failed",
			zap.Int64("channel_id", channelID),
			zap.Error(err),
		)
	}
}

func buildMessage(ev *postgres.AlertEvent) notify.Message {
	body := ev.Title
	if len(ev.Detail) > 0 && string(ev.Detail) != "null" {
		var detail map[string]interface{}
		if err := json.Unmarshal(ev.Detail, &detail); err == nil {
			if d, ok := detail["description"].(string); ok && d != "" {
				body = fmt.Sprintf("%s\n\n%s", ev.Title, d)
			}
		}
	}
	return notify.Message{
		Subject:  fmt.Sprintf("[%s] %s", ev.Severity, ev.Title),
		Body:     body,
		Severity: ev.Severity,
	}
}
