package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"domain-platform/internal/cost"
	"domain-platform/store/postgres"
)

// CostHandler handles HTTP requests for fee schedules and domain cost records.
type CostHandler struct {
	svc    *cost.Service
	logger *zap.Logger
}

func NewCostHandler(svc *cost.Service, logger *zap.Logger) *CostHandler {
	return &CostHandler{svc: svc, logger: logger}
}

// ── Request / Response types ──────────────────────────────────────────────────

type CreateFeeScheduleRequest struct {
	RegistrarID     int64   `json:"registrar_id" binding:"required"`
	TLD             string  `json:"tld" binding:"required"`
	RegistrationFee float64 `json:"registration_fee"`
	RenewalFee      float64 `json:"renewal_fee"`
	TransferFee     float64 `json:"transfer_fee"`
	PrivacyFee      float64 `json:"privacy_fee"`
	Currency        string  `json:"currency" binding:"required"`
}

type UpdateFeeScheduleRequest struct {
	RegistrationFee float64 `json:"registration_fee"`
	RenewalFee      float64 `json:"renewal_fee"`
	TransferFee     float64 `json:"transfer_fee"`
	PrivacyFee      float64 `json:"privacy_fee"`
	Currency        string  `json:"currency" binding:"required"`
}

type FeeScheduleResponse struct {
	ID              int64   `json:"id"`
	RegistrarID     int64   `json:"registrar_id"`
	TLD             string  `json:"tld"`
	RegistrationFee float64 `json:"registration_fee"`
	RenewalFee      float64 `json:"renewal_fee"`
	TransferFee     float64 `json:"transfer_fee"`
	PrivacyFee      float64 `json:"privacy_fee"`
	Currency        string  `json:"currency"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

func feeScheduleResponse(fs *postgres.DomainFeeSchedule) FeeScheduleResponse {
	return FeeScheduleResponse{
		ID:              fs.ID,
		RegistrarID:     fs.RegistrarID,
		TLD:             fs.TLD,
		RegistrationFee: fs.RegistrationFee,
		RenewalFee:      fs.RenewalFee,
		TransferFee:     fs.TransferFee,
		PrivacyFee:      fs.PrivacyFee,
		Currency:        fs.Currency,
		CreatedAt:       fs.CreatedAt.Format(time.RFC3339),
		UpdatedAt:       fs.UpdatedAt.Format(time.RFC3339),
	}
}

type CreateCostRequest struct {
	CostType    string  `json:"cost_type" binding:"required"`
	Amount      float64 `json:"amount" binding:"required"`
	Currency    string  `json:"currency" binding:"required"`
	PeriodStart *string `json:"period_start"` // YYYY-MM-DD
	PeriodEnd   *string `json:"period_end"`   // YYYY-MM-DD
	PaidAt      *string `json:"paid_at"`      // YYYY-MM-DD
	Notes       *string `json:"notes"`
}

type DomainCostResponse struct {
	ID          int64   `json:"id"`
	DomainID    int64   `json:"domain_id"`
	CostType    string  `json:"cost_type"`
	Amount      float64 `json:"amount"`
	Currency    string  `json:"currency"`
	PeriodStart *string `json:"period_start"`
	PeriodEnd   *string `json:"period_end"`
	PaidAt      *string `json:"paid_at"`
	Notes       *string `json:"notes"`
	CreatedAt   string  `json:"created_at"`
}

func domainCostResponse(c *postgres.DomainCost) DomainCostResponse {
	r := DomainCostResponse{
		ID:        c.ID,
		DomainID:  c.DomainID,
		CostType:  c.CostType,
		Amount:    c.Amount,
		Currency:  c.Currency,
		Notes:     c.Notes,
		CreatedAt: c.CreatedAt.Format(time.RFC3339),
	}
	if c.PeriodStart != nil {
		s := c.PeriodStart.Format("2006-01-02")
		r.PeriodStart = &s
	}
	if c.PeriodEnd != nil {
		s := c.PeriodEnd.Format("2006-01-02")
		r.PeriodEnd = &s
	}
	if c.PaidAt != nil {
		s := c.PaidAt.Format("2006-01-02")
		r.PaidAt = &s
	}
	return r
}

// ── Fee Schedule Handlers ─────────────────────────────────────────────────────

// CreateFeeSchedule handles POST /api/v1/fee-schedules.
func (h *CostHandler) CreateFeeSchedule(c *gin.Context) {
	var req CreateFeeScheduleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "data": nil, "message": err.Error()})
		return
	}

	fs, err := h.svc.CreateFeeSchedule(c.Request.Context(), cost.CreateFeeScheduleInput{
		RegistrarID:     req.RegistrarID,
		TLD:             req.TLD,
		RegistrationFee: req.RegistrationFee,
		RenewalFee:      req.RenewalFee,
		TransferFee:     req.TransferFee,
		PrivacyFee:      req.PrivacyFee,
		Currency:        req.Currency,
	})
	if err != nil {
		if errors.Is(err, cost.ErrInvalidCurrency) {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "data": nil, "message": err.Error()})
			return
		}
		h.logger.Error("create fee schedule", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "data": nil, "message": "create fee schedule failed"})
		return
	}

	resp := feeScheduleResponse(fs)
	c.JSON(http.StatusCreated, gin.H{"code": 0, "data": resp, "message": "ok"})
}

// ListFeeSchedules handles GET /api/v1/fee-schedules?registrar_id=N.
func (h *CostHandler) ListFeeSchedules(c *gin.Context) {
	var registrarID *int64
	if v := c.Query("registrar_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "data": nil, "message": "invalid registrar_id"})
			return
		}
		registrarID = &id
	}

	schedules, err := h.svc.ListFeeSchedules(c.Request.Context(), registrarID)
	if err != nil {
		h.logger.Error("list fee schedules", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "data": nil, "message": "list fee schedules failed"})
		return
	}

	items := make([]FeeScheduleResponse, len(schedules))
	for i, fs := range schedules {
		items[i] = feeScheduleResponse(&fs)
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"items": items, "total": len(items)}, "message": "ok"})
}

// UpdateFeeSchedule handles PUT /api/v1/fee-schedules/:id.
func (h *CostHandler) UpdateFeeSchedule(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "data": nil, "message": "invalid id"})
		return
	}

	var req UpdateFeeScheduleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "data": nil, "message": err.Error()})
		return
	}

	updated, err := h.svc.UpdateFeeSchedule(c.Request.Context(), id, cost.UpdateFeeScheduleInput{
		RegistrationFee: req.RegistrationFee,
		RenewalFee:      req.RenewalFee,
		TransferFee:     req.TransferFee,
		PrivacyFee:      req.PrivacyFee,
		Currency:        req.Currency,
	})
	if err != nil {
		if errors.Is(err, cost.ErrFeeScheduleNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"code": 40401, "data": nil, "message": "fee schedule not found"})
			return
		}
		if errors.Is(err, cost.ErrInvalidCurrency) {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "data": nil, "message": err.Error()})
			return
		}
		h.logger.Error("update fee schedule", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "data": nil, "message": "update fee schedule failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": feeScheduleResponse(updated), "message": "ok"})
}

// DeleteFeeSchedule handles DELETE /api/v1/fee-schedules/:id.
func (h *CostHandler) DeleteFeeSchedule(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "data": nil, "message": "invalid id"})
		return
	}

	if err := h.svc.DeleteFeeSchedule(c.Request.Context(), id); err != nil {
		if errors.Is(err, cost.ErrFeeScheduleNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"code": 40401, "data": nil, "message": "fee schedule not found"})
			return
		}
		h.logger.Error("delete fee schedule", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "data": nil, "message": "delete fee schedule failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": nil, "message": "ok"})
}

// ── Domain Cost Handlers ──────────────────────────────────────────────────────

// CreateDomainCost handles POST /api/v1/domains/:id/costs.
func (h *CostHandler) CreateDomainCost(c *gin.Context) {
	domainID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "data": nil, "message": "invalid domain id"})
		return
	}

	var req CreateCostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "data": nil, "message": err.Error()})
		return
	}

	in := cost.CreateCostInput{
		DomainID: domainID,
		CostType: req.CostType,
		Amount:   req.Amount,
		Currency: req.Currency,
		Notes:    req.Notes,
	}
	if req.PeriodStart != nil {
		if t, err := time.Parse("2006-01-02", *req.PeriodStart); err == nil {
			in.PeriodStart = &t
		}
	}
	if req.PeriodEnd != nil {
		if t, err := time.Parse("2006-01-02", *req.PeriodEnd); err == nil {
			in.PeriodEnd = &t
		}
	}
	if req.PaidAt != nil {
		if t, err := time.Parse("2006-01-02", *req.PaidAt); err == nil {
			in.PaidAt = &t
		}
	}

	created, err := h.svc.CreateCost(c.Request.Context(), in)
	if err != nil {
		if errors.Is(err, cost.ErrInvalidCostType) {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40003, "data": nil, "message": err.Error()})
			return
		}
		if errors.Is(err, cost.ErrInvalidCurrency) {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40004, "data": nil, "message": err.Error()})
			return
		}
		h.logger.Error("create domain cost", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "data": nil, "message": "create cost failed"})
		return
	}

	resp := domainCostResponse(created)
	c.JSON(http.StatusCreated, gin.H{"code": 0, "data": resp, "message": "ok"})
}

// ListDomainCosts handles GET /api/v1/domains/:id/costs.
func (h *CostHandler) ListDomainCosts(c *gin.Context) {
	domainID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "data": nil, "message": "invalid domain id"})
		return
	}

	costs, err := h.svc.ListCostsByDomain(c.Request.Context(), domainID)
	if err != nil {
		h.logger.Error("list domain costs", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "data": nil, "message": "list costs failed"})
		return
	}

	items := make([]DomainCostResponse, len(costs))
	for i, co := range costs {
		items[i] = domainCostResponse(&co)
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"items": items, "total": len(items)}, "message": "ok"})
}

// ── Summary Handler ───────────────────────────────────────────────────────────

type CostSummaryResponse struct {
	GroupKey  string  `json:"group_key"`
	TotalCost float64 `json:"total_cost"`
	Currency  string  `json:"currency"`
	Count     int64   `json:"count"`
}

// GetCostSummary handles GET /api/v1/costs/summary?group_by=registrar|tld.
func (h *CostHandler) GetCostSummary(c *gin.Context) {
	groupBy := c.DefaultQuery("group_by", "registrar")

	summaries, err := h.svc.GetCostSummary(c.Request.Context(), groupBy)
	if err != nil {
		if err.Error() != "" && len(err.Error()) > 0 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "data": nil, "message": err.Error()})
			return
		}
		h.logger.Error("get cost summary", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "data": nil, "message": "cost summary failed"})
		return
	}

	items := make([]CostSummaryResponse, len(summaries))
	for i, s := range summaries {
		items[i] = CostSummaryResponse{
			GroupKey:  s.GroupKey,
			TotalCost: s.TotalCost,
			Currency:  s.Currency,
			Count:     s.Count,
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"items":    items,
		"total":    len(items),
		"group_by": groupBy,
	}, "message": "ok"})
}
