package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"domain-platform/internal/dnsquery"
	"domain-platform/internal/lifecycle"
)

// DNSQueryHandler handles live DNS record lookups for domains.
type DNSQueryHandler struct {
	dns       *dnsquery.Service
	lifecycle *lifecycle.Service
	logger    *zap.Logger
}

// NewDNSQueryHandler constructs a DNSQueryHandler.
func NewDNSQueryHandler(dns *dnsquery.Service, lifecycle *lifecycle.Service, logger *zap.Logger) *DNSQueryHandler {
	return &DNSQueryHandler{dns: dns, lifecycle: lifecycle, logger: logger}
}

// LookupByDomain handles GET /api/v1/domains/:id/dns-records
// Fetches the domain's FQDN from the database, then performs a live DNS lookup.
func (h *DNSQueryHandler) LookupByDomain(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "data": nil, "message": "invalid domain id"})
		return
	}

	domain, err := h.lifecycle.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 40401, "data": nil, "message": "domain not found"})
		return
	}

	result := h.dns.Lookup(c.Request.Context(), domain.FQDN)

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": result, "message": "ok"})
}

// LookupByFQDN handles GET /api/v1/dns/lookup?fqdn=example.com
// Performs a live DNS lookup for any arbitrary FQDN (not necessarily in the DB).
func (h *DNSQueryHandler) LookupByFQDN(c *gin.Context) {
	fqdn := c.Query("fqdn")
	if fqdn == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "data": nil, "message": "fqdn query parameter is required"})
		return
	}

	result := h.dns.Lookup(c.Request.Context(), fqdn)

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": result, "message": "ok"})
}
