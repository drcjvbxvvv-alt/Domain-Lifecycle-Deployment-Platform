package handler

import (
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	tagsvc "domain-platform/internal/tag"
	"domain-platform/store/postgres"
)

// TagHandler handles HTTP requests for tags and bulk domain operations.
type TagHandler struct {
	svc    *tagsvc.Service
	logger *zap.Logger
}

func NewTagHandler(svc *tagsvc.Service, logger *zap.Logger) *TagHandler {
	return &TagHandler{svc: svc, logger: logger}
}

// ── Request / Response ────────────────────────────────────────────────────────

type CreateTagRequest struct {
	Name  string  `json:"name" binding:"required"`
	Color *string `json:"color"`
}

type UpdateTagRequest struct {
	Name  string  `json:"name" binding:"required"`
	Color *string `json:"color"`
}

type TagResponse struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	Color       *string `json:"color"`
	DomainCount *int64  `json:"domain_count,omitempty"`
}

func tagResponse(t *postgres.Tag) TagResponse {
	return TagResponse{ID: t.ID, Name: t.Name, Color: t.Color}
}

func tagWithCountResponse(t *postgres.TagWithCount) TagResponse {
	return TagResponse{ID: t.ID, Name: t.Name, Color: t.Color, DomainCount: &t.DomainCount}
}

type SetDomainTagsRequest struct {
	TagIDs []int64 `json:"tag_ids"`
}

type BulkActionRequest struct {
	DomainIDs          []int64 `json:"domain_ids" binding:"required"`
	Action             string  `json:"action" binding:"required"`
	TagIDs             []int64 `json:"tag_ids,omitempty"`
	RegistrarAccountID *int64  `json:"registrar_account_id,omitempty"`
	DNSProviderID      *int64  `json:"dns_provider_id,omitempty"`
	AutoRenew          *bool   `json:"auto_renew,omitempty"`
}

// ── Tag CRUD ──────────────────────────────────────────────────────────────────

// Create handles POST /api/v1/tags.
func (h *TagHandler) Create(c *gin.Context) {
	var req CreateTagRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "data": nil, "message": err.Error()})
		return
	}
	t, err := h.svc.Create(c.Request.Context(), tagsvc.CreateInput{Name: req.Name, Color: req.Color})
	if err != nil {
		if errors.Is(err, tagsvc.ErrEmptyName) || errors.Is(err, tagsvc.ErrInvalidColor) {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "data": nil, "message": err.Error()})
			return
		}
		if errors.Is(err, tagsvc.ErrDuplicateName) {
			c.JSON(http.StatusConflict, gin.H{"code": 40901, "data": nil, "message": err.Error()})
			return
		}
		h.logger.Error("create tag", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "data": nil, "message": "create tag failed"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"code": 0, "data": tagResponse(t), "message": "ok"})
}

// List handles GET /api/v1/tags (with domain counts).
func (h *TagHandler) List(c *gin.Context) {
	tags, err := h.svc.ListWithCounts(c.Request.Context())
	if err != nil {
		h.logger.Error("list tags", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "data": nil, "message": "list tags failed"})
		return
	}
	items := make([]TagResponse, len(tags))
	for i, t := range tags {
		items[i] = tagWithCountResponse(&t)
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"items": items, "total": len(items)}, "message": "ok"})
}

// Update handles PUT /api/v1/tags/:id.
func (h *TagHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "data": nil, "message": "invalid id"})
		return
	}
	var req UpdateTagRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "data": nil, "message": err.Error()})
		return
	}
	t, err := h.svc.Update(c.Request.Context(), id, req.Name, req.Color)
	if err != nil {
		if errors.Is(err, tagsvc.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"code": 40401, "data": nil, "message": "tag not found"})
			return
		}
		if errors.Is(err, tagsvc.ErrDuplicateName) {
			c.JSON(http.StatusConflict, gin.H{"code": 40901, "data": nil, "message": err.Error()})
			return
		}
		if errors.Is(err, tagsvc.ErrInvalidColor) || errors.Is(err, tagsvc.ErrEmptyName) {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "data": nil, "message": err.Error()})
			return
		}
		h.logger.Error("update tag", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "data": nil, "message": "update tag failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": tagResponse(t), "message": "ok"})
}

// Delete handles DELETE /api/v1/tags/:id.
func (h *TagHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "data": nil, "message": "invalid id"})
		return
	}
	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		if errors.Is(err, tagsvc.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"code": 40401, "data": nil, "message": "tag not found"})
			return
		}
		h.logger.Error("delete tag", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "data": nil, "message": "delete tag failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": nil, "message": "ok"})
}

// ── Domain tags ───────────────────────────────────────────────────────────────

// GetDomainTags handles GET /api/v1/domains/:id/tags.
func (h *TagHandler) GetDomainTags(c *gin.Context) {
	domainID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "data": nil, "message": "invalid domain id"})
		return
	}
	tags, err := h.svc.GetDomainTags(c.Request.Context(), domainID)
	if err != nil {
		h.logger.Error("get domain tags", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "data": nil, "message": "get domain tags failed"})
		return
	}
	items := make([]TagResponse, len(tags))
	for i, t := range tags {
		items[i] = tagResponse(&t)
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": items, "message": "ok"})
}

// SetDomainTags handles PUT /api/v1/domains/:id/tags.
func (h *TagHandler) SetDomainTags(c *gin.Context) {
	domainID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "data": nil, "message": "invalid domain id"})
		return
	}
	var req SetDomainTagsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "data": nil, "message": err.Error()})
		return
	}
	if err := h.svc.SetDomainTags(c.Request.Context(), domainID, req.TagIDs); err != nil {
		h.logger.Error("set domain tags", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "data": nil, "message": "set domain tags failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": nil, "message": "ok"})
}

