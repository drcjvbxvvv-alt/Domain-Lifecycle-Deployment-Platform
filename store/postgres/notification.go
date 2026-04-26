package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

var ErrNotificationChannelNotFound = errors.New("notification channel not found")

// ── Models ────────────────────────────────────────────────────────────────────

// NotificationChannel maps to the notification_channels table.
type NotificationChannel struct {
	ID          int64           `db:"id"`
	UUID        string          `db:"uuid"`
	Name        string          `db:"name"`
	ChannelType string          `db:"channel_type"` // telegram | slack | webhook | email
	Config      json.RawMessage `db:"config"`
	IsDefault   bool            `db:"is_default"`
	Enabled     bool            `db:"enabled"`
	CreatedBy   *int64          `db:"created_by"`
	CreatedAt   time.Time       `db:"created_at"`
	UpdatedAt   time.Time       `db:"updated_at"`
}

// NotificationHistory maps to the notification_history table.
type NotificationHistory struct {
	ID           int64      `db:"id"`
	ChannelID    int64      `db:"channel_id"`
	AlertEventID *int64     `db:"alert_event_id"`
	Status       string     `db:"status"` // sent | failed | suppressed
	Message      *string    `db:"message"`
	Error        *string    `db:"error"`
	SentAt       time.Time  `db:"sent_at"`
}

// ── NotificationStore ─────────────────────────────────────────────────────────

type NotificationStore struct {
	db *sqlx.DB
}

func NewNotificationStore(db *sqlx.DB) *NotificationStore {
	return &NotificationStore{db: db}
}

// ── NotificationChannel writes ────────────────────────────────────────────────

// CreateChannel inserts a new notification channel.
func (s *NotificationStore) CreateChannel(ctx context.Context, c *NotificationChannel) error {
	return s.db.QueryRowContext(ctx,
		`INSERT INTO notification_channels
		   (name, channel_type, config, is_default, enabled, created_by)
		 VALUES ($1,$2,$3,$4,$5,$6)
		 RETURNING id, uuid, created_at, updated_at`,
		c.Name, c.ChannelType, nullJSON(c.Config),
		c.IsDefault, c.Enabled, c.CreatedBy,
	).Scan(&c.ID, &c.UUID, &c.CreatedAt, &c.UpdatedAt)
}

// UpdateChannel updates mutable fields of a notification channel.
func (s *NotificationStore) UpdateChannel(ctx context.Context, c *NotificationChannel) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE notification_channels SET
		   name=$1, channel_type=$2, config=$3, is_default=$4, enabled=$5,
		   updated_at=NOW()
		 WHERE id=$6`,
		c.Name, c.ChannelType, nullJSON(c.Config), c.IsDefault, c.Enabled, c.ID)
	if err != nil {
		return fmt.Errorf("update notification channel %d: %w", c.ID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotificationChannelNotFound
	}
	return nil
}

// DeleteChannel removes a notification channel (cascades to rules + history).
func (s *NotificationStore) DeleteChannel(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM notification_channels WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("delete notification channel %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotificationChannelNotFound
	}
	return nil
}

// ── NotificationChannel reads ─────────────────────────────────────────────────

// GetChannel returns a single notification channel by ID.
func (s *NotificationStore) GetChannel(ctx context.Context, id int64) (*NotificationChannel, error) {
	var c NotificationChannel
	err := s.db.GetContext(ctx, &c,
		`SELECT id, uuid, name, channel_type, config, is_default, enabled, created_by, created_at, updated_at
		 FROM notification_channels WHERE id=$1`, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotificationChannelNotFound
		}
		return nil, fmt.Errorf("get notification channel %d: %w", id, err)
	}
	return &c, nil
}

// ListChannels returns all notification channels ordered by name.
func (s *NotificationStore) ListChannels(ctx context.Context) ([]NotificationChannel, error) {
	var out []NotificationChannel
	err := s.db.SelectContext(ctx, &out,
		`SELECT id, uuid, name, channel_type, config, is_default, enabled, created_by, created_at, updated_at
		 FROM notification_channels ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list notification channels: %w", err)
	}
	return out, nil
}

// ListEnabledChannels returns enabled channels with their config.
// Used by Dispatcher to build senders.
func (s *NotificationStore) ListEnabledChannels(ctx context.Context) ([]NotificationChannel, error) {
	var out []NotificationChannel
	err := s.db.SelectContext(ctx, &out,
		`SELECT id, uuid, name, channel_type, config, is_default, enabled, created_by, created_at, updated_at
		 FROM notification_channels WHERE enabled = true ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list enabled notification channels: %w", err)
	}
	return out, nil
}

// ── NotificationHistory writes ────────────────────────────────────────────────

// InsertHistory records a dispatch attempt.
func (s *NotificationStore) InsertHistory(ctx context.Context, h *NotificationHistory) error {
	return s.db.QueryRowContext(ctx,
		`INSERT INTO notification_history
		   (channel_id, alert_event_id, status, message, error)
		 VALUES ($1,$2,$3,$4,$5)
		 RETURNING id, sent_at`,
		h.ChannelID, h.AlertEventID, h.Status, h.Message, h.Error,
	).Scan(&h.ID, &h.SentAt)
}

// ── NotificationHistory reads ─────────────────────────────────────────────────

// NotificationHistoryFilter configures history listing.
type NotificationHistoryFilter struct {
	ChannelID    *int64
	AlertEventID *int64
	Status       string // "" = all
	Limit        int    // default 100
	Offset       int
}

// ListHistory returns notification_history rows matching the filter.
func (s *NotificationStore) ListHistory(ctx context.Context, f NotificationHistoryFilter) ([]NotificationHistory, error) {
	if f.Limit <= 0 {
		f.Limit = 100
	}
	var out []NotificationHistory
	err := s.db.SelectContext(ctx, &out,
		`SELECT id, channel_id, alert_event_id, status, message, error, sent_at
		 FROM notification_history
		 WHERE ($1::BIGINT IS NULL OR channel_id = $1)
		   AND ($2::BIGINT IS NULL OR alert_event_id = $2)
		   AND ($3 = '' OR status = $3)
		 ORDER BY sent_at DESC
		 LIMIT $4 OFFSET $5`,
		f.ChannelID, f.AlertEventID, f.Status, f.Limit, f.Offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list notification history: %w", err)
	}
	return out, nil
}
