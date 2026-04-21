package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"domain-platform/internal/domain"
)

// ExpiryHandler serves expiry dashboard data.
type ExpiryHandler struct {
	svc    *domain.ExpiryService
	logger *zap.Logger
}

func NewExpiryHandler(svc *domain.ExpiryService, logger *zap.Logger) *ExpiryHandler {
	return &ExpiryHandler{svc: svc, logger: logger}
}

// Dashboard handles GET /api/v1/dashboard/expiry.
func (h *ExpiryHandler) Dashboard(c *gin.Context) {
	data, err := h.svc.GetDashboardData(c.Request.Context())
	if err != nil {
		h.logger.Error("expiry dashboard", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "data": nil, "message": "dashboard data failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": data, "message": "ok"})
}
