package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"domain-platform/internal/gfw"
	"domain-platform/pkg/probeprotocol"
	"domain-platform/store/postgres"
)

// ProbeNodeHandler implements the probe protocol endpoints (/probe/v1/*)
// and the GFW admin management endpoints (/api/v1/gfw/*).
type ProbeNodeHandler struct {
	svc    *gfw.NodeService
	store  *postgres.GFWNodeStore
	logger *zap.Logger
}

func NewProbeNodeHandler(svc *gfw.NodeService, store *postgres.GFWNodeStore, logger *zap.Logger) *ProbeNodeHandler {
	return &ProbeNodeHandler{svc: svc, store: store, logger: logger}
}

// ── Probe protocol endpoints (/probe/v1/*) ────────────────────────────────────

// Register handles POST /probe/v1/register.
// Probe nodes call this on every start-up (idempotent).
func (h *ProbeNodeHandler) Register(c *gin.Context) {
	var req probeprotocol.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 40000, "data": nil, "message": "invalid request body",
		})
		return
	}

	resp, err := h.svc.Register(c.Request.Context(), req)
	if err != nil {
		h.logger.Warn("probe node register failed", zap.String("node_id", req.NodeID), zap.Error(err))
		status, code, msg := probeErrStatus(err)
		c.JSON(status, gin.H{"code": code, "data": nil, "message": msg})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"code": 0, "data": resp, "message": "ok"})
}

// Heartbeat handles POST /probe/v1/heartbeat.
func (h *ProbeNodeHandler) Heartbeat(c *gin.Context) {
	var req probeprotocol.HeartbeatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 40000, "data": nil, "message": "invalid request body",
		})
		return
	}

	resp, err := h.svc.Heartbeat(c.Request.Context(), req)
	if err != nil {
		if errors.Is(err, postgres.ErrProbeNodeNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"code": 40400, "data": nil, "message": "probe node not registered",
			})
			return
		}
		h.logger.Error("probe heartbeat failed", zap.String("node_id", req.NodeID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 50000, "data": nil, "message": "heartbeat failed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": resp, "message": "ok"})
}

// GetAssignments handles GET /probe/v1/assignments?node_id=xxx.
func (h *ProbeNodeHandler) GetAssignments(c *gin.Context) {
	nodeID := c.Query("node_id")
	if nodeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 40000, "data": nil, "message": "node_id query parameter is required",
		})
		return
	}

	resp, err := h.svc.GetAssignments(c.Request.Context(), nodeID)
	if err != nil {
		if errors.Is(err, postgres.ErrProbeNodeNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"code": 40400, "data": nil, "message": "probe node not found",
			})
			return
		}
		h.logger.Error("get probe assignments failed", zap.String("node_id", nodeID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 50000, "data": nil, "message": "failed to fetch assignments",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": resp, "message": "ok"})
}

// SubmitMeasurements handles POST /probe/v1/measurements.
// PD.1 stub: validates and logs the submission; persistence is implemented in PD.2.
func (h *ProbeNodeHandler) SubmitMeasurements(c *gin.Context) {
	var req probeprotocol.SubmitMeasurementsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 40000, "data": nil, "message": "invalid request body",
		})
		return
	}

	if req.NodeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 40001, "data": nil, "message": "node_id is required",
		})
		return
	}

	h.logger.Info("probe measurements received (PD.1 stub — not persisted)",
		zap.String("node_id", req.NodeID),
		zap.Int("count", len(req.Measurements)),
	)

	// TODO(PD.2): persist req.Measurements into gfw_measurements (TimescaleDB hypertable).
	c.JSON(http.StatusAccepted, gin.H{
		"code": 0, "data": gin.H{"accepted": len(req.Measurements)}, "message": "accepted",
	})
}

// ── GFW admin endpoints (/api/v1/gfw/*) ──────────────────────────────────────

// ListNodes handles GET /api/v1/gfw/nodes?role=probe|control.
func (h *ProbeNodeHandler) ListNodes(c *gin.Context) {
	role := c.Query("role") // "" = all roles
	nodes, err := h.svc.ListNodes(c.Request.Context(), role)
	if err != nil {
		h.logger.Error("list gfw nodes", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 50000, "data": nil, "message": "failed to list nodes",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"items": nodes, "total": len(nodes)}, "message": "ok"})
}

