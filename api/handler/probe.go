package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"domain-platform/store/postgres"
)

// ProbeHandler handles REST endpoints for probe policies and results.
type ProbeHandler struct {
	store *postgres.ProbeStore
}

func NewProbeHandler(store *postgres.ProbeStore) *ProbeHandler {
	return &ProbeHandler{store: store}
}

// ── Probe Policies ────────────────────────────────────────────────────────────

// ListPolicies GET /probe-policies
func (h *ProbeHandler) ListPolicies(c *gin.Context) {
	var projectID *int64
	if pid := c.Query("project_id"); pid != "" {
		v, err := strconv.ParseInt(pid, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40000, "data": nil, "message": "invalid project_id"})
			return
		}
		projectID = &v
	}

	policies, err := h.store.ListPolicies(c.Request.Context(), projectID, false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "data": nil, "message": "internal error"})
		return
	}

	items := make([]gin.H, len(policies))
	for i, p := range policies {
		items[i] = probePolicyResponse(&p)
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": gin.H{"items": items, "total": len(items)}})
}

// GetPolicy GET /probe-policies/:id
func (h *ProbeHandler) GetPolicy(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40000, "data": nil, "message": "invalid id"})
		return
	}
	p, err := h.store.GetPolicy(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, postgres.ErrProbePolicyNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"code": 40400, "data": nil, "message": "probe policy not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "data": nil, "message": "internal error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": probePolicyResponse(p)})
}

