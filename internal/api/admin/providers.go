package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"github.com/stevenchen/llm-gateway/internal/api/dto"
	"github.com/stevenchen/llm-gateway/internal/domain"
	"github.com/stevenchen/llm-gateway/internal/pkg/response"
	"github.com/stevenchen/llm-gateway/internal/service"
)

type ProviderHandler struct {
	market *service.ModelMarketService
}

func NewProviderHandler(market *service.ModelMarketService) *ProviderHandler {
	return &ProviderHandler{market: market}
}

func (h *ProviderHandler) Create(c *gin.Context) {
	var req dto.CreateProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.AdminError(c, http.StatusBadRequest, err.Error())
		return
	}

	p := &domain.Provider{Code: req.Code, Name: req.Name, BaseURL: req.BaseURL, AuthType: req.AuthType, AuthValue: req.AuthValue,
		DiscountRate: parseDiscount(req.DiscountRate), Description: req.Description, SupplierType: req.SupplierType, Contact: req.Contact}
	if err := h.market.CreateProvider(c.Request.Context(), p); err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusCreated, p)
}

func (h *ProviderHandler) Update(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		response.AdminError(c, http.StatusBadRequest, "invalid id")
		return
	}
	var req dto.UpdateProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.AdminError(c, http.StatusBadRequest, err.Error())
		return
	}

	p, err := h.market.GetProvider(c.Request.Context(), id)
	if err != nil {
		writeAdminError(c, err)
		return
	}
	if req.Name != "" {
		p.Name = req.Name
	}
	if req.BaseURL != "" {
		p.BaseURL = req.BaseURL
	}
	if req.AuthType != nil {
		p.AuthType = *req.AuthType
	}
	if req.AuthValue != "" {
		p.AuthValue = req.AuthValue
	}
	if req.Status != "" {
		p.Status = domain.ProviderStatus(req.Status)
	}
	if req.DiscountRate != nil {
		p.DiscountRate = parseDiscount(*req.DiscountRate)
	}
	if req.Description != nil {
		p.Description = *req.Description
	}
	if req.SupplierType != nil {
		p.SupplierType = *req.SupplierType
	}
	if req.Contact != nil {
		p.Contact = *req.Contact
	}

	if err := h.market.UpdateProvider(c.Request.Context(), p); err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, p)
}

func (h *ProviderHandler) List(c *gin.Context) {
	providers, err := h.market.ListProviders(c.Request.Context())
	if err != nil {
		writeAdminError(c, err)
		return
	}
	respondList(c, filterList(providers, func(p *domain.Provider) bool {
		return eqParam(c, "status", string(p.Status)) && eqParam(c, "supplier_type", p.SupplierType) &&
			matchQ(c, p.Name, p.Code, p.Description, p.BaseURL, p.Contact)
	}))
}

func (h *ProviderHandler) Get(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		response.AdminError(c, http.StatusBadRequest, "invalid id")
		return
	}
	p, err := h.market.GetProvider(c.Request.Context(), id)
	if err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, p)
}

// parseDiscount turns "0.85" into a rate; empty/invalid/out-of-range means no
// discount (1).
func parseDiscount(s string) decimal.Decimal {
	d, err := decimal.NewFromString(s)
	if err != nil || d.LessThanOrEqual(decimal.Zero) || d.GreaterThan(decimal.NewFromInt(1)) {
		return decimal.NewFromInt(1)
	}
	return d
}
