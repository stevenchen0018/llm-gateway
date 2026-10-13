package admin

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/stevenchen/llm-gateway/internal/api/middleware"
	"github.com/stevenchen/llm-gateway/internal/domain"
	"github.com/stevenchen/llm-gateway/internal/pkg/response"
	"github.com/stevenchen/llm-gateway/internal/service"
)

// ParamRuleHandler manages parameter compatibility rules (接口参数适配).
type ParamRuleHandler struct {
	rules  *service.ParamRuleService
	market *service.ModelMarketService
}

func NewParamRuleHandler(rules *service.ParamRuleService, market *service.ModelMarketService) *ParamRuleHandler {
	return &ParamRuleHandler{rules: rules, market: market}
}

type paramRuleReq struct {
	Name         string               `json:"name"`
	Enabled      *bool                `json:"enabled"`
	Protocol     string               `json:"protocol"`
	ProviderID   *int64               `json:"provider_id"`
	ModelPattern string               `json:"model_pattern"`
	Op           string               `json:"op"`
	Actions      []domain.ParamAction `json:"actions"`
	Priority     int                  `json:"priority"`
	Remark       string               `json:"remark"`
}

func (r paramRuleReq) apply(p *domain.ParamRule) {
	p.Name, p.Protocol, p.ModelPattern, p.Op, p.Actions, p.Priority, p.Remark =
		r.Name, r.Protocol, r.ModelPattern, r.Op, r.Actions, r.Priority, strings.TrimSpace(r.Remark)
	p.ProviderID = r.ProviderID
	if p.ProviderID != nil && *p.ProviderID <= 0 {
		p.ProviderID = nil
	}
	if r.Enabled != nil {
		p.Enabled = *r.Enabled
	}
}

func (h *ParamRuleHandler) List(c *gin.Context) {
	rules, err := h.rules.List(c.Request.Context())
	if err != nil {
		writeAdminError(c, err)
		return
	}
	respondList(c, filterList(rules, func(r *domain.ParamRule) bool {
		return eqParam(c, "source", r.Source) && eqParam(c, "protocol", r.Protocol) && matchQ(c, r.Name, r.ModelPattern, r.Remark)
	}))
}

func (h *ParamRuleHandler) Create(c *gin.Context) {
	var req paramRuleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.AdminError(c, http.StatusBadRequest, err.Error())
		return
	}
	r := &domain.ParamRule{Enabled: true, CreatedBy: middleware.AdminUserFromContext(c)}
	req.apply(r)
	if err := h.rules.Create(c.Request.Context(), r); err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusCreated, r)
}

func (h *ParamRuleHandler) Update(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		response.AdminError(c, http.StatusBadRequest, "invalid id")
		return
	}
	var req paramRuleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.AdminError(c, http.StatusBadRequest, err.Error())
		return
	}
	r, err := h.rules.Get(c.Request.Context(), id)
	if err != nil {
		writeAdminError(c, err)
		return
	}
	if req.Name == "" && len(req.Actions) == 0 && req.Enabled != nil { // switch only
		r.Enabled = *req.Enabled
	} else {
		req.apply(r)
	}
	if err := h.rules.Update(c.Request.Context(), r); err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, r)
}

func (h *ParamRuleHandler) Delete(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		response.AdminError(c, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.rules.Delete(c.Request.Context(), id); err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, gin.H{"deleted": id})
}

// Preview runs the rules over a sample body for one supplier + model.
func (h *ParamRuleHandler) Preview(c *gin.Context) {
	var req struct {
		ProviderID int64          `json:"provider_id" binding:"required"`
		Model      string         `json:"model" binding:"required"`
		Op         string         `json:"op"`
		Body       map[string]any `json:"body"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.AdminError(c, http.StatusBadRequest, err.Error())
		return
	}
	p, err := h.market.GetProvider(c.Request.Context(), req.ProviderID)
	if err != nil {
		writeAdminError(c, err)
		return
	}
	if req.Op == "" {
		req.Op = domain.ParamOpChat
	}
	if req.Body == nil {
		req.Body = map[string]any{}
	}
	response.AdminOK(c, http.StatusOK, h.rules.Preview(c.Request.Context(), *p, req.Model, req.Op, req.Body))
}
