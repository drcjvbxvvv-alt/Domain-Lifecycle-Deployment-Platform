package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"domain-platform/internal/alert"
	"domain-platform/store/postgres"
)

// NotificationHandler handles REST endpoints for notification channels and history.
type NotificationHandler struct {
	store      *postgres.NotificationStore
	alertStore *postgres.AlertStore
	dispatcher *alert.Dispatcher
	logger     *zap.Logger
}

func NewNotificationHandler(
	store *postgres.NotificationStore,
	alertStore *postgres.AlertStore,
	dispatcher *alert.Dispatcher,
	logger *zap.Logger,
) *NotificationHandler {
	return &NotificationHandler{
		store:      store,
		alertStore: alertStore,
		dispatcher: dispatcher,
		logger:     logger,
	}
}

// ── Channels ──────────────────────────────────────────────────────────────────

// ListChannels GET /notifications/channels
func (h *NotificationHandler) ListChannels(c *gin.Context) {
	channels, err := h.store.ListChannels(c.Request.Context())
	if err != nil {
		h.logger.Error("list notification channels", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "data": nil, "message": "internal error"})
		return
	}
	items := make([]gin.H, len(channels))
	for i := range channels {
		items[i] = channelResponse(&channels[i], false)
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": gin.H{"items": items, "total": len(items)}})
}

// GetChannel GET /notifications/channels/:id
func (h *NotificationHandler) GetChannel(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40000, "data": nil, "message": "invalid id"})
		return
	}
	ch, err := h.store.GetChannel(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, postgres.ErrNotificationChannelNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"code": 40400, "data": nil, "message": "channel not found"})
			return
		}
		h.logger.Error("get notification channel", zap.Int64("id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "data": nil, "message": "internal error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": channelResponse(ch, false)})
}

// CreateChannel POST /notifications/channels
func (h *NotificationHandler) CreateChannel(c *gin.Context) {
	var req struct {
		Name        string          `json:"name" binding:"required"`
		ChannelType string          `json:"channel_type" binding:"required"`
		Config      json.RawMessage `json:"config" binding:"required"`
		IsDefault   bool            `json:"is_default"`
		Enabled     bool            `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40000, "data": nil, "message": err.Error()})
		return
	}

	userID := c.GetInt64("user_id")
	ch := &postgres.NotificationChannel{
		Name:        req.Name,
		ChannelType: req.ChannelType,
		Config:      req.Config,
		IsDefault:   req.IsDefault,
		Enabled:     req.Enabled,
	}
	if userID != 0 {
		ch.CreatedBy = &userID
	}

	if err := h.store.CreateChannel(c.Request.Context(), ch); err != nil {
		h.logger.Error("create notification channel", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "data": nil, "message": "internal error"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"code": 0, "message": "ok", "data": channelResponse(ch, false)})
}

// UpdateChannel PUT /notifications/channels/:id
func (h *NotificationHandler) UpdateChannel(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40000, "data": nil, "message": "invalid id"})
		return
	}

	existing, err := h.store.GetChannel(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, postgres.ErrNotificationChannelNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"code": 40400, "data": nil, "message": "channel not found"})
			return
		}
		h.logger.Error("get notification channel for update", zap.Int64("id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "data": nil, "message": "internal error"})
		return
	}

	var req struct {
		Name        *string         `json:"name"`
		ChannelType *string         `json:"channel_type"`
		Config      json.RawMessage `json:"config"`
		IsDefault   *bool           `json:"is_default"`
		Enabled     *bool           `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40000, "data": nil, "message": err.Error()})
		return
	}

	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.ChannelType != nil {
		existing.ChannelType = *req.ChannelType
	}
	if len(req.Config) > 0 {
		existing.Config = req.Config
	}
	if req.IsDefault != nil {
		existing.IsDefault = *req.IsDefault
	}
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}

	if err := h.store.UpdateChannel(c.Request.Context(), existing); err != nil {
		h.logger.Error("update notification channel", zap.Int64("id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "data": nil, "message": "internal error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": channelResponse(existing, false)})
}

// DeleteChannel DELETE /notifications/channels/:id
func (h *NotificationHandler) DeleteChannel(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40000, "data": nil, "message": "invalid id"})
		return
	}
	if err := h.store.DeleteChannel(c.Request.Context(), id); err != nil {
		if errors.Is(err, postgres.ErrNotificationChannelNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"code": 40400, "data": nil, "message": "channel not found"})
			return
		}
		h.logger.Error("delete notification channel", zap.Int64("id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "data": nil, "message": "internal error"})
		return
	}
	c.Status(http.StatusNoContent)
}

