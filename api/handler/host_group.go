package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"domain-platform/store/postgres"
)

// HostGroupHandler serves the host-groups management API.
type HostGroupHandler struct {
	store  *postgres.HostGroupStore
	logger *zap.Logger
}

// NewHostGroupHandler creates a HostGroupHandler.
func NewHostGroupHandler(store *postgres.HostGroupStore, logger *zap.Logger) *HostGroupHandler {
	return &HostGroupHandler{store: store, logger: logger}
}

// HostGroupResponse is the API representation of a host group.
type HostGroupResponse struct {
	ID                  int64   `json:"id"`
	UUID                string  `json:"uuid"`
	ProjectID           int64   `json:"project_id"`
	Name                string  `json:"name"`
	Description         *string `json:"description,omitempty"`
	Region              *string `json:"region,omitempty"`
	MaxConcurrency      int     `json:"max_concurrency"`
	ReloadBatchSize     int     `json:"reload_batch_size"`
	ReloadBatchWaitSecs int     `json:"reload_batch_wait_secs"`
	CreatedAt           string  `json:"created_at"`
	UpdatedAt           string  `json:"updated_at"`
}

func toHostGroupResponse(hg *postgres.HostGroup) HostGroupResponse {
	return HostGroupResponse{
		ID:                  hg.ID,
		UUID:                hg.UUID,
		ProjectID:           hg.ProjectID,
		Name:                hg.Name,
		Description:         hg.Description,
		Region:              hg.Region,
		MaxConcurrency:      hg.MaxConcurrency,
		ReloadBatchSize:     hg.ReloadBatchSize,
		ReloadBatchWaitSecs: hg.ReloadBatchWaitSecs,
		CreatedAt:           hg.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:           hg.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// List godoc
// GET /api/v1/host-groups
func (h *HostGroupHandler) List(c *gin.Context) {
	ctx := c.Request.Context()

	items, err := h.store.List(ctx)
	if err != nil {
		h.logger.Error("list host groups", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "message": "internal error"})
		return
	}

	out := make([]HostGroupResponse, len(items))
	for i := range items {
		out[i] = toHostGroupResponse(&items[i])
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": out, "message": "ok"})
}

// Get godoc
// GET /api/v1/host-groups/:id
func (h *HostGroupHandler) Get(c *gin.Context) {
	ctx := c.Request.Context()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "invalid id"})
		return
	}

	hg, err := h.store.GetByID(ctx, id)
	if err != nil {
		h.logger.Error("get host group", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "message": "internal error"})
		return
	}
	if hg == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40401, "message": "host group not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": toHostGroupResponse(hg), "message": "ok"})
}

// UpdateConcurrencyRequest is the request body for PUT /host-groups/:id.
type UpdateConcurrencyRequest struct {
	MaxConcurrency      int `json:"max_concurrency"`       // 0 = unlimited
	ReloadBatchSize     int `json:"reload_batch_size"`     // 0 = use default (50)
	ReloadBatchWaitSecs int `json:"reload_batch_wait_secs"` // 0 = use default (30)
}

// UpdateConcurrency godoc
// PUT /api/v1/host-groups/:id
func (h *HostGroupHandler) UpdateConcurrency(c *gin.Context) {
	ctx := c.Request.Context()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "invalid id"})
		return
	}

	var req UpdateConcurrencyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": err.Error()})
		return
	}
	if req.MaxConcurrency < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40003, "message": "max_concurrency must be >= 0"})
		return
	}
	batchSize := req.ReloadBatchSize
	if batchSize <= 0 {
		batchSize = 50
	}
	waitSecs := req.ReloadBatchWaitSecs
	if waitSecs <= 0 {
		waitSecs = 30
	}

	if err := h.store.UpdateConcurrency(ctx, id, req.MaxConcurrency, batchSize, waitSecs); err != nil {
		h.logger.Error("update host group concurrency", zap.Error(err), zap.Int64("id", id))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "message": "internal error"})
		return
	}

	hg, err := h.store.GetByID(ctx, id)
	if err != nil || hg == nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": toHostGroupResponse(hg), "message": "ok"})
}
