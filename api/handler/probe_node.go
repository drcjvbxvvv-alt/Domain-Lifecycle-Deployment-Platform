package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"domain-platform/internal/gfw"
	"domain-platform/pkg/probeprotocol"
	"domain-platform/store/postgres"
)

// ProbeNodeHandler implements the probe protocol endpoints (/probe/v1/*)
// and the GFW admin management endpoints (/api/v1/gfw/*).
type ProbeNodeHandler struct {
	svc     *gfw.NodeService
	msvc    *gfw.MeasurementService
	store   *postgres.GFWNodeStore
	logger  *zap.Logger
}

func NewProbeNodeHandler(svc *gfw.NodeService, msvc *gfw.MeasurementService, store *postgres.GFWNodeStore, logger *zap.Logger) *ProbeNodeHandler {
	return &ProbeNodeHandler{svc: svc, msvc: msvc, store: store, logger: logger}
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
// Validates, persists measurements to gfw_measurements (TimescaleDB hypertable),
// and returns a count of accepted rows.
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

	if err := h.msvc.StoreMeasurements(c.Request.Context(), req.NodeID, req.Measurements); err != nil {
		if errors.Is(err, postgres.ErrProbeNodeNotFound) {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code": 40100, "data": nil, "message": "probe node not registered",
			})
			return
		}
		h.logger.Error("store measurements failed",
			zap.String("node_id", req.NodeID),
			zap.Int("count", len(req.Measurements)),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 50000, "data": nil, "message": "failed to store measurements",
		})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"code": 0, "data": gin.H{"accepted": len(req.Measurements)}, "message": "accepted",
	})
}

// ListMeasurements handles GET /api/v1/gfw/measurements/:domainId.
// Query params: from, to (RFC3339), limit (int, max 500).
func (h *ProbeNodeHandler) ListMeasurements(c *gin.Context) {
	domainID, err := strconv.ParseInt(c.Param("domainId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 40000, "data": nil, "message": "invalid domain_id",
		})
		return
	}

	var from, to time.Time
	if s := c.Query("from"); s != "" {
		if from, err = time.Parse(time.RFC3339, s); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code": 40000, "data": nil, "message": "invalid 'from' — use RFC3339",
			})
			return
		}
	}
	if s := c.Query("to"); s != "" {
		if to, err = time.Parse(time.RFC3339, s); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code": 40000, "data": nil, "message": "invalid 'to' — use RFC3339",
			})
			return
		}
	}

	limit := 100
	if s := c.Query("limit"); s != "" {
		if limit, err = strconv.Atoi(s); err != nil || limit <= 0 {
			limit = 100
		}
		if limit > 500 {
			limit = 500
		}
	}

	rows, err := h.msvc.ListMeasurements(c.Request.Context(), domainID, from, to, limit)
	if err != nil {
		h.logger.Error("list measurements", zap.Int64("domain_id", domainID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 50000, "data": nil, "message": "failed to list measurements",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"items": rows, "total": len(rows)}, "message": "ok"})
}

// GetLatestMeasurements handles GET /api/v1/gfw/measurements/:domainId/latest.
// Returns the most recent probe + control measurement pair for the domain.
func (h *ProbeNodeHandler) GetLatestMeasurements(c *gin.Context) {
	domainID, err := strconv.ParseInt(c.Param("domainId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 40000, "data": nil, "message": "invalid domain_id",
		})
		return
	}

	probe, control, err := h.msvc.GetLatestMeasurements(c.Request.Context(), domainID)
	if err != nil {
		h.logger.Error("get latest measurements", zap.Int64("domain_id", domainID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 50000, "data": nil, "message": "failed to get latest measurements",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{"probe": probe, "control": control},
		"message": "ok",
	})
}

// ListBogonIPs handles GET /api/v1/gfw/bogons.
func (h *ProbeNodeHandler) ListBogonIPs(c *gin.Context) {
	ips, err := h.msvc.ListBogonIPs(c.Request.Context())
	if err != nil {
		h.logger.Error("list bogon IPs", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 50000, "data": nil, "message": "failed to list bogon IPs",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"items": ips, "total": len(ips)}, "message": "ok"})
}

// AddBogonIP handles POST /api/v1/gfw/bogons.
func (h *ProbeNodeHandler) AddBogonIP(c *gin.Context) {
	var req struct {
		IP   string `json:"ip_address" binding:"required"`
		Note string `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 40000, "data": nil, "message": "ip_address is required",
		})
		return
	}
	if err := h.msvc.AddBogonIP(c.Request.Context(), req.IP, req.Note); err != nil {
		h.logger.Error("add bogon IP", zap.String("ip", req.IP), zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"code": 40000, "data": nil, "message": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"code": 0, "data": gin.H{"ip_address": req.IP}, "message": "ok"})
}

// DeleteBogonIP handles DELETE /api/v1/gfw/bogons/:ip.
func (h *ProbeNodeHandler) DeleteBogonIP(c *gin.Context) {
	ip := c.Param("ip")
	if err := h.msvc.DeleteBogonIP(c.Request.Context(), ip); err != nil {
		h.logger.Error("delete bogon IP", zap.String("ip", ip), zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"code": 40000, "data": nil, "message": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
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