// GetNode handles GET /api/v1/gfw/nodes/:nodeId.
func (h *ProbeNodeHandler) GetNode(c *gin.Context) {
	nodeID := c.Param("nodeId")
	node, err := h.svc.GetNode(c.Request.Context(), nodeID)
	if err != nil {
		if errors.Is(err, postgres.ErrProbeNodeNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"code": 40400, "data": nil, "message": "probe node not found",
			})
			return
		}
		h.logger.Error("get gfw node", zap.String("node_id", nodeID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 50000, "data": nil, "message": "failed to get node",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": node, "message": "ok"})
}

// ListAssignments handles GET /api/v1/gfw/assignments?enabled_only=true.
func (h *ProbeNodeHandler) ListAssignments(c *gin.Context) {
	enabledOnly := c.Query("enabled_only") == "true"
	rows, err := h.store.ListAllAssignments(c.Request.Context(), enabledOnly)
	if err != nil {
		h.logger.Error("list gfw assignments", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 50000, "data": nil, "message": "failed to list assignments",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"items": rows, "total": len(rows)}, "message": "ok"})
}

// GetAssignment handles GET /api/v1/gfw/assignments/:domainId.
func (h *ProbeNodeHandler) GetAssignment(c *gin.Context) {
	domainID, err := strconv.ParseInt(c.Param("domainId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 40000, "data": nil, "message": "invalid domain_id",
		})
		return
	}

	a, err := h.store.GetAssignmentByDomain(c.Request.Context(), domainID)
	if err != nil {
		if errors.Is(err, postgres.ErrCheckAssignmentNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"code": 40400, "data": nil, "message": "assignment not found",
			})
			return
		}
		h.logger.Error("get gfw assignment", zap.Int64("domain_id", domainID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 50000, "data": nil, "message": "failed to get assignment",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": a, "message": "ok"})
}

// upsertAssignmentRequest is the request body for PUT /api/v1/gfw/assignments/:domainId.
type upsertAssignmentRequest struct {
	ProbeNodeIDs   []string `json:"probe_node_ids"`
	ControlNodeIDs []string `json:"control_node_ids"`
	CheckInterval  int      `json:"check_interval"` // seconds; 0 = use default (180)
	Enabled        *bool    `json:"enabled"`
}

// UpsertAssignment handles PUT /api/v1/gfw/assignments/:domainId.
func (h *ProbeNodeHandler) UpsertAssignment(c *gin.Context) {
	domainID, err := strconv.ParseInt(c.Param("domainId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 40000, "data": nil, "message": "invalid domain_id",
		})
		return
	}

	var req upsertAssignmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 40000, "data": nil, "message": "invalid request body",
		})
		return
	}

	probeIDs, _ := json.Marshal(req.ProbeNodeIDs)
	ctrlIDs, _ := json.Marshal(req.ControlNodeIDs)

	interval := req.CheckInterval
	if interval <= 0 {
		interval = 180
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	a := &postgres.GFWCheckAssignment{
		DomainID:       domainID,
		ProbeNodeIDs:   probeIDs,
		ControlNodeIDs: ctrlIDs,
		CheckInterval:  interval,
		Enabled:        enabled,
	}

	if err := h.store.UpsertAssignment(c.Request.Context(), a); err != nil {
		h.logger.Error("upsert gfw assignment", zap.Int64("domain_id", domainID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 50000, "data": nil, "message": "failed to upsert assignment",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": a, "message": "ok"})
}

// DeleteAssignment handles DELETE /api/v1/gfw/assignments/:domainId.
func (h *ProbeNodeHandler) DeleteAssignment(c *gin.Context) {
	domainID, err := strconv.ParseInt(c.Param("domainId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 40000, "data": nil, "message": "invalid domain_id",
		})
		return
	}

	if err := h.store.DeleteAssignment(c.Request.Context(), domainID); err != nil {
		if errors.Is(err, postgres.ErrCheckAssignmentNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"code": 40400, "data": nil, "message": "assignment not found",
			})
			return
		}
		h.logger.Error("delete gfw assignment", zap.Int64("domain_id", domainID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 50000, "data": nil, "message": "failed to delete assignment",
		})
		return
	}

	c.JSON(http.StatusNoContent, nil)
}

// probeErrStatus maps service errors to HTTP status + codes.
func probeErrStatus(err error) (int, int, string) {
	switch {
	case errors.Is(err, postgres.ErrProbeNodeNotFound):
		return http.StatusNotFound, 40400, "probe node not found"
	default:
		return http.StatusBadRequest, 40000, err.Error()
	}
}