// ── Bulk operations ───────────────────────────────────────────────────────────

// BulkAction handles POST /api/v1/domains/bulk.
// Supported actions: "update", "add_tags", "remove_tags".
func (h *TagHandler) BulkAction(c *gin.Context) {
	var req BulkActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "data": nil, "message": err.Error()})
		return
	}
	if len(req.DomainIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "data": nil, "message": "domain_ids is required"})
		return
	}
	if len(req.DomainIDs) > 500 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40003, "data": nil, "message": "max 500 domains per bulk operation"})
		return
	}

	ctx := c.Request.Context()
	switch req.Action {
	case "update":
		affected, err := h.svc.BulkUpdateFields(ctx, tagsvc.BulkUpdateInput{
			DomainIDs:          req.DomainIDs,
			RegistrarAccountID: req.RegistrarAccountID,
			DNSProviderID:      req.DNSProviderID,
			AutoRenew:          req.AutoRenew,
		})
		if err != nil {
			h.logger.Error("bulk update", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "data": nil, "message": "bulk update failed"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"affected": affected}, "message": "ok"})

	case "add_tags":
		if len(req.TagIDs) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40004, "data": nil, "message": "tag_ids required for add_tags"})
			return
		}
		if err := h.svc.BulkAddTags(ctx, req.DomainIDs, req.TagIDs); err != nil {
			h.logger.Error("bulk add tags", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "data": nil, "message": "bulk add tags failed"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"affected": len(req.DomainIDs)}, "message": "ok"})

	case "remove_tags":
		if len(req.TagIDs) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 40004, "data": nil, "message": "tag_ids required for remove_tags"})
			return
		}
		if err := h.svc.BulkRemoveTags(ctx, req.DomainIDs, req.TagIDs); err != nil {
			h.logger.Error("bulk remove tags", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "data": nil, "message": "bulk remove tags failed"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"affected": len(req.DomainIDs)}, "message": "ok"})

	default:
		c.JSON(http.StatusBadRequest, gin.H{"code": 40005, "data": nil, "message": "action must be update, add_tags, or remove_tags"})
	}
}

// ── CSV Export ─────────────────────────────────────────────────────────────────

// Export handles GET /api/v1/domains/export — streams CSV of domain data.
// Supported query params: project_id, tag_id, lifecycle_state, purpose,
// cdn_provider_id. The CSV includes enriched registrar and CDN display names.
func (h *TagHandler) Export(c *gin.Context) {
	ctx := c.Request.Context()
	f := postgres.ListFilter{Limit: 10000}
	if v := c.Query("project_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			f.ProjectID = &id
		}
	}
	if v := c.Query("tag_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			f.TagID = &id
		}
	}
	if v := c.Query("lifecycle_state"); v != "" {
		f.LifecycleState = &v
	}
	if v := c.Query("purpose"); v != "" {
		f.Purpose = &v
	}
	if v := c.Query("cdn_provider_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			f.CDNProviderID = &id
		}
	}

	domains, err := h.svc.ExportDomainsEnriched(ctx, f)
	if err != nil {
		h.logger.Error("export domains", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "data": nil, "message": "export failed"})
		return
	}

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=domains.csv")

	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{
		"id", "fqdn", "tld", "lifecycle_state", "project_id",
		"purpose",
		"registrar_name", "cdn_provider_type", "cdn_account_name",
		"origin_ips",
		"expiry_date", "auto_renew", "annual_cost", "currency",
		"tags",
	})
	for _, d := range domains {
		tags, _ := h.svc.GetDomainTags(ctx, d.ID)
		tagNames := make([]string, len(tags))
		for i, t := range tags {
			tagNames[i] = t.Name
		}
		expiryDate := ""
		if d.ExpiryDate != nil {
			expiryDate = d.ExpiryDate.Format("2006-01-02")
		}
		annualCost := ""
		if d.AnnualCost != nil {
			annualCost = fmt.Sprintf("%.2f", *d.AnnualCost)
		}
		currency := ""
		if d.Currency != nil {
			currency = *d.Currency
		}
		tld := ""
		if d.TLD != nil {
			tld = *d.TLD
		}
		purpose := ""
		if d.Purpose != nil {
			purpose = *d.Purpose
		}
		registrarName := ""
		if d.RegistrarName != nil {
			registrarName = *d.RegistrarName
		}
		cdnProviderType := ""
		if d.CDNProviderType != nil {
			cdnProviderType = *d.CDNProviderType
		}
		cdnAccountName := ""
		if d.CDNAccountName != nil {
			cdnAccountName = *d.CDNAccountName
		}
		originIPs := joinCSVTags([]string(d.OriginIPs)) // reuse semicolon joiner

		_ = w.Write([]string{
			strconv.FormatInt(d.ID, 10),
			d.FQDN,
			tld,
			d.LifecycleState,
			strconv.FormatInt(d.ProjectID, 10),
			purpose,
			registrarName,
			cdnProviderType,
			cdnAccountName,
			originIPs,
			expiryDate,
			strconv.FormatBool(d.AutoRenew),
			annualCost,
			currency,
			joinCSVTags(tagNames),
		})
	}
	w.Flush()
}

func joinCSVTags(tags []string) string {
	out := ""
	for i, t := range tags {
		if i > 0 {
			out += ";"
		}
		out += t
	}
	return out
}
