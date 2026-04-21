package domain

import (
	"context"
	"fmt"
	"strings"

	"github.com/hibiken/asynq"
	"go.uber.org/zap"

	"domain-platform/pkg/notify"
)

// HandleExpiryCheck is the asynq handler for TypeDomainExpiryCheck.
// It runs the batch expiry check and sends batched notifications for status changes.
type HandleExpiryCheck struct {
	svc      *ExpiryService
	notifier notify.Notifier
	logger   *zap.Logger
}

func NewHandleExpiryCheck(svc *ExpiryService, notifier notify.Notifier, logger *zap.Logger) *HandleExpiryCheck {
	return &HandleExpiryCheck{svc: svc, notifier: notifier, logger: logger}
}

func (h *HandleExpiryCheck) ProcessTask(ctx context.Context, _ *asynq.Task) error {
	result, err := h.svc.CheckAllExpiry(ctx)
	if err != nil {
		return fmt.Errorf("check all expiry: %w", err)
	}

	if len(result.Changed) == 0 {
		h.logger.Info("domain:expiry_check — no status changes")
		return nil
	}

	// Group changes by new status (band) for batched notification — Critical Rule #8.
	grouped := groupByStatus(result.Changed)

	for status, changes := range grouped {
		severity := SeverityForStatus(&status)
		subject := formatSubject(status, len(changes))
		body := formatBody(status, changes)

		if err := h.notifier.Send(ctx, notify.Message{
			Subject:  subject,
			Body:     body,
			Severity: severity,
		}); err != nil {
			h.logger.Warn("notification send failed",
				zap.String("status", status),
				zap.Int("count", len(changes)),
				zap.Error(err),
			)
		}
	}

	h.logger.Info("domain:expiry_check complete",
		zap.Int("checked", result.Checked),
		zap.Int("changed", len(result.Changed)),
	)
	return nil
}

// ── message formatting ────────────────────────────────────────────────────────

func groupByStatus(changes []ExpiryStateChange) map[string][]ExpiryStateChange {
	m := make(map[string][]ExpiryStateChange)
	for _, c := range changes {
		key := "expired" // fallback
		if c.NewStatus != nil {
			key = *c.NewStatus
		}
		m[key] = append(m[key], c)
	}
	return m
}

var statusLabel = map[string]string{
	StatusExpiring90d: "90 天內到期",
	StatusExpiring30d: "30 天內到期",
	StatusExpiring7d:  "7 天內到期",
	StatusExpired:     "已過期",
	StatusGrace:       "Grace Period",
}

var statusEmoji = map[string]string{
	StatusExpiring90d: "ℹ️",
	StatusExpiring30d: "⚠️",
	StatusExpiring7d:  "🔴",
	StatusExpired:     "❌",
	StatusGrace:       "⏳",
}

func formatSubject(status string, count int) string {
	emoji := statusEmoji[status]
	label := statusLabel[status]
	if label == "" {
		label = status
	}
	return fmt.Sprintf("%s 域名到期提醒 — %s (%d 筆)", emoji, label, count)
}

func formatBody(status string, changes []ExpiryStateChange) string {
	var b strings.Builder
	label := statusLabel[status]
	if label == "" {
		label = status
	}
	fmt.Fprintf(&b, "%d 個域名狀態變更為「%s」:\n\n", len(changes), label)

	for _, c := range changes {
		dateStr := c.ExpiryDate.Format("2006-01-02")
		fromLabel := "ok"
		if c.OldStatus != nil {
			fromLabel = *c.OldStatus
		}
		fmt.Fprintf(&b, "• %s — 到期 %s (%s → %s)\n", c.FQDN, dateStr, fromLabel, safeDeref(c.NewStatus))
	}
	return b.String()
}

func safeDeref(s *string) string {
	if s == nil {
		return "ok"
	}
	return *s
}
