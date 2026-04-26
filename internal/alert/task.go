package alert

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
	"go.uber.org/zap"

	"domain-platform/internal/tasks"
	"domain-platform/pkg/notify"
	"domain-platform/store/postgres"
)

// ── HandleAlertFire ───────────────────────────────────────────────────────────

// HandleAlertFire processes TypeAlertFire tasks.
// It reconstructs an AlertEvent from the payload and delegates to Engine.Fire.
type HandleAlertFire struct {
	engine *Engine
	logger *zap.Logger
}

func NewHandleAlertFire(engine *Engine, logger *zap.Logger) *HandleAlertFire {
	return &HandleAlertFire{engine: engine, logger: logger}
}

func (h *HandleAlertFire) ProcessTask(ctx context.Context, t *asynq.Task) error {
	var p tasks.AlertFirePayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("alert fire: unmarshal payload: %w", err)
	}

	ev := &postgres.AlertEvent{
		Severity:   p.Severity,
		Source:     p.Source,
		TargetKind: p.TargetKind,
		TargetID:   p.TargetID,
		Title:      p.Title,
	}
	if p.DedupKey != "" {
		dk := p.DedupKey
		ev.DedupKey = &dk
	}
	if p.Detail != "" {
		ev.Detail = json.RawMessage(p.Detail)
	}

	if err := h.engine.Fire(ctx, ev); err != nil {
		h.logger.Error("alert fire task failed",
			zap.String("severity", p.Severity),
			zap.String("title", p.Title),
			zap.Error(err),
		)
		return err
	}
	return nil
}

// ── HandleNotifySend ─────────────────────────────────────────────────────────

// HandleNotifySend processes TypeNotifySend tasks.
// It loads the channel from DB via the Dispatcher and sends the message.
type HandleNotifySend struct {
	dispatcher *Dispatcher
	alertStore *postgres.AlertStore
	logger     *zap.Logger
}

func NewHandleNotifySend(dispatcher *Dispatcher, alertStore *postgres.AlertStore, logger *zap.Logger) *HandleNotifySend {
	return &HandleNotifySend{dispatcher: dispatcher, alertStore: alertStore, logger: logger}
}

func (h *HandleNotifySend) ProcessTask(ctx context.Context, t *asynq.Task) error {
	var p tasks.NotifySendPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("notify send: unmarshal payload: %w", err)
	}

	// Build the message from the alert event if available.
	var msg notify.Message
	if p.AlertEventID != 0 {
		ev, err := h.alertStore.GetByID(ctx, p.AlertEventID)
		if err != nil {
			h.logger.Warn("notify send: get alert event",
				zap.Int64("alert_event_id", p.AlertEventID),
				zap.Error(err),
			)
			// Proceed with a minimal message rather than failing the task.
			msg = notify.Message{Subject: "Alert notification", Severity: p.Severity}
		} else {
			msg = buildMessage(ev)
		}
	} else {
		msg = notify.Message{Subject: "Alert notification", Severity: p.Severity}
	}

	var evID *int64
	if p.AlertEventID != 0 {
		evID = &p.AlertEventID
	}
	h.dispatcher.DispatchByChannelID(ctx, p.ChannelID, evID, msg)
	return nil
}
