package admin

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"github.com/stevenchen/llm-gateway/internal/api/dto"
	"github.com/stevenchen/llm-gateway/internal/domain"
	"github.com/stevenchen/llm-gateway/internal/pkg/response"
	"github.com/stevenchen/llm-gateway/internal/service"
)

type ModelHandler struct {
	market *service.ModelMarketService
}

func NewModelHandler(market *service.ModelMarketService) *ModelHandler {
	return &ModelHandler{market: market}
}

func (h *ModelHandler) Create(c *gin.Context) {
	var req dto.CreateModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.AdminError(c, http.StatusBadRequest, err.Error())
		return
	}

	m := &domain.Model{
		ProviderID: req.ProviderID, VendorID: req.VendorID, ModelKey: req.ModelKey, DisplayName: req.DisplayName,
		Type: domain.ModelType(req.Type), TPMLimit: req.TPMLimit, QPSLimit: req.QPSLimit, CreatedBy: req.CreatedBy,
		InputPricePer1K:  parseDecimal(req.InputPricePer1K),
		OutputPricePer1K: parseDecimal(req.OutputPricePer1K),
		Category:         req.Category, ContextLength: req.ContextLength, Tags: req.Tags, Description: req.Description,
		ReleasedAt: parseDate(req.ReleasedAt),
	}
	if err := h.market.CreateModel(c.Request.Context(), m); err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusCreated, m)
}

func (h *ModelHandler) Update(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		response.AdminError(c, http.StatusBadRequest, "invalid id")
		return
	}
	var req dto.UpdateModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.AdminError(c, http.StatusBadRequest, err.Error())
		return
	}

	mwp, err := h.market.GetModel(c.Request.Context(), id)
	if err != nil {
		writeAdminError(c, err)
		return
	}
	m := mwp.Model
	if req.DisplayName != "" {
		m.DisplayName = req.DisplayName
	}
	if req.InputPricePer1K != "" {
		m.InputPricePer1K = parseDecimal(req.InputPricePer1K)
	}
	if req.OutputPricePer1K != "" {
		m.OutputPricePer1K = parseDecimal(req.OutputPricePer1K)
	}
	if req.TPMLimit != nil {
		m.TPMLimit = *req.TPMLimit
	}
	if req.QPSLimit != nil {
		m.QPSLimit = *req.QPSLimit
	}
	if req.Status != "" {
		m.Status = domain.ModelStatus(req.Status)
	}
	if req.Category != nil {
		m.Category = *req.Category
	}
	if req.ContextLength != nil {
		m.ContextLength = *req.ContextLength
	}
	if req.Tags != nil {
		m.Tags = *req.Tags
	}
	if req.Description != nil {
		m.Description = *req.Description
	}
	if req.ReleasedAt != nil {
		m.ReleasedAt = parseDate(*req.ReleasedAt)
	}
	if req.VendorID != nil {
		m.VendorID = req.VendorID
	}

	if err := h.market.UpdateModel(c.Request.Context(), &m); err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, m)
}

// List implements the "模型市场" marketplace listing surface.
func (h *ModelHandler) List(c *gin.Context) {
	models, err := h.market.ListMarket(c.Request.Context())
	if err != nil {
		writeAdminError(c, err)
		return
	}
	providerID := queryInt64Ptr(c, "provider_id")
	vendorID := queryInt64Ptr(c, "vendor_id")
	respondList(c, filterList(models, func(m *domain.ModelWithProvider) bool {
		return (providerID == nil || m.ProviderID == *providerID) &&
			(vendorID == nil || (m.VendorID != nil && *m.VendorID == *vendorID)) && eqParam(c, "category", m.Category) &&
			eqParam(c, "status", string(m.Status)) && matchQ(c, m.DisplayName, m.ModelKey, m.Description, m.Provider.Name)
	}))
}

func (h *ModelHandler) Get(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		response.AdminError(c, http.StatusBadRequest, "invalid id")
		return
	}
	m, err := h.market.GetModel(c.Request.Context(), id)
	if err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, m)
}

// Try implements "模型体验": a direct one-shot call against this model so a
// business user can evaluate it before requesting a routed alias.
func (h *ModelHandler) Try(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		response.AdminError(c, http.StatusBadRequest, "invalid id")
		return
	}
	var req dto.TryModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.AdminError(c, http.StatusBadRequest, err.Error())
		return
	}

	resp, err := h.market.TryModel(c.Request.Context(), id, req.Prompt)
	if err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, resp)
}

func parseDecimal(s string) decimal.Decimal {
	if s == "" {
		return decimal.Zero
	}
	d, err := decimal.NewFromString(s)
	if err != nil {
		return decimal.Zero
	}
	return d
}

func parseDate(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.ParseInLocation("2006-01-02", s, time.Local)
	if err != nil {
		return nil
	}
	return &t
}