// CreatePolicy POST /probe-policies
func (h *ProbeHandler) CreatePolicy(c *gin.Context) {
	var req struct {
		ProjectID       *int64  `json:"project_id"`
		Name            string  `json:"name" binding:"required"`
		Tier            int16   `json:"tier" binding:"required,min=1,max=3"`
		IntervalSeconds int     `json:"interval_seconds" binding:"required,min=10"`
		TimeoutSeconds  int     `json:"timeout_seconds"`
		ExpectedStatus  *int    `json:"expected_status"`
		ExpectedKeyword *string `json:"expected_keyword"`
		ExpectedMetaTag *string `json:"expected_meta_tag"`
		Enabled         bool    `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40000, "data": nil, "message": err.Error()})
		return
	}
	if req.TimeoutSeconds <= 0 {
		req.TimeoutSeconds = 8
	}

	p := &postgres.ProbePolicy{
		ProjectID:       req.ProjectID,
		Name:            req.Name,
		Tier:            req.Tier,
		IntervalSeconds: req.IntervalSeconds,
		TimeoutSeconds:  req.TimeoutSeconds,
		ExpectedStatus:  req.ExpectedStatus,
		ExpectedKeyword: req.ExpectedKeyword,
		ExpectedMetaTag: req.ExpectedMetaTag,
		Enabled:         req.Enabled,
	}
	if err := h.store.CreatePolicy(c.Request.Context(), p); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "data": nil, "message": "internal error"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"code": 0, "message": "ok", "data": probePolicyResponse(p)})
}

// UpdatePolicy PUT /probe-policies/:id
func (h *ProbeHandler) UpdatePolicy(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40000, "data": nil, "message": "invalid id"})
		return
	}

	existing, err := h.store.GetPolicy(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, postgres.ErrProbePolicyNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"code": 40400, "data": nil, "message": "probe policy not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "data": nil, "message": "internal error"})
		return
	}

	var req struct {
		Name            *string `json:"name"`
		Tier            *int16  `json:"tier"`
		IntervalSeconds *int    `json:"interval_seconds"`
		TimeoutSeconds  *int    `json:"timeout_seconds"`
		ExpectedStatus  *int    `json:"expected_status"`
		ExpectedKeyword *string `json:"expected_keyword"`
		ExpectedMetaTag *string `json:"expected_meta_tag"`
		Enabled         *bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40000, "data": nil, "message": err.Error()})
		return
	}

	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.Tier != nil {
		existing.Tier = *req.Tier
	}
	if req.IntervalSeconds != nil {
		existing.IntervalSeconds = *req.IntervalSeconds
	}
	if req.TimeoutSeconds != nil {
		existing.TimeoutSeconds = *req.TimeoutSeconds
	}
	if req.ExpectedStatus != nil {
		existing.ExpectedStatus = req.ExpectedStatus
	}
	if req.ExpectedKeyword != nil {
		existing.ExpectedKeyword = req.ExpectedKeyword
	}
	if req.ExpectedMetaTag != nil {
		existing.ExpectedMetaTag = req.ExpectedMetaTag
	}
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}

	if err := h.store.UpdatePolicy(c.Request.Context(), existing); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "data": nil, "message": "internal error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": probePolicyResponse(existing)})
}

// DeletePolicy DELETE /probe-policies/:id
func (h *ProbeHandler) DeletePolicy(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40000, "data": nil, "message": "invalid id"})
		return
	}
	if err := h.store.DeletePolicy(c.Request.Context(), id); err != nil {
		if errors.Is(err, postgres.ErrProbePolicyNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"code": 40400, "data": nil, "message": "probe policy not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "data": nil, "message": "internal error"})
		return
	}
	c.Status(http.StatusNoContent)
}

// ── Domain Probe Results ──────────────────────────────────────────────────────

// ListDomainResults GET /domains/:id/probe-results
func (h *ProbeHandler) ListDomainResults(c *gin.Context) {
	domainID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40000, "data": nil, "message": "invalid domain id"})
		return
	}

	var tier *int16
	if t := c.Query("tier"); t != "" {
		v, err := strconv.ParseInt(t, 10, 16)
		if err != nil || v < 1 || v > 3 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40000, "data": nil, "message": "tier must be 1, 2, or 3"})
			return
		}
		t16 := int16(v)
		tier = &t16
	}

	limit := 50
	if l := c.Query("limit"); l != "" {
		v, _ := strconv.Atoi(l)
		if v > 0 && v <= 500 {
			limit = v
		}
	}

	results, err := h.store.ListDomainResults(c.Request.Context(), domainID, tier, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "data": nil, "message": "internal error"})
		return
	}

	items := make([]gin.H, len(results))
	for i, r := range results {
		items[i] = probeResultResponse(&r)
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": gin.H{"items": items, "total": len(items)}})
}

// ── Response builders ─────────────────────────────────────────────────────────

func probePolicyResponse(p *postgres.ProbePolicy) gin.H {
	return gin.H{
		"id":               p.ID,
		"uuid":             p.UUID,
		"project_id":       p.ProjectID,
		"name":             p.Name,
		"tier":             p.Tier,
		"interval_seconds": p.IntervalSeconds,
		"timeout_seconds":  p.TimeoutSeconds,
		"expected_status":  p.ExpectedStatus,
		"expected_keyword": p.ExpectedKeyword,
		"expected_meta_tag": p.ExpectedMetaTag,
		"enabled":          p.Enabled,
		"created_at":       p.CreatedAt,
		"updated_at":       p.UpdatedAt,
	}
}

func probeResultResponse(r *postgres.ProbeResult) gin.H {
	return gin.H{
		"id":               r.ID,
		"domain_id":        r.DomainID,
		"policy_id":        r.PolicyID,
		"probe_task_id":    r.ProbeTaskID,
		"tier":             r.Tier,
		"status":           r.Status,
		"http_status":      r.HTTPStatus,
		"response_time_ms": r.ResponseTimeMS,
		"response_size_b":  r.ResponseSizeB,
		"tls_handshake_ok": r.TLSHandshakeOK,
		"cert_expires_at":  r.CertExpiresAt,
		"content_hash":     r.ContentHash,
		"error_message":    r.ErrorMessage,
		"detail":           r.Detail,
		"checked_at":       r.CheckedAt,
	}
}