// TestChannel POST /notifications/channels/:id/test
func (h *NotificationHandler) TestChannel(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40000, "data": nil, "message": "invalid id"})
		return
	}
	ch, err := h.store.GetChannel(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, postgres.ErrNotificationChannelNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"code": 40400, "data": nil, "message": "channel not found"})
			return
		}
		h.logger.Error("get notification channel for test", zap.Int64("id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "data": nil, "message": "internal error"})
		return
	}

	if err := h.dispatcher.TestChannel(c.Request.Context(), ch); err != nil {
		h.logger.Warn("test notification channel",
			zap.Int64("channel_id", id),
			zap.String("type", ch.ChannelType),
			zap.Error(err),
		)
		c.JSON(http.StatusBadGateway, gin.H{"code": 50200, "data": nil, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "test message sent", "data": nil})
}

// ── Rules (channel-scoped) ────────────────────────────────────────────────────

// ListChannelRules GET /notifications/channels/:id/rules
func (h *NotificationHandler) ListChannelRules(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40000, "data": nil, "message": "invalid id"})
		return
	}
	rules, err := h.alertStore.ListRulesByChannel(c.Request.Context(), id)
	if err != nil {
		h.logger.Error("list channel rules", zap.Int64("channel_id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "data": nil, "message": "internal error"})
		return
	}
	items := make([]gin.H, len(rules))
	for i := range rules {
		items[i] = notificationRuleResponse(&rules[i])
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": gin.H{"items": items, "total": len(items)}})
}

// ── History ───────────────────────────────────────────────────────────────────

// ListHistory GET /notifications/history
func (h *NotificationHandler) ListHistory(c *gin.Context) {
	f := postgres.NotificationHistoryFilter{}

	if v := c.Query("channel_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40000, "data": nil, "message": "invalid channel_id"})
			return
		}
		f.ChannelID = &id
	}
	if v := c.Query("alert_event_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40000, "data": nil, "message": "invalid alert_event_id"})
			return
		}
		f.AlertEventID = &id
	}
	f.Status = c.Query("status")
	if l := c.Query("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			f.Limit = v
		}
	}
	if o := c.Query("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil && v >= 0 {
			f.Offset = v
		}
	}

	items, err := h.store.ListHistory(c.Request.Context(), f)
	if err != nil {
		h.logger.Error("list notification history", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "data": nil, "message": "internal error"})
		return
	}
	resp := make([]gin.H, len(items))
	for i := range items {
		resp[i] = historyResponse(&items[i])
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": gin.H{"items": resp, "total": len(resp)}})
}

// ── Response builders ─────────────────────────────────────────────────────────

// channelResponse converts a NotificationChannel to the API response shape.
// If withConfig is false, the config field is redacted to avoid leaking secrets.
func channelResponse(ch *postgres.NotificationChannel, withConfig bool) gin.H {
	resp := gin.H{
		"id":           ch.ID,
		"uuid":         ch.UUID,
		"name":         ch.Name,
		"channel_type": ch.ChannelType,
		"is_default":   ch.IsDefault,
		"enabled":      ch.Enabled,
		"created_by":   ch.CreatedBy,
		"created_at":   ch.CreatedAt,
		"updated_at":   ch.UpdatedAt,
	}
	if withConfig {
		resp["config"] = ch.Config
	} else {
		resp["config"] = json.RawMessage(`"<redacted>"`)
	}
	return resp
}

func historyResponse(h *postgres.NotificationHistory) gin.H {
	return gin.H{
		"id":             h.ID,
		"channel_id":     h.ChannelID,
		"alert_event_id": h.AlertEventID,
		"status":         h.Status,
		"message":        h.Message,
		"error":          h.Error,
		"sent_at":        h.SentAt,
	}
}

// parseID extracts the :id path parameter as int64.
func parseID(c *gin.Context) (int64, error) {
	return strconv.ParseInt(c.Param("id"), 10, 64)
}
