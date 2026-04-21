package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"go.uber.org/zap"

	"domain-platform/internal/dnsquery"
	"domain-platform/internal/lifecycle"
	"domain-platform/internal/tasks"
	"domain-platform/store/postgres"
)

// DNSQueryHandler handles live DNS record lookups for domains.
type DNSQueryHandler struct {
	dns          *dnsquery.Service
	lifecycle    *lifecycle.Service
	dnsProviders *postgres.DNSProviderStore
	asynqClient  *asynq.Client
	logger       *zap.Logger
}

// NewDNSQueryHandler constructs a DNSQueryHandler.
func NewDNSQueryHandler(dns *dnsquery.Service, lifecycle *lifecycle.Service, dnsProviders *postgres.DNSProviderStore, asynqClient *asynq.Client, logger *zap.Logger) *DNSQueryHandler {
	return &DNSQueryHandler{dns: dns, lifecycle: lifecycle, dnsProviders: dnsProviders, asynqClient: asynqClient, logger: logger}
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

// PropagationByDomain handles GET /api/v1/domains/:id/dns-propagation
// Checks DNS propagation across multiple public resolvers + authoritative NS.
// Query param: types (comma-separated, default "A,AAAA")
func (h *DNSQueryHandler) PropagationByDomain(c *gin.Context) {
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

	queryTypes := parseQueryTypes(c.Query("types"))

	result := h.dns.CheckPropagation(c.Request.Context(), domain.FQDN, queryTypes)

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": result, "message": "ok"})
}

// PropagationByFQDN handles GET /api/v1/dns/propagation?fqdn=example.com
// Query param: types (comma-separated, default "A,AAAA")
func (h *DNSQueryHandler) PropagationByFQDN(c *gin.Context) {
	fqdn := c.Query("fqdn")
	if fqdn == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "data": nil, "message": "fqdn query parameter is required"})
		return
	}

	queryTypes := parseQueryTypes(c.Query("types"))

	result := h.dns.CheckPropagation(c.Request.Context(), fqdn, queryTypes)

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": result, "message": "ok"})
}

// DriftCheck handles GET /api/v1/domains/:id/dns-drift
// Compares DNS records from the provider API ("expected") against live DNS ("actual").
// Requires the domain to have a dns_provider_id configured with valid credentials.
func (h *DNSQueryHandler) DriftCheck(c *gin.Context) {
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

	// Fetch the DNS provider if one is configured
	var provider *postgres.DNSProvider
	if domain.DNSProviderID != nil {
		p, provErr := h.dnsProviders.GetByID(c.Request.Context(), *domain.DNSProviderID)
		if provErr == nil {
			provider = p
		}
	}

	result := h.dns.CheckDrift(c.Request.Context(), domain, provider)

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": result, "message": "ok"})
}

// TriggerDriftCheckAll handles POST /api/v1/dns/drift-check-all
// Manually enqueues a TypeDNSDriftCheckAll task for immediate execution.
// The scheduled task also runs automatically every 30 minutes via the worker scheduler.
func (h *DNSQueryHandler) TriggerDriftCheckAll(c *gin.Context) {
	payload, _ := json.Marshal(tasks.DNSDriftCheckAllPayload{})
	task := asynq.NewTask(tasks.TypeDNSDriftCheckAll, payload,
		asynq.Queue("default"),
		asynq.MaxRetry(1),
		asynq.Timeout(5*time.Minute),
	)

	info, err := h.asynqClient.EnqueueContext(c.Request.Context(), task)
	if err != nil {
		h.logger.Error("enqueue dns:drift_check_all", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "data": nil, "message": "failed to enqueue drift check"})
		return
	}

	h.logger.Info("dns:drift_check_all enqueued manually", zap.String("task_id", info.ID))
	c.JSON(http.StatusAccepted, gin.H{
		"code":    0,
		"data":    gin.H{"task_id": info.ID, "queue": info.Queue},
		"message": "drift check enqueued",
	})
}

// parseQueryTypes splits a comma-separated types string (e.g. "A,AAAA,MX")
// into a slice. Returns nil if empty (service will use defaults).
func parseQueryTypes(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(strings.ToUpper(p))
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
