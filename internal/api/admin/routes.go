package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/stevenchen/llm-gateway/internal/api/dto"
	"github.com/stevenchen/llm-gateway/internal/api/middleware"
	"github.com/stevenchen/llm-gateway/internal/domain"
	"github.com/stevenchen/llm-gateway/internal/pkg/response"
	"github.com/stevenchen/llm-gateway/internal/service"
)

// RoutingHandler manages 模型调度 policies — the (application, API key,
// source model) → target model mappings GatewayService resolves on every
// gateway call — and exposes a simulator that explains a resolution.
type RoutingHandler struct {
	routing *service.RoutingService
	keys    *service.APIKeyService
}

func NewRoutingHandler(routing *service.RoutingService, keys *service.APIKeyService) *RoutingHandler {
	return &RoutingHandler{routing: routing, keys: keys}
}

// keyDept returns the department owning keyID (nil for global policies).
func (h *RoutingHandler) keyDept(c *gin.Context, keyID *int64) (*int64, error) {
	if keyID == nil {
		return nil, nil
	}
	k, err := h.keys.Get(c.Request.Context(), *keyID)
	if err != nil {
		return nil, err
	}
	return k.DepartmentID, nil
}

// ensureRouteAccess: global policies (no API key) affect every tenant, so only
// super admins may change them; key-scoped policies belong to the key's department.
func (h *RoutingHandler) ensureRouteAccess(c *gin.Context, keyID *int64) bool {
	if keyID == nil {
		if !middleware.PrincipalFrom(c).IsSuper() {
			response.AdminError(c, http.StatusForbidden, "全局调度策略影响所有部门，仅超级管理员可维护；部门策略请绑定本部门的 API Key")
			return false
		}
		return true
	}
	dept, err := h.keyDept(c, keyID)
	if err != nil {
		writeAdminError(c, err)
		return false
	}
	return ensureOwns(c, dept)
}

func (h *RoutingHandler) Create(c *gin.Context) {
	var req dto.CreateRouteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.AdminError(c, http.StatusBadRequest, err.Error())
		return
	}

	route := &domain.ModelRoute{
		Name: req.Name, AppID: req.AppID, APIKeyID: req.APIKeyID, SourceType: req.SourceType,
		SourceVendorID: req.SourceVendorID, Alias: req.Alias, CandidateModelID: req.CandidateModelID,
		Priority: req.Priority, Weight: req.Weight, Strategy: domain.RouteStrategy(req.Strategy), Remark: req.Remark,
	}
	if route.Name == "" {
		route.Name = route.Alias
	}
	if !h.ensureRouteAccess(c, route.APIKeyID) {
		return
	}
	if err := h.routing.CreateRoute(c.Request.Context(), route); err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusCreated, route)
}

// Update edits a policy. The bound API key is intentionally immutable (the
// console shows it read-only when editing).
func (h *RoutingHandler) Update(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		response.AdminError(c, http.StatusBadRequest, "invalid id")
		return
	}
	var req dto.UpdateRouteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.AdminError(c, http.StatusBadRequest, err.Error())
		return
	}

	route, err := h.routing.GetRoute(c.Request.Context(), id)
	if err != nil {
		writeAdminError(c, err)
		return
	}
	if !h.ensureRouteAccess(c, route.APIKeyID) {
		return
	}
	if req.Name != nil {
		route.Name = *req.Name
	}
	if req.AppID != nil {
		route.AppID = req.AppID
	}
	if req.SourceType != nil {
		route.SourceType = *req.SourceType
	}
	if req.SourceVendorID != nil {
		route.SourceVendorID = req.SourceVendorID
	}
	if req.Alias != nil && *req.Alias != "" {
		route.Alias = *req.Alias
	}
	if req.CandidateModelID != nil {
		route.CandidateModelID = *req.CandidateModelID
	}
	if req.Priority != nil {
		route.Priority = *req.Priority
	}
	if req.Weight != nil {
		route.Weight = *req.Weight
	}
	if req.Strategy != nil && *req.Strategy != "" {
		route.Strategy = domain.RouteStrategy(*req.Strategy)
	}
	if req.Remark != nil {
		route.Remark = *req.Remark
	}
	if req.Enabled != nil {
		route.Enabled = *req.Enabled
	}
	if route.SourceType != "vendor" {
		route.SourceVendorID = nil
	}
	if err := h.routing.UpdateRoute(c.Request.Context(), route); err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, route)
}

func (h *RoutingHandler) Delete(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		response.AdminError(c, http.StatusBadRequest, "invalid id")
		return
	}
	route, err := h.routing.GetRoute(c.Request.Context(), id)
	if err != nil {
		writeAdminError(c, err)
		return
	}
	if !h.ensureRouteAccess(c, route.APIKeyID) {
		return
	}
	if err := h.routing.DeleteRoute(c.Request.Context(), id); err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, gin.H{"deleted": id})
}

func (h *RoutingHandler) List(c *gin.Context) {
	routes, err := h.routing.ListRoutes(c.Request.Context())
	if err != nil {
		writeAdminError(c, err)
		return
	}
	scope := deptScope(c)
	if scope == nil {
		respondList(c, routes)
		return
	}
	keys, err := h.keys.List(c.Request.Context())
	if err != nil {
		writeAdminError(c, err)
		return
	}
	dept := map[int64]*int64{}
	for _, k := range keys {
		dept[k.ID] = k.DepartmentID
	}
	out := make([]*domain.ModelRoute, 0, len(routes))
	for _, rt := range routes {
		// global policies apply to every department, so everyone can see them
		if rt.APIKeyID == nil || inScope(scope, dept[*rt.APIKeyID]) {
			out = append(out, rt)
		}
	}
	respondList(c, out)
}

// Simulate explains how a request from a key for a model name would be
// routed: 厂商匹配 → 命中的调度策略 → 目标候选顺序.
func (h *RoutingHandler) Simulate(c *gin.Context) {
	var req dto.SimulateRouteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.AdminError(c, http.StatusBadRequest, err.Error())
		return
	}
	if req.APIKeyID != 0 {
		id := req.APIKeyID
		dept, err := h.keyDept(c, &id)
		if err != nil {
			writeAdminError(c, err)
			return
		}
		if !ensureOwns(c, dept) {
			return
		}
	}
	trace, err := h.routing.Explain(c.Request.Context(), req.APIKeyID, req.Model)
	if trace != nil {
		// an unresolved request still returns the partial trace for display
		response.AdminOK(c, http.StatusOK, gin.H{"trace": trace, "resolved": err == nil})
		return
	}
	writeAdminError(c, err)
}
